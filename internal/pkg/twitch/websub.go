package twitch

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const twitchWebSubHubURL = "https://api.twitch.tv/helix/webhooks/hub"
const twitchWebSubTopicURL = "https://api.twitch.tv/helix/"

type websubReq struct {
	httpReq *http.Request
	id      string
}

// newHubRequest returns a request template to the Twitch Websub Hub
func (t *twitchAPI) newHubRequest(topic string, topicParams *url.Values, sub bool) (*websubReq, error) {
	params := url.Values{}
	callback := *t.webhookURL
	query := callback.Query()
	query.Add("topic", topic)
	callback.RawQuery = query.Encode()

	params.Add("hub.callback", callback.String())
	if sub {
		params.Add("hub.mode", "subscribe")
	} else {
		params.Add("hub.mode", "unsubscribe")
	}

	topicURL, err := url.Parse(fmt.Sprintf("%s/%s", apiURL, topic))
	if err != nil {
		return nil, err
	}
	if topicParams != nil {
		topicURL.RawQuery = topicParams.Encode()
	}
	params.Add("hub.topic", topicURL.String())
	if sub {
		params.Add("hub.lease_seconds", "120")
		params.Add("hub.secret", t.websubSecret)
	}

	body := ioutil.NopCloser(strings.NewReader(params.Encode()))
	req, err := t.newRequest("POST", "webhooks/hub", body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return &websubReq{httpReq: req, id: webSubID(params)}, nil
}

func (t *twitchAPI) websubValidate(response http.ResponseWriter, req *http.Request) {
	logger := t.logger.WithField("phase", "websubValidate")

	key := webSubID(req.URL.Query())
	load, ok := t.pendingWebSub.Load(key)
	if !ok {
		logger.Warnf("invalid challenge, key: %s, URL: %s", key, req.URL.String())
		response.WriteHeader(404)
		return
	}

	realLease, _ := strconv.Atoi(req.URL.Query().Get("hub.lease_seconds"))
	realLeaseChan, _ := load.(chan int)
	realLeaseChan <- realLease

	response.Write([]byte(req.URL.Query().Get("hub.challenge")))
}

func webSubID(query url.Values) string {
	return fmt.Sprintf("%s&%s", query.Get("hub.topic"), query.Get("hub.mode"))
}

/*
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

*/
