package discord

import (
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/audio/tts"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/channels"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	channels.RegisterFactory("discord", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		ch, err := NewDiscordChannel(cfg.Channels.Discord, b)
		if err == nil {
			ch.tts = tts.DetectTTS(cfg)
		}
		return ch, err
	})
}

