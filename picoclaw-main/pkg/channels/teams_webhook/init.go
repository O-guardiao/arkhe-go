package teamswebhook

import (
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/channels"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	channels.RegisterFactory("teams_webhook", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewTeamsWebhookChannel(cfg.Channels.TeamsWebhook, b)
	})
}

