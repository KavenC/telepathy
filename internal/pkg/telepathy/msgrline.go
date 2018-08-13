package telepathy

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
)

func init() {
	RegisterMessenger(&LineMessenger{replyTokenMap: sync.Map{}})
}

// LineMessenger implements the communication with Line APP
type LineMessenger struct {
	bot           *linebot.Client
	replyTokenMap sync.Map
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
			message := InboundMessage{FromChannel: Channel{
				MessengerID: m.name(),
			}}
			profile, channelID := m.getSourceProfile(event.Source)
			message.SourceProfile = profile
			message.FromChannel.ChannelID = channelID
			message.IsDirectMessage = message.SourceProfile.ID == channelID
			item, _ := m.replyTokenMap.LoadOrStore(channelID, &sync.Pool{})
			pool, _ := item.(*sync.Pool)
			pool.Put(event.ReplyToken)
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
	// Try to use reply token
	item, _ := m.replyTokenMap.LoadOrStore(message.TargetID, &sync.Pool{})
	pool, _ := item.(*sync.Pool)
	item = pool.Get()
	lineMessage := linebot.NewTextMessage(message.Text)
	if item != nil {
		replyTokenStr, _ := item.(string)
		call := m.bot.ReplyMessage(replyTokenStr, lineMessage)
		_, err := call.Do()

		if err == nil {
			return
		}

		// If send failed with replyToken, output err msg and retry with PushMessage
		logger := logrus.WithFields(logrus.Fields{
			"messenger":   m.name(),
			"target":      message.TargetID,
			"reply_token": replyTokenStr,
		})
		logger.Warn("Reply message fail.")
	}

	call := m.bot.PushMessage(message.TargetID, lineMessage)
	_, err := call.Do()
	if err != nil {
		logger := logrus.WithFields(logrus.Fields{
			"messenger": m.name(),
			"target":    message.TargetID,
		})
		logger.Error("Push message fail.")
	}

}
