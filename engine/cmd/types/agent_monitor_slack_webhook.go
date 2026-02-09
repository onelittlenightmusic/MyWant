package types

import (
	"context"

	. "mywant/engine/src"
)

func init() {
	RegisterPollAgentType("monitor_slack_webhook",
		[]Capability{Cap("slack_webhook_monitoring")},
		func(ctx context.Context, want *Want) (bool, error) {
			return PollWebhook(ctx, want, slackWebhookConfig)
		})
}
