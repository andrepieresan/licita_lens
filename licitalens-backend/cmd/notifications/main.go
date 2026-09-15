package main

import (
	"log"
	"os"

	app "licitalens.dev/backend/internal/runtime"
)

func main() {
	if os.Getenv("NOTIFICATIONS_WORKER") == "true" || len(os.Args) > 1 && os.Args[1] == "worker" {
		if err := app.RunNotificationWorker(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "once" {
		_ = os.Setenv("NOTIFICATIONS_ONCE", "true")
		if err := app.RunNotificationWorker(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := app.RunHealth("notifications"); err != nil {
		log.Fatal(err)
	}
}
