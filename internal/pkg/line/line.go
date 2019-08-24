// Package line implements the Messenger handler of LINE for Telepathy framework.
// Needed configs:
// - SECRET: Channel scecret of LINE Messenging API
// - TOKEN: Channel access token of LINE Messenging API
package line

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"gitlab.com/kavenc/telepathy/internal/pkg/imgur"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	inMsgLen = 5
)

// InitError indicates an error when initializing Discord messenger handler
type InitError struct {
	msg string
}

// Messenger implements the communication with Line APP
type Messenger struct {
	Secret        string
	Token         string
	inMsgChannel  chan telepathy.InboundMessage
	outMsgChannel <-chan telepathy.OutboundMessage
	bot           *linebot.Client
	replyTokenMap sync.Map
	logger        *logrus.Entry
}

func (e InitError) Error() string {
	return "LINE init failed: " + e.msg
}

// ID returns messenger ID
func (m *Messenger) ID() string {
	return "LINE"
}

// Start starts the plugin
func (m *Messenger) Start() {
	bot, err := linebot.New(m.Secret, m.Token)
	if err != nil {
		m.logger.Panic(err.Error())
	}
	m.bot = bot

	m.logger.Info("started")
	m.transmitter()
	m.logger.Info("terminated")
}

// Stop terminates the plugin
func (m *Messenger) Stop() {
	close(m.inMsgChannel)
}

// SetLogger sets the logger
func (m *Messenger) SetLogger(logger *logrus.Entry) {
	m.logger = logger
}

// InMsgChannel provides inbound message channel
func (m *Messenger) InMsgChannel() <-chan telepathy.InboundMessage {
	if m.inMsgChannel == nil {
		m.inMsgChannel = make(chan telepathy.InboundMessage, inMsgLen)
	}
	return m.inMsgChannel
}

// AttachOutMsgChannel attaches outbound message channel
func (m *Messenger) AttachOutMsgChannel(ch <-chan telepathy.OutboundMessage) {
	m.outMsgChannel = ch
}

// Webhook provides webhook endpoints
func (m *Messenger) Webhook() map[string]telepathy.HTTPHandler {
	return map[string]telepathy.HTTPHandler{
		"line-callback": m.webhookHandler,
	}
}

// SetWebhookURL receives webhook urls
func (m *Messenger) SetWebhookURL(urls map[string]*url.URL) {

}

func (m *Messenger) transmitter() {
	for message := range m.outMsgChannel {
		messages := []linebot.SendingMessage{}

		if message.Text == "" && message.Image == nil {
			continue
		}

		text := strings.Builder{}
		if message.AsName != "" {
			fmt.Fprintf(&text, "[ %s ]", message.AsName)
			if message.Text != "" {
				fmt.Fprintf(&text, "\n%s", message.Text)
			}
		} else {
			text.WriteString(message.Text)
		}

		if text.Len() > 0 {
			messages = append(messages, linebot.NewTextMessage(text.String()))
		}

		if message.Image != nil {
			fullURL, err := message.Image.FullURL()
			var prevURL string
			if err == nil {
				prevURL, err = message.Image.SmallThumbnailURL()
			}
			if err != nil {
				m.logger.Error("unable to get image URL: " + err.Error())
			} else {
				messages = append(messages, linebot.NewImageMessage(fullURL, prevURL))
			}
		}

		// Try to use reply token
		channelID := message.ToChannel.ChannelID
		item, _ := m.replyTokenMap.LoadOrStore(channelID, &sync.Pool{})
		pool, _ := item.(*sync.Pool)
		item = pool.Get()
		if item != nil {
			replyTokenStr, _ := item.(string)
			call := m.bot.ReplyMessage(replyTokenStr, messages...)
			_, err := call.Do()

			if err == nil {
				continue
			}

			// If send failed with replyToken, output err msg and retry with PushMessage
			logger := m.logger.WithFields(logrus.Fields{
				"target":      channelID,
				"reply_token": replyTokenStr,
			})
			logger.Warn("reply message failed: " + err.Error())
			logger.Info("trying push message")
		}

		call := m.bot.PushMessage(channelID, messages...)
		_, err := call.Do()
		if err != nil {
			logger := m.logger.WithField("target", channelID)
			logger.Error("push message failed: " + err.Error())
		}
	}
}

