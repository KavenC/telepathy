package telepathy

import (
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"
)

// HTTPHandler defines the callback function signature for Webhook handler
type HTTPHandler func(http.ResponseWriter, *http.Request)

// WebhookExistsError indicates a failure when registering Webhook
// since the pattern is already registered
type WebhookExistsError struct {
	Pattern string
}

// WebhookInvalidError indicates a failure when registering Webhook
// since the pattern does not comply naming rule
type WebhookInvalidError struct {
	Pattern string
}

var (
	webhookList = make(map[string]HTTPHandler)
	validHook   = regexp.MustCompile(`^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$`)
)

// RegisterWebhook is used to register a http callback, like webhooks
// The pattern can only be in this regular expression format: ^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$, otherwise it is ignored.
// If the pattern is already registered, it panics.
// Webhooks are always registered at (host)/webhook/<patter>
func RegisterWebhook(pattern string, handler HTTPHandler) error {
	logger := logrus.WithField("module", "httpserv")

	if !validHook.MatchString(pattern) {
		logger.WithField("webhook", pattern).Error("illegal webhook name")
		return WebhookInvalidError{Pattern: pattern}
	}

	if _, ok := webhookList[pattern]; ok {
		logger.WithField("webhook", pattern).Error("registered multiple times")
		return WebhookExistsError{Pattern: pattern}
	}

	webhookList[pattern] = handler
	logger.WithField("webhook", pattern).Info("registered")
	return nil
}

func serveMux() *http.ServeMux {
	mux := http.ServeMux{}
	for pattern, handler := range webhookList {
		mux.HandleFunc("/webhook/"+pattern, handler)
	}
	return &mux
}

func (e WebhookExistsError) Error() string {
	return "Pattern: " + e.Pattern + " is already registered"
}

func (e WebhookInvalidError) Error() string {
	return "Pattern: " + e.Pattern + " is invalid"
}
