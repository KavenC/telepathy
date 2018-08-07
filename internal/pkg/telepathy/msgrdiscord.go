package telepathy

import (
	"context"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

func init() {
	RegisterMessenger(&DiscordMessenger{})
}

// DiscordMessenger implements the communication with Discord APP
type DiscordMessenger struct {
	bot *discordgo.Session
	ctx context.Context
}

func (m *DiscordMessenger) name() string {
	return "DISCORD"
}

func (m *DiscordMessenger) init() error {
	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		return err
	}
	m.bot = session
	m.bot.AddHandler(m.handler)
	return nil
}

func (m *DiscordMessenger) start(ctx context.Context) {
	// Open a websocket connection to Discord and begin listening.
	err := m.bot.Open()
	if err != nil {
		logger := logrus.WithField("messenger", m.name())
		logger.Error("Open websocket connection fail.")
		return
	}

	m.ctx = ctx

	// Run until being cancelled
	<-ctx.Done()

	logrus.WithField("messenger", m.name()).Info("Terminating.")
	// Cleanly close down the Discord session.
	m.bot.Close()
}

func (m *DiscordMessenger) send(message *OutboundMessage) {
	m.bot.ChannelMessageSend(message.TargetID, message.Text)
}

func (m *DiscordMessenger) handler(_ *discordgo.Session, dgmessage *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if dgmessage.Author.ID == m.bot.State.User.ID {
		return
	}

	message := InboundMessage{
		Messenger: m,
		SourceProfile: &MsgrUserProfile{
			ID:          dgmessage.Author.ID,
			DisplayName: dgmessage.Author.Username,
		},
		SourceID: dgmessage.ChannelID,
		Text:     dgmessage.Content,
	}

	channel, err := m.bot.Channel(dgmessage.ChannelID)
	if err != nil {
		logger := logrus.WithField("mssenger", m.name())
		logger.Error("Get channel fail: " + err.Error())
	}
	message.IsDirectMessage = channel.Type == discordgo.ChannelTypeDM

	HandleInboundMessage(m.ctx, &message)
}
