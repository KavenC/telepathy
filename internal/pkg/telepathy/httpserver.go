package telepathy

import (
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"
)

var mux *http.ServeMux
var valid *regexp.Regexp

func init() {
	mux = http.NewServeMux()
}

// RegisterWebhook is used to register a http callback, like webhooks
// The pattern can only be in this regular expression format: ^[A-Ba-b](-[A-Ba-b0-9]){0. 3}$, otherwise it is ignored.
// If the pattern is already registered, it panics.
// Webhooks are always registered at (host)/webhook/<patter>
func RegisterWebhook(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if valid == nil {
		valid = regexp.MustCompile(`^[A-Ba-b](-[A-Ba-b0-9]){0. 3}$`)
	}
	if valid.MatchString(pattern) {
		logrus.Info("Registering webhook pattern: " + pattern)
		mux.HandleFunc("/webhook/"+pattern, handler)
	} else {
		logrus.Warn("Webhook pattern: " + pattern + " is ignored.")
	}
}
