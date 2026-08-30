package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	. "mywant/engine/core"
)

// agent_backup_google — a `do` agent that dumps every deployed want's state to
// a single JSON file on Google Drive, overwriting it in place each run.
//
// Wire it up with `spec.when: [{ every: "10 minutes" }]` on the want: the
// scheduler restarts the want on that interval, the want re-reaches, and this
// runs again.
//
// Auth follows the same OAuth path as the Spotify / Gmail types (see
// handlers_oauth.go): give the want google_client_id / google_client_secret,
// send the user to Google's consent screen with state=<want name> and
// redirect_uri=<host>/api/v1/oauth/callback, and the callback stores the code
// as `oauth_code`. The first run exchanges that code for a refresh token and
// keeps it in ~/.mywant/secrets; every run after refreshes an access token from it.
const backupGoogleAgentName = "agent_backup_google"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(backupGoogleAgentName, executeBackupToGoogle)
	})
}

const (
	googleTokenURL = "https://oauth2.googleapis.com/token"
	driveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
	driveFilesURL  = "https://www.googleapis.com/drive/v3/files"
)

func executeBackupToGoogle(ctx context.Context, want *Want) error {
	clientID := firstNonEmpty(GetCurrent(want, "google_client_id", ""), os.Getenv("GOOGLE_CLIENT_ID"))
	clientSecret := firstNonEmpty(GetCurrent(want, "google_client_secret", ""), os.Getenv("GOOGLE_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		want.SetCurrent("backup_status", "waiting_auth")
		want.StoreLog("[BACKUP-GOOGLE] missing google_client_id / google_client_secret")
		return nil
	}
	// Publish the client id into state (it is not a secret — it rides in the
	// consent redirect anyway) so the card can build the "Authorize" link even
	// when the id only ever came from an env var.
	if GetCurrent(want, "google_client_id", "") != clientID {
		want.SetCurrent("google_client_id", clientID)
	}

	// The refresh token lives in ~/.mywant/secrets, not in want state — so it
	// stays out of state.yaml and out of the snapshot this very agent writes to
	// Drive. Same place the Spotify plugin keeps its tokens.
	refreshToken := LoadSecretField("backup_google", want.Metadata.ID, "refresh_token")
	// One-time migration: a want authorized before the token moved out of state.
	if refreshToken == "" {
		if legacy := GetCurrent(want, "google_refresh_token", ""); legacy != "" {
			if err := SaveSecretField("backup_google", want.Metadata.ID, "refresh_token", legacy); err == nil {
				want.SetCurrent("google_refresh_token", "")
				refreshToken = legacy
				want.StoreLog("[BACKUP-GOOGLE] migrated refresh token from state to ~/.mywant/secrets")
			}
		}
	}
	if refreshToken == "" {
		// First run: turn the code the OAuth callback captured into a refresh
		// token. If it is not here yet the user has not authorized — wait,
		// don't fail.
		code := GetCurrent(want, "oauth_code", "")
		if code == "" {
			want.SetCurrent("authorized", false)
			want.SetCurrent("backup_status", "waiting_auth")
			want.StoreLog("[BACKUP-GOOGLE] no refresh token yet — authorize with Google (state=%s)", want.Metadata.Name)
			return nil
		}
		redirectURI := firstNonEmpty(GetCurrent(want, "oauth_redirect_uri", ""), "http://localhost:8080/api/v1/oauth/callback")
		rt, err := googleExchangeCode(ctx, clientID, clientSecret, code, redirectURI)
		if err != nil {
			return backupGoogleFail(want, "code exchange", err)
		}
		if err := SaveSecretField("backup_google", want.Metadata.ID, "refresh_token", rt); err != nil {
			return backupGoogleFail(want, "save token", err)
		}
		refreshToken = rt
		want.SetCurrent("oauth_code", "") // consumed
		want.SetCurrent("authorized", true)
		want.StoreLog("[BACKUP-GOOGLE] obtained refresh token (stored in ~/.mywant/secrets)")
	} else {
		want.SetCurrent("authorized", true)
	}

	accessToken, err := googleRefreshAccessToken(ctx, clientID, clientSecret, refreshToken)
	if err != nil {
		// A revoked or expired refresh token (test-mode tokens lapse after a
		// week) can only be fixed by authorizing again — drop it so the card
		// goes back to "authorize" rather than failing forever.
		if strings.Contains(err.Error(), "invalid_grant") {
			ClearSecret("backup_google", want.Metadata.ID)
			want.SetCurrent("authorized", false)
		}
		return backupGoogleFail(want, "token refresh", err)
	}

	payload, err := buildBackupSnapshot(want)
	if err != nil {
		return backupGoogleFail(want, "snapshot", err)
	}

	fileID := GetCurrent(want, "drive_file_id", "")
	fileName := firstNonEmpty(GetCurrent(want, "drive_file_name", ""), "mywant-backup.json")
	folderID := GetCurrent(want, "drive_folder_id", "")

	newID, err := driveUpsertJSON(ctx, accessToken, fileID, fileName, folderID, payload)
	if err != nil {
		return backupGoogleFail(want, "drive upload", err)
	}
	if newID != fileID {
		want.SetCurrent("drive_file_id", newID)
	}
	want.SetCurrent("last_backup_at", time.Now().Format(time.RFC3339))
	want.SetCurrent("backup_bytes", len(payload))
	want.SetCurrent("error", "")
	want.SetCurrent("backup_status", "done")
	want.StoreLog("[BACKUP-GOOGLE] wrote %d bytes to Drive file %s", len(payload), newID)
	return nil
}

func backupGoogleFail(want *Want, stage string, err error) error {
	msg := fmt.Sprintf("%s: %v", stage, err)
	want.SetCurrent("backup_status", "failed")
	want.SetCurrent("error", msg)
	want.StoreLog("[BACKUP-GOOGLE] %s", msg)
	// Swallowed: a failed run should not wedge the want. The next `when` tick
	// tries again, and `error` / `backup_status` say what happened.
	return nil
}

type backupWantDump struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Status string            `json:"status"`
	Labels map[string]string `json:"labels,omitempty"`
	State  map[string]any    `json:"state"`
}

// buildBackupSnapshot renders every deployed want's identity and current state
// as one JSON document.
func buildBackupSnapshot(self *Want) ([]byte, error) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil, fmt.Errorf("chain builder not available")
	}
	states := cb.GetAllWantStates()
	dumps := make([]backupWantDump, 0, len(states))
	for _, w := range states {
		if w == nil {
			continue
		}
		dumps = append(dumps, backupWantDump{
			ID:     w.Metadata.ID,
			Name:   w.Metadata.Name,
			Type:   w.Metadata.Type,
			Status: string(w.GetStatus()),
			Labels: w.Metadata.Labels,
			State:  redactSecrets(w.GetAllStateDeep()),
		})
	}
	// Stable order so a diff between two backups reads cleanly.
	sort.Slice(dumps, func(i, j int) bool {
		if dumps[i].Name != dumps[j].Name {
			return dumps[i].Name < dumps[j].Name
		}
		return dumps[i].ID < dumps[j].ID
	})
	doc := map[string]any{
		"generated_at": time.Now().Format(time.RFC3339),
		"generated_by": self.Metadata.Name,
		"want_count":   len(dumps),
		"wants":        dumps,
	}
	return json.MarshalIndent(doc, "", "  ")
}

// redactSecrets walks a state map and blanks any value whose key name looks
// like a credential, so a backup never carries a token / password / secret out
// to Drive — this agent's own refresh token included (it lives in a file, but
// a stray copy in some other field should not leak either).
func redactSecrets(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		if isSecretKey(k) {
			if s, _ := val.(string); s != "" {
				out[k] = "***redacted***"
			} else {
				out[k] = val
			}
			continue
		}
		if sub, ok := val.(map[string]any); ok {
			out[k] = redactSecrets(sub)
		} else {
			out[k] = val
		}
	}
	return out
}

func isSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, needle := range []string{"secret", "token", "password", "passwd", "api_key", "apikey", "client_secret", "refresh", "private_key", "credential"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

// ── Google OAuth ────────────────────────────────────────────────────────────

type googleTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func googleExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)
	tr, err := googlePostToken(ctx, form)
	if err != nil {
		return "", err
	}
	if tr.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token in response (was access_type=offline & prompt=consent set on the auth URL?)")
	}
	return tr.RefreshToken, nil
}

func googleRefreshAccessToken(ctx context.Context, clientID, clientSecret, refreshToken string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	tr, err := googlePostToken(ctx, form)
	if err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("no access_token in refresh response")
	}
	return tr.AccessToken, nil
}

func googlePostToken(ctx context.Context, form url.Values) (*googleTokenResp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tr googleTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse token response (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return &tr, nil
}

// ── Google Drive ───────────────────────────────────────────────────────────

// driveUpsertJSON overwrites the file at fileID with body (uploadType=media,
// PATCH). With no fileID it creates one (multipart: metadata + media) and
// returns the new id.
func driveUpsertJSON(ctx context.Context, accessToken, fileID, fileName, folderID string, body []byte) (string, error) {
	if fileID != "" {
		u := fmt.Sprintf("%s/%s?uploadType=media", driveUploadURL, url.PathEscape(fileID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		id, err := driveDo(req, fileID)
		if err == nil {
			return id, nil
		}
		// The stored id may be stale (file trashed / deleted) — fall through
		// and make a fresh one rather than failing every run forever.
		if !strings.Contains(err.Error(), "404") {
			return "", err
		}
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	meta := map[string]any{"name": fileName, "mimeType": "application/json"}
	if folderID != "" {
		meta["parents"] = []string{folderID}
	}
	metaJSON, _ := json.Marshal(meta)
	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Type", "application/json; charset=UTF-8")
	pw, _ := mw.CreatePart(mh)
	pw.Write(metaJSON)

	mh2 := make(textproto.MIMEHeader)
	mh2.Set("Content-Type", "application/json")
	pw2, _ := mw.CreatePart(mh2)
	pw2.Write(body)
	mw.Close()

	u := driveUploadURL + "?uploadType=multipart&fields=id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	return driveDo(req, "")
}

func driveDo(req *http.Request, fallbackID string) (string, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("drive HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &out)
	if out.ID != "" {
		return out.ID, nil
	}
	return fallbackID, nil
}

// ── small helpers ──────────────────────────────────────────────────────────

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
