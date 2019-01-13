package telepathy

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"
)

var validHook = regexp.MustCompile(`^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$`)

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

type httpServer struct {
	http.Server
	webhookList map[string]HTTPHandler
}

// RegisterWebhook is used to register a http callback, like webhooks
// The pattern can only be in this regular expression format: ^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$, otherwise it is ignored.
// If the pattern is already registered, it panics.
// Webhooks are always registered at (host)/webhook/<patter>
func (server *httpServer) RegisterWebhook(pattern string, handler HTTPHandler) error {
	logger := logrus.WithField("module", "httpserv").WithField("webhook", pattern)

	if !validHook.MatchString(pattern) {
		logger.Errorf("illegal webhook name: %s", pattern)
		return WebhookInvalidError{Pattern: pattern}
	}

	if server.webhookList == nil {
		server.webhookList = make(map[string]HTTPHandler)
	}

	if _, ok := server.webhookList[pattern]; ok {
		logger.Errorf("already registered: %s", pattern)
		return WebhookExistsError{Pattern: pattern}
	}

	server.webhookList[pattern] = handler
	logger.Infof("registered webhook: %s", pattern)
	return nil
}

func (server *httpServer) serveMux() *http.ServeMux {
	mux := http.ServeMux{}
	for pattern, handler := range server.webhookList {
		mux.HandleFunc("/webhook/"+pattern, handler)
	}
	return &mux
}

func (server *httpServer) init(port string) {
	logrus.WithFields(logrus.Fields{
		"module": "httpServer",
	}).Info("httpServer init port: " + port)

	server.Addr = ":" + port
	mux := server.serveMux()

	// Add a simple response at root
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, "Telepathy Bot is Running")
	})
	server.Handler = mux
}

func (e WebhookExistsError) Error() string {
	return "Pattern: " + e.Pattern + " is already registered"
}

func (e WebhookInvalidError) Error() string {
	return "Pattern: " + e.Pattern + " is invalid"
}
