package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func main() {
	email := flag.String("email", "", "owner e-mail")
	name := flag.String("name", "", "owner full name")
	organization := flag.String("organization", "", "organization name")
	plan := flag.String("plan", "essential", "initial plan: essential or pro")
	termsVersion := flag.String("terms-version", "", "accepted terms version")
	privacyVersion := flag.String("privacy-version", "", "accepted privacy-policy version")
	acceptLegal := flag.Bool("accept-legal", false, "confirm acceptance of the supplied legal versions")
	flag.Parse()

	password := os.Getenv("BOOTSTRAP_PASSWORD")
	if strings.TrimSpace(*email) == "" || strings.TrimSpace(*name) == "" || strings.TrimSpace(*organization) == "" || len(password) < 8 {
		log.Fatal("email, name, organization and BOOTSTRAP_PASSWORD with at least 8 characters are required")
	}
	if !*acceptLegal || strings.TrimSpace(*termsVersion) == "" || strings.TrimSpace(*privacyVersion) == "" {
		log.Fatal("--accept-legal, --terms-version and --privacy-version are required")
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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	account, createdOrganization, _, err := database.RegisterSaaSAccount(ctx, *email, string(hash), *name, *organization, *plan, domain.LegalAcceptance{
		TermsVersion:   strings.TrimSpace(*termsVersion),
		PrivacyVersion: strings.TrimSpace(*privacyVersion),
		AcceptedAt:     time.Now().UTC(),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.MarkEmailVerified(ctx, account.SubjectID); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("owner account created: email=%s organization_id=%s\n", account.Email, createdOrganization.ID)
}
