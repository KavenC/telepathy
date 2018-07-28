package telepathy

import "net/http"

var mux *http.ServeMux

func init() {
	mux = http.NewServeMux()
}

// RegisterHTTPHandleFunc is used for messenger handler to register http callback
func RegisterHTTPHandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(pattern, handler)
}
