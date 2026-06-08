package handlers

import "net/http"

// NotImplemented returns a generic 501 response for staged endpoints.
func NotImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
