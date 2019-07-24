// Package discord implements the Messenger handler of Discord for Telepathy framework.
// Needed configs:
// - BOT_TOKEN: A valid discord bot token. For more information, please refer
//              to https://discordapp.com/developers/applications/me
package discord

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is a unique string to identify this Messenger handler
const ID = "DISCORD"

type Messenger struct {
	telepathy.Plugin
	telepathy.PluginMessenger

	bot    *discordgo.Session
	logger *logrus.Entry
}

func (m *Messenger) ID() string{
	return "DISCORD"
}

func (m *Messenger) SetLogger(logger *logrus.Entry) {
	m.logger = logger
}

func (m *Messenger)

func new(param *telepathy.MsgrCtorParam) (telepathy.Messenger, error) {
	msgr := messenger{
		session: param.Session,
		handler: param.MsgHandler,
		logger:  param.Logger,
	}

	var err error
	config, ok := param.Config["BOT_TOKEN"]
	if !ok {
		return nil, InitError{msg: "config: BOT_TOKEN not found"}
	}

	token, ok := config.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: BOT_TOKEN"}
	}

	msgr.bot, err = discordgo.New("Bot " + token)
	if err != nil {
		return nil, InitError{msg: err.Error()}
	}

	msgr.bot.AddHandler(msgr.msgHandler)
	return &msgr, nil
}

func (m *messenger) ID() string {
	return ID
}

func (m *messenger) Start(ctx context.Context) {
	// Open a websocket connection to Discord and begin listening.
	err := m.bot.Open()
	if err != nil {
		m.logger.Errorf("open websocket connection failed: %s", err.Error())
		return
	}

	m.ctx = ctx

	// Run until being cancelled
	go func() {
		<-ctx.Done()

		m.logger.Info("terminating")
		// Cleanly close down the Discord session.
		err = m.bot.Close()

		if err != nil {
			m.logger.Errorf("error when closing: %s", err.Error())
		}
	}()
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
	var err error
	text := strings.Builder{}
	if message.AsName != "" {
		fmt.Fprintf(&text, "**[ %s ]**\n%s", message.AsName, message.Text)
	} else {
		text.WriteString(message.Text)
	}

	if message.Image != nil {
		_, err = m.bot.ChannelMessageSendComplex(
			message.TargetID,
			&discordgo.MessageSend{
				Content: text.String(),
				File: &discordgo.File{
					Name:        "sent-from-telepathy.png", // always use png, just to make discord show the image
					ContentType: message.Image.Type,
					Reader:      bytes.NewReader(message.Image.Content),
				},
			},
		)
	} else {
		if len(message.Text) > 0 {
			_, err = m.bot.ChannelMessageSend(message.TargetID, text.String())
		}
	}

	if err != nil {
		m.logger.Error("msg send failed: " + err.Error())
	}
}

func createImgContent(att *discordgo.MessageAttachment, logger *logrus.Entry) *telepathy.ByteContent {
	dl, err := http.Get(att.ProxyURL)
	if err != nil {
		logger.Error("download attached image failed: " + err.Error())
		return nil
	}
	defer dl.Body.Close()
	buf := bytes.NewBuffer([]byte{})
	buf.ReadFrom(dl.Body)
	content := telepathy.ByteContent{
		Content: buf.Bytes(),
	}
	ext := strings.ToLower(path.Ext(att.Filename))
	if ext == "" {
		logger.Warn("unknown attach image type: " + att.Filename)
		ext = "png"
	}
	content.Type = "image/" + ext
	return &content
}

func (m *messenger) msgHandler(_ *discordgo.Session, dgmessage *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if dgmessage.Author.ID == m.bot.State.User.ID {
		return
	}

	message := telepathy.InboundMessage{
		FromChannel: telepathy.Channel{
			MessengerID: m.ID(),
			ChannelID:   dgmessage.ChannelID,
		},
		SourceProfile: &telepathy.MsgrUserProfile{
			ID:          dgmessage.Author.ID,
			DisplayName: dgmessage.Author.Username,
		},
		Text: dgmessage.Content,
	}

	if len(dgmessage.Attachments) > 0 {
		// Widht > 0 && Height > 0 indicates that this is a image file
		if att := dgmessage.Attachments[0]; att.Height > 0 && att.Width > 0 {
			content := createImgContent(att, m.logger)
			if content != nil {
				message.Image = telepathy.NewImage(*content)
			}
		}
	}

	channel, err := m.bot.Channel(dgmessage.ChannelID)
	if err != nil {
		m.logger.Error("get channel fail: " + err.Error())
	}
	message.IsDirectMessage = channel.Type == discordgo.ChannelTypeDM

	m.handler(m.ctx, message)
}
