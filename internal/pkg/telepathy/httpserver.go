package telepathy

import (
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"
)

var (
	webhookList = []string{}
	mux         http.ServeMux
	validHook   = regexp.MustCompile(`^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$`)
)

// RegisterWebhook is used to register a http callback, like webhooks
// The pattern can only be in this regular expression format: ^[A-Za-z]+(-[A-Za-z0-9]+){0,3}$, otherwise it is ignored.
// If the pattern is already registered, it panics.
// Webhooks are always registered at (host)/webhook/<patter>
func RegisterWebhook(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	logrus.Info("Registering webhook pattern: " + pattern)

	for _, value := range webhookList {
		if value == pattern {
			panic("RegisterWebhook is called twice with pattern: " + pattern)
		}
	}

	if validHook.MatchString(pattern) {
		mux.HandleFunc("/webhook/"+pattern, handler)
	} else {
		panic("Invalid webhook pattern: " + pattern)
	}
}

func getServeMux() *http.ServeMux {
	return &mux
}
