package types

import (
	. "mywant/engine/core"
)

// One Google account per server, so one refresh token.
//
// Each Google-backed want used to keep its own token, keyed by its want id
// (backup_google / <want id>). That made every new Google want start from the
// consent screen even when the server was already authorized, and a token
// could not outlive the want it was stored under. The token belongs to the
// account the server talks to Google as, so it is kept once, under a shared
// key, and every Google want reads it from there.
//
// Scopes are additive: authorizing one want with include_granted_scopes keeps
// what the others were granted, so the one token grows to cover every Google
// type in use.
const (
	googleSecretNamespace = "google"
	googleSecretKey       = "shared"
	googleRefreshField    = "refresh_token"
)

// loadGoogleRefreshToken returns the server's Google refresh token, or "".
//
// A token that was stored per want before the token was shared is moved
// across the first time a want that owns one asks for it.
func loadGoogleRefreshToken(wantID string) string {
	if tok := LoadSecretField(googleSecretNamespace, googleSecretKey, googleRefreshField); tok != "" {
		return tok
	}
	legacy := LoadSecretField("backup_google", wantID, googleRefreshField)
	if legacy == "" {
		return ""
	}
	if err := saveGoogleRefreshToken(legacy); err != nil {
		return legacy
	}
	ClearSecret("backup_google", wantID)
	return legacy
}

func saveGoogleRefreshToken(tok string) error {
	return SaveSecretField(googleSecretNamespace, googleSecretKey, googleRefreshField, tok)
}

// clearGoogleRefreshToken drops the shared token — for a token Google no longer
// honours, which is dead for every want that uses it, not only the one that
// found out.
func clearGoogleRefreshToken() {
	ClearSecret(googleSecretNamespace, googleSecretKey)
}
