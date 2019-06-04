package telepathy

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/sirupsen/logrus"
)

var validHook = regexp.MustCompile(`^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$`)

const webhookRoot = "/webhook/"

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
	uRL         *url.URL
	webhookList map[string]HTTPHandler
}

// RegisterWebhook is used to register a http callback, like webhooks
// The pattern can only be in this regular expression format: ^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$, otherwise
// the registeration will be ignored.
// If the pattern is already registered, registeration will be ignored.
// Webhooks are always registered at (host)/webhook/<patter>
// Returns the Webhook callback URL if no error
func (server *httpServer) RegisterWebhook(pattern string, handler HTTPHandler) (*url.URL, error) {
	logger := logrus.WithField("module", "httpserv").WithField("webhook", pattern)

	if !validHook.MatchString(pattern) {
		logger.Errorf("illegal webhook name: %s", pattern)
		return nil, WebhookInvalidError{Pattern: pattern}
	}

	if server.webhookList == nil {
		server.webhookList = make(map[string]HTTPHandler)
	}

	if _, ok := server.webhookList[pattern]; ok {
		logger.Errorf("already registered: %s", pattern)
		return nil, WebhookExistsError{Pattern: pattern}
	}

	server.webhookList[pattern] = handler
	logger.Infof("registered webhook: %s", pattern)
	retURL := server.webhookURL()
	retURL.Path += pattern
	return retURL, nil
}

func (server *httpServer) serveMux() *http.ServeMux {
	mux := http.ServeMux{}
	for pattern, handler := range server.webhookList {
		mux.HandleFunc(webhookRoot+pattern, handler)
	}
	return &mux
}

func (server *httpServer) init(port string) error {
	logger := logrus.WithFields(logrus.Fields{
		"module": "httpServer",
	})
	logger.Infof("httpServer port: %s", port)
	server.Addr = ":" + port
	return nil
}

func (server *httpServer) finalize() {
	mux := server.serveMux()

	// Add a simple response at root
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, "Telepathy Bot is Running")
	})
	server.Handler = mux
}

func (server *httpServer) webhookURL() *url.URL {
	copyURL, _ := url.Parse(server.uRL.String())
	copyURL.Path = webhookRoot
	return copyURL
}

func (e WebhookExistsError) Error() string {
	return "Pattern: " + e.Pattern + " is already registered"
}

func (e WebhookInvalidError) Error() string {
	return "Pattern: " + e.Pattern + " is invalid"
}
