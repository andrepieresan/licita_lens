package main

import (
	app "licitalens.dev/backend/internal/runtime"
	"log"
)

func main() {
	if err := app.RunHealth("tenancy"); err != nil {
		log.Fatal(err)
	}
}
