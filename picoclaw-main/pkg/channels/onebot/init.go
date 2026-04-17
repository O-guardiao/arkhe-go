package onebot

import (
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/channels"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	channels.RegisterFactory("onebot", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewOneBotChannel(cfg.Channels.OneBot, b)
	})
}

