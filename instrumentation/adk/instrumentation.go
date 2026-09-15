package adk

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
)

// pluginName is the stable identity of this adapter in ADK's plugin
// system. ADK's plugin manager treats plugin names as identity and
// rejects duplicate registration.
const pluginName = "otelgenai-go"

// Option configures the ADK adapter plugin.
type Option func(*config)

// config holds the adapter configuration.
type config struct {
	system             string
	toolSystemResolver ToolSystemResolver
	provider           string
	providerResolver   InferenceProviderResolver
}

// ToolSystemResolver maps an ADK tool to a gen_ai.system value for the
// execute_tool span. When unset or when the resolver returns an empty
// string, gen_ai.system is omitted. Resolver panics are recovered; the
// attribute is omitted and a diagnostic is reported.
type ToolSystemResolver func(toolNameProvider) string

// InferenceProviderResolver maps a request model name to a
// gen_ai.provider.name value for the standard inference-details event.
// The resolver runs only when content-bearing events are enabled.
// Resolver panics are recovered; the event is not emitted and a
// diagnostic is reported.
type InferenceProviderResolver func(modelName string) string

// WithSystem sets the gen_ai.system value used for inference metric
// dimensions. Defaults to "adk" when not specified. This value
// identifies the AI system/provider for metric attribution and must
// not be inferred from model names or tool descriptions. It is not
// used as gen_ai.provider.name; use WithInferenceProvider for that.
func WithSystem(system string) Option {
	return func(c *config) { c.system = system }
}

// WithToolSystemResolver sets a resolver that maps an ADK tool to a
// gen_ai.system value for the execute_tool span. When unset or when
// the resolver returns an empty string, gen_ai.system is omitted.
// Resolver panics are recovered; the attribute is omitted and a
// diagnostic is reported.
func WithToolSystemResolver(fn ToolSystemResolver) Option {
	return func(c *config) { c.toolSystemResolver = fn }
}

// WithInferenceProvider sets the gen_ai.provider.name reported on the
// standard gen_ai.client.inference.operation.details event. The
// upstream event schema requires a provider name; without one (or a
// resolver) the event is not emitted. ADK is a framework, not a
// provider, so this value is never inferred.
func WithInferenceProvider(provider string) Option {
	return func(c *config) { c.provider = provider }
}

// WithInferenceProviderResolver sets a resolver that maps a request
// model name to a gen_ai.provider.name value. When both a resolver and
// a static provider are configured, a non-empty resolver result takes
// precedence; an empty resolver result falls back to the static value.
func WithInferenceProviderResolver(fn InferenceProviderResolver) Option {
	return func(c *config) { c.providerResolver = fn }
}

// Plugin holds the adapter state and is the receiver for all ADK
// callback methods. It is distinct from the ADK plugin.Plugin returned
// by New.
type Plugin struct {
	instr              *otelgenai.Instrumenter
	registry           *registry
	system             string
	toolSystemResolver ToolSystemResolver
	provider           string
	providerResolver   InferenceProviderResolver
}

// New creates an ADK plugin that instruments agent, model, and tool
// lifecycle through the otelgenai core APIs. The plugin is named
// "otelgenai-go"; duplicate-name validation occurs when ADK's plugin
// manager is built during runner construction.
//
// Register the otelgenai plugin first for strongest terminal-metric
// completeness when other plugins may intercept After* callbacks.
func New(instr *otelgenai.Instrumenter, opts ...Option) (*plugin.Plugin, error) {
	if instr == nil {
		return nil, errors.New("otelgenai: nil instrumenter")
	}

	cfg := config{system: "adk"}
	for _, opt := range opts {
		opt(&cfg)
	}

	p := &Plugin{
		instr:              instr,
		registry:           newRegistry(),
		system:             cfg.system,
		toolSystemResolver: cfg.toolSystemResolver,
		provider:           cfg.provider,
		providerResolver:   cfg.providerResolver,
	}

	return plugin.New(plugin.Config{
		Name: pluginName,

		BeforeAgentCallback: p.beforeAgentCallback,
		AfterAgentCallback:  p.afterAgentCallback,

		BeforeModelCallback:  p.beforeModelCallback,
		AfterModelCallback:   p.afterModelCallback,
		OnModelErrorCallback: p.onModelErrorCallback,

		BeforeToolCallback:  p.beforeToolCallback,
		AfterToolCallback:   p.afterToolCallback,
		OnToolErrorCallback: p.onToolErrorCallback,

		AfterRunCallback: p.afterRunCallback,
	})
}

// beforeAgentCallback adapts the ADK BeforeAgentCallback signature to
// the adapter's callbackContext interface. Returns (nil, nil).
func (p *Plugin) beforeAgentCallback(ctx agent.Context) (*genai.Content, error) {
	return p.beforeAgent(ctx)
}

// afterAgentCallback adapts the ADK AfterAgentCallback signature to
// the adapter's callbackContext interface. Returns (nil, nil).
func (p *Plugin) afterAgentCallback(ctx agent.Context) (*genai.Content, error) {
	return p.afterAgent(ctx)
}

// beforeModelCallback adapts the ADK BeforeModelCallback signature.
// The occurrence time is captured at callback entry, before any
// normalization or projection work. Returns (nil, nil).
func (p *Plugin) beforeModelCallback(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	occurredAt := time.Now()
	p.beforeModel(ctx, req, occurredAt)
	return nil, nil
}

// afterModelCallback adapts the ADK AfterModelCallback signature.
// The occurrence time is captured at callback entry. Returns (nil, nil).
func (p *Plugin) afterModelCallback(ctx agent.Context, llmResponse *model.LLMResponse, llmResponseError error) (*model.LLMResponse, error) {
	occurredAt := time.Now()
	p.afterModel(ctx, llmResponse, llmResponseError, occurredAt)
	return nil, nil
}

// onModelErrorCallback adapts the ADK OnModelErrorCallback signature.
// The occurrence time is captured at callback entry. Returns (nil, nil).
func (p *Plugin) onModelErrorCallback(ctx agent.Context, req *model.LLMRequest, err error) (*model.LLMResponse, error) {
	occurredAt := time.Now()
	p.onModelError(ctx, err, occurredAt)
	return nil, nil
}

// beforeToolCallback adapts the ADK BeforeToolCallback signature.
// The occurrence time is captured at callback entry. Returns (nil, nil).
func (p *Plugin) beforeToolCallback(ctx agent.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
	occurredAt := time.Now()
	p.beforeTool(ctx, t, args, occurredAt)
	return nil, nil
}

// afterToolCallback adapts the ADK AfterToolCallback signature.
// The occurrence time is captured at callback entry. Returns (nil, nil).
func (p *Plugin) afterToolCallback(ctx agent.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
	occurredAt := time.Now()
	p.afterTool(ctx, result, err, occurredAt)
	return nil, nil
}

// onToolErrorCallback adapts the ADK OnToolErrorCallback signature.
// The occurrence time is captured at callback entry. Returns (nil, nil).
func (p *Plugin) onToolErrorCallback(ctx agent.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
	occurredAt := time.Now()
	p.onToolError(ctx, err, occurredAt)
	return nil, nil
}

// afterRunCallback adapts the ADK AfterRunCallback signature to
// invocation-scoped cleanup. It is teardown-only.
func (p *Plugin) afterRunCallback(ctx agent.InvocationContext) {
	if ctx == nil {
		return
	}
	p.afterRun(ctx.InvocationID())
}

// String returns a human-readable description of the plugin.
func (p *Plugin) String() string {
	return fmt.Sprintf("adk.Plugin{name=%s, system=%s}", pluginName, p.system)
}
