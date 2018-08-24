package discord

import (
	"context"
	"os"

	"github.com/bwmarrin/discordgo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const name = "DISCORD"

type messenger struct {
	*telepathy.MsgrCtorParam
	ctx context.Context
	bot *discordgo.Session
}

// InitError indicates an error when initializing Discord messenger handler
type InitError struct {
	msg string
}

func init() {
	telepathy.RegisterMessenger(name, new)
}

func (e InitError) Error() string {
	return "Discord init failed: " + e.msg
}

func new(param *telepathy.MsgrCtorParam) (telepathy.Messenger, error) {
	msgr := messenger{
		MsgrCtorParam: param,
	}

	var err error
	msgr.bot, err = discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		return nil, InitError{msg: err.Error()}
	}

	msgr.bot.AddHandler(msgr.handler)
	return &msgr, nil
}

func (m *messenger) Name() string {
	return name
}

func (m *messenger) Start(ctx context.Context) {
	// Open a websocket connection to Discord and begin listening.
	err := m.bot.Open()
	if err != nil {
		m.Logger.Error("Open websocket connection fail.")
		return
	}

	m.ctx = ctx

	// Run until being cancelled
	<-ctx.Done()

	m.Logger.Info("Terminating.")
	// Cleanly close down the Discord session.
	m.bot.Close()
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
	m.bot.ChannelMessageSend(message.TargetID, message.Text)
}

func (m *messenger) handler(_ *discordgo.Session, dgmessage *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if dgmessage.Author.ID == m.bot.State.User.ID {
		return
	}

	message := telepathy.InboundMessage{
		FromChannel: telepathy.Channel{
			MessengerID: m.Name(),
			ChannelID:   dgmessage.ChannelID,
		},
		SourceProfile: &telepathy.MsgrUserProfile{
			ID:          dgmessage.Author.ID,
			DisplayName: dgmessage.Author.Username,
		},
		Text: dgmessage.Content,
	}

	channel, err := m.bot.Channel(dgmessage.ChannelID)
	if err != nil {
		m.Logger.Error("Get channel fail: " + err.Error())
	}
	message.IsDirectMessage = channel.Type == discordgo.ChannelTypeDM

	m.MsgHandler(m.ctx, m.MsgrCtorParam.Session, message)
}
