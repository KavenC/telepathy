// Package slackmsg implements the Messenger handler of Slack for Telepathy framework.
// Needed configs:
// - CLIENT_ID: The client ID of slack app
// - CLIENT_SECRET: The client secret of slack app
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

var validSubType = map[string]bool{
	"":           true,
	"file_share": true,
}

type appInfo struct {
	clientID      string
	clientSecret  string
	signingSecret []byte
}

type messenger struct {
	telepathy.MessengerPlugin
	session    *telepathy.Session
	handler    telepathy.InboundMsgHandler
	ctx        context.Context
	botInfoMap botInfoMap
	app        appInfo
	logger     *logrus.Entry
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

	secret, ok := param.Config["SIGNING_SECRET"]
	if !ok {
		return nil, InitError{msg: "config: SIGNING_SECRET not found"}
	}
	secretString, ok := secret.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: SIGNING_SECRET"}
	}
	msgr.app.signingSecret = []byte(secretString)

	config, ok := param.Config["CLIENT_ID"]
	if !ok {
		return nil, InitError{msg: "config: CLIENT_ID not found"}
	}
	msgr.app.clientID, ok = config.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: CLIENT_ID"}
	}

	config, ok = param.Config["CLIENT_SECRET"]
	if !ok {
		return nil, InitError{msg: "config: CLIENT_SECRET not found"}
	}
	msgr.app.clientSecret, ok = config.(string)
	if !ok {
		return nil, InitError{msg: "invalid config: CLIENT_SECRET"}
	}

	return &msgr, nil
}

func (m *messenger) Start(ctx context.Context) {
	m.ctx = ctx
	m.botInfoMap = botInfoMap{}
	if err := m.readBotInfoFromDB(); err != nil {
		m.logger.Warnf("load bot info failed: %s", err.Error())
	}

	m.session.WebServer.RegisterWebhook("slack-callback", m.webhook)
	m.session.WebServer.RegisterWebhook("slack-oauth", m.oauth)
}

func (m *messenger) Send(message *telepathy.OutboundMessage) {
	channel, err := newUniqueChannel(message.TargetID)
	if err != nil {
		m.logger.WithField("phase", "send").Errorf("invalid target ID: %s (%s)", message.TargetID, err.Error())
		return
	}

	info, ok := m.botInfoMap[channel.TeamID]
	if !ok {
		m.logger.WithField("phase", "send").Errorf("unauthorized team: %s", channel.TeamID)
		return
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
	mac := hmac.New(sha256.New, m.app.signingSecret)
	mac.Write([]byte(verifyString))
	expectedMac := fmt.Sprintf("v0=%s", hex.EncodeToString(mac.Sum(nil)))

	if subtle.ConstantTimeCompare([]byte(checkMac[0]), []byte(expectedMac)) != 1 {
		m.logger.Errorf("verify failed. Our: %s Their: %s", expectedMac, checkMac[0])
		return false
	}

	return true
}

func (m *messenger) createImgContent(bot *slack.Client, file slackevents.File) *telepathy.ByteContent {
	imgBuffer := bytes.Buffer{}
	err := bot.GetFile(file.URLPrivateDownload, &imgBuffer)
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

func (m *messenger) handleMessage(teamID string, ev *slackevents.MessageEvent) {
	info, ok := m.botInfoMap[teamID]
	if !ok {
		m.logger.Warnf("received from unknwon team: %s", teamID)
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
		user, err := bot.GetUserInfo(ev.User)
		if err == nil {
			srcProfile.DisplayName = user.Profile.DisplayName
		} else {
			m.logger.WithField("user", ev.User).Warnf("get user failed: %s", err.Error())
		}
	} else {
		m.logger.Warnf("unknown message: %+v", ev)
		return
	}

	uniqueChannelID, err := unqiueChannel{TeamID: teamID, ChannelID: ev.Channel}.encode()
	if err != nil {
		m.logger.Errorf("failed to create uniqueChannel: %s", err.Error())
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
					message.Image = telepathy.NewImage(*content)
				}
				// TODO: support multiple image sharing
				break
			}
		}
	}

	m.handler(m.ctx, message)
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
			m.handleMessage(eventsAPIEvent.TeamID, ev)
		case *slackevents.TokensRevokedEvent:
			m.logger.Infof("tokens revoked, team: %s", eventsAPIEvent.TeamID)
			delete(m.botInfoMap, eventsAPIEvent.TeamID)
			go func() { <-m.writeBotInfoToDB() }()
		}
	}
}

func (m *messenger) oauth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	logger := m.logger.WithField("phase", "oauth")

	switch state := request.URL.Query().Get("state"); state {
	case "new":
		code := request.URL.Query().Get("code")
		if code != "" {
			oauthResp, err := slack.GetOAuthResponse(&http.Client{}, m.app.clientID, m.app.clientSecret, code, "")
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
