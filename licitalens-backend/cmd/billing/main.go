package main

import (
	"log"

	app "licitalens.dev/backend/internal/runtime"
)

func main() {
	if err := app.RunBillingReconciler(); err != nil {
		log.Fatal(err)
	}
}
