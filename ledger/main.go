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
    _ "github.com/lib/pq"
    "github.com/aws/aws-sdk-go-v2/aws"
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

    // Skip test payment creation for now (commented out for production)
    /*
    // Create a sample Stripe payment
    params := &stripe.PaymentIntentParams{
        Amount:   stripe.Int64(10050), // 100.50 EUR in cents
        Currency: stripe.String(string(stripe.CurrencyEUR)),
        Description: stripe.String("ChatPay test payment"),
        Metadata: map[string]string{
            "user_id": "user123",
        },
    }
    pi, err := paymentintent.New(params)
    if err != nil {
        log.Printf("Error creating Stripe payment: %v", err)
    } else {
        fmt.Printf("Stripe PaymentIntent created: %s\n", pi.ID)
        // Check if payment already exists
        var exists bool
        err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM payments WHERE stripe_payment_id = $1)", pi.ID).Scan(&exists)
        if err != nil {
            log.Printf("Error checking payment existence: %v", err)
        }
        if !exists {
            // Insert payment into database
            _, err = db.Exec("INSERT INTO payments (user_id, amount, currency, status, stripe_payment_id) VALUES ($1, $2, $3, $4, $5)",
                "user123", 100.50, "EUR", string(pi.Status), pi.ID)
            if err != nil {
                log.Printf("Error inserting payment: %v", err)
            } else {
                fmt.Println("Sample payment inserted successfully!")
            }
        } else {
            fmt.Println("Payment already exists, skipping insertion")
        }
    }
    */

    // Query payments by user ID
    rows, err := db.Query("SELECT id, user_id, amount, currency, status, stripe_payment_id FROM payments WHERE user_id = $1", "user123")
    if err != nil {
        log.Fatalf("Error querying payments: %v", err)
    }
    defer rows.Close()

    fmt.Println("Payments for user123:")
    for rows.Next() {
        var id int
        var userID, currency, status string
        var amount float64
        var stripePaymentID *string
        err := rows.Scan(&id, &userID, &amount, &currency, &status, &stripePaymentID)
        if err != nil {
            log.Printf("Error scanning payments: %v", err)
            continue
        }
        stripeID := ""
        if stripePaymentID != nil {
            stripeID = *stripePaymentID
        }
        fmt.Printf("ID: %d, User: %s, Amount: %.2f %s, Status: %s, Stripe ID: %s\n", id, userID, amount, currency, status, stripeID)
    }

    if err = rows.Err(); err != nil {
        log.Fatalf("Error iterating over payments: %v", err)
    }
}