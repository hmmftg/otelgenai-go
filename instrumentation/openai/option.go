package openai

import "github.com/hmmftg/otelgenai-go"

// adapterConfig holds adapter-level configuration resolved from
// adapter Options. The default system is "openai".
type adapterConfig struct {
	system string
}

// defaultAdapterConfig returns the safe default adapter configuration.
func defaultAdapterConfig() adapterConfig {
	return adapterConfig{
		system: otelgenai.SystemOpenAI,
	}
}

// Option configures an OpenAI adapter wrapper.
type Option func(*adapterConfig)

// WithSystem overrides the gen_ai.system attribute emitted by the
// wrapper. This supports OpenAI-compatible providers (Ollama, vLLM,
// Groq, Together, Fireworks, internal gateways, etc.) that use the
// official OpenAI SDK with a custom base URL.
//
// An empty string falls back to the default "openai". Whitespace in
// the supplied value is preserved (not trimmed); only the empty
// string is special-cased to the default. This avoids silently
// modifying caller-provided identifiers.
func WithSystem(system string) Option {
	return func(c *adapterConfig) {
		if system == "" {
			c.system = otelgenai.SystemOpenAI
			return
		}
		c.system = system
	}
}

// resolveSystem applies options to the default config and returns the
// resolved system value.
func resolveSystem(opts []Option) string {
	cfg := defaultAdapterConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.system
}
