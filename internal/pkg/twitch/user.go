package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// UserList maps to json response from twitch
type UserList struct {
	Data []User `json:"data"`
}

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

func (t *twitchAPI) fetchUser(ctx context.Context, login string,
	respChan chan<- *User, errChan chan<- error) {
	req, err := t.newRequest("GET", "users", nil)
	if err != nil {
		errChan <- fmt.Errorf("new request failed: %s", err.Error())
		return
	}

	query := req.URL.Query()
	query.Add("login", login)
	req.URL.RawQuery = query.Encode()

	t.fetchUserImpl(ctx, req, respChan, errChan)
}

func (t *twitchAPI) fetchUserWithID(ctx context.Context, ID string,
	respChan chan<- *User, errChan chan<- error) {
	req, err := t.newRequest("GET", "users", nil)
	if err != nil {
		errChan <- fmt.Errorf("new request failed: %s", err.Error())
		return
	}

	query := req.URL.Query()
	query.Add("id", ID)
	req.URL.RawQuery = query.Encode()

	t.fetchUserImpl(ctx, req, respChan, errChan)
}

func (t *twitchAPI) fetchUserImpl(ctx context.Context, req *http.Request,
	respChan chan<- *User, errChan chan<- error) {
	// Create go routine for http request
	getResp := make(chan *http.Response)
	getErr := make(chan error)
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go t.sendReq(getCtx, req, getResp, getErr)

	var resp *http.Response
	select {
	case resp = <-getResp:
		break
	case err := <-getErr:
		errChan <- err
		return
	case <-ctx.Done():
		t.logger.Warn("fetchUser() timed-out / cancelled")
		errChan <- errors.New("fetchUser() has been canceled")
		return
	}

	// Handle response
	if resp.StatusCode != 200 {
		errChan <- fmt.Errorf("fetchUser() resp status: %d (ReqURL: %s)", resp.StatusCode, req.URL.String())
		return
	}

	decoder := json.NewDecoder(resp.Body)
	var userList UserList
	err := decoder.Decode(&userList)
	if err != nil {
		errChan <- fmt.Errorf("Resp.Body Read failed: %s", err.Error())
		return
	}

	if len(userList.Data) == 0 {
		respChan <- nil
		return
	}

	respChan <- &userList.Data[0]
}
