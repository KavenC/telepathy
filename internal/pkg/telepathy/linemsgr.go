package telepathy

import (
	"net/http"
	"os"

	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/sirupsen/logrus"
)

func init() {
	RegisterMessenger(&LineMessenger{})
}

// LineMessenger implements the communication with Line APP
type LineMessenger struct {
	bot *linebot.Client
}

func (m *LineMessenger) name() string {
	return "LINE"
}

func (m *LineMessenger) start() {
	bot, err := linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_TOKEN"),
	)
	if err != nil {
		logrus.Error("LINE starts failed: " + err.Error())
		return
	}
	m.bot = bot
	RegisterWebhook("line-callback", m.handler)
}

func (m *LineMessenger) handler(response http.ResponseWriter, request *http.Request) {
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
			message := Message{Messenger: m, ReplyID: event.ReplyToken}
			switch lineMessage := event.Message.(type) {
			case *linebot.TextMessage:
				// Parse Command
				message.SourceProfile = m.getSourceProfile(event.Source)
				message.Text = lineMessage.Text
				if message.SourceProfile == nil {
					logrus.Warn("Ignored message with unknown source")
				} else {
					HandleMessage(&message)
				}
			}
		}
	}
}

func (m *LineMessenger) getSourceProfile(source *linebot.EventSource) *MsgrUserProfile {
	if source.GroupID != "" {
		profile, err := m.bot.GetGroupMemberProfile(source.GroupID, source.UserID).Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{"GroupID": source.GroupID, "UserID": source.UserID})
			logger.Error("GetGroupMemberProfile failed: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else if source.UserID != "" {
		profile, err := m.bot.GetProfile(source.UserID).Do()
		if err != nil {
			logger := logrus.WithField("UserID", source.UserID)
			logger.Error("GetProfile failed: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else if source.RoomID != "" {
		profile, err := m.bot.GetRoomMemberProfile(source.RoomID, source.UserID).Do()
		if err != nil {
			logger := logrus.WithFields(logrus.Fields{"RoomID": source.RoomID, "UserID": source.UserID})
			logger.Error("GetRoomMemberProfile failed: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else {
		logrus.Warn("Unknown source " + source.Type)
		return nil
	}
}
