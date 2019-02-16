package twitch

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const twitchWebSubHubURL = "https://api.twitch.tv/helix/webhooks/hub"
const twitchWebSubTopicURL = "https://api.twitch.tv/helix/"

func newWebhookTopicURL(topic string) (*url.URL, error) {
	return url.Parse(twitchWebSubTopicURL + topic)
}

func (t *twitchAPI) newWebSubHub() *url.Values {
	ret := &url.Values{}
	ret.Set("hub.lease_seconds", "864000")
	//ret.Set("hub.secret", "")
	return ret
}

func webSubKey(query url.Values) string {
	return fmt.Sprintf("%s&%s", query.Get("hub.topic"), query.Get("hub.mode"))
}

func (t *twitchAPI) requestToHub(ctx context.Context, webSubHub *url.Values) int {
	localLogger := t.logger.WithField("phase", "reqeustToHub")

	// Create channel to wait for Hub validation
	topic := webSubHub.Get("hub.topic")
	mode := webSubHub.Get("hub.mode")
	if topic == "" || mode == "" {
		localLogger.Errorf("invalid WebSubHub: topic: %s, mode: %s", topic, mode)
		return 0
	}

	key := webSubKey(*webSubHub)
	realLeaseChan := make(chan int)
	_, exists := t.pendingWebSub.LoadOrStore(key, realLeaseChan)
	if exists {
		localLogger.Warnf("skipping subscription (ongoing): %s", key)
		return 0
	}
	defer t.pendingWebSub.Delete(key)

	// Send subscription request
	req, err := t.newRequest("POST", "webhooks/hub", strings.NewReader(webSubHub.Encode()))
	if err != nil {
		localLogger.Errorf("failed to create hub request: %s", err.Error())
		return 0
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	respChan := make(chan *http.Response)
	errChan := make(chan error)
	go t.sendReq(ctx, req, respChan, errChan)

	var resp *http.Response
	select {
	case resp = <-respChan:
		defer resp.Body.Close()
		break
	case err := <-errChan:
		localLogger.Errorf("failed to send request to hub: %s", err.Error())
		return 0
	case <-ctx.Done():
		localLogger.Warn("timed-out / cancelled")
		return 0
	}

	// Handle response
	if resp.StatusCode != 202 {
		body, _ := ioutil.ReadAll(resp.Body)
		localLogger.Errorf("invalid response from hub, status: %d, body: %s", resp.StatusCode, body)
		return 0
	}

	// Wait for hub validation done
	localLogger.Infof("websub request sent, waiting for challenge...")
	realLease := <-realLeaseChan
	localLogger.Infof("websub request done. Lease seconds: %d", realLease)
	return realLease
}

func (t *twitchAPI) websubValidate(response http.ResponseWriter, req *http.Request) {
	localLogger := t.logger.WithField("phase", "websubValidate")
	key := webSubKey(req.URL.Query())
	load, ok := t.pendingWebSub.Load(key)
	if !ok {
		localLogger.Warnf("invalid challenge, key: %s, URL: %s", key, req.URL.String())
		response.WriteHeader(404)
		return
	}

	realLease, _ := strconv.Atoi(req.URL.Query().Get("hub.lease_seconds"))
	realLeaseChan, _ := load.(chan int)
	realLeaseChan <- realLease

	response.WriteHeader(200)
	response.Write([]byte(req.URL.Query().Get("hub.challenge")))
}
