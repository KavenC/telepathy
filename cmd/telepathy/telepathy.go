package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/discord"
	_ "gitlab.com/kavenc/telepathy/internal/pkg/fwd"
	"gitlab.com/kavenc/telepathy/internal/pkg/line"
	"gitlab.com/kavenc/telepathy/internal/pkg/slackmsg"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
	"gitlab.com/kavenc/telepathy/internal/pkg/twitch"
)

func main() {
	// colorized log
	logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})

	config := telepathy.SessionConfig{
		Port:                 os.Getenv("PORT"),
		RootURL:              os.Getenv("URL"),
		RedisURL:             os.Getenv("REDIS_URL"),
		MongoURL:             os.Getenv("MONGODB_URL"),
		DatabaseName:         os.Getenv("MONGODB_NAME"),
		MessengerConfigTable: make(map[string]telepathy.PluginConfig),
		ServiceConfigTable:   make(map[string]telepathy.PluginConfig),
	}

	// Setup Messenger Configs
	config.MessengerConfigTable[line.ID] = make(telepathy.PluginConfig)
	config.MessengerConfigTable[line.ID]["SECRET"] = os.Getenv("LINE_CHANNEL_SECRET")
	config.MessengerConfigTable[line.ID]["TOKEN"] = os.Getenv("LINE_CHANNEL_TOKEN")
	config.MessengerConfigTable[discord.ID] = make(telepathy.PluginConfig)
	config.MessengerConfigTable[discord.ID]["BOT_TOKEN"] = os.Getenv("DISCORD_BOT_TOKEN")
	config.MessengerConfigTable[slackmsg.ID] = make(telepathy.PluginConfig)
	config.MessengerConfigTable[slackmsg.ID]["BOT_TOKEN"] = os.Getenv("SLACK_BOT_TOKEN")
	config.MessengerConfigTable[slackmsg.ID]["SIGNING_SECRET"] = os.Getenv("SLACK_SIGNING_SECRET")
	config.MessengerConfigTable[slackmsg.ID]["CLIENT_ID"] = os.Getenv("SLACK_CLIENT_ID")
	config.MessengerConfigTable[slackmsg.ID]["CLIENT_SECRET"] = os.Getenv("SLACK_CLIENT_SECRET")

	// Setup Service Configs
	config.ServiceConfigTable[twitch.ID] = make(telepathy.PluginConfig)
	config.ServiceConfigTable[twitch.ID]["CLIENT_ID"] = os.Getenv("TWITCH_CLIENT_ID")

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
	logrus.Info("terminating")
}
