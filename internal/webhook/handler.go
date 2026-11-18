package webhook

import "net/http"

// Handler serves POST /hooks/onwatch with 501 until #6 lands.
func Handler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
