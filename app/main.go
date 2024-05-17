package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"log"
	"sync"
)

const (
	amountLimit = 1000
)

type Transaction struct {
	UserID      int    `json:"user_id"`
	Amount      int    `json:"amount"`
	Description string `json:"description"`
}

func main() {
	// Initialize Fiber app
	app := fiber.New()

	// Database connection
	db, err := sql.Open("mysql", "root@tcp(db:3306)/codetest")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 互斥锁
	var mu sync.Mutex

	// Route handlers
	app.Post("/transactions", createTransactionHandler(db, &mu))

	// Start server
	log.Fatal(app.Listen(":8888"))
}

func createTransactionHandler(db *sql.DB, mu *sync.Mutex) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var transaction Transaction
		if err := c.BodyParser(&transaction); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}

		// 通过互斥锁保护对数据库的访问
		mu.Lock()
		defer mu.Unlock()

		// Begin transaction
		tx, err := db.Begin()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Query total amount for the user
		var totalAmount sql.NullInt64
		if err := tx.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE user_id=?", transaction.UserID).Scan(&totalAmount); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Check if adding the new transaction will exceed the limit
		if totalAmount.Int64+int64(transaction.Amount) > amountLimit {
			if err := tx.Rollback(); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
			}
			return c.Status(fiber.StatusPaymentRequired).SendString(
				fmt.Sprintf("Total amount exceeds limit of %d", amountLimit))
		}

		// Insert transaction into database
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, description) VALUES (?, ?, ?)",
			transaction.UserID, transaction.Amount, transaction.Description)
		if err != nil {
			err = tx.Rollback()
			if err != nil {
				fmt.Println(err)
				return err
			}
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Commit the transaction
		if err = tx.Commit(); err != nil {
			err = tx.Rollback()
			if err != nil {
				fmt.Println(err)
				return err
			}
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Return appropriate response based on the transaction result
		// Use StatusCreated if the transaction was successfully created
		// Use StatusPaymentRequired if the total amount exceeds the limit
		if totalAmount.Int64+int64(transaction.Amount) > amountLimit {
			return c.Status(fiber.StatusPaymentRequired).SendString(
				fmt.Sprintf("Total amount exceeds limit of %d", amountLimit))
		}
		return c.Status(fiber.StatusCreated).SendString("Transaction created successfully")
	}
}
