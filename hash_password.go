package main

import (
    "fmt"
    "golang.org/x/crypto/bcrypt"
)

func main() {
    password := "password123"
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        fmt.Println("Error:", err)
    }
    fmt.Println("Hash:", string(hash))
}