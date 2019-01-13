package twitch

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

const apiURL = "https://api.twitch.tv/helix/"

type twitchAPI struct {
	clientID      string
	clientSecret  string
	httpTransport *http.Transport
	httpClient    *http.Client
	logger        *logrus.Entry
}

func (t *twitchAPI) get(ctx context.Context, target string, param map[string]string,
	respChan chan<- []byte, errChan chan<- error) {
	paramList := make([]string, 0, len(param))
	for key, value := range param {
		paramList = append(paramList, fmt.Sprintf("%s=%s", key, value))
	}
	paramStr := strings.Join(paramList, "&")
	url := fmt.Sprintf("%s%s?%s", apiURL, target, paramStr)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		errChan <- err
		return
	}

	req.Header.Add("Client-ID", t.clientID)
	req = req.WithContext(ctx)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		errChan <- err
		t.httpTransport.CancelRequest(req)
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.logger.Errorf("get(%d): %s", resp.StatusCode, url)
		errChan <- errors.New("HTTP Resp: " + resp.Status)
		t.httpTransport.CancelRequest(req)
		return
	}

	buffer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errChan <- err
		t.httpTransport.CancelRequest(req)
		return
	}

	respChan <- buffer
	return
}
