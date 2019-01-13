// Package line implements the Messenger handler of LINE for Telepathy framework.
// Needed configs:
// - SECRET: Channel scecret of LINE Messenging API
// - TOKEN: Channel access token of LINE Messenging API
package line

import (
	"context"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// ID is a unique string to identify this Messenger handler
const ID = "LINE"

func init() {
	telepathy.RegisterMessenger(ID, new)
}

// InitError indicates an error when initializing Discord messenger handler
type InitError struct {
	msg string
}

// LineMessenger implements the communication with Line APP
type messenger struct {
	telepathy.MessengerPlugin
	session       *telepathy.Session
	handler       telepathy.InboundMsgHandler
	ctx           context.Context
	bot           *linebot.Client
	replyTokenMap sync.Map
	logger        *logrus.Entry
}

func (e InitError) Error() string {
	return "LINE init failed: " + e.msg
}

func new(param *telepathy.MsgrCtorParam) (telepathy.Messenger, error) {
	msg := messenger{
		session: param.Session,
		handler: param.MsgHandler,
		logger:  param.Logger,
	}

	sconfig, ok := param.Config["SECRET"]
	if !ok {
		return nil, InitError{msg: "config: SECRET loading failed"}
	}
	tconfig, ok := param.Config["TOKEN"]
	if !ok {
		return nil, InitError{msg: "config: TOKEN loading failed"}
	}
	secret, ok := sconfig.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: SECRET"}
	}
	token, ok := tconfig.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: CONFIG"}
	}
	bot, err := linebot.New(secret, token)
	if err != nil {
		return nil, InitError{msg: err.Error()}
	}

	msg.bot = bot
	param.Session.WebServer.RegisterWebhook("line-callback", msg.webhook)

	return &msg, nil
}

func (m *messenger) ID() string {
	return ID
}

func (m *messenger) Start(ctx context.Context) {
	m.ctx = ctx
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {

	messages := []linebot.SendingMessage{}

	if message.Text != "" {
		messages = append(messages, linebot.NewTextMessage(message.Text))
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
	item, _ := m.replyTokenMap.LoadOrStore(message.TargetID, &sync.Pool{})
	pool, _ := item.(*sync.Pool)
	item = pool.Get()
	if item != nil {
		replyTokenStr, _ := item.(string)
		call := m.bot.ReplyMessage(replyTokenStr, messages...)
		_, err := call.Do()

		if err == nil {
			return
		}

		// If send failed with replyToken, output err msg and retry with PushMessage
		logger := m.logger.WithFields(logrus.Fields{
			"target":      message.TargetID,
			"reply_token": replyTokenStr,
		})
		logger.Warn("reply message failed: " + err.Error())
		logger.Info("trying push message")
	}

	call := m.bot.PushMessage(message.TargetID, messages...)
	_, err := call.Do()
	if err != nil {
		logger := m.logger.WithField("target", message.TargetID)
		logger.Error("push message failed: " + err.Error())
	}
}

func (m *messenger) webhook(response http.ResponseWriter, request *http.Request) {
	if m.ctx == nil {
		m.logger.Warn("event dropped")
		return
	}

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
				MessengerID: ID,
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
				message.Image = telepathy.NewImage(
					telepathy.ByteContent{
						Type:    response.ContentType,
						Content: content,
					})
			default:
				m.logger.Warnf("unsupported message type: %T", event.Message)
				continue
			}
			m.handler(m.ctx, message)
		}
	}
}

func (m *messenger) getSourceProfile(source *linebot.EventSource) (*telepathy.MsgrUserProfile, string) {
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
