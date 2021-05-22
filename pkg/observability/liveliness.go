package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// MakeObservable attaches the prometheus handler and a services/ping endpoint and starts a HTTP server.
func MakeObservable() {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/services/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal().Err(err).Msg("could not start HTTP server")
	}
}
