package exporter

import "net/http"

// Handler serves GET /metrics with 501 until #5 lands.
func Handler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
