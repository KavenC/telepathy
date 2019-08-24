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

type httpServer struct {
	http.Server
	uRL         *url.URL
	webhookList map[string]HTTPHandler
}

func (server *httpServer) registerWebhook(pattern string, handler HTTPHandler) (*url.URL, error) {
	logger := logrus.WithField("module", "httpserv").WithField("webhook", pattern)

	if !validHook.MatchString(pattern) {
		return nil, fmt.Errorf("Pattern: %s is invalid", pattern)
	}

	if server.webhookList == nil {
		server.webhookList = make(map[string]HTTPHandler)
	}

	if _, ok := server.webhookList[pattern]; ok {
		return nil, fmt.Errorf("Pattern: %s has been registered", pattern)
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

func newWebServer(urlstr string, port string) (*httpServer, error) {
	server := httpServer{}
	var err error
	server.uRL, err = url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	server.Addr = ":" + port
	return &server, nil
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
