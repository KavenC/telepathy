package twitch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
)

const (
	apiURL      = "https://api.twitch.tv/helix"
	cacheExpire = 5 * time.Minute
)

type twitchAPI struct {
	clientID     string
	clientSecret string
	websubSecret string
	webhookURL   *url.URL
	httpClient   *http.Client

	pendingWebSub sync.Map

	userIDCache *cache.Cache // login -> user id
	gameCache   *cache.Cache // game id -> Game

	renewCtx       context.Context // context controls all websub renewal routines
	renewCancelAll context.CancelFunc
	renewCancel    sync.Map

	logger *logrus.Entry
}

func newTwitchAPI() *twitchAPI {
	ctx, cancel := context.WithCancel(context.Background())
	return &twitchAPI{
		httpClient:     &http.Client{},
		userIDCache:    cache.New(cacheExpire, cacheExpire),
		gameCache:      cache.New(cacheExpire, cacheExpire),
		renewCtx:       ctx,
		renewCancelAll: cancel,
	}
}

func (t *twitchAPI) newRequest(method, target string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", apiURL, target), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Client-ID", t.clientID)
	return req, nil
}

// Query user information with login name
// If an error happened during query, returned channel will be closed without providing any User object
// If user not found, returned channel provides nil
// Otherwise, returned channel provides the User object and closes channel
func (t *twitchAPI) userByLogin(ctx context.Context, name string) <-chan *User {
	ret := make(chan *User)
	go func() {
		defer close(ret)
		resp, err := t.getUsers(ctx, userQuery{login: []string{name}})
		if err != nil {
			logger := t.logger.WithField("phase", "userByLogin")
			logger.Error(err.Error())
			logger.Errorf("login: %s", name)
			return
		}
		if len(resp.Data) < 1 {
			ret <- nil
			t.userIDCache.Add(name, nil, cache.DefaultExpiration)
		} else {
			user := resp.Data[0]
			ret <- &user
			t.userIDCache.Add(name, user.ID, cache.DefaultExpiration)
		}
	}()
	return ret
}

// Query user information with user id
// If an error happened during query, returned channel will be closed without providing any User object
// If user not found, returned channel provides nil
// Otherwise, returned channel provides the User object and closes channel
func (t *twitchAPI) userByID(ctx context.Context, id string) <-chan *User {
	ret := make(chan *User)
	go func() {
		defer close(ret)
		resp, err := t.getUsers(ctx, userQuery{id: []string{id}})
		if err != nil {
			logger := t.logger.WithField("phase", "userByID")
			logger.Error(err.Error())
			logger.Errorf("id: %s", id)
			return
		}
		if len(resp.Data) < 1 {
			ret <- nil
		} else {
			user := resp.Data[0]
			ret <- &user
			t.userIDCache.Add(user.Login, id, cache.DefaultExpiration)
		}
	}()
	return ret
}

// Query stream information with login name
// If an error happened during query, returned channel will be closed without providing any object
// If user not found, returned channel provides nil
// Otherwise, returned channel provides the Stream object and closes channel
func (t *twitchAPI) streamByLogin(ctx context.Context, login string) <-chan *Stream {
	ret := make(chan *Stream)
	go func() {
		defer close(ret)
		resp, err := t.getStreams(ctx, streamQuery{userLogin: []string{login}})
		if err != nil {
			t.logger.WithField("phase", "streamByLogin").Error(err.Error())
			return
		}
		if len(resp.Data) < 1 {
			// check whether user exists
			userID, ok := <-t.userIDByLogin(ctx, login)
			if !ok {
				return
			}
			if userID == nil {
				// user not found
				ret <- nil
				return
			}

			// Stream offline
			ret <- &Stream{offline: true}
			return
		}
		ret <- &resp.Data[0]
	}()
	return ret
}

// Query user ID by user login name
// If error happaned, close channel without pushing
// If user not found, channel returns nil
// Otherwise, return user id
func (t *twitchAPI) userIDByLogin(ctx context.Context, login string) <-chan *string {
	ret := make(chan *string)
	cached, hit := t.userIDCache.Get(login)

	go func() {
		defer close(ret)
		if !hit {
			user, ok := <-t.userByLogin(ctx, login)
			if !ok {
				return
			}
			if user == nil {
				ret <- nil
			} else {
				ret <- &user.ID
			}
			return
		}
		id, ok := cached.(string)
		if !ok {
			ret <- nil
		} else {
			ret <- &id
		}
	}()
	return ret
}

// Query game info by game id
// If error happaned, close channel without pushing
// If game not found, channel returns nil
// Otherwise, returns game object
func (t *twitchAPI) gameByID(ctx context.Context, gameID string) <-chan *Game {
	ret := make(chan *Game)
	cached, hit := t.gameCache.Get(gameID)

	go func() {
		defer close(ret)
		if !hit {
			games, err := t.getGames(ctx, gameQuery{id: []string{gameID}})
			if err != nil {
				t.logger.WithField("phase", "gameByID").Error(err.Error())
				return
			}

			if len(games.Data) < 1 {
				ret <- nil
				t.gameCache.Add(gameID, nil, cache.DefaultExpiration)
				return
			}

			game := games.Data[0]
			ret <- &game
			t.gameCache.Add(gameID, &game, cache.DefaultExpiration)
			return
		}

		game, ok := cached.(*Game)
		if !ok {
			ret <- nil
			return
		}

		ret <- game
		return
	}()

	return ret
}

// subscribeStream subscribes to stream changed event
// If error happened, returned channel will be closed without pushing
// Otherwise, returns nil
func (t *twitchAPI) subscribeStream(ctx context.Context, userID string) <-chan interface{} {
	logger := t.logger.WithField("phase", "subscribeStream")
	ret := make(chan interface{})
	go func() {
		defer close(ret)
		hubparams := make(url.Values)
		hubparams.Add("user_id", userID)
		err := t.websubSubscription(ctx, "streams", &hubparams, true)
		if err != nil {
			logger.Errorf(err.Error())
			return
		}
		ret <- nil
	}()
	return ret
}
