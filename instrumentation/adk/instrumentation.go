package adk

import (
	"errors"
	"fmt"

	"google.golang.org/adk/v2/agent"
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
	toolSystemResolver func(tool.Tool) string
}

// WithSystem sets the gen_ai.system value used for inference metric
// dimensions. Defaults to "adk" when not specified. This value
// identifies the AI system/provider for metric attribution and must
// not be inferred from model names or tool descriptions.
func WithSystem(system string) Option {
	return func(c *config) { c.system = system }
}

// WithToolSystemResolver sets a resolver that maps an ADK tool to a
// gen_ai.system value for the execute_tool span. When unset or when
// the resolver returns an empty string, gen_ai.system is omitted.
// Resolver panics are recovered; the attribute is omitted and a
// diagnostic is reported.
func WithToolSystemResolver(fn func(tool.Tool) string) Option {
	return func(c *config) { c.toolSystemResolver = fn }
}

// Plugin holds the adapter state and is the receiver for all ADK
// callback methods. It is distinct from the ADK plugin.Plugin returned
// by New.
type Plugin struct {
	instr              *otelgenai.Instrumenter
	registry           *registry
	system             string
	toolSystemResolver func(tool.Tool) string
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
	}

	return plugin.New(plugin.Config{
		Name: pluginName,

		BeforeAgentCallback: p.beforeAgentCallback,
		AfterAgentCallback:  p.afterAgentCallback,

		BeforeModelCallback:  p.beforeModel,
		AfterModelCallback:   p.afterModel,
		OnModelErrorCallback: p.onModelError,

		BeforeToolCallback:  p.beforeTool,
		AfterToolCallback:   p.afterTool,
		OnToolErrorCallback: p.onToolError,

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
