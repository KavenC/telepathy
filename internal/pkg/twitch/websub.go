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
	if topicParams != nil {
		for key, values := range *topicParams {
			for _, value := range values {
				query.Add(key, value)
			}
		}
	}
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
		params.Add("hub.lease_seconds", "864000")
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

func (t *twitchAPI) websubSubscription(ctx context.Context, topic string, params *url.Values, sub bool) error {
	logger := t.logger.WithField("phase", "websubSubscription")

	// Construct websub request
	hubreq, err := t.newHubRequest(topic, params, true)
	if err != nil {
		return err
	}

	// Create channel for waiting verification from hub
	verified := make(chan int)
	key := hubreq.id
	defer close(verified)
	_, exists := t.pendingWebSub.LoadOrStore(key, verified)
	if exists {
		return fmt.Errorf("subscription process racing: %s", key)
	}
	defer t.pendingWebSub.Delete(key)
	// issue the request
	req := hubreq.httpReq.WithContext(ctx)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}

	// Processing http response
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 202 {
		return fmt.Errorf("http status code: %d - %s", resp.StatusCode, body)
	}

	// Wait for verification of intent from hub
	// This will be handled by the webhook callbacks
	select {
	// the verification from hub also provides the actual lease seconds
	// we will use the lease second to start a subscription renewal routine
	case realLease := <-verified:
		if sub {
			// if this is a subscribe request
			// start a goroutine to renew the subscription
			go func() {
				logger := logger.WithField("phase", "renew")
				duration := time.Duration(realLease-10) * time.Second
				// apart from waiting for the lease expired, the routing also accepts early termination
				// this usually happens when unsubscribed is requested or system shutdown
				ctx, cancel := context.WithCancel(t.renewCtx)
				defer cancel()

				// If something goes wrong, we might have a cancel function for another renewal routing
				// already registered in the table. This does not suppose to happen, but when it do, we
				// cancel the other ones before filling in ours.
				// Also produces warning messages so that we will know this from logs
				for actual, loaded := t.renewCancel.LoadOrStore(key, cancel); loaded; {
					previousCancel, _ := actual.(context.CancelFunc)
					logger.Warnf("cancelling overlapped renewal routine: %s", key)
					previousCancel()
				}

				select {
				case <-time.After(duration):
					t.renewCancel.Delete(key)
					ctx, subCancel := context.WithTimeout(t.renewCtx, reqTimeOut)
					defer subCancel()
					logger.Infof("renewing websub: %s", key)
					err := t.websubSubscription(ctx, topic, params, true)
					if err != nil {
						logger.Errorf(err.Error())
					}
				case <-ctx.Done():
					// renew routine cancelled
					t.renewCancel.Delete(key)
					logger.Warnf("renew cancelled: %s", key)
				}
			}()
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("verification timeout")
	}
}

func webSubID(query url.Values) string {
	return fmt.Sprintf("%s&%s", query.Get("hub.topic"), query.Get("hub.mode"))
}
