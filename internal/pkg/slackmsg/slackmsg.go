// Package slackmsg implements the Messenger handler of Slack for Telepathy framework.
// Needed configs:
// - CLIENT_ID: The client ID of slack app
// - CLIENT_SECRET: The client secret of slack app
// - SIGNING_SECRET: Used to verify the integrity of requests from Slack.
//                   It can be found in Slack app management panel
package slackmsg

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/imgur"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	inMsgLen = 10
	dbReqLen = 1
)

var validSubType = map[string]bool{
	"":           true,
	"file_share": true,
}

// Messenger defines the plugin structure
type Messenger struct {
	SigningSecret []byte
	ClientID      string
	ClientSecret  string
	botInfoMap    botInfoMap
	inMsg         chan telepathy.InboundMessage
	outMsg        <-chan telepathy.OutboundMessage
	dbReq         chan telepathy.DatabaseRequest
	logger        *logrus.Entry
}

// ID implements telepathy.Plugin
func (m *Messenger) ID() string {
	return "SLACK"
}

// SetLogger implements telepathy.Plugin
func (m *Messenger) SetLogger(logger *logrus.Entry) {
	m.logger = logger
}

// Start implements telepathy.Plugin
func (m *Messenger) Start() {
	m.botInfoMap = botInfoMap{}
	if err := m.readBotInfoFromDB(); err != nil {
		m.logger.Warnf("load bot info failed: %s", err.Error())
	}
	m.logger.Info("started")
	m.transmitter()
	m.logger.Info("terminated")
}

// Stop implements telepathy.Plugin
func (m *Messenger) Stop() {
	close(m.inMsg)
	<-m.writeBotInfoToDB()
	close(m.dbReq)
}

// InMsgChannel implements telepathy.PluginMessenger
func (m *Messenger) InMsgChannel() <-chan telepathy.InboundMessage {
	if m.inMsg == nil {
		m.inMsg = make(chan telepathy.InboundMessage, inMsgLen)
	}
	return m.inMsg
}

// AttachOutMsgChannel implements telepathy.PluginMessenger
func (m *Messenger) AttachOutMsgChannel(ch <-chan telepathy.OutboundMessage) {
	m.outMsg = ch
}

// Webhook implements telepathy.PluginWebhookHandler
func (m *Messenger) Webhook() map[string]telepathy.HTTPHandler {
	return map[string]telepathy.HTTPHandler{
		"slack-callback": m.webhook,
		"slack-oauth":    m.oauth,
	}
}

// SetWebhookURL implements telepathy.PluginWebhookHandler
func (m *Messenger) SetWebhookURL(map[string]*url.URL) {

}

// DBRequestChannel implements PluginDatabaseUser
func (m *Messenger) DBRequestChannel() <-chan telepathy.DatabaseRequest {
	if m.dbReq == nil {
		m.dbReq = make(chan telepathy.DatabaseRequest, dbReqLen)
	}
	return m.dbReq
}

func (m *Messenger) transmitter() {
	for message := range m.outMsg {
		chID := message.ToChannel.ChannelID
		channel, err := newUniqueChannel(chID)
		if err != nil {
			m.logger.WithField("phase", "send").Errorf("invalid target ID: %s (%s)", chID, err.Error())
			continue
		}

		info, ok := m.botInfoMap[channel.TeamID]
		if !ok {
			m.logger.WithField("phase", "send").Errorf("unauthorized team: %s", channel.TeamID)
			continue
		}

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

		bot := slack.New(info.AccessToken)
		bot.PostMessage(channel.ChannelID, options...)
	}
}

func (m *Messenger) verifyRequest(header http.Header, body []byte) bool {
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
	mac := hmac.New(sha256.New, m.SigningSecret)
	mac.Write([]byte(verifyString))
	expectedMac := fmt.Sprintf("v0=%s", hex.EncodeToString(mac.Sum(nil)))

	if subtle.ConstantTimeCompare([]byte(checkMac[0]), []byte(expectedMac)) != 1 {
		m.logger.Errorf("verify failed. Our: %s Their: %s", expectedMac, checkMac[0])
		return false
	}

	return true
}

