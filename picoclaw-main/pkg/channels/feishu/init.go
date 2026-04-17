package feishu

import (
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/channels"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	channels.RegisterFactory("feishu", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewFeishuChannel(cfg.Channels.Feishu, b)
	})
}

