package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

// A Stream represents a twtich stream
type Stream struct {
	CommunityID  []string `json:"community_ids"`
	GameID       string   `json:"game_id"`
	ID           string   `json:"id"`
	Language     string   `json:"language"`
	StartedAt    string   `json:"started_at"`
	ThumbnailURL string   `json:"thumbnail_url"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	UserID       string   `json:"user_id"`
	UserName     string   `json:"user_name"`
	ViewerCount  int      `json:"viewer_count"`
	Online       bool     // Flag for online
}

func (t *twitchAPI) fetchStream(ctx context.Context, idOrLogin interface{},
	respChan chan<- Stream, errChan chan<- error) {
	// Identify parameters
	options := make(map[string]string)
	userID, ok := idOrLogin.(int)
	if ok {
		options["user_id"] = strconv.Itoa(userID)
	} else if login, ok := idOrLogin.(string); ok {
		options["user_login"] = login
	} else {
		errChan <- errors.New("wrong id/login type")
		return
	}

	// Create go routine for http request
	bodyResp := make(chan []byte, 1)
	getErr := make(chan error, 1)
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	t.get(getCtx, "streams", options, bodyResp, getErr)

	var body []byte
	select {
	case body = <-bodyResp:
		break
	case err := <-getErr:
		errChan <- err
		return
	case <-ctx.Done():
		t.logger.Warn("fetchStream() timed-out / cancelled")
		errChan <- errors.New("fetchStream() has been cancelled")
		return
	}

	// Unmarshaling the response
	type StreamList struct {
		Data []Stream `json:"data"`
	}
	var list StreamList
	err := json.Unmarshal(body, &list)
	if err != nil {
		errChan <- err
		return
	}

	if len(list.Data) == 0 {
		// Stream offline
		respChan <- Stream{}
		return
	}

	list.Data[0].Online = true
	respChan <- list.Data[0]
}
