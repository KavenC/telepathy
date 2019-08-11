package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

// UserList maps to json response from twitch
type UserList struct {
	Data []User `json:"data"`
}

// User stores twitch user data
type User struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
	Description     string `json:"description"`
	ProfileImgURL   string `json:"profile_image_url"`
	OfflineImgURL   string `json:"offline_image_url"`
	ViewCount       int    `json:"view_count"`
}

type userQuery struct {
	id    []string
	login []string
}

func (t *twitchAPI) getUsers(ctx context.Context, uq userQuery) (*UserList, error) {
	// twitch api limits maximum query count at 100
	if len(uq.id)+len(uq.login) > 100 {
		return nil, errors.New("id + login query count exceeds limitation")
	}

	req, err := t.newRequest("GET", "users", nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)
	query := req.URL.Query()
	for _, id := range uq.id {
		query.Add("id", id)
	}
	for _, login := range uq.login {
		query.Add("login", login)
	}
	req.URL.RawQuery = query.Encode()

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status code: %d - %s", resp.StatusCode, string(respBody))
	}

	users := &UserList{}
	err = json.Unmarshal(respBody, users)
	if err != nil {
		return nil, err
	}

	return users, nil
}
