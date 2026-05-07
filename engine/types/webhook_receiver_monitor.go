package types

import (
	"context"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterMonitorAgent("monitor_webhook_receiver", func(ctx context.Context, want *Want) (bool, error) {
			cfg := WebhookWantConfig{
				StatePrefix: want.GetStringParam("state_prefix", "webhook"),
				LogPrefix:   "[WEBHOOK-RECEIVER-MONITOR]",
			}
			return PollWebhook(ctx, want, cfg)
		})
	})
}
