package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func main() {
	logrus.Fatal(telepathy.Start(os.Getenv("TELEPATHY_DB_TYPE"), os.Getenv("PORT")))
}
