package main

import (
    "bytes"
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "time"
    "github.com/dgrijalva/jwt-go"
    "github.com/google/uuid"
    "github.com/stripe/stripe-go/v76"
    "github.com/stripe/stripe-go/v76/paymentintent"
    "github.com/stripe/stripe-go/v76/webhook"
    _ "github.com/lib/pq"
)

type Claims struct {
    UserID string `json:"user_id"`
    jwt.StandardClaims
}

func main() {
    stripe.Key = os.Getenv("STRIPE_KEY")
    if stripe.Key == "" {
        log.Fatalf("STRIPE_KEY environment variable not set")
    }
    jwtSecretKey := os.Getenv("JWT_SECRET")
    if jwtSecretKey == "" {
        log.Fatalf("JWT_SECRET environment variable not set")
    }
    webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
    if webhookSecret == "" {
        log.Fatalf("STRIPE_WEBHOOK_SECRET environment variable not set")
    }

    connStr := os.Getenv("DATABASE_URL")
    if connStr == "" {
        log.Fatalf("DATABASE_URL environment variable not set")
    }

    db, err := sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("Error connecting to database: %v", err)
    }
    defer db.Close()

    err = db.Ping()
    if err != nil {
        log.Fatalf("Error pinging database: %v", err)
    }
    fmt.Println("Successfully connected to database!")

    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id VARCHAR(50) PRIMARY KEY,
            email VARCHAR(100) UNIQUE NOT NULL
        );
        CREATE TABLE IF NOT EXISTS wallets (
            id SERIAL PRIMARY KEY,
            user_id VARCHAR(50) NOT NULL,
            balance DECIMAL(10,2) NOT NULL DEFAULT 0.0,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            FOREIGN KEY (user_id) REFERENCES users(id)
        );
        CREATE TABLE IF NOT EXISTS payments (
            id SERIAL PRIMARY KEY,
            payment_intent_id VARCHAR(100) UNIQUE,
            sender_id VARCHAR(50) NOT NULL,
            receiver_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            method VARCHAR(50) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (sender_id) REFERENCES users(id),
            FOREIGN KEY (receiver_id) REFERENCES users(id)
        );
        CREATE TABLE IF NOT EXISTS payment_requests (
            id VARCHAR(36) PRIMARY KEY,
            requester_id VARCHAR(50) NOT NULL,
            target_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (requester_id) REFERENCES users(id),
            FOREIGN KEY (target_id) REFERENCES users(id)
        );
    `)
    if err != nil {
        log.Fatalf("Error creating tables: %v", err)
    }
    fmt.Println("Tables ensured successfully!")

    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
    })

    http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        var creds struct {
            Email    string `json:"email"`
            Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
            http.Error(w, "Invalid request body", http.StatusBadRequest)
            return
        }
        var userID string
        err := db.QueryRow("SELECT id FROM users WHERE email = $1", creds.Email).Scan(&userID)
        if err != nil {
            http.Error(w, "Invalid credentials", http.StatusUnauthorized)
            return
        }
        token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
            UserID: userID,
            StandardClaims: jwt.StandardClaims{
                ExpiresAt: time.Now().Add(time.Hour).Unix(),
            },
        })
        tokenString, err := token.SignedString([]byte(jwtSecretKey))
        if err != nil {
            http.Error(w, "Error generating token", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
    })

    http.HandleFunc("/wallet/balance", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        tokenString := r.Header.Get("Authorization")
        if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
            http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token, err := jwt.ParseWithClaims(tokenString[7:], &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecretKey), nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        claims, ok := token.Claims.(*Claims)
        if !ok {
            http.Error(w, "Invalid token claims", http.StatusUnauthorized)
            return
        }
        rows, err := db.Query("SELECT balance, currency FROM wallets WHERE user_id = $1 AND status = $2", claims.UserID, "active")
        if err != nil {
            http.Error(w, fmt.Sprintf("Error querying wallets: %v", err), http.StatusInternalServerError)
            return
        }
        defer rows.Close()
        var wallets []map[string]interface{}
        for rows.Next() {
            var balance float64
            var currency string
            if err := rows.Scan(&balance, &currency); err != nil {
                http.Error(w, fmt.Sprintf("Error scanning wallets: %v", err), http.StatusInternalServerError)
                return
            }
            wallets = append(wallets, map[string]interface{}{
                "balance":  balance,
                "currency": currency,
            })
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(wallets)
    })

    http.HandleFunc("/payments/received", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        tokenString := r.Header.Get("Authorization")
        if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
            http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token, err := jwt.ParseWithClaims(tokenString[7:], &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecretKey), nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        claims, ok := token.Claims.(*Claims)
        if !ok {
            http.Error(w, "Invalid token claims", http.StatusUnauthorized)
            return
        }
        rows, err := db.Query(
            "SELECT payment_intent_id, amount, currency, status, created_at FROM payments WHERE receiver_id = $1 AND status = $2",
            claims.UserID, "succeeded")
        if err != nil {
            http.Error(w, fmt.Sprintf("Error querying payments: %v", err), http.StatusInternalServerError)
            return
        }
        defer rows.Close()
        var payments []map[string]interface{}
        for rows.Next() {
            var paymentIntentId, currency, status string
            var amount float64
            var createdAt time.Time
            if err := rows.Scan(&paymentIntentId, &amount, &currency, &status, &createdAt); err != nil {
                http.Error(w, fmt.Sprintf("Error scanning payments: %v", err), http.StatusInternalServerError)
                return
            }
            payments = append(payments, map[string]interface{}{
                "paymentIntentId": paymentIntentId,
                "amount":          amount,
                "currency":        currency,
                "status":          status,
                "createdAt":       createdAt.Format(time.RFC3339),
            })
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(payments)
    })

    http.HandleFunc("/create-payment", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        tokenString := r.Header.Get("Authorization")
        if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
            http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token, err := jwt.ParseWithClaims(tokenString[7:], &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecretKey), nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        claims, ok := token.Claims.(*Claims)
        if !ok {
            http.Error(w, "Invalid token claims", http.StatusUnauthorized)
            return
        }
        var req struct {
            Amount   float64 `json:"amount"`
            Currency string  `json:"currency"`
            UserID   string  `json:"user_id"`
            TargetID string  `json:"target_id"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request body", http.StatusBadRequest)
            return
        }
        if req.UserID != claims.UserID {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        params := &stripe.PaymentIntentParams{
            Amount:             stripe.Int64(int64(req.Amount * 100)),
            Currency:           stripe.String(req.Currency),
            PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
        }
        pi, err := paymentintent.New(params)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error creating Stripe payment: %v", err), http.StatusInternalServerError)
            return
        }
        var exists bool
        err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM payments WHERE payment_intent_id = $1)", pi.ID).Scan(&exists)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error checking payment existence: %v", err), http.StatusInternalServerError)
            return
        }
        if !exists {
            _, err = db.Exec(
                "INSERT INTO payments (payment_intent_id, sender_id, receiver_id, amount, currency, status, method) VALUES ($1, $2, $3, $4, $5, $6, $7)",
                pi.ID, req.UserID, req.TargetID, req.Amount, req.Currency, string(pi.Status), "card")
            if err != nil {
                http.Error(w, fmt.Sprintf("Error inserting payment: %v", err), http.StatusInternalServerError)
                return
            }
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "paymentIntentId": pi.ID,
            "amount":          req.Amount,
            "currency":        req.Currency,
            "status":          string(pi.Status),
            "createdAt":       time.Now().Format(time.RFC3339),
        })
    })

    http.HandleFunc("/request-payment", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        tokenString := r.Header.Get("Authorization")
        if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
            http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token, err := jwt.ParseWithClaims(tokenString[7:], &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecretKey), nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        claims, ok := token.Claims.(*Claims)
        if !ok {
            http.Error(w, "Invalid token claims", http.StatusUnauthorized)
            return
        }
        var req struct {
            Amount   float64 `json:"amount"`
            Currency string  `json:"currency"`
            UserID   string  `json:"user_id"`
            TargetID string  `json:"target_id"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request body", http.StatusBadRequest)
            return
        }
        if req.UserID != claims.UserID {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        _, err = db.Exec(
            "INSERT INTO payment_requests (id, requester_id, target_id, amount, currency, status) VALUES ($1, $2, $3, $4, $5, $6)",
            uuid.New().String(), req.UserID, req.TargetID, req.Amount, req.Currency, "pending")
        if err != nil {
            http.Error(w, fmt.Sprintf("Error inserting payment request: %v", err), http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "request_created"})
    })

    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Stripe-Signature")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        const maxBodyBytes = int64(65536)
        r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
        payload, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error reading webhook body: %v", err), http.StatusBadRequest)
            return
        }
        event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), webhookSecret)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error verifying webhook signature: %v", err), http.StatusBadRequest)
            return
        }
        if event.Type == "payment_intent.succeeded" || event.Type == "payment_intent.payment_failed" {
            var pi stripe.PaymentIntent
            if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
                http.Error(w, fmt.Sprintf("Error parsing payment intent: %v", err), http.StatusBadRequest)
                return
            }
            status := "succeeded"
            if event.Type == "payment_intent.payment_failed" {
                status = "failed"
            }
            _, err = db.Exec("UPDATE payments SET status = $1 WHERE payment_intent_id = $2", status, pi.ID)
            if err != nil {
                http.Error(w, fmt.Sprintf("Error updating payment status: %v", err), http.StatusInternalServerError)
                return
            }
            clientWebhookURL := os.Getenv("CLIENT_WEBHOOK_URL")
            if clientWebhookURL != "" {
                notification := map[string]string{
                    "event":           event.Type,
                    "paymentIntentId": pi.ID,
                    "status":          status,
                }
                notificationBytes, err := json.Marshal(notification)
                if err != nil {
                    log.Printf("Error marshaling notification: %v", err)
                } else {
                    go func() {
                        _, err = http.Post(clientWebhookURL, "application/json", bytes.NewBuffer(notificationBytes))
                        if err != nil {
                            log.Printf("Error sending client webhook: %v", err)
                        }
                    }()
                }
            }
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{
            "status": "received",
        })
    })

    http.HandleFunc("/list-payments", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        tokenString := r.Header.Get("Authorization")
        if len(tokenString) < 7 || tokenString[:7] != "Bearer " {
            http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token, err := jwt.ParseWithClaims(tokenString[7:], &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecretKey), nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        claims, ok := token.Claims.(*Claims)
        if !ok {
            http.Error(w, "Invalid token claims", http.StatusUnauthorized)
            return
        }
        rows, err := db.Query(
            "SELECT payment_intent_id, amount, currency, status, created_at FROM payments WHERE sender_id = $1 OR receiver_id = $1",
            claims.UserID)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error querying payments: %v", err), http.StatusInternalServerError)
            return
        }
        defer rows.Close()
        var payments []map[string]interface{}
        for rows.Next() {
            var paymentIntentId, currency, status string
            var amount float64
            var createdAt time.Time
            if err := rows.Scan(&paymentIntentId, &amount, &currency, &status, &createdAt); err != nil {
                http.Error(w, fmt.Sprintf("Error scanning payments: %v", err), http.StatusInternalServerError)
                return
            }
            payments = append(payments, map[string]interface{}{
                "paymentIntentId": paymentIntentId,
                "amount":          amount,
                "currency":        currency,
                "status":          status,
                "createdAt":       createdAt.Format(time.RFC3339),
            })
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(payments)
    })

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    fmt.Printf("Starting server on :%s\n", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}