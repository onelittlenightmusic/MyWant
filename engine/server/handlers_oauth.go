package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	mywant "mywant/engine/core"
)

// receiveOAuthCallback handles GET /api/v1/oauth/callback
//
// Generic OAuth 2.0 Authorization Code callback endpoint for any want type.
// Providers (Spotify, Google, etc.) redirect here after user authorization.
//
// Expected URL: GET /api/v1/oauth/callback?code=AUTH_CODE&state=want-name
//
// The `state` parameter must be the want's name or ID. The handler stores all
// query parameters as webhook_payload on the matching want so that its monitor
// script can pick up the code on the next tick and exchange it for tokens.
//
// Custom types that use OAuth should:
//  1. Set redirect_uri = http://localhost:8080/api/v1/oauth/callback
//  2. Set state = <want name> in the authorization URL
//  3. Declare a `webhook_payload` current-labelled state field in their YAML
//  4. Read `code` from webhook_payload in their monitor script to exchange for tokens
func (s *Server) receiveOAuthCallback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	if stateParam == "" {
		http.Error(w, "Missing 'state' parameter (expected want name)", http.StatusBadRequest)
		return
	}

	want := s.findWantByIDOrName(stateParam)
	if want == nil {
		errPage(w, http.StatusNotFound, fmt.Sprintf("Want not found: %q — make sure the want is deployed before authorizing.", stateParam))
		return
	}

	// Verify the want has a webhook_payload field (opt-in to OAuth callback)
	if _, ok := want.StateLabels["webhook_payload"]; !ok {
		errPage(w, http.StatusBadRequest, fmt.Sprintf("Want %q does not declare a webhook_payload state field. Add it to the want type YAML.", stateParam))
		return
	}

	// Collect all query params as the payload map
	payload := make(map[string]any)
	for k, vals := range r.URL.Query() {
		if len(vals) == 1 {
			payload[k] = vals[0]
		} else {
			payload[k] = vals
		}
	}

	updates := map[string]any{
		"webhook_payload":     payload,
		"webhook_received_at": time.Now().Format(time.RFC3339),
	}
	// Store the authorization code directly so monitor scripts can access it
	// as a plain string without needing to parse the webhook_payload object.
	if code := r.URL.Query().Get("code"); code != "" {
		updates["oauth_code"] = code
	}
	mywant.StoreStateMulti(want, updates)
	log.Printf("[OAUTH-CALLBACK] Stored OAuth payload for want %q (type=%s) keys=%v\n",
		want.Metadata.Name, want.Metadata.Type, mapKeys(payload))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, successPage, want.Metadata.Name)
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

const successPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Authorization Successful — mywant</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
           max-width: 420px; margin: 80px auto; text-align: center; color: #333; padding: 0 20px; }
    .check { font-size: 56px; margin-bottom: 12px; }
    h2 { margin: 0 0 8px; font-size: 22px; font-weight: 600; color: #1a1a1a; }
    p  { margin: 0; color: #666; font-size: 15px; line-height: 1.5; }
    .want-name { background: #f0f0f0; border-radius: 6px; padding: 2px 8px;
                 font-family: monospace; font-size: 13px; color: #333; }
  </style>
</head>
<body>
  <div class="check">✓</div>
  <h2>Authorization Successful</h2>
  <p>The code was captured by want <span class="want-name">%s</span>.<br>
     You can close this tab and return to mywant.</p>
</body>
</html>`

const errPageTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Authorization Error — mywant</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
           max-width: 420px; margin: 80px auto; text-align: center; color: #333; padding: 0 20px; }
    .icon { font-size: 56px; margin-bottom: 12px; }
    h2 { margin: 0 0 8px; font-size: 22px; color: #c0392b; }
    p  { color: #666; font-size: 14px; }
  </style>
</head>
<body>
  <div class="icon">✗</div>
  <h2>Authorization Error</h2>
  <p>%s</p>
</body>
</html>`

func errPage(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, errPageTmpl, msg)
}
