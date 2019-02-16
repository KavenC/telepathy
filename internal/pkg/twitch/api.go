package twitch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

const apiURL = "https://api.twitch.tv/helix/"

type twitchAPI struct {
	clientID      string
	clientSecret  string
	websubSecret  string
	httpTransport *http.Transport
	httpClient    *http.Client
	pendingWebSub sync.Map
	logger        *logrus.Entry
}

func (t *twitchAPI) newRequest(method, target string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, fmt.Sprintf("%s/%s", apiURL, target), body)
}

func (t *twitchAPI) sendReq(ctx context.Context, req *http.Request,
	respChan chan<- *http.Response, errChan chan<- error) {
	req.Header.Add("Client-ID", t.clientID)
	req = req.WithContext(ctx)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		errChan <- err
		t.httpTransport.CancelRequest(req)
		return
	}

	respChan <- resp
	return
}
