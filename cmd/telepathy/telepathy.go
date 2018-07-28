package main

import (
	"log"
	"os"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func main() {
	log.Fatal(telepathy.Start(os.Getenv("TELEPATHY_DB_TYPE"), os.Getenv("PORT")))
}
