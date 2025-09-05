package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
    "github.com/stripe/stripe-go/v76"
    "github.com/stripe/stripe-go/v76/paymentintent"
    _ "github.com/lib/pq"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/gin-gonic/gin"
)

func main() {
    // Retrieve Stripe key and PostgreSQL password from AWS Secrets Manager
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("eu-west-3"))
    if err != nil {
        log.Fatalf("Error loading AWS config: %v", err)
    }
    client := secretsmanager.NewFromConfig(cfg)

    // Retrieve Stripe key
    secretOutput, err := client.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
        SecretId: aws.String("chatpay/stripe-key"),
    })
    if err != nil {
        log.Fatalf("Error retrieving Stripe secret: %v", err)
    }
    var secret map[string]string
    if err := json.Unmarshal([]byte(*secretOutput.SecretString), &secret); err != nil {
        log.Fatalf("Error parsing Stripe secret: %v", err)
    }
    stripe.Key = secret["STRIPE_KEY"]

    // Retrieve PostgreSQL password
    secretOutput, err = client.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
        SecretId: aws.String("chatpay/postgres-password"),
    })
    if err != nil {
        log.Fatalf("Error retrieving PostgreSQL secret: %v", err)
    }
    var pgSecret map[string]string
    if err := json.Unmarshal([]byte(*secretOutput.SecretString), &pgSecret); err != nil {
        log.Fatalf("Error parsing PostgreSQL secret: %v", err)
    }
    postgresPassword := pgSecret["POSTGRES_PASSWORD"]

    // Database connection string
    connStr := fmt.Sprintf("host=chatpay-postgres-new.cxwak020irdl.eu-west-3.rds.amazonaws.com port=5432 user=chatpay password=%s dbname=postgres sslmode=verify-full sslrootcert=rds-ca-rsa2048-g1.pem", postgresPassword)
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
    fmt.Println("Successfully connected to chatpay-postgres-new!")

    // Create payments table if it doesn't exist
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS payments (
            id SERIAL PRIMARY KEY,
            user_id VARCHAR(50) NOT NULL,
            amount DECIMAL(10,2) NOT NULL,
            currency VARCHAR(3) NOT NULL,
            status VARCHAR(50) NOT NULL,
            stripe_payment_id VARCHAR(100) UNIQUE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
    `)
    if err != nil {
        log.Fatalf("Error creating payments table: %v", err)
    }
    fmt.Println("Payments table ensured successfully!")

    // Set up Gin router
    r := gin.Default()

    // Add CORS middleware
    r.Use(func(c *gin.Context) {
        c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
        c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)
            return
        }
        c.Next()
    })

    // Endpoint to list payments
    r.GET("/payments", func(c *gin.Context) {
        rows, err := db.Query("SELECT id, user_id, amount, currency, status, stripe_payment_id FROM payments WHERE user_id = $1", "user123")
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        defer rows.Close()
        var payments []map[string]interface{}
        for rows.Next() {
            var id int
            var userID, currency, status, stripeID string
            var amount float64
            var stripePaymentID *string
            if err := rows.Scan(&id, &userID, &amount, &currency, &status, &stripePaymentID); err != nil {
                c.JSON(500, gin.H{"error": err.Error()})
                return
            }
            stripeID = ""
            if stripePaymentID != nil {
                stripeID = *stripePaymentID
            }
            payments = append(payments, map[string]interface{}{
                "id": id,
                "user_id": userID,
                "amount": amount,
                "currency": currency,
                "status": status,
                "stripe_payment_id": stripeID,
            })
        }
        c.JSON(200, payments)
    })

    // Endpoint to create a payment
    r.POST("/create-payment", func(c *gin.Context) {
        var input struct {
            Amount   int64  "json:\"amount\""
            Currency string "json:\"currency\""
            UserID   string "json:\"user_id\""
        }
        if err := c.ShouldBindJSON(&input); err != nil {
            c.JSON(400, gin.H{"error": "Invalid input"})
            return
        }
        params := &stripe.PaymentIntentParams{
            Amount:   stripe.Int64(input.Amount),
            Currency: stripe.String(input.Currency),
            Description: stripe.String("ChatPay payment"),
            Metadata: map[string]string{
                "user_id": input.UserID,
            },
        }
        pi, err := paymentintent.New(params)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        var exists bool
        err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM payments WHERE stripe_payment_id = $1)", pi.ID).Scan(&exists)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        if !exists {
            _, err = db.Exec("INSERT INTO payments (user_id, amount, currency, status, stripe_payment_id) VALUES ($1, $2, $3, $4, $5)",
                input.UserID, float64(input.Amount)/100, input.Currency, string(pi.Status), pi.ID)
            if err != nil {
                c.JSON(500, gin.H{"error": err.Error()})
                return
            }
        }
        c.JSON(200, gin.H{"paymentIntentId": pi.ID, "status": pi.Status})
    })

    // Start server
    r.Run(":8080") // Listen on port 8080
}