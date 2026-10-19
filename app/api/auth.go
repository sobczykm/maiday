package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

const (
	sessionCookie = "maiday_session"
	stateCookie   = "maiday_oauth_state"
	sessionTTL    = 30 * 24 * time.Hour
	stateTTL      = 10 * time.Minute
)

var googleEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
}

const googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

func newGoogleOAuth(cfg Config) *oauth2.Config {
	if !cfg.GoogleSignInEnabled() {
		return nil
	}
	return &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		Endpoint:     googleEndpoint,
		RedirectURL:  cfg.BaseURL + "/api/auth/callback",
		Scopes:       []string{"openid", "email", "profile"},
	}
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if s.google == nil {
		writeError(w, http.StatusServiceUnavailable, "google sign-in is not configured")
		return
	}
	state := randomToken()
	s.setCookie(w, stateCookie, state, stateTTL)
	http.Redirect(w, r, s.google.AuthCodeURL(state), http.StatusFound)
}

func (s *server) callback(w http.ResponseWriter, r *http.Request) {
	if s.google == nil {
		writeError(w, http.StatusServiceUnavailable, "google sign-in is not configured")
		return
	}
	state, err := r.Cookie(stateCookie)
	if err != nil || state.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "invalid sign-in state, try again")
		return
	}
	s.clearCookie(w, stateCookie)

	token, err := s.google.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "google sign-in failed")
		return
	}
	user, err := fetchGoogleUser(r.Context(), s.google.Client(r.Context(), token))
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.UpsertUser(r.Context(), user); err != nil {
		s.fail(w, err)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, sessionTTL)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.setCookie(w, sessionCookie, session, sessionTTL)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(r.Context(), c.Value); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.clearCookie(w, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, user User) {
	writeJSON(w, http.StatusOK, user)
}

type userHandler func(w http.ResponseWriter, r *http.Request, user User)

func (s *server) withUser(next userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		user, err := s.store.UserBySession(r.Context(), c.Value)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		next(w, r, user)
	}
}

func fetchGoogleUser(ctx context.Context, client *http.Client) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return User{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("google userinfo: %s", res.Status)
	}

	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return User{}, err
	}
	if info.Sub == "" || !info.EmailVerified {
		return User{}, errors.New("google account has no verified email")
	}
	return User{ID: info.Sub, Email: info.Email, Name: info.Name, Picture: info.Picture}, nil
}

func (s *server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *server) clearCookie(w http.ResponseWriter, name string) {
	s.setCookie(w, name, "", -time.Second)
}

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
