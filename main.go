package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
    _ "github.com/lib/pq"
)

var db *sql.DB

func main() {
    // Initialize database connection
    connStr := "host=chatpay-postgres-new.cxwak020irdl.eu-west-3.rds.amazonaws.com port=5432 user=chatpay password=FirstPboss00.Nakata dbname=postgres sslmode=require"
    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("Error connecting to database: %v", err)
    }
    if err := db.Ping(); err != nil {
        log.Fatalf("Error pinging database: %v", err)
    }
    log.Println("Successfully connected to database")

    // Create tables if not exist
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS payments (
            id SERIAL PRIMARY KEY,
            stripe_payment_id VARCHAR(50) NOT NULL,
            user_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        CREATE TABLE IF NOT EXISTS payment_requests (
            id SERIAL PRIMARY KEY,
            requester_id VARCHAR(50) NOT NULL,
            target_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
    `)
    if err != nil {
        log.Fatalf("Error creating tables: %v", err)
    }
    log.Println("Tables ensured successfully")

    // Use ServeMux with CORS middleware for all routes
    mux := http.NewServeMux()
    handler := enableCORS(mux)
    mux.HandleFunc("/create-payment", createPayment)
    mux.HandleFunc("/request-payment", requestPayment)
    mux.HandleFunc("/list-payments", listPayments)
    mux.HandleFunc("/log-interaction", logInteraction)

    log.Println("Starting server on :8080")
    if err := http.ListenAndServe(":8080", handler); err != nil {
        log.Fatalf("Server failed to start: %v", err)
    }
}

func enableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Log request details for debugging
        log.Printf("Received %s request for %s with headers: %v", r.Method, r.URL.Path, r.Header)
        // Set CORS headers
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
        w.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours
        // Handle preflight OPTIONS request
        if r.Method == http.MethodOptions {
            log.Printf("Handling OPTIONS request for %s", r.URL.Path)
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func createPayment(w http.ResponseWriter, r *http.Request) {
    // Only allow POST requests
    if r.Method != http.MethodPost {
        log.Printf("Invalid method %s for /create-payment", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var input struct {
        Amount   int64  `json:"amount"`
        Currency string `json:"currency"`
        UserID   string `json:"user_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        log.Printf("Error decoding create-payment request: %v", err)
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }
    log.Printf("Received create-payment: Amount=%d, Currency=%s, UserID=%s", input.Amount, input.Currency, input.UserID)
    if input.Currency == "" || input.UserID == "" {
        log.Printf("Invalid input: Currency=%s, UserID=%s", input.Currency, input.UserID)
        http.Error(w, "Invalid input: Currency and UserID are required", http.StatusBadRequest)
        return
    }
    tx, err := db.Begin()
    if err != nil {
        log.Printf("Error starting transaction: %v", err)
        http.Error(w, fmt.Sprintf("Error starting transaction: %v", err), http.StatusInternalServerError)
        return
    }
    paymentIntentId := fmt.Sprintf("pi_%d", time.Now().UnixNano())
    _, err = tx.Exec(
        "INSERT INTO payments (stripe_payment_id, user_id, amount, currency, status, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
        paymentIntentId, input.UserID, float64(input.Amount)/100, input.Currency, "succeeded", time.Now(),
    )
    if err != nil {
        tx.Rollback()
        log.Printf("Error inserting payment: %v", err)
        http.Error(w, fmt.Sprintf("Error inserting payment: %v", err), http.StatusInternalServerError)
        return
    }
    if err := tx.Commit(); err != nil {
        log.Printf("Error committing transaction: %v", err)
        http.Error(w, fmt.Sprintf("Error committing transaction: %v", err), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "paymentIntentId": paymentIntentId,
        "status":          "succeeded",
    })
}

func requestPayment(w http.ResponseWriter, r *http.Request) {
    // Only allow POST requests
    if r.Method != http.MethodPost {
        log.Printf("Invalid method %s for /request-payment", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var input struct {
        Amount   int64  `json:"amount"`
        Currency string `json:"currency"`
        UserID   string `json:"user_id"`
        TargetID string `json:"target_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        log.Printf("Error decoding request-payment request: %v", err)
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }
    log.Printf("Received request-payment: Amount=%d, Currency=%s, UserID=%s, TargetID=%s", input.Amount, input.Currency, input.UserID, input.TargetID)
    if input.Currency == "" || input.UserID == "" || input.TargetID == "" {
        log.Printf("Invalid input: Currency=%s, UserID=%s, TargetID=%s", input.Currency, input.UserID, input.TargetID)
        http.Error(w, "Invalid input: Currency, UserID, and TargetID are required", http.StatusBadRequest)
        return
    }
    tx, err := db.Begin()
    if err != nil {
        log.Printf("Error starting transaction: %v", err)
        http.Error(w, fmt.Sprintf("Error starting transaction: %v", err), http.StatusInternalServerError)
        return
    }
    _, err = tx.Exec(
        "INSERT INTO payment_requests (requester_id, target_id, amount, currency, status, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
        input.UserID, input.TargetID, float64(input.Amount)/100, input.Currency, "pending", time.Now(),
    )
    if err != nil {
        tx.Rollback()
        log.Printf("Error inserting payment request: %v", err)
        http.Error(w, fmt.Sprintf("Error inserting payment request: %v", err), http.StatusInternalServerError)
        return
    }
    if err := tx.Commit(); err != nil {
        log.Printf("Error committing transaction: %v", err)
        http.Error(w, fmt.Sprintf("Error committing transaction: %v", err), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "request_created"})
}

func listPayments(w http.ResponseWriter, r *http.Request) {
    // Only allow GET requests
    if r.Method != http.MethodGet {
        log.Printf("Invalid method %s for /list-payments", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    // Log query parameters for debugging
    log.Printf("Query parameters for /list-payments: %v", r.URL.Query())
    rows, err := db.Query("SELECT stripe_payment_id, user_id, amount, currency, status, created_at FROM payments")
    if err != nil {
        log.Printf("Error querying payments: %v", err)
        http.Error(w, fmt.Sprintf("Error querying payments: %v", err), http.StatusInternalServerError)
        return
    }
    defer rows.Close()
    var payments []map[string]interface{}
    for rows.Next() {
        var paymentIntentId, userId, currency, status string
        var amount float64
        var createdAt time.Time
        if err := rows.Scan(&paymentIntentId, &userId, &amount, &currency, &status, &createdAt); err != nil {
            log.Printf("Error scanning payments: %v", err)
            http.Error(w, fmt.Sprintf("Error scanning payments: %v", err), http.StatusInternalServerError)
            return
        }
        payments = append(payments, map[string]interface{}{
            "paymentIntentId": paymentIntentId,
            "user_id":         userId,
            "amount":          amount,
            "currency":        currency,
            "status":          status,
            "createdAt":       createdAt.Format(time.RFC3339),
        })
    }
    if err := rows.Err(); err != nil {
        log.Printf("Error iterating payments: %v", err)
        http.Error(w, fmt.Sprintf("Error iterating payments: %v", err), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(payments)
}

func logInteraction(w http.ResponseWriter, r *http.Request) {
    // Only allow POST requests
    if r.Method != http.MethodPost {
        log.Printf("Invalid method %s for /log-interaction", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var input struct {
        Component string `json:"component"`
        Action    string `json:"action"`
        Timestamp string `json:"timestamp"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        log.Printf("Error decoding log-interaction request: %v", err)
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }
    log.Printf("Interaction: %s - %s at %s", input.Component, input.Action, input.Timestamp)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "logged"})
}