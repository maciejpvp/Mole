package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"mole-control-plane/internal/user"
)

const maxAuthRequestBytes = 8 << 10

const (
	googleStateCookie    = "mole-google-oauth-state"
	googleVerifierCookie = "mole-google-oauth-verifier"
	googleUserInfoURL    = "https://openidconnect.googleapis.com/v1/userinfo"
)

type loginCodeRequest struct {
	Code string `json:"code"`
}

type googleUserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func (s *Server) googleOAuthConfig() (*oauth2.Config, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	redirectURL := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if clientID == "" || clientSecret == "" || redirectURL == "" {
		return nil, errors.New("Google OAuth is not configured")
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email"},
	}, nil
}

func (s *Server) googleStartHandler(w http.ResponseWriter, r *http.Request) {
	config, err := s.googleOAuthConfig()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Google authentication is not configured"})
		return
	}
	state := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()
	setOAuthCookie(w, googleStateCookie, state, 600)
	setOAuthCookie(w, googleVerifierCookie, verifier, 600)
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (s *Server) googleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth service unavailable"})
		return
	}
	frontendURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GOOGLE_FRONTEND_URL")), "/")
	if frontendURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Google frontend redirect is not configured"})
		return
	}
	if r.URL.Query().Get("error") != "" {
		redirectGoogleResult(w, r, frontendURL, "google_error=authentication_cancelled")
		return
	}
	stateCookie, stateErr := r.Cookie(googleStateCookie)
	verifierCookie, verifierErr := r.Cookie(googleVerifierCookie)
	if stateErr != nil || verifierErr != nil || stateCookie.Value == "" || verifierCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		redirectGoogleResult(w, r, frontendURL, "google_error=invalid_oauth_state")
		return
	}
	clearOAuthCookie(w, googleStateCookie)
	clearOAuthCookie(w, googleVerifierCookie)

	config, err := s.googleOAuthConfig()
	if err != nil {
		redirectGoogleResult(w, r, frontendURL, "google_error=authentication_unavailable")
		return
	}
	token, err := config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		redirectGoogleResult(w, r, frontendURL, "google_error=google_exchange_failed")
		return
	}
	identity, err := fetchGoogleIdentity(r.Context(), token)
	if err != nil || !identity.EmailVerified {
		redirectGoogleResult(w, r, frontendURL, "google_error=unverified_google_account")
		return
	}
	userID, err := s.users.LoginWithGoogle(r.Context(), user.GoogleIdentity{Subject: identity.Subject, Email: identity.Email})
	if err != nil {
		if errors.Is(err, user.ErrUnauthenticated) {
			redirectGoogleResult(w, r, frontendURL, "google_error=account_unavailable")
			return
		}
		redirectGoogleResult(w, r, frontendURL, "google_error=unable_to_create_account")
		return
	}
	code, err := s.users.CreateLoginCode(r.Context(), userID)
	if err != nil {
		redirectGoogleResult(w, r, frontendURL, "google_error=unable_to_create_session")
		return
	}
	redirectGoogleResult(w, r, frontendURL, "google_code="+url.QueryEscape(code))
}

func (s *Server) googleExchangeHandler(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth service unavailable"})
		return
	}
	var request loginCodeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	authentication, err := s.users.ExchangeLoginCode(r.Context(), request.Code)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication code is invalid or expired"})
		return
	}
	writeJSON(w, http.StatusOK, authentication)
}

func fetchGoogleIdentity(ctx context.Context, token *oauth2.Token) (googleUserInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return googleUserInfo{}, err
	}
	token.SetAuthHeader(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return googleUserInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return googleUserInfo{}, errors.New("Google userinfo request failed")
	}
	var identity googleUserInfo
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAuthRequestBytes)).Decode(&identity); err != nil {
		return googleUserInfo{}, err
	}
	return identity, nil
}

func setOAuthCookie(w http.ResponseWriter, name, value string, maxAge int) {
	w.Header().Add("Set-Cookie", (&http.Cookie{
		Name: name, Value: value, Path: "/api/v1/auth/google", MaxAge: maxAge,
		HttpOnly: true, Secure: envBool("GOOGLE_COOKIE_SECURE", false), SameSite: http.SameSiteLaxMode,
	}).String())
}

func clearOAuthCookie(w http.ResponseWriter, name string) {
	setOAuthCookie(w, name, "", -1)
}

func redirectGoogleResult(w http.ResponseWriter, r *http.Request, frontendURL, fragment string) {
	destination := frontendURL
	if parsed, err := url.Parse(frontendURL); err == nil {
		parsed.Fragment = fragment
		destination = parsed.String()
	}
	http.Redirect(w, r, destination, http.StatusFound)
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one JSON object"})
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
