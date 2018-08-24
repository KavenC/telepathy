package discord

import (
	"bytes"
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
		msgr.Logger.Errorf("init failed: %s", err.Error())
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
		m.Logger.Errorf("open websocket connection failed: %s", err.Error())
		return
	}

	m.ctx = ctx

	// Run until being cancelled
	<-ctx.Done()

	m.Logger.Info("terminating")
	// Cleanly close down the Discord session.
	err = m.bot.Close()

	if err != nil {
		m.Logger.Errorf("error when closing: %s", err.Error())
	}
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
	if message.Image.Length > 0 {
		m.bot.ChannelMessageSendComplex(
			message.TargetID,
			&discordgo.MessageSend{
				File: &discordgo.File{
					Name:        "sent-from-telepathy.png", // always use png, just to make discord show the image
					ContentType: message.Image.Type,
					Reader:      bytes.NewReader(*message.Image.Content),
				},
			},
		)
	}

	if len(message.Text) > 0 {
		m.bot.ChannelMessageSend(message.TargetID, message.Text)
	}
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
		m.Logger.Error("get channel fail: " + err.Error())
	}
	message.IsDirectMessage = channel.Type == discordgo.ChannelTypeDM

	m.MsgHandler(m.ctx, m.MsgrCtorParam.Session, message)
}