func (m *Messenger) createImgContent(bot *slack.Client, file slackevents.File) *imgur.ByteContent {
	imgBuffer := bytes.Buffer{}
	err := bot.GetFile(file.URLPrivateDownload, &imgBuffer)
	if err != nil {
		m.logger.Error("download attached image failed: " + err.Error())
		return nil
	}
	content := imgur.ByteContent{
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

func (m *Messenger) handleMessage(teamID string, ev *slackevents.MessageEvent) {
	logger := m.logger.WithField("phase", "handleMessage")
	info, ok := m.botInfoMap[teamID]
	if !ok {
		logger.Warnf("received from unknwon team: %s", teamID)
		return
	}

	if ev.BotID == info.BotID {
		// skip self messages
		return
	}

	if _, ok := validSubType[ev.SubType]; !ok {
		// Messages with subtype are system notifications
		// we should not response to these messages
		return
	}

	bot := slack.New(info.AccessToken)
	srcProfile := telepathy.MsgrUserProfile{}
	if ev.BotID != "" {
		srcProfile.ID = info.BotUserID
		srcProfile.DisplayName = ev.Username
	} else if ev.User != "" {
		srcProfile.ID = ev.User

		// Workaround for issue #19
		retryCount := 3
		for try := 1; try <= retryCount; try++ {
			user, err := bot.GetUserInfo(ev.User)
			if err == nil {
				srcProfile.DisplayName = user.Profile.DisplayName
				break
			}
			logger.WithField("user", ev.User).Warnf("get user failed (%d/%d): %s", try, retryCount, err.Error())
		}
	} else {
		logger.Warnf("unknown message: %+v", ev)
		return
	}

	uniqueChannelID, err := unqiueChannel{TeamID: teamID, ChannelID: ev.Channel}.encode()
	if err != nil {
		logger.Errorf("failed to create uniqueChannel: %s", err.Error())
		return
	}
	message := telepathy.InboundMessage{
		FromChannel: telepathy.Channel{
			MessengerID: m.ID(),
			ChannelID:   uniqueChannelID,
		},
		SourceProfile:   &srcProfile,
		Text:            ev.Text,
		IsDirectMessage: ev.ChannelType == "im",
	}

	// handle image upload/share
	if len(ev.Files) > 0 {
		for _, file := range ev.Files {
			if strings.HasPrefix(file.Mimetype, "image") {
				content := m.createImgContent(bot, file)
				if content != nil {
					message.Image = imgur.NewImage(*content)
				}
				// TODO: support multiple image sharing
				break
			}
		}
	}

	m.inMsg <- message
}

func (m *Messenger) webhook(response http.ResponseWriter, request *http.Request) {
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
			m.handleMessage(eventsAPIEvent.TeamID, ev)
		case *slackevents.TokensRevokedEvent:
			m.logger.Infof("tokens revoked, team: %s", eventsAPIEvent.TeamID)
			delete(m.botInfoMap, eventsAPIEvent.TeamID)
			go func() { <-m.writeBotInfoToDB() }()
		}
	}
}

func (m *Messenger) oauth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	logger := m.logger.WithField("phase", "oauth")

	switch state := request.URL.Query().Get("state"); state {
	case "new":
		code := request.URL.Query().Get("code")
		if code != "" {
			oauthResp, err := slack.GetOAuthResponse(&http.Client{}, m.ClientID, m.ClientSecret, code, "")
			if err != nil {
				msg := fmt.Sprintf("oauth failed: %s", err.Error())
				logger.Errorf(msg)
				response.Write([]byte(msg))
				return
			}

			if oldInfo, ok := m.botInfoMap[oauthResp.TeamID]; ok {
				logger.Warnf("TeamID exists: %s, Info: %+v", oauthResp.TeamID, oldInfo)
			}
			info := botInfo{
				AccessToken: oauthResp.Bot.BotAccessToken,
				BotUserID:   oauthResp.Bot.BotUserID}
			userInfo, _ := slack.New(info.AccessToken).GetUserInfo(info.BotUserID)
			info.BotID = userInfo.Profile.BotID
			m.botInfoMap[oauthResp.TeamID] = info
			go func() { <-m.writeBotInfoToDB() }()

			response.Write([]byte("Telepathy has been added to your team"))
			response.WriteHeader(http.StatusOK)
			return
		}
	}
	response.WriteHeader(http.StatusBadRequest)
	return
}
