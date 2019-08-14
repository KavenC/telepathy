package twitch

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// Webhook implements telepathy.PluginWebhookHandler
func (s *Service) Webhook() map[string]telepathy.HTTPHandler {
	return map[string]telepathy.HTTPHandler{
		"twitch": s.webhook,
	}
}

// SetWebhookURL implements telepathy.PluginWebhookHandler
func (s *Service) SetWebhookURL(urlmap map[string]*url.URL) {
	s.webhookURL = urlmap["twitch"]
}

func (s *Service) webhook(response http.ResponseWriter, req *http.Request) {
	if req.Method == "GET" {
		s.api.websubValidate(response, req)
		return
	}

	resp := s.handleWebhookReq(req)
	response.WriteHeader(resp)
}

func (s *Service) handleWebhookReq(req *http.Request) int {
	logger := s.logger.WithField("phase", "handleWebhookReq")

	// Validate
	body, err := ioutil.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		logger.Errorf("error on reading body: %s", err.Error())
		return 200
	}

	contentLen, err := strconv.Atoi(req.Header.Get("content-length"))
	if err != nil {
		logger.Errorf("error on getting content length: %s", err.Error())
		return 200
	}
	if contentLen != len(body) {
		logger.Error("mismatch content length")
		logger.Errorf("header: %d body: %d", contentLen, len(body))
		logger.Errorf("content: %s", body)
		return 200
	}

	methodMac := req.Header.Get("X-Hub-Signature")
	splitted := strings.SplitN(methodMac, "=", 2)
	if len(splitted) < 2 {
		logger.Warnf("invalid signature header: %s", methodMac)
		return 200
	}
	method := splitted[0]
	messageMAC := []byte(splitted[1])
	var mac hash.Hash
	switch method {
	case "sha1":
		mac = hmac.New(sha1.New, s.websubSecret)
	case "sha256":
		mac = hmac.New(sha256.New, s.websubSecret)
	case "sha512":
		mac = hmac.New(sha512.New, s.websubSecret)
	default:
		logger.Warnf("invalid validation method: %s", methodMac)
		return 200
	}

	_, err = mac.Write(body)
	if err != nil {
		logger.Error("error when writing to hash computer")
		return 200
	}
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal(messageMAC, []byte(expected)) {
		logger.Warnf("wrong signature   : %s", messageMAC)
		logger.Warnf("expected signature: %s", expected)
		return 200
	}

	return <-s.handleWebsubNotification(req, body)
}
