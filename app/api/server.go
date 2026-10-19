package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"golang.org/x/oauth2"
)

type server struct {
	store         *Store
	blobs         *Blobs
	google        *oauth2.Config
	secureCookies bool
}

func newServer(cfg Config, store *Store, blobs *Blobs) *server {
	return &server{
		store:         store,
		blobs:         blobs,
		google:        newGoogleOAuth(cfg),
		secureCookies: cfg.SecureCookies(),
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/callback", s.callback)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.withUser(s.me))

	mux.HandleFunc("GET /api/items", s.withUser(s.listItems))
	mux.HandleFunc("PUT /api/items/{collection}/{id}", s.withUser(s.putItem))
	mux.HandleFunc("DELETE /api/items/{collection}/{id}", s.withUser(s.deleteItem))

	mux.HandleFunc("POST /api/assets", s.withUser(s.uploadAsset))
	mux.HandleFunc("DELETE /api/assets/{id}", s.withUser(s.deleteAsset))
	mux.HandleFunc("GET /_blob/{id}", s.withUser(s.serveAsset))

	return http.NewCrossOriginProtection().Handler(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *server) fail(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
