// Copyright 2026 AgentOS Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	sessionCookieName = "agentos_session"
	stateCookieName   = "agentos_oauth_state"
)

type authConfig struct {
	Required      bool
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	SessionSecret string
	AuthorizeURL  string
	TokenURL      string
	UserURL       string
	AdminUsers    map[string]bool
}

type authUser struct {
	Login     string    `json:"login"`
	Name      string    `json:"name,omitempty"`
	AvatarURL string    `json:"avatarUrl,omitempty"`
	HTMLURL   string    `json:"htmlUrl,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type sessionResponse struct {
	AuthRequired  bool      `json:"authRequired"`
	Authenticated bool      `json:"authenticated"`
	User          *authUser `json:"user,omitempty"`
	LoginURL      string    `json:"loginUrl,omitempty"`
	LogoutURL     string    `json:"logoutUrl,omitempty"`
}

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type githubUserResponse struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

func loadAuthConfig() authConfig {
	cfg := authConfig{
		ClientID:      strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CLIENT_ID")),
		ClientSecret:  os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
		RedirectURL:   strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CALLBACK_URL")),
		SessionSecret: os.Getenv("AGENTOS_SESSION_SECRET"),
		AuthorizeURL:  envOrDefault("GITHUB_OAUTH_AUTHORIZE_URL", "https://github.com/login/oauth/authorize"),
		TokenURL:      envOrDefault("GITHUB_OAUTH_TOKEN_URL", "https://github.com/login/oauth/access_token"),
		UserURL:       envOrDefault("GITHUB_OAUTH_USER_URL", "https://api.github.com/user"),
		AdminUsers:    parseAdminUsers(os.Getenv("AGENTOS_ADMIN_USERS")),
	}
	cfg.Required = strings.EqualFold(os.Getenv("AGENTOS_AUTH_REQUIRED"), "true") || (cfg.ClientID != "" && cfg.ClientSecret != "")
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = os.Getenv("GITHUB_OAUTH_CLIENT_SECRET")
	}
	return cfg
}

func (c *authConfig) enabled() bool {
	return c.Required
}

func (c *authConfig) oauthConfigured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != "" && c.SessionSecret != ""
}

func parseAdminUsers(raw string) map[string]bool {
	users := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		login := strings.ToLower(strings.TrimSpace(item))
		if login != "" {
			users[login] = true
		}
	}
	return users
}

func (c *authConfig) userCanAutomate(user *authUser) bool {
	if !c.enabled() {
		return true
	}
	if user == nil || user.Login == "" {
		return false
	}
	if len(c.AdminUsers) == 0 {
		return true
	}
	return c.AdminUsers[strings.ToLower(user.Login)]
}

func (s *Server) requireAutomationPermission(w http.ResponseWriter, r *http.Request, user *authUser, action, target, repo, runID string) bool {
	actor := "anonymous"
	if user != nil && user.Login != "" {
		actor = user.Login
	}
	if s.auth.userCanAutomate(user) {
		_ = appendAuditEvent(&auditEvent{ //nolint:errcheck // best-effort audit
			Actor:   actor,
			Action:  action,
			Target:  target,
			Repo:    repo,
			RunID:   runID,
			Outcome: auditOutcomeAllowed,
		})
		return true
	}
	_ = appendAuditEvent(&auditEvent{ //nolint:errcheck // best-effort audit
		Actor:   actor,
		Action:  action,
		Target:  target,
		Repo:    repo,
		RunID:   runID,
		Outcome: auditOutcomeDenied,
		Message: "permission denied",
	})
	w.Header().Set("Content-Type", "application/json")
	http.Error(w, "permission denied", http.StatusForbidden)
	return false
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, _ := s.userFromRequest(r)
	_ = json.NewEncoder(w).Encode(sessionResponse{
		AuthRequired:  s.auth.enabled(),
		Authenticated: user != nil,
		User:          user,
		LoginURL:      "/auth/login",
		LogoutURL:     "/auth/logout",
	}) //nolint:errcheck // best-effort response
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !s.auth.enabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if !s.auth.oauthConfigured() {
		http.Error(w, "GitHub OAuth is not configured", http.StatusServiceUnavailable)
		return
	}

	state := randomToken()
	http.SetCookie(w, signedCookie(stateCookieName, state, 10*time.Minute, s.auth.SessionSecret))
	u, err := url.Parse(s.auth.AuthorizeURL)
	if err != nil {
		http.Error(w, "invalid authorize URL", http.StatusInternalServerError)
		return
	}
	q := u.Query()
	q.Set("client_id", s.auth.ClientID)
	q.Set("redirect_uri", s.auth.RedirectURL)
	q.Set("scope", "read:user")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.auth.oauthConfigured() {
		http.Error(w, "GitHub OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	expected, err := s.readSignedCookie(r, stateCookieName)
	if err != nil || state == "" || !hmac.Equal([]byte(state), []byte(expected)) {
		http.Error(w, "invalid OAuth state", http.StatusBadRequest)
		return
	}
	if code == "" {
		http.Error(w, "missing OAuth code", http.StatusBadRequest)
		return
	}

	token, err := s.exchangeGitHubCode(r.Context(), code)
	if err != nil {
		http.Error(w, "exchange OAuth code: "+err.Error(), http.StatusBadGateway)
		return
	}
	user, err := s.fetchGitHubUser(r.Context(), token)
	if err != nil {
		http.Error(w, "fetch GitHub user: "+err.Error(), http.StatusBadGateway)
		return
	}
	user.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	session, err := json.Marshal(user)
	if err != nil {
		http.Error(w, "create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, signedCookie(sessionCookieName, string(session), 24*time.Hour, s.auth.SessionSecret))
	http.SetCookie(w, expiredCookie(stateCookieName))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, expiredCookie(sessionCookieName))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (*authUser, bool) {
	if !s.auth.enabled() {
		return nil, true
	}
	user, err := s.userFromRequest(r)
	if err != nil || user == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil, false
	}
	return user, true
}

func (s *Server) userFromRequest(r *http.Request) (*authUser, error) {
	if !s.auth.enabled() {
		return nil, nil
	}
	raw, err := s.readSignedCookie(r, sessionCookieName)
	if err != nil {
		return nil, err
	}
	var user authUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return nil, err
	}
	if user.Login == "" || time.Now().UTC().After(user.ExpiresAt) {
		return nil, fmt.Errorf("expired session")
	}
	return &user, nil
}

func (s *Server) exchangeGitHubCode(ctx context.Context, code string) (string, error) {
	payload := map[string]string{
		"client_id":     s.auth.ClientID,
		"client_secret": s.auth.ClientSecret,
		"code":          code,
		"redirect_uri":  s.auth.RedirectURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.auth.TokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var token githubTokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return "", err
	}
	if token.Error != "" {
		return "", fmt.Errorf("%s: %s", token.Error, token.Description)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("missing access token")
	}
	return token.AccessToken, nil
}

func (s *Server) fetchGitHubUser(ctx context.Context, token string) (*authUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.auth.UserURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var ghUser githubUserResponse
	if err := json.Unmarshal(data, &ghUser); err != nil {
		return nil, err
	}
	if ghUser.Login == "" {
		return nil, fmt.Errorf("missing login")
	}
	return &authUser{
		Login:     ghUser.Login,
		Name:      ghUser.Name,
		AvatarURL: ghUser.AvatarURL,
		HTMLURL:   ghUser.HTMLURL,
	}, nil
}

func signedCookie(name, value string, maxAge time.Duration, secret string) *http.Cookie {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(value))
	sig := signCookieValue(encoded, secret)
	return &http.Cookie{
		Name:     name,
		Value:    encoded + "." + sig,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
}

func expiredCookie(name string) *http.Cookie {
	return &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: true}
}

func (s *Server) readSignedCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid cookie")
	}
	expected := signCookieValue(parts[0], s.auth.SessionSecret)
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", fmt.Errorf("invalid cookie signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func signCookieValue(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomToken() string {
	return strings.TrimPrefix(generateID(), "run-")
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
