package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// User stores twitch user data
type User struct {
	BroadcasterType string `json:"boradcaster_type"`
	Description     string `json:"description"`
	DisplayName     string `json:"display_name"`
	Email           string `json:"email"`
	ID              string `json:"id"`
	Login           string `json:"login"`
	OfflineImage    string `json:"offline_image_url"`
	ProfileImage    string `json:"profile_image_url"`
	Type            string `json:"type"`
	ViewCount       int    `json:"view_count"`
}

func (t *twitchAPI) fetchUser(ctx context.Context, idOrLogin interface{},
	respChan chan<- User, errChan chan<- error) {
	// Identify parameters
	options := make(map[string]string)
	userID, ok := idOrLogin.(int)
	if ok {
		options["id"] = strconv.Itoa(userID)
	} else if login, ok := idOrLogin.(string); ok {
		options["login"] = login
	} else {
		errChan <- errors.New("wrong id/login type")
		return
	}

	// Create go routine for http request
	bodyResp := make(chan []byte, 1)
	getErr := make(chan error, 1)
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	t.get(getCtx, "users", options, bodyResp, getErr)

	var body []byte
	select {
	case body = <-bodyResp:
		break
	case err := <-getErr:
		errChan <- err
		return
	case <-ctx.Done():
		t.logger.Warn("fetchUser() timed-out / cancelled")
		errChan <- errors.New("fetchUser() has been canceled")
		return
	}

	// Unmarshaling the response
	type UserList struct {
		Data []User `json:"data"`
	}
	var list UserList
	err := json.Unmarshal(body, &list)
	if err != nil {
		errChan <- err
		return
	}

	if len(list.Data) == 0 {
		errChan <- fmt.Errorf("invalid resp.body: %s", string(body))
		return
	}

	respChan <- list.Data[0]
}
