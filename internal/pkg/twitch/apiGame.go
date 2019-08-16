package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

// Game object from Twitch API
type Game struct {
	BoxArtURL string `json:"box_art_url"`
	ID        string `json:"id"`
	Name      string `json:"Name"`
}

// GameList for the responses of Get Games api
type GameList struct {
	Data []Game `json:"data"`
}

type gameQuery struct {
	id []string
}

func (t *twitchAPI) getGames(ctx context.Context, gq gameQuery) (*GameList, error) {
	if len(gq.id) > 100 {
		return nil, errors.New("id query count exceeds limitation")
	}

	req, err := t.newRequest("GET", "games", nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	query := req.URL.Query()
	for _, id := range gq.id {
		query.Add("id", id)
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

	games := &GameList{}
	err = json.Unmarshal(respBody, games)
	if err != nil {
		return nil, err
	}

	return games, nil
}
