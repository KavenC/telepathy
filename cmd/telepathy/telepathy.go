package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func main() {
	session := telepathy.Session{
		Ctx:          context.Background(),
		DatabaseType: os.Getenv("TELEPATHY_DB_TYPE"),
		Port:         os.Getenv("PORT"),
	}
	err := session.Start()

	if err != nil {
		logrus.Panic(err)
	}
}
