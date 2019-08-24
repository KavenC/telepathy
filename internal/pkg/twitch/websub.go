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

	"github.com/patrickmn/go-cache"
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

func (s *Service) websubValidate(response http.ResponseWriter, req *http.Request) {
	logger := s.logger.WithField("phase", "websubValidate")

	key := webSubID(req.URL.Query())
	load, ok := s.verifyingSubs.Load(key)
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

func (s *Service) handleNotification(req *http.Request, body []byte) <-chan int {
	status := make(chan int, 1)
	notification := notification{
		request: req,
		body:    body,
		status:  status,
	}
	go func() {
		select {
		case s.notifQueue <- &notification:
		case <-req.Context().Done():
			// this request is dropped
			s.logger.WithField("phase", "handleNotification").Warnf("notification dropped")
			status <- 200
		}
	}()
	return status
}

// notifHandler handles websub notification callbacks in centeralized manner
// we need to predefine the handlers so that it is possible to gracefully shutdown everything
func (s *Service) notifHandler() {
	logger := s.logger.WithField("phase", "notifHandler")
	for notification := range s.notifQueue {
		req := notification.request
		ret := notification.status
		body := notification.body

		notifID := notification.request.Header.Get("Twitch-Notification-Id")
		if len(notifID) == 0 {
			logger.Warnf("notification without ID")
			logger.Warn(notification.request.Header)
			logger.Warn(notification.body)
			ret <- 200
			continue
		}

		err := s.notifPrevID.Add(notifID, nil, cache.DefaultExpiration)
		if err != nil {
			// skip duplicated notifications
			logger.Warnf("duplicated notif ID: %s", notifID)
			ret <- 200
			continue
		}
		s.notifPrevID.DeleteExpired()

		// A "topic" query is appended as callback url when subscribing
		// Here we can use the "topic" query to identify the topic of this callback request
		topic := req.URL.Query()["topic"]
		if topic == nil {
			logger.Warnf("invalid callback with no topic query. URL: %s", req.URL.String())
			ret <- 400
			continue
		}

		// Take only the first mode parameters, ignore others
		switch topic[0] {
		case "streams":
			// stream changed
			ret <- s.streamChanged(req, body)
		default:
			// return sub as deleted for any unknown topics
			logger.Warnf("unknown topic. URL: %s", req.URL.String())
			ret <- 410
		}
	}
	close(s.notifDone)
}

// subscribe/unsubscribe to a websub topic
func (s *Service) subscription(ctx context.Context, topic string, params *url.Values, sub bool) error {
	logger := s.logger.WithField("phase", "subscription")

	// Construct websub request
	hubreq, err := s.api.newHubRequest(topic, params, true)
	if err != nil {
		return err
	}

	// Create channel for waiting verification from hub
	verified := make(chan int)
	key := hubreq.id
	defer close(verified)
	_, exists := s.verifyingSubs.LoadOrStore(key, verified)
	if exists {
		return fmt.Errorf("subscription process racing: %s", key)
	}
	defer s.verifyingSubs.Delete(key)

	// issue the request
	req := hubreq.httpReq.WithContext(ctx)
	resp, err := s.api.httpClient.Do(req)
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
				ctx, cancel := context.WithCancel(s.renewCtx)
				defer cancel()

				// If something goes wrong, we might have a cancel function for another renewal routing
				// already registered in the table. This does not suppose to happen, but when it do, we
				// cancel the other ones before filling in ours.
				// Also produces warning messages so that we will know this from logs
				for actual, loaded := s.renewCancelMap.LoadOrStore(key, cancel); loaded; {
					previousCancel, _ := actual.(context.CancelFunc)
					logger.Warnf("cancelling overlapped renewal routine: %s", key)
					previousCancel()
				}

				select {
				case <-time.After(duration):
					s.renewCancelMap.Delete(key)
					if !s.subTopics[topic].hasKey(hubreq.id) {
						// unsubscribed
						logger.Infof("renew terminated: %s", key)
						return
					}

					ctx, subCancel := context.WithTimeout(s.renewCtx, reqTimeOut)
					defer subCancel()
					logger.Infof("renewing websub: %s", key)
					err := s.subscription(ctx, topic, params, true)
					if err != nil {
						logger.Errorf(err.Error())
					}
				case <-ctx.Done():
					// renew routine cancelled
				}
			}()
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("verification timeout")
	}
}
