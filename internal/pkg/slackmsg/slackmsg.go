// Package slackmsg implements the Messenger handler of Slack for Telepathy framework.
// Needed configs:
// - BOT_TOKEN: A valid slack bot token. For more information, please refer
//              to https://api.slack.com/bot-users
// - SIGNING_SECRET: Used to verify the integrity of requests from Slack.
//                   It can be found in Slack app management panel
package slackmsg

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	// ID is a unique messenger identifier
	ID = "SLACK"
)

var validSubType = map[string]bool{"": true, "file_share": true}

type messenger struct {
	telepathy.MessengerPlugin
	session       *telepathy.Session
	handler       telepathy.InboundMsgHandler
	ctx           context.Context
	bot           *slack.Client
	self          *slack.User
	signingSecret []byte
	logger        *logrus.Entry
}

// InitError indicates an error when initializing messenger plugin
type InitError struct {
	msg string
}

func init() {
	telepathy.RegisterMessenger(ID, new)
}

func (e InitError) Error() string {
	return "Slack init failed: " + e.msg
}

func (m *messenger) ID() string {
	return ID
}

func new(param *telepathy.MsgrCtorParam) (telepathy.Messenger, error) {
	msgr := messenger{
		session: param.Session,
		handler: param.MsgHandler,
		logger:  param.Logger,
	}

	config, ok := param.Config["BOT_TOKEN"]
	if !ok {
		return nil, InitError{msg: "config: BOT_TOKEN not found"}
	}
	token, ok := config.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: BOT_TOKEN"}
	}

	secret, ok := param.Config["SIGNING_SECRET"]
	if !ok {
		return nil, InitError{msg: "config: SIGNING_SECRET not found"}
	}
	secretString, ok := secret.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: SIGNING_SECRET"}
	}
	msgr.signingSecret = []byte(secretString)

	msgr.bot = slack.New(token)
	param.Session.WebServer.RegisterWebhook("slack-callback", msgr.webhook)

	return &msgr, nil
}

func (m *messenger) Start(ctx context.Context) {
	m.ctx = ctx
	resp, err := m.bot.AuthTest()
	if err != nil {
		m.logger.Errorf("slack bot init failed: %s", err.Error())
	}
	m.self, err = m.bot.GetUserInfo(resp.UserID)
	if err != nil {
		m.logger.Errorf("slack bot init failed: %s", err.Error())
	}
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
	var options []slack.MsgOption
	text := strings.Builder{}
	text.WriteString(message.Text)
	if message.Image != nil {
		imgURL, err := message.Image.FullURL()
		if err == nil {
			if text.Len() > 0 {
				fmt.Fprintf(&text, "\n%s", imgURL)
			} else {
				text.WriteString(imgURL)
			}
		} else {
			m.logger.Warnf("image get FullURL failed: %s", err.Error())
		}
	}

	options = append(options, slack.MsgOptionText(text.String(), false))
	if message.AsName != "" {
		options = append(options, slack.MsgOptionUsername(message.AsName))
	}

	m.bot.PostMessage(message.TargetID, options...)
}

func (m *messenger) verifyRequest(header http.Header, body []byte) bool {
	timestamp, ok := header["X-Slack-Request-Timestamp"]
	if !ok || len(timestamp) > 1 {
		return false
	}

	timeInt, err := strconv.ParseInt(timestamp[0], 10, 64)
	if err != nil {
		return false
	}

	tm := time.Unix(timeInt, 0)
	if time.Now().Sub(tm) > 5*time.Minute {
		return false
	}

	checkMac, ok := header["X-Slack-Signature"]
	if !ok || len(checkMac) > 1 {
		return false
	}

	verifyString := fmt.Sprintf("v0:%s:%s", timestamp[0], body)
	mac := hmac.New(sha256.New, m.signingSecret)
	mac.Write([]byte(verifyString))
	expectedMac := fmt.Sprintf("v0=%s", hex.EncodeToString(mac.Sum(nil)))

	if subtle.ConstantTimeCompare([]byte(checkMac[0]), []byte(expectedMac)) != 1 {
		m.logger.Errorf("verify failed. Our: %s Their: %s", expectedMac, checkMac[0])
		return false
	}

	return true
}

func (m *messenger) createImgContent(file slackevents.File) *telepathy.ByteContent {
	imgBuffer := bytes.Buffer{}
	err := m.bot.GetFile(file.URLPrivateDownload, &imgBuffer)
	if err != nil {
		m.logger.Error("download attached image failed: " + err.Error())
		return nil
	}
	content := telepathy.ByteContent{
		Content: imgBuffer.Bytes(),
	}
	ext := file.Filetype
	if ext == "" {
		m.logger.Warnf("unknown attach image type: %s", ext)
		ext = "png"
	}
	content.Type = "image/" + ext
	return &content
}

func (m *messenger) webhook(response http.ResponseWriter, request *http.Request) {
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		m.logger.Errorf("unable to read request body: %s", err.Error())
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !m.verifyRequest(request.Header, body) {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventsAPIEvent, err := slackevents.ParseEvent(
		json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		m.logger.Errorf("slackevents parsing failed: %s", err.Error())
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	if eventsAPIEvent.Type == slackevents.URLVerification {
		var challengeResponse slackevents.ChallengeResponse
		err := json.Unmarshal(body, &challengeResponse)
		if err != nil {
			m.logger.Errorf("body unmarshal failed: %s", err.Error())
			response.WriteHeader(http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "text")
		response.Write([]byte(challengeResponse.Challenge))
	}

	if eventsAPIEvent.Type == slackevents.CallbackEvent {
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			if ev.BotID == m.self.Profile.BotID {
				// skip self messages
				return
			}

			if _, ok := validSubType[ev.SubType]; !ok {
				// Messages with subtype are system notifications
				// we should not response to these messages
				return
			}

			srcProfile := telepathy.MsgrUserProfile{}
			if ev.BotID != "" {
				srcProfile.ID = ev.BotID
				srcProfile.DisplayName = ev.Username
			} else if ev.User != "" {
				srcProfile.ID = ev.User
				user, err := m.bot.GetUserInfo(ev.User)
				if err == nil {
					srcProfile.DisplayName = user.Profile.DisplayName
				} else {
					m.logger.WithField("user", ev.User).Warnf("get user failed: %s", err.Error())
				}
			} else {
				m.logger.Warnf("unknown message: %+v", ev)
				return
			}

			message := telepathy.InboundMessage{
				FromChannel: telepathy.Channel{
					MessengerID: m.ID(),
					ChannelID:   ev.Channel,
				},
				SourceProfile:   &srcProfile,
				Text:            ev.Text,
				IsDirectMessage: ev.ChannelType == "im",
			}

			// handle image upload/share
			if len(ev.Files) > 0 {
				for _, file := range ev.Files {
					if strings.HasPrefix(file.Mimetype, "image") {
						content := m.createImgContent(file)
						if content != nil {
							message.Image = telepathy.NewImage(*content)
						}
						// TODO: support multiple image sharing
						break
					}
				}
			}

			m.handler(m.ctx, message)
		}
	}
}
