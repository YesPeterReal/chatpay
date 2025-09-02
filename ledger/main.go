package main

import (
    "database/sql"
    "fmt"
    _ "github.com/lib/pq"
    "log"
)

func main() {
    // Database connection string
    connStr := "host=chatpay-postgres-new.cxwak020irdl.eu-west-3.rds.amazonaws.com port=5432 user=chatpay password=FirstPboss00. dbname=postgres sslmode=verify-full sslrootcert=rds-ca-rsa2048-g1.pem"

    db, err := sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("Error connecting to database: %v", err)
    }
    defer db.Close()

    // Verify database connection
    err = db.Ping()
    if err != nil {
        log.Fatalf("Error pinging database: %v", err)
    }
    fmt.Println("✅ Successfully connected to chatpay-postgres-new!")

    // Create payments table if it doesn't exist
    createTable := `
        CREATE TABLE IF NOT EXISTS payments (
            id SERIAL PRIMARY KEY,
            user_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(20) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
    `
    _, err = db.Exec(createTable)
    if err != nil {
        log.Fatalf("Error creating payments table: %v", err)
    }
    fmt.Println("✅ Payments table ensured successfully!")

    // Insert a sample payment
    _, err = db.Exec("INSERT INTO payments (user_id, amount, currency, status) VALUES ($1, $2, $3, $4)", "user123", 100.50, "EUR", "pending")
    if err != nil {
        log.Printf("Error inserting payment: %v", err)
    } else {
        fmt.Println("✅ Sample payment inserted successfully!")
    }

    // Update payment status
    _, err = db.Exec("UPDATE payments SET status = $1 WHERE id = $2", "completed", 1)
    if err != nil {
        log.Printf("Error updating payment status: %v", err)
    } else {
        fmt.Println("✅ Payment ID 1 updated to completed successfully!")
    }

    // Delete a payment
    _, err = db.Exec("DELETE FROM payments WHERE id = $1", 1)
    if err != nil {
        log.Printf("Error deleting payment: %v", err)
    } else {
        fmt.Println("✅ Payment ID 1 deleted successfully!")
    }

    // Query payments by user ID
    rows, err := db.Query("SELECT id, user_id, amount, currency, status, created_at FROM payments WHERE user_id = $1", "user123")
    if err != nil {
        log.Fatalf("Error querying payments by user ID: %v", err)
    }
    defer rows.Close()

    fmt.Println("📌 Payments for user123:")
    for rows.Next() {
        var id int
        var userID, currency, status string
        var amount float64
        var createdAt string

        err := rows.Scan(&id, &userID, &amount, &currency, &status, &createdAt)
        if err != nil {
            log.Printf("Error scanning payments: %v", err)
            continue
        }

        fmt.Printf("ID: %d, User: %s, Amount: %.2f %s, Status: %s, Created: %s\n", id, userID, amount, currency, status, createdAt)
    }

    if err = rows.Err(); err != nil {
        log.Fatalf("Error iterating over payments: %v", err)
    }
}
