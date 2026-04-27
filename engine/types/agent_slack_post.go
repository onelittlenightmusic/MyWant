package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	. "mywant/engine/core"
)

const slackPostAgentName = "agent_slack_post"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(slackPostAgentName, executeSlackPost)
	})
}

// executeSlackPost reads composed state and posts the briefing to Slack
func executeSlackPost(ctx context.Context, want *Want) error {
	slackURL := GetCurrent(want, "slack_webhook_url", "")
	if slackURL == "" {
		slackURL = os.Getenv("SLACK_WEBHOOK_URL")
	}
	if slackURL == "" {
		return fmt.Errorf("slack_webhook_url not provided and SLACK_WEBHOOK_URL env var not set")
	}

	// Read message from state (set by SlackPostWant.Progress via outgoing_message)
	message, _ := want.GetCurrent("last_message")
	msg, _ := message.(string)
	if msg == "" {
		return fmt.Errorf("no message to post (set 'message' param or 'last_message' state)")
	}

	want.StoreLog("[SLACK] Posting briefing (%d chars)", len(msg))
	if err := postToSlack(ctx, slackURL, msg); err != nil {
		return err
	}

	now := time.Now()
	want.SetCurrent("last_message", msg)
	want.SetCurrent("briefing_status", "done")
	want.SetCurrent("last_posted_date", now.Format("2006-01-02"))
	want.SetCurrent("last_posted_at", now.Format(time.RFC3339))
	want.SetAgentActivity(slackPostAgentName, fmt.Sprintf("Briefing posted to Slack at %s", now.Format("15:04")))
	want.StoreLog("[SLACK] Done")
	return nil
}

// postToSlack sends a message to a Slack Incoming Webhook URL
func postToSlack(ctx context.Context, webhookURL, message string) error {
	payload := map[string]string{"text": message}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// composeBriefingMessage builds the full Slack message from weather and transit lines
func composeBriefingMessage(weatherText string, transitLines []string) string {
	now := time.Now()
	weekdays := map[time.Weekday]string{
		time.Monday: "月", time.Tuesday: "火", time.Wednesday: "水",
		time.Thursday: "木", time.Friday: "金", time.Saturday: "土", time.Sunday: "日",
	}

	header := fmt.Sprintf(":sunny: おはようございます！ %s（%s）",
		now.Format("2006年01月02日"), weekdays[now.Weekday()])

	var sb strings.Builder
	sb.WriteString(header + "\n\n")
	sb.WriteString(":partly_sunny: *天気*\n  " + weatherText + "\n\n")

	if len(transitLines) > 0 {
		sb.WriteString(":train: *乗換案内*\n")
		for _, line := range transitLines {
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}
