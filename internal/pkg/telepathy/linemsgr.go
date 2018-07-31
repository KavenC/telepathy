package telepathy

import (
	"log"
	"net/http"
	"os"

	"github.com/line/line-bot-sdk-go/linebot"
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
		log.Fatal(err)
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
					log.Print("Warning: ignored message with unknown source")
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
			log.Print("Error: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else if source.UserID != "" {
		profile, err := m.bot.GetProfile(source.UserID).Do()
		if err != nil {
			log.Print("Error: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else if source.RoomID != "" {
		profile, err := m.bot.GetRoomMemberProfile(source.RoomID, source.UserID).Do()
		if err != nil {
			log.Print("Error: " + err.Error())
			return nil
		}
		return &MsgrUserProfile{ID: profile.UserID, DisplayName: profile.DisplayName}
	} else {
		log.Print("Warning: Unknown source " + source.Type)
		return nil
	}
}
