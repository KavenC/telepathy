package line

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const name = "LINE"

func init() {
	telepathy.RegisterMessenger(name, new)
}

// LineMessenger implements the communication with Line APP
type messenger struct {
	*telepathy.MsgrCtorParam
	ctx           context.Context
	bot           *linebot.Client
	replyTokenMap sync.Map
}

func new(param *telepathy.MsgrCtorParam) (telepathy.Messenger, error) {
	msg := messenger{MsgrCtorParam: param}
	bot, err := linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_TOKEN"),
	)
	if err != nil {
		return nil, err
	}
	msg.bot = bot
	telepathy.RegisterWebhook("line-callback", msg.handler)

	return &msg, nil
}

func (m *messenger) Name() string {
	return name
}

func (m *messenger) Start(ctx context.Context) {
	m.ctx = ctx
}

func (m *messenger) handler(response http.ResponseWriter, request *http.Request) {
	if m.ctx == nil {
		m.Logger.Warn("Dropped event")
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
			message := telepathy.InboundMessage{FromChannel: telepathy.Channel{
				MessengerID: name,
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
					m.Logger.Warn("Ignored message with unknown source")
				} else {
					m.MsgHandler(m.ctx, m.MsgrCtorParam.Session, message)
				}
			}
		}
	}
}

func (m *messenger) getSourceProfile(source *linebot.EventSource) (*telepathy.MsgrUserProfile, string) {
	if source.GroupID != "" {
		profile, err := m.bot.GetGroupMemberProfile(source.GroupID, source.UserID).Do()
		if err != nil {
			logger := m.Logger.WithFields(logrus.Fields{"GroupID": source.GroupID, "UserID": source.UserID})
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
			logger := m.Logger.WithField("UserID", source.UserID)
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
			logger := m.Logger.WithFields(logrus.Fields{"RoomID": source.RoomID, "UserID": source.UserID})
			logger.Error("GetRoomMemberProfile failed: " + err.Error())
			return nil, ""
		}
		return &telepathy.MsgrUserProfile{
			ID:          profile.UserID,
			DisplayName: profile.DisplayName,
		}, source.RoomID
	} else {
		m.Logger.Warn("Unknown source " + source.Type)
		return nil, ""
	}
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
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
		logger := m.Logger.WithFields(logrus.Fields{
			"target":      message.TargetID,
			"reply_token": replyTokenStr,
		})
		logger.Warn("Reply message fail.")
	}

	call := m.bot.PushMessage(message.TargetID, lineMessage)
	_, err := call.Do()
	if err != nil {
		logger := m.Logger.WithField("target", message.TargetID)
		logger.Error("Push message fail.")
	}

}
