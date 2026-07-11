package provider

import (
	"testing"

	"mewcode/internal/config"
)

func TestFactorySelectsProviders(t *testing.T) {
	base := config.Config{Model: "test", BaseURL: "http://localhost", APIKey: "key"}

	anthropicProvider, err := NewFactory().New(config.Config{Protocol: "anthropic", Model: base.Model, BaseURL: base.BaseURL, APIKey: base.APIKey})
	if err != nil {
		t.Fatalf("anthropic provider returned error: %v", err)
	}
	if _, ok := anthropicProvider.(*Anthropic); !ok {
		t.Fatalf("got %T, want *Anthropic", anthropicProvider)
	}

	openAIProvider, err := NewFactory().New(config.Config{Protocol: "openai", Model: base.Model, BaseURL: base.BaseURL, APIKey: base.APIKey})
	if err != nil {
		t.Fatalf("openai provider returned error: %v", err)
	}
	if _, ok := openAIProvider.(*OpenAI); !ok {
		t.Fatalf("got %T, want *OpenAI", openAIProvider)
	}
}

func TestFactoryRejectsUnsupportedProtocol(t *testing.T) {
	_, err := NewFactory().New(config.Config{Protocol: "other", Model: "test", BaseURL: "http://localhost", APIKey: "key"})
	if err == nil || err.Error() != "unsupported protocol: other" {
		t.Fatalf("got %v, want unsupported protocol error", err)
	}
}
