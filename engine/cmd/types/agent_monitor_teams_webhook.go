package types

import (
	"context"

	. "mywant/engine/src"
)

func init() {
	RegisterPollAgentType("monitor_teams_webhook",
		[]Capability{Cap("teams_webhook_monitoring")},
		func(ctx context.Context, want *Want) (bool, error) {
			return PollWebhook(ctx, want, teamsWebhookConfig)
		})
}
