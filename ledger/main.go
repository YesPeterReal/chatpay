package main
import (
"database/sql"
"fmt"
_ "github.com/lib/pq"
)
func main() {
connStr := "host=chatpay-postgres-new.cxwak020irdl.eu-west-3.rds.amazonaws.com port=5432 user=chatpay password=FirstPboss00. dbname=postgres sslmode=require"
db, err := sql.Open("postgres", connStr)
if err != nil {
fmt.Println("Error connecting to database:", err)
return
}
defer db.Close()
err = db.Ping()
if err != nil {
fmt.Println("Error pinging database:", err)
return
}
fmt.Println("Successfully connected to chatpay-postgres-new!")
// Create payments table
_, err = db.Exec("CREATE TABLE IF NOT EXISTS payments (id SERIAL PRIMARY KEY, user_id VARCHAR(50) NOT NULL, amount DECIMAL(10,2) NOT NULL, currency VARCHAR(3) NOT NULL, status VARCHAR(20) NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);")
if err != nil {
fmt.Println("Error creating payments table:", err)
return
}
fmt.Println("Payments table created successfully!")
// Insert a sample payment
_, err = db.Exec("INSERT INTO payments (user_id, amount, currency, status) VALUES ($1, $2, $3, $4)", "user123", 100.50, "EUR", "pending")
if err != nil {
fmt.Println("Error inserting payment:", err)
return
}
fmt.Println("Sample payment inserted successfully!")
// Query payments
rows, err := db.Query("SELECT id, user_id, amount, currency, status, created_at FROM payments")
if err != nil {
fmt.Println("Error querying payments:", err)
return
}
defer rows.Close()
fmt.Println("Payments:")
for rows.Next() {
var id int
var userID string
var amount float64
var currency, status string
var createdAt string
err := rows.Scan(&id, &userID, &amount, &currency, &status, &createdAt)
if err != nil {
fmt.Println("Error scanning payments:", err)
return
}
fmt.Printf("ID: %d, User: %s, Amount: %.2f %s, Status: %s, Created: %s\n", id, userID, amount, currency, status, createdAt)
}
}
