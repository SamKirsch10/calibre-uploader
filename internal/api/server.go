// Package api wires up the HTTP surface for calibre-uploader: an
// upload endpoint that validates and stores an ebook, then hands it
// off to calibredb inside the calibre container, plus a health check.
package api

import (
	"log"
	"net/http"
	"time"

	"calibre-uploader/internal/calibre"
	"calibre-uploader/internal/config"
)

// Server holds the dependencies HTTP handlers need.
type Server struct {
	cfg     config.Config
	calibre *calibre.Client
}

// NewServer builds a Server from cfg, constructing the calibre client
// it depends on.
func NewServer(cfg config.Config) *Server {
	return &Server{
		cfg:     cfg,
		calibre: calibre.NewClient(cfg.CalibreContainerName, cfg.CalibredbTimeout),
	}
}

// Routes returns the configured HTTP handler for the whole API.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /upload", s.handleUpload)
	return logRequests(mux)
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}
