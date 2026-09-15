package main

import (
	app "licitalens.dev/backend/internal/runtime"
	"log"
)

func main() {
	if err := app.RunHealth("intelligence"); err != nil {
		log.Fatal(err)
	}
}
