package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/discord"
	"gitlab.com/kavenc/telepathy/internal/pkg/fwd"
	"gitlab.com/kavenc/telepathy/internal/pkg/line"
	"gitlab.com/kavenc/telepathy/internal/pkg/slackmsg"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func main() {
	// colorized log
	logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})
	config := telepathy.SessionConfig{
		Port:         os.Getenv("PORT"),
		RootURL:      os.Getenv("URL"),
		RedisURL:     os.Getenv("REDIS_URL"),
		MongoURL:     os.Getenv("MONGODB_URL"),
		DatabaseName: os.Getenv("MONGODB_NAME"),
	}

	plugins := []telepathy.Plugin{
		&line.Messenger{
			Secret: os.Getenv("LINE_CHANNEL_SECRET"),
			Token:  os.Getenv("LINE_CHANNEL_TOKEN"),
		},
		&discord.Messenger{
			Token: os.Getenv("DISCORD_BOT_TOKEN"),
		},
		&slackmsg.Messenger{
			ClientID:      os.Getenv("SLACK_CLIENT_ID"),
			ClientSecret:  os.Getenv("SLACK_CLIENT_SECRET"),
			SigningSecret: []byte(os.Getenv("SLACK_SIGNING_SECRET")),
		},
		&fwd.Service{},
	}

	session, err := telepathy.NewSession(config, plugins)
	if err != nil {
		logrus.Panic(err)
	}

	// Start Telepathy session
	sessionEnd := make(chan interface{})
	go func() {
		session.Start()
		close(sessionEnd)
	}()

	// Catch termination interrupts
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	logrus.Info("terminating")
	session.Stop()
	<-sessionEnd
}
