package telepathy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterWebhook(t *testing.T) {
	assert := assert.New(t)
	server, err := newWebServer("http://localhost", "8080")
	assert.NoError(err)

	handler := func(http.ResponseWriter, *http.Request) {}

	url, err := server.registerWebhook("test-hook", handler)
	assert.NoError(err)
	assert.Equal(fmt.Sprintf("http://localhost%stest-hook", webhookRoot), url.String())
}

func TestRegisterWebhookIllegal(t *testing.T) {
	assert := assert.New(t)
	server, err := newWebServer("http://localhost", "8080")
	assert.NoError(err)

	handler := func(http.ResponseWriter, *http.Request) {}

	_, err = server.registerWebhook("@abc", handler)
	assert.Error(err)
	_, err = server.registerWebhook("-abc", handler)
	assert.Error(err)
	_, err = server.registerWebhook("a-b-c-d-e", handler)
	assert.Error(err)
}

func TestWebhookTrigger(t *testing.T) {
	assert := assert.New(t)
	server, err := newWebServer("http://localhost", "80")
	assert.NoError(err)

	called := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal("POST", r.Method)
		w.WriteHeader(200)
	}

	url, err := server.registerWebhook("test-hook", handler)
	assert.NoError(err)

	server.finalize()
	go server.ListenAndServe()

	resp, err := http.Post(url.String(), "text/plain", strings.NewReader("test"))
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)
	assert.True(called)

	assert.NoError(server.Shutdown(context.Background()))
}
