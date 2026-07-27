package scannode

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// statelessAIEngineRuntimeDriver is a parallel runtime driver (S3c) that runs
// a fresh stateless aiengine.AIEngine per turn. Unlike yakAIEngineRuntimeDriver,
// Bind does NOT create an engine; SendInput creates one per turn, runs SendMsg,
// and closes it on return. History/tools/user_input come from the S3a
// ContextPackage carried on aiSessionInput. NOT wired into legionJobBridge —
// wiring + driver selection is S3d.
type statelessAIEngineRuntimeDriver struct{}

func newStatelessAIEngineRuntimeDriver() aiSessionRuntimeDriver {
	return statelessAIEngineRuntimeDriver{}
}

func (statelessAIEngineRuntimeDriver) Bind(
	ctx context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	// Pre-compute the option slice that every turn will replay (attachment
	// content, credential projection, AI callback). This reuses buildYakAIEngineOptions
	// WITHOUT calling NewAIEngine — buildYakAIEngineOptions returns the options,
	// and Bind normally calls NewAIEngine(options...) itself. We skip that call
	// and cache the options for per-turn replay.
	cachedOptions, err := buildYakAIEngineOptions(ctx, binding, emitter)
	if err != nil {
		return nil, fmt.Errorf("stateless bind: build options: %w", err)
	}
	return &statelessAIEngineRuntimeHandle{
		binding:       binding,
		emitter:       emitter,
		cachedOptions: cachedOptions,
		newEngine:     aiengine.NewAIEngine, // overridable in tests
	}, nil
}

type statelessAIEngineRuntimeHandle struct {
	binding       aiSessionBinding
	emitter       aiSessionRuntimeEmitter
	cachedOptions []aiengine.AIEngineConfigOption
	closed        bool

	// newEngine is the engine constructor, overridable in tests to assert
	// per-turn lifecycle without needing a real AI provider.
	newEngine func(opts ...aiengine.AIEngineConfigOption) (*aiengine.AIEngine, error)
}

func (h *statelessAIEngineRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	// Build a fresh engine per turn with WithStateless(true) + cached options +
	// ContextPackage-derived history injection.
	options := append([]aiengine.AIEngineConfigOption{}, h.cachedOptions...)
	options = append(options, aiengine.WithStateless(true))

	// Inject ContextPackage history if present (MVP: format as attached file content).
	if input.ContextPackage != nil {
		historyBlock := buildContextPackageHistoryBlock(input.ContextPackage)
		if historyBlock != "" {
			options = append(options, aiengine.WithAttachedFileContent(historyBlock))
		}
	}

	// Determine the user input text BEFORE building the engine — avoids
	// constructing an engine only to discover there's no input to send.
	// Prefer ContextPackage.user_input; fall back to the legacy PayloadJSON
	// content (for compatibility if ContextPackage is nil).
	userInput := ""
	if input.ContextPackage != nil && input.ContextPackage.UserInput != "" {
		userInput = input.ContextPackage.UserInput
	} else {
		content, _, _, _, perr := yakAIInputContent(input)
		if perr != nil {
			return fmt.Errorf("stateless sendinput: decode input: %w", perr)
		}
		userInput = content
	}
	if userInput == "" {
		return fmt.Errorf("stateless sendinput: empty user input")
	}

	engine, err := h.newEngine(options...)
	if err != nil {
		return fmt.Errorf("stateless sendinput: new engine: %w", err)
	}
	defer engine.Close()

	return engine.SendMsg(userInput)
}

func (h *statelessAIEngineRuntimeHandle) AppendContext(_ context.Context, _ aiSessionContextUpdate) error {
	// Stateless engine has no cross-turn state; AppendContext is a no-op.
	// (If needed in the future, the next turn's ContextPackage will carry it.)
	return nil
}

func (h *statelessAIEngineRuntimeHandle) Cancel(_ string) {
	// Per-turn engine is already closed after SendMsg; Cancel is a no-op.
	// If a turn is in flight, the engine's ctx cancel (set in NewAIEngine) handles it.
}

func (h *statelessAIEngineRuntimeHandle) Close(_ string) {
	h.closed = true
	// No persistent engine to close; resources were per-turn.
}

// buildContextPackageHistoryBlock formats the replayed conversation messages
// into a text block that aiengine injects as an "attached file" so the LLM
// sees prior turns. This is the MVP history-injection mechanism (S3 spec §11
// open question — resolved: use WithAttachedFileContent since no direct
// WithHistory option exists in aiengine).
func buildContextPackageHistoryBlock(pkg *aiv1.ContextPackage) string {
	if pkg == nil || len(pkg.Messages) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[Conversation history replayed by server (S3 stateless engine)]\n\n")
	for _, m := range pkg.Messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		fmt.Fprintf(&sb, "%s: %s\n", role, m.Content)
	}
	return sb.String()
}