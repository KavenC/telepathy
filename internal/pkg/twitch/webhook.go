package twitch

import (
	"context"
	"net/http"
)

// supported topic for the webhook callback
const whTopicStream = "streams"

func (s *twitchService) webhook(response http.ResponseWriter, req *http.Request) {
	if req.Method == "GET" {
		s.api.websubValidate(response, req)
		return
	}

	respChan := make(chan int, 1)
	go s.handleWebhookReq(s.ctx, req, respChan)

	select {
	case resp := <-respChan:
		response.WriteHeader(resp)
		return
	case <-s.ctx.Done():
		s.logger.Warn("webhook handling has been canceled")
		response.WriteHeader(503)
		return
	}
}

func (s *twitchService) handleWebhookReq(ctx context.Context, req *http.Request, resp chan int) {
	// TODO: Auth

	// A "topic" query is appended as callback url when subscribing
	// Here we can use the "topic" query to identify the topic of this callback request
	topic := req.URL.Query()["topic"]
	if topic == nil {
		s.logger.Warnf("handleWebhookReq: invalid callback with no topic query. URL: %s", req.URL.String())
		resp <- 400
		return
	}

	// Take only the first mode parameters, ignore others
	switch topic[0] {
	case whTopicStream:
		// stream changed
		s.streamChanged(ctx, req, resp)
	}
}
