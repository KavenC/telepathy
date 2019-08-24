// Package discord implements the Messenger handler of Discord for Telepathy framework.
// Needed configs:
// - BOT_TOKEN: A valid discord bot token. For more information, please refer
//              to https://discordapp.com/developers/applications/me
package discord

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"

	"gitlab.com/kavenc/telepathy/internal/pkg/imgur"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	inMsgLen = 10
)

// Messenger is the main discord plugin structure
type Messenger struct {
	Token         string
	stopListening func()
	bot           *discordgo.Session
	inMsg         chan telepathy.InboundMessage
	outMsg        <-chan telepathy.OutboundMessage
	logger        *logrus.Entry
}

// ID implements telepathy.Plugin interface
func (m *Messenger) ID() string {
	return "DISCORD"
}

// SetLogger implements telepathy.Plugin interface
func (m *Messenger) SetLogger(logger *logrus.Entry) {
	m.logger = logger
}

// Start implements telepathy.Plugin interface
func (m *Messenger) Start() {
	var err error
	m.bot, err = discordgo.New("Bot " + m.Token)
	if err != nil {
		m.logger.Errorf("start failed: %s", err.Error())
		return
	}

	m.stopListening = m.bot.AddHandler(m.msgHandler)
	err = m.bot.Open()
	if err != nil {
		m.logger.Errorf("open websocket connection failed: %s", err.Error())
		return
	}

	m.logger.Info("started")
	m.transmitter()
	err = m.bot.Close()
	if err != nil {
		m.logger.Errorf("termination failed: %s", err.Error())
	}
	m.logger.Info("terminated")
}

// Stop implements telepathy.Plugin interface
func (m *Messenger) Stop() {
	m.stopListening()
	close(m.inMsg)
}

// InMsgChannel implements telepathy.PluginMessenger
func (m *Messenger) InMsgChannel() <-chan telepathy.InboundMessage {
	if m.inMsg == nil {
		m.inMsg = make(chan telepathy.InboundMessage, inMsgLen)
	}
	return m.inMsg
}

// AttachOutMsgChannel impelements telepathy.PluginMessenger
func (m *Messenger) AttachOutMsgChannel(ch <-chan telepathy.OutboundMessage) {
	m.outMsg = ch
}

func (m *Messenger) transmitter() {
	var err error
	for message := range m.outMsg {
		text := strings.Builder{}
		if message.AsName != "" {
			fmt.Fprintf(&text, "**[ %s ]**\n%s", message.AsName, message.Text)
		} else {
			text.WriteString(message.Text)
		}

		if message.Image != nil {
			_, err = m.bot.ChannelMessageSendComplex(
				message.ToChannel.ChannelID,
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
				_, err = m.bot.ChannelMessageSend(message.ToChannel.ChannelID, text.String())
			}
		}

		if err != nil {
			m.logger.Error("msg send failed: " + err.Error())
		}
	}
}

func createImgContent(att *discordgo.MessageAttachment, logger *logrus.Entry) *imgur.ByteContent {
	dl, err := http.Get(att.ProxyURL)
	if err != nil {
		logger.Error("download attached image failed: " + err.Error())
		return nil
	}
	defer dl.Body.Close()
	buf := bytes.NewBuffer([]byte{})
	buf.ReadFrom(dl.Body)
	content := imgur.ByteContent{
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

func (m *Messenger) msgHandler(_ *discordgo.Session, dgmessage *discordgo.MessageCreate) {
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
				message.Image = imgur.NewImage(*content)
			}
		}
	}

	channel, err := m.bot.Channel(dgmessage.ChannelID)
	if err != nil {
		m.logger.Error("get channel fail: " + err.Error())
	}
	message.IsDirectMessage = channel.Type == discordgo.ChannelTypeDM

	m.inMsg <- message
}