func (m *Messenger) webhookHandler(response http.ResponseWriter, request *http.Request) {
	events, err := m.bot.ParseRequest(request)
	if err != nil {
		m.logger.Errorf("invalid request: %s", err.Error())
		if err == linebot.ErrInvalidSignature {
			response.WriteHeader(400)
		} else {
			response.WriteHeader(500)
		}
		return
	}

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			message := telepathy.InboundMessage{FromChannel: telepathy.Channel{
				MessengerID: m.ID(),
			}}
			profile, channelID := m.getSourceProfile(event.Source)
			message.SourceProfile = profile
			if message.SourceProfile == nil {
				m.logger.Warn("ignored message with unknown source")
				continue
			}
			message.FromChannel.ChannelID = channelID
			message.IsDirectMessage = message.SourceProfile.ID == channelID
			item, _ := m.replyTokenMap.LoadOrStore(channelID, &sync.Pool{})
			pool, _ := item.(*sync.Pool)
			pool.Put(event.ReplyToken)
			switch lineMessage := event.Message.(type) {
			case *linebot.TextMessage:
				message.Text = lineMessage.Text
			case *linebot.StickerMessage:
				message.Text = "(Sticker)"
			case *linebot.ImageMessage:
				response, err := m.bot.GetMessageContent(lineMessage.ID).Do()
				if err != nil {
					m.logger.Warn("fail to get image content")
					continue
				}
				content, err := ioutil.ReadAll(response.Content)
				response.Content.Close()
				if err != nil {
					m.logger.Warn("fail to read image content")
					continue
				}
				message.Image = imgur.NewImage(
					imgur.ByteContent{
						Type:    response.ContentType,
						Content: content,
					})
			default:
				m.logger.Warnf("unsupported message type: %T", event.Message)
				continue
			}
			m.inMsgChannel <- message
		}
	}
}

func (m *Messenger) getSourceProfile(source *linebot.EventSource) (*telepathy.MsgrUserProfile, string) {
	if source.GroupID != "" {
		profile, err := m.bot.GetGroupMemberProfile(source.GroupID, source.UserID).Do()
		if err != nil {
			logger := m.logger.WithFields(logrus.Fields{"GroupID": source.GroupID, "UserID": source.UserID})
			logger.Error("GetGroupMemberProfile failed: " + err.Error())
			return nil, ""
		}
		return &telepathy.MsgrUserProfile{
			ID:          profile.UserID,
			DisplayName: profile.DisplayName,
		}, source.GroupID
	} else if source.UserID != "" {
		profile, err := m.bot.GetProfile(source.UserID).Do()
		if err != nil {
			logger := m.logger.WithField("UserID", source.UserID)
			logger.Error("GetProfile failed: " + err.Error())
			return nil, ""
		}
		return &telepathy.MsgrUserProfile{
			ID:          profile.UserID,
			DisplayName: profile.DisplayName,
		}, source.UserID
	} else if source.RoomID != "" {
		profile, err := m.bot.GetRoomMemberProfile(source.RoomID, source.UserID).Do()
		if err != nil {
			logger := m.logger.WithFields(logrus.Fields{"RoomID": source.RoomID, "UserID": source.UserID})
			logger.Error("GetRoomMemberProfile failed: " + err.Error())
			return nil, ""
		}
		return &telepathy.MsgrUserProfile{
			ID:          profile.UserID,
			DisplayName: profile.DisplayName,
		}, source.RoomID
	} else {
		m.logger.Warn("unknown source " + source.Type)
		return nil, ""
	}
}
