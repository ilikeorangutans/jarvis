package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MakeObservable attaches the prometheus handler and a services/ping endpoint and starts a HTTP server.
func MakeObservable() {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/services/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	http.ListenAndServe(":8080", nil)
}
