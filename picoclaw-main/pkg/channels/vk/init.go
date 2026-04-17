package vk

import (
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/channels"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	channels.RegisterFactory("vk", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewVKChannel(cfg, b)
	})
}

