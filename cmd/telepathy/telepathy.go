package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	_ "gitlab.com/kavenc/telepathy/internal/pkg/discord"
	_ "gitlab.com/kavenc/telepathy/internal/pkg/fwd"
	_ "gitlab.com/kavenc/telepathy/internal/pkg/line"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func main() {
	config := telepathy.SessionConfig{
		Port:         os.Getenv("PORT"),
		RedisURL:     os.Getenv("REDIS_URL"),
		MongoURL:     os.Getenv("MONGODB_URL"),
		DatabaseName: os.Getenv("MONGODB_NAME"),
	}
	session, err := telepathy.NewSession(config)
	if err != nil {
		logrus.Panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Telepathy session
	go session.Start(ctx)

	// Catch termination interrupts
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	logrus.Info("Terminating.")
}
