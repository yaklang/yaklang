package browsercrypto

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const ForgeName = "browser_crypto_analysis"

//go:embed prompts/init.txt
var initializePrompt string

//go:embed prompts/persistent.txt
var persistentPrompt string

var requiredCapabilities = []string{
	"browser.recording.trace.list",
	"browser.recording.evidence.inspect",
	"browser.callable.inspect",
	"browser.callable.replay",
	"browser.packet.compare",
	"browser.profile.propose",
	"browser.profile.validate",
}

// Runner binds the reusable browser-cryptography Forge to one live browser
// bridge. The bridge is supplied by the hosting runtime instead of being kept
// in the process-wide Forge registry.
type Runner struct {
	bridge browsertools.Bridge
}

func NewRunner(bridge browsertools.Bridge) *Runner {
	return &Runner{bridge: bridge}
}

func NewExecutor(bridge browsertools.Bridge) aiforge.ForgeExecutor {
	return NewRunner(bridge).Execute
}

func (r *Runner) prepare(
	_ context.Context,
	items []*ypb.ExecParamItem,
) (*Config, *aiforge.ForgeBlueprint, error) {
	if r == nil || r.bridge == nil || !r.bridge.Available() {
		return nil, nil, errors.New("browser extension bridge is not running")
	}

	config, err := ParseConfig(items)
	if err != nil {
		return nil, nil, err
	}

	capabilityCatalog, connected := r.bridge.CapabilityCatalog(config.DeviceID)
	if !connected {
		return nil, nil, errors.New("browser extension device does not provide a signed capability catalog")
	}
	connectionCapabilities := make([]string, 0, len(capabilityCatalog.Capabilities))
	for _, capability := range capabilityCatalog.Capabilities {
		connectionCapabilities = append(connectionCapabilities, capability.Method)
	}
	available := make(map[string]struct{}, len(connectionCapabilities))
	for _, capability := range connectionCapabilities {
		available[capability] = struct{}{}
	}
	var missing []string
	for _, capability := range requiredCapabilities {
		if _, ok := available[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Errorf(
			"browser extension is missing AI analysis capabilities: %s",
			strings.Join(missing, ", "),
		)
	}

	tools, err := buildTools(
		r.bridge,
		config.DeviceID,
		config.Target,
		capabilityCatalog,
	)
	if err != nil {
		return nil, nil, err
	}

	forge := aiforge.NewForgeBlueprint(
		ForgeName,
		aiforge.WithInitializePrompt(initializePrompt),
		aiforge.WithPersistentPrompt(persistentPrompt),
		aiforge.WithTools(tools...),
	)
	return &config, forge, nil
}

func (r *Runner) PrepareReAct(
	ctx context.Context,
	items []*ypb.ExecParamItem,
) (*aiforge.RuntimeForgeReActPreparation, error) {
	config, forge, err := r.prepare(ctx, items)
	if err != nil {
		return nil, err
	}
	return aiforge.PrepareRuntimeForgeReAct(forge, []*ypb.ExecParamItem{
		{Key: "query", Value: config.Query},
		{
			Key: "page_target",
			Value: fmt.Sprintf(
				"tabId=%d frameId=%d documentId=%s",
				config.Target.TabID,
				config.Target.FrameID,
				config.Target.DocumentID,
			),
		},
	})
}

func (r *Runner) Execute(
	ctx context.Context,
	items []*ypb.ExecParamItem,
	options ...aicommon.ConfigOption,
) (*aiforge.ForgeResult, error) {
	config, forge, err := r.prepare(ctx, items)
	if err != nil {
		return nil, err
	}
	scopedOptions := append([]aicommon.ConfigOption{}, options...)
	scopedOptions = append(scopedOptions, aiforge.WithToolScope(forge.Tools...))
	coordinator, err := forge.CreateCoordinator(ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: config.Query},
		{
			Key: "page_target",
			Value: fmt.Sprintf(
				"tabId=%d frameId=%d documentId=%s",
				config.Target.TabID,
				config.Target.FrameID,
				config.Target.DocumentID,
			),
		},
	}, scopedOptions...)
	if err != nil {
		return nil, err
	}
	if err := coordinator.Run(); err != nil {
		return nil, err
	}
	return &aiforge.ForgeResult{Forge: forge}, nil
}
