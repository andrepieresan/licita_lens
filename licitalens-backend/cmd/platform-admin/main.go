package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"licitalens.dev/backend/internal/store"
)

func main() {
	email := flag.String("email", "", "operator e-mail")
	name := flag.String("name", "", "operator full name")
	flag.Parse()

	password := os.Getenv("BOOTSTRAP_PASSWORD")
	if strings.TrimSpace(*email) == "" || strings.TrimSpace(*name) == "" || len(password) < 8 {
		log.Fatal("email, name and BOOTSTRAP_PASSWORD with at least 8 characters are required")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := store.NewPostgres(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	hash, err := store.HashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	operator, err := database.RegisterPlatformOperator(ctx, *email, hash, *name)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("platform operator created: email=%s subject_id=%s\n", operator.Email, operator.SubjectID)
}
