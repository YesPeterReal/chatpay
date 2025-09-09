package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
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
        fmt.Println("Error connecting to database:", err)
        return
    }
    defer db.Close()

    http.HandleFunc("/create-payment", handleCors(createPayment))
    http.HandleFunc("/request-payment", handleCors(requestPayment))
    http.HandleFunc("/list-payments", handleCors(listPayments))
    http.HandleFunc("/log-interaction", handleCors(logInteraction))
    fmt.Println("Starting server on :8080")
    http.ListenAndServe(":8080", nil)
}

func handleCors(handler http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        handler(w, r)
    }
}

func createPayment(w http.ResponseWriter, r *http.Request) {
    var input struct {
        Amount   int64  `json:"amount"`
        Currency string `json:"currency"`
        UserID   string `json:"user_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }

    paymentIntentId := fmt.Sprintf("pi_%d", time.Now().UnixNano())
    _, err := db.Exec(
        "INSERT INTO payments (payment_intent_id, user_id, amount, currency, status, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
        paymentIntentId, input.UserID, float64(input.Amount)/100, input.Currency, "succeeded", time.Now(),
    )
    if err != nil {
        http.Error(w, fmt.Sprintf("Error inserting payment: %v", err), http.StatusInternalServerError)
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
    var input struct {
        Amount   int64  `json:"amount"`
        Currency string `json:"currency"`
        UserID   string `json:"user_id"`
        TargetID string `json:"target_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }

    _, err := db.Exec(
        "INSERT INTO payment_requests (requester_id, target_id, amount, currency, status, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
        input.UserID, input.TargetID, float64(input.Amount)/100, input.Currency, "pending", time.Now(),
    )
    if err != nil {
        http.Error(w, fmt.Sprintf("Error inserting payment request: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "request_created"})
}

func listPayments(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query("SELECT payment_intent_id, user_id, amount, currency, status, created_at FROM payments")
    if err != nil {
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
            http.Error(w, fmt.Sprintf("Error scanning payments: %v", err), http.StatusInternalServerError)
            return
        }
        payments = append(payments, map[string]interface{}{
            "paymentIntentId": paymentIntentId,
            "userId":          userId,
            "amount":          amount,
            "currency":        currency,
            "status":          status,
            "createdAt":       createdAt.String(),
        })
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(payments)
}

func logInteraction(w http.ResponseWriter, r *http.Request) {
    var input struct {
        Component string `json:"component"`
        Action    string `json:"action"`
        Timestamp string `json:"timestamp"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
        return
    }
    fmt.Printf("Interaction: %s - %s at %s\n", input.Component, input.Action, input.Timestamp)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "logged"})
}