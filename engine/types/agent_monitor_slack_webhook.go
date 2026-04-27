package types

import (
	"context"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterMonitorAgent("monitor_slack_webhook", func(ctx context.Context, want *Want) (bool, error) {
			return PollWebhook(ctx, want, slackWebhookConfig)
		})
	})
}
