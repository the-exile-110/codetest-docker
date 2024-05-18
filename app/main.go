package main

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
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
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatal("Failed to close database connection: ", err)
		}
	}(db)

	// Mutex for protecting database access
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

		defer func() {
			if p := recover(); p != nil {
				tx.Rollback()
				log.Printf("Recovered from panic: %v", p)
				c.Status(fiber.StatusInternalServerError).SendString("Internal server error")
			}
		}()

		// Query total amount for the user
		var totalAmount sql.NullInt64
		if err := tx.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE user_id=?", transaction.UserID).Scan(&totalAmount); err != nil {
			tx.Rollback()
			log.Printf("QueryRow error: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Check if adding the new transaction will exceed the limit
		if totalAmount.Int64+int64(transaction.Amount) > amountLimit {
			tx.Rollback()
			return c.Status(fiber.StatusPaymentRequired).SendString(
				fmt.Sprintf("Total amount exceeds limit of %d", amountLimit))
		}

		// Insert transaction into database
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, description) VALUES (?, ?, ?)",
			transaction.UserID, transaction.Amount, transaction.Description)
		if err != nil {
			tx.Rollback()
			log.Printf("Exec error: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Commit the transaction
		if err = tx.Commit(); err != nil {
			tx.Rollback()
			log.Printf("Commit error: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Return appropriate response based on the transaction result
		return c.Status(fiber.StatusCreated).SendString("Transaction created successfully")
	}
}
