package provider

import (
	"fmt"
	"strings"

	"mewcode/internal/config"
)

type Factory struct {
	options []Option
}

func NewFactory(opts ...Option) Factory {
	return Factory{options: opts}
}

func (f Factory) New(cfg config.Config) (Provider, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "anthropic":
		return NewAnthropic(cfg, f.options...), nil
	case "openai":
		return NewOpenAI(cfg, f.options...), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}
}
