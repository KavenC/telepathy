package telepathy

import (
	"context"
	"net/http"
	"os"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
)

func init() {
	RegisterMessenger(&LineMessenger{replyTokenMap: make(map[string]string)})
}

// LineMessenger implements the communication with Line APP
type LineMessenger struct {
	bot           *linebot.Client
	replyTokenMap map[string]string
	ctx           context.Context
}

func (m *LineMessenger) name() string {
	return "LINE"
}

func (m *LineMessenger) init() error {
	bot, err := linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_TOKEN"),
	)
	if err != nil {
		return err
	}
	m.bot = bot
	RegisterWebhook("line-callback", m.handler)

	return nil
}

func (m *LineMessenger) start(ctx context.Context) {
	m.ctx = ctx
}

func (m *LineMessenger) handler(response http.ResponseWriter, request *http.Request) {
	if m.ctx == nil {
		logger := logrus.WithField("messenger", m.name())
		logger.Warn("Dropped event.")
		return
	}

	events, err := m.bot.ParseRequest(request)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			response.WriteHeader(400)
		} else {
			response.WriteHeader(500)
		}
		return
	}

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			message := InboundMessage{Messenger: m}
			message.SourceProfile, message.SourceID = m.getSourceProfile(event.Source)
			message.IsDirectMessage = message.SourceProfile.ID == message.SourceID
			m.replyTokenMap[message.SourceID] = event.ReplyToken
			switch lineMessage := event.Message.(type) {
			case *linebot.TextMessage:
				// Parse Command
				message.Text = lineMessage.Text
				if message.SourceProfile == nil {
					logrus.Warn("Ignored message with unknown source")
				} else {
					HandleInboundMessage(m.ctx, &message)
				}
			}
		}
	}
}

func (m *LineMessenger) getSourceProfile(source *linebot.EventSource) (*MsgrUserProfile, string) {
	if source.GroupID != "" {
		profile, err := m.bot.GetGroupMemberProfile(source.GroupID, source.UserID).Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{"GroupID": source.GroupID, "UserID": source.UserID})
			logger.Error("GetGroupMemberProfile failed: " + err.Error())
			return nil, ""
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}, source.GroupID
	} else if source.UserID != "" {
		profile, err := m.bot.GetProfile(source.UserID).Do()
		if err != nil {
			logger := logrus.WithField("UserID", source.UserID)
			logger.Error("GetProfile failed: " + err.Error())
			return nil, ""
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}, source.UserID
	} else if source.RoomID != "" {
		profile, err := m.bot.GetRoomMemberProfile(source.RoomID, source.UserID).Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{"RoomID": source.RoomID, "UserID": source.UserID})
			logger.Error("GetRoomMemberProfile failed: " + err.Error())
			return nil, ""
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}, source.RoomID
	} else {
		logrus.Warn("Unknown source " + source.Type)
		return nil, ""
	}
}

func (m *LineMessenger) send(message *OutboundMessage) {
	replyToken, ok := m.replyTokenMap[message.TargetID]
	lineMessage := linebot.NewTextMessage(message.Text)
	if ok {
		delete(m.replyTokenMap, message.TargetID)
		call := m.bot.ReplyMessage(replyToken, lineMessage)
		_, err := call.Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{
				"messenger":   m.name(),
				"target":      message.TargetID,
				"reply_token": replyToken,
			})
			logger.Error("Send message fail.")
		}
	} else {
		call := m.bot.PushMessage(message.TargetID, lineMessage)
		_, err := call.Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{
				"messenger": m.name(),
				"target":    message.TargetID,
			})
			logger.Error("Send message fail.")
		}
	}
}
