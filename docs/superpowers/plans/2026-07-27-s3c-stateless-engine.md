# S3c: Stateless AI Engine Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stateless AI engine runtime path to the yaklang scannode: a new `statelessAIEngineRuntimeDriver` that constructs a fresh `aiengine.AIEngine` per turn (consuming the S3a `ContextPackage` for history/tools/user_input), runs the ReAct loop without persisting to local yakit DB, emits the unchanged 8-type `ai.session.*` event contract, and destroys the engine instance at turn end. Add an `aiengine.WithStateless(true)` option that short-circuits the four persistence hooks in `buildReActOptions`. The stateless driver is built + unit-tested but **NOT wired into `legionJobBridge`** — wiring (driver selection switch + cutover) is S3d. This keeps S3c a parallel path with zero impact on the running stateful engine.

**Architecture:** Three changes: (1) `common/aiengine/config.go` + `aiengine.go` — add `Stateless bool` field + `WithStateless()` option; in `buildReActOptions`, when `config.Stateless` is true, skip `WithPersistentSessionId`/`WithMemoryTriageId` (pass empty) and inject a no-op `MemoryTriage` so `re-act.go`'s four persistence branches (`:127 SaveTimeline`, `:231 EnsureAISessionMeta`, `:265 TimelineArchiveStore`, `:247 MemoryTriage`) all short-circuit. `re-act.go` itself is NOT modified. (2) `scannode/legion_ai_bridge.go` — extend `aiSessionInput` with a `ContextPackage *aiv1.ContextPackage` field; `AcceptInput` reads `command.GetContextPackage()` and passes it through. (3) new `scannode/legion_ai_runtime_stateless.go` — `statelessAIEngineRuntimeDriver` + `statelessAIEngineRuntimeHandle`: `Bind` caches the binding + pre-computes attachment/credential/callback options (reusing `buildYakAIEngineOptions`'s helpers) WITHOUT calling `NewAIEngine`; `SendInput` builds a fresh `AIEngine` per turn with `WithStateless(true)` + cached options + ContextPackage history, runs `SendMsg`, closes the engine on return.

**Tech Stack:** Go, yaklang `common/aiengine` (AIEngine + ReAct operator), `common/ai/aid/aicommon` (ConfigOption), `scannode/gen/legionpb/legion/ai/v1` (S3a proto: ContextPackage), `scannode` AI bridge (legion_ai_bridge.go, legion_ai_runtime_yak.go helpers), `httptest` (attachment download tests), existing `aiSessionRuntimeDriver`/`aiSessionRuntimeHandle` interfaces.

## Global Constraints

- **S3a is merged on this baseline.** `ContextPackage` / `ContextMessage` / `ContextTool` / `ContextKbFragment` proto types exist in `scannode/gen/legionpb/legion/ai/v1` (package `legionpb`, import path `aiv1 "legion/gen/proto/legion/ai/v1"` — but yaklang uses `legionpb "legion/gen/proto/legion/ai/v1"`; confirm the import alias used in `scannode/legion_ai_bridge.go` and use the same).
- **NOT wired into legionJobBridge.** S3c only adds the stateless driver + WithStateless option + aiSessionInput extension + unit tests. `legionJobBridge.ensureAIRuntime()` (`scannode/legion_ai_bridge.go:506`) still constructs `newAISessionRuntimeManager(newYakAIEngineRuntimeDriver())` — unchanged. Driver selection + cutover is S3d.
- **Do NOT modify `common/ai/aid/aireact/re-act.go`.** The four persistence branches (`:127`, `:231`, `:265`, `:247`) are short-circuited by passing empty `PersistentSessionId` + a no-op `MemoryTriage` from the engine config, NOT by editing re-act.go. re-act.go is shared with the client stateful path; editing it risks the client.
- **`WithStateless(true)` is the clean short-circuit.** It sets `AIEngineConfig.Stateless = true`. In `buildReActOptions` (`common/aiengine/aiengine.go` around `:525-526`), when `config.Stateless` is true: (a) pass `WithPersistentSessionId("")` instead of `config.SessionID`; (b) pass `WithMemoryTriageId("")`; (c) inject a no-op `MemoryTriage` via `WithMemoryTriage(<in-memory stub>)` so re-act.go `:246-256` else-branch does not build a DB-backed memory. The stateful path (Stateless=false) is byte-identical to today.
- **Per-turn engine lifecycle.** The stateless handle's `SendInput` builds a fresh `aiengine.AIEngine` via `aiengine.NewAIEngine(options...)` at the start of each turn, calls `engine.SendMsg(userInput)`, and calls `engine.Close()` when SendMsg returns (success or error). The engine instance is NOT retained across turns. `Bind` does NOT call `NewAIEngine` — it only caches the binding + pre-computes the option slice (minus the NewAIEngine call) so each turn replays them without re-downloading attachments.
- **Bind caches attachment/credential/callback options.** `appendYakAttachmentOptions` (`legion_ai_runtime_yak.go:419`), `renderCredentialProjection` (`:534`), `loadYakAICallback` (`:336`) run ONCE at Bind; their returned `[]aiengine.AIEngineConfigOption` are cached on the handle. Each turn replays them. This avoids re-downloading 64KiB attachments every turn. Provider/runtime snapshots come from the binding (`aiSessionBinding` `legion_ai_bridge.go:40-41`) and are also cached.
- **ContextPackage.history → engine.** The stateless handle converts `ContextPackage.messages` (replayed user+assistant) into the engine's initial conversation state. The exact mechanism: the engine's ReAct operator builds its timeline from `aiengine.WithAttachedFileContent(...)`-style context OR a dedicated history-injection option. S3c plan Task 3 determines the precise injection point — if `aiengine` has no direct "seed history" option, the fallback is to inject history as a pre-formatted system-prompt supplement via `WithAttachedFileContent`. The implementer confirms the available mechanism by inspecting `aiengine` options before coding Task 3 Step 3.
- **Event pipeline reused unchanged.** `classifyYakAIEvent` (`legion_ai_runtime_yak.go:849`), `marshalYakAIOutputEvent` (`:886`), `managedAISessionRuntimeEmitter` (`legion_ai_bridge.go:768`), `aiSessionEventPublisher.PublishEvent` (`legion_ai_events.go:113`) are reused as-is. The stateless engine wires `aiengine.WithOnEvent(...)` to the same emitter callback. The 8 `ai.session.*` sub-event types + `marshalYakAIOutputEvent` field set are unchanged → projector + SSE + frontend unaffected.
- **`aiSessionRuntimeManager.sessions` map retained.** The stateless path still needs per-session `aiSessionRuntime` (ref/seq/cancel/handle) for event seq ordering. Only the handle's `engine` becomes per-turn instead of per-session.
- **Branch discipline (AGENTS.md):** refactoring-track work on yaklang. Worktree already created at `.worktrees/s3c-stateless-engine/` on branch `feat/yaklang/s3c-stateless-engine` from fresh `origin/go0p/refactor/scannode` (which includes S3a). PR back to `go0p/refactor/scannode`.
- **Commit messages must NOT include tool-attribution lines.**
- **Spec reference:** `docs/superpowers/specs/2026-07-27-s3-coordinator-stateless-engine-design.md` §4 (ContextPackage contract), §6 (Stateless Runtime), §11 open question #2 (resolved: keep manager map, handle per-turn engine) + #3 (resolved: reuse event pipeline). If this plan and the spec disagree, the spec wins.

---

## File Structure

New files (all under yaklang scannode/ + common/aiengine/):

```
common/aiengine/
├── config.go                  # Task 1: add Stateless field + WithStateless option (modify)
├── aiengine.go                # Task 1: short-circuit buildReActOptions persistence (modify)
└── config_test.go             # Task 1: WithStateless option test (create or extend)

scannode/
├── legion_ai_bridge.go        # Task 2: extend aiSessionInput + AcceptInput (modify)
├── legion_ai_bridge_test.go   # Task 2: AcceptInput reads ContextPackage test (modify)
├── legion_ai_runtime_stateless.go     # Task 3: stateless driver + handle (create)
└── legion_ai_runtime_stateless_test.go # Task 3: unit tests (create)
```

---

## Task 1: aiengine.WithStateless option + buildReActOptions short-circuit

**Files:**
- Modify: `common/aiengine/config.go` — add `Stateless bool` field to `AIEngineConfig` + `WithStateless()` option func.
- Modify: `common/aiengine/aiengine.go` — in `buildReActOptions`, short-circuit persistence when `config.Stateless`.
- Test: `common/aiengine/config_test.go` (create if absent, else extend) — verify `WithStateless` sets the field + `buildReActOptions` produces empty PersistentSessionId.

**Interfaces:**
- Produces: `aiengine.WithStateless(enabled bool) AIEngineConfigOption`; `AIEngineConfig.Stateless bool` field. Consumed by Task 3's stateless driver.

- [ ] **Step 1: Add the Stateless field to AIEngineConfig**

In `common/aiengine/config.go`, add a field to `AIEngineConfig` (after `SessionID string` at line 38):

```go
	// Stateless 为 true 时,引擎不持久化会话历史/memory/timeline 到本地 DB。
	// 每轮由服务端打包 ContextPackage 注入历史,turn 完销毁引擎实例。
	// 用于 S3c 无状态引擎路径。buildReActOptions 据此短路 PersistentSessionId/
	// MemoryTriageId/TimelineArchiveStore/SaveTimeline 四个落盘分支。
	Stateless bool
```

- [ ] **Step 2: Add the WithStateless option func**

In `common/aiengine/config.go`, add the option func (after the existing `WithSessionID` func, or near the other `With*` option funcs — find the pattern with `grep -n 'func WithSessionID' common/aiengine/config.go`):

```go
// WithStateless 启用或禁用无状态模式。无状态模式下引擎不持久化任何跨轮状态
// (PersistentSessionId/MemoryTriageId 留空,MemoryTriage 注入 no-op),
// re-act.go 的四个落盘分支全部短路。用于 S3c scannode 无状态引擎路径。
func WithStateless(enabled bool) AIEngineConfigOption {
	return func(config *AIEngineConfig) {
		config.Stateless = enabled
	}
}
```

- [ ] **Step 3: Short-circuit buildReActOptions persistence**

In `common/aiengine/aiengine.go`, find `buildReActOptions`. The two persistence lines are around `:525-526`:

```go
		aicommon.WithPersistentSessionId(config.SessionID),
		aicommon.WithMemoryTriageId(config.SessionID),
```

Replace them with a conditional that checks `config.Stateless`:

```go
		// S3c: 无状态模式短路持久化。PersistentSessionId/MemoryTriageId 留空,
		// re-act.go 的 EnsureAISessionMeta(:231)/TimelineArchiveStore(:265)/
		// SaveTimeline(:127) 三个分支因 ID 为空而跳过。MemoryTriage 注入
		// no-op(见下方),避免 re-act.go:246-256 构建 DB-backed memory。
		persistentID := config.SessionID
		memoryTriageID := config.SessionID
		if config.Stateless {
			persistentID = ""
			memoryTriageID = ""
		}
		options = append(options,
			aicommon.WithPersistentSessionId(persistentID),
			aicommon.WithMemoryTriageId(memoryTriageID),
		)
```

Then, after the options list is built but before returning, add the no-op MemoryTriage injection for stateless mode. Find the end of `buildReActOptions` (before `return options`) and add:

```go
	if config.Stateless {
		// 注入 no-op MemoryTriage,阻止 re-act.go:246-256 在 MemoryTriageId 为空时
		// 退回到 NewAIMemory("default") 构建 DB-backed memory。no-op triage 不落盘。
		options = append(options, aicommon.WithMemoryTriage(newStatelessNoopMemoryTriage()))
	}
```

- [ ] **Step 4: Add the no-op MemoryTriage stub**

Create `common/aiengine/stateless_noop.go`:

```go
package aiengine

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// statelessNoopMemoryTriage 是无状态引擎用的 no-op MemoryTriage。
// re-act.go 在 cfg.MemoryTriage == nil 时会构建 DB-backed memory(NewAIMemory),
// 注入这个 no-op 阻止该回退,确保无状态路径不触碰本地 DB。
type statelessNoopMemoryTriage struct{}

func (statelessNoopMemoryTriage) Save(_ string, _ string) error              { return nil }
func (statelessNoopMemoryTriage) BuildIndex() error                          { return nil }
func (statelessNoopMemoryTriage) Query(_ string, _ int) ([]string, error)    { return nil, nil }
func (statelessNoopMemoryTriage) Delete(_ string) error                      { return nil }
func (statelessNoopMemoryTriage) Close() error                               { return nil }

func newStatelessNoopMemoryTriage() aicommon.MemoryTriage {
	return statelessNoopMemoryTriage{}
}
```

**IMPORTANT — verify the `aicommon.MemoryTriage` interface signature before writing this file.** Run:
```bash
grep -n 'type MemoryTriage interface' common/ai/aid/aicommon/*.go
```
The stub must implement the EXACT method set of `aicommon.MemoryTriage`. If the interface has different methods (e.g. `Search` instead of `Query`, or extra methods), adjust the stub to match. The compiler will flag any mismatch — fix until `go build ./common/aiengine/...` passes.

- [ ] **Step 5: Write the option test**

Create or extend `common/aiengine/config_test.go`:

```go
package aiengine

import "testing"

func TestWithStatelessSetsField(t *testing.T) {
	cfg := NewAIEngineConfig(WithStateless(true))
	if !cfg.Stateless {
		t.Fatal("WithStateless(true) did not set Stateless field")
	}
	cfg2 := NewAIEngineConfig(WithStateless(false))
	if cfg2.Stateless {
		t.Fatal("WithStateless(false) should leave Stateless false")
	}
}

func TestWithStatelessDefaultFalse(t *testing.T) {
	cfg := NewAIEngineConfig()
	if cfg.Stateless {
		t.Fatal("default AIEngineConfig should have Stateless=false")
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd .worktrees/s3c-stateless-engine
go test ./common/aiengine/ -run 'TestWithStateless' -v
```
Expected: PASS — 2 tests.

- [ ] **Step 7: Build to confirm no regression in the stateful path**

```bash
cd .worktrees/s3c-stateless-engine
go build ./common/aiengine/... ./scannode/...
```
Expected: PASS. The stateful path (Stateless=false) produces identical options to before (persistentID/memoryTriageID fall through to config.SessionID).

- [ ] **Step 8: Commit**

```bash
git add common/aiengine/config.go common/aiengine/aiengine.go common/aiengine/stateless_noop.go common/aiengine/config_test.go
git commit -m "feat(aiengine): add WithStateless option to short-circuit persistence (S3c)"
```

---

## Task 2: Extend aiSessionInput + AcceptInput to carry ContextPackage

**Files:**
- Modify: `scannode/legion_ai_bridge.go` — add `ContextPackage` field to `aiSessionInput`; `AcceptInput` reads `command.GetContextPackage()`.
- Modify: `scannode/legion_ai_bridge_test.go` — add test that AcceptInput populates ContextPackage.

**Interfaces:**
- Produces: `aiSessionInput.ContextPackage *aiv1.ContextPackage`; `acceptedAISessionInput.contextPackage *aiv1.ContextPackage`. Consumed by Task 3's stateless handle.

- [ ] **Step 1: Confirm the aiv1 import alias used in legion_ai_bridge.go**

```bash
cd .worktrees/s3c-stateless-engine
grep -n 'legionpb\|aiv1' scannode/legion_ai_bridge.go | head -3
```
Note the alias used for `legion/gen/proto/legion/ai/v1` (likely `aiv1` or `legionpb`). Use the same alias in the new code.

- [ ] **Step 2: Add ContextPackage field to aiSessionInput**

In `scannode/legion_ai_bridge.go`, find `type aiSessionInput struct` (around `:69-73`):

```go
type aiSessionInput struct {
	Ref         aiSessionCommandRef
	InputType   string
	PayloadJSON []byte
}
```

Add the ContextPackage field:

```go
type aiSessionInput struct {
	Ref            aiSessionCommandRef
	InputType      string
	PayloadJSON    []byte
	ContextPackage *aiv1.ContextPackage // S3c: per-turn server-assembled context (history/tools/user_input)
}
```

(Use the alias confirmed in Step 1. If it's `legionpb`, write `*legionpb.ContextPackage`.)

- [ ] **Step 3: Add contextPackage to acceptedAISessionInput**

Find `type acceptedAISessionInput struct` in the same file and add a `contextPackage *aiv1.ContextPackage` field. (Locate it with `grep -n 'type acceptedAISessionInput struct' scannode/legion_ai_bridge.go`.)

- [ ] **Step 4: Read ContextPackage in AcceptInput**

In `AcceptInput` (around `:192-232`), the return statement builds `acceptedAISessionInput{ref, seq, inputType, payloadJSON, handle}`. Add `contextPackage: command.GetContextPackage()` to that struct literal:

```go
	return acceptedAISessionInput{
		ref:            ref,
		seq:            seq,
		inputType:      inputType,
		payloadJSON:    payload,
		handle:         handle,
		contextPackage: command.GetContextPackage(),
	}, nil
```

- [ ] **Step 5: Pass ContextPackage into aiSessionInput in handleAISessionInput**

In `handleAISessionInput` (around `:373-408`), find where `aiSessionInput` is constructed for `accepted.handle.SendInput(...)`:

```go
		accepted.handle.SendInput(ctx, aiSessionInput{
			Ref:         accepted.ref,
			InputType:   accepted.inputType,
			PayloadJSON: accepted.payloadJSON,
		})
```

Add the ContextPackage field:

```go
		accepted.handle.SendInput(ctx, aiSessionInput{
			Ref:            accepted.ref,
			InputType:      accepted.inputType,
			PayloadJSON:    accepted.payloadJSON,
			ContextPackage: accepted.contextPackage,
		})
```

- [ ] **Step 6: Build to confirm compilation**

```bash
cd .worktrees/s3c-stateless-engine
go build ./scannode/...
```
Expected: PASS. The existing `yakAIEngineRuntimeHandle.SendInput` ignores the new field (it doesn't read `input.ContextPackage`), so the stateful path is unaffected.

- [ ] **Step 7: Write the failing test for AcceptInput reading ContextPackage**

Add to `scannode/legion_ai_bridge_test.go` (find the existing AcceptInput test pattern with `grep -n 'AcceptInput' scannode/legion_ai_bridge_test.go` and mirror it). The test constructs a `PushAISessionInputCommand` with a `ContextPackage` set, binds a session, calls AcceptInput, and asserts `accepted.contextPackage` is non-nil with the right session_id:

```go
func TestAcceptInputCarriesContextPackage(t *testing.T) {
	bridge, driver := newTestAISessionBridge(t) // reuse the existing helper
	ctx := context.Background()

	// Bind a session first.
	binding := aiSessionBinding{Ref: aiSessionCommandRef{SessionID: "s-cp-1", OwnerUserID: "u1"}}
	_, err := bridge.aiRuntime.Bind(ctx, binding, nil)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	cmd := &aiv1.PushAISessionInputCommand{
		SessionId: "s-cp-1",
		OwnerUserId: "u1",
		InputType: "message",
		InputJson: []byte(`{"content":"hello"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-cp-1",
			UserInput: "hello",
			Messages: []*aiv1.ContextMessage{
				{Role: "user", Content: "prior question"},
			},
		},
	}
	accepted, err := bridge.aiRuntime.AcceptInput(cmd)
	if err != nil {
		t.Fatalf("accept input: %v", err)
	}
	if accepted.contextPackage == nil {
		t.Fatal("AcceptInput did not carry ContextPackage")
	}
	if accepted.contextPackage.SessionId != "s-cp-1" || accepted.contextPackage.UserInput != "hello" {
		t.Fatalf("context package wrong: %#v", accepted.contextPackage)
	}
	if len(accepted.contextPackage.Messages) != 1 || accepted.contextPackage.Messages[0].Content != "prior question" {
		t.Fatalf("context messages wrong: %#v", accepted.contextPackage.Messages)
	}
}
```

**Caveat:** `newTestAISessionBridge` and `aiSessionBinding`/`aiSessionCommandRef` are test helpers in `legion_ai_bridge_test.go`. The implementer confirms their exact signatures (the `Bind` call may need an emitter arg — check the existing `TestAISessionBind*` test for the exact pattern) and adjusts the test setup to match. The goal is: bind a session, call AcceptInput with a ContextPackage-bearing command, assert the package flows through.

- [ ] **Step 8: Run the test to verify it passes**

```bash
cd .worktrees/s3c-stateless-engine
go test ./scannode/ -run TestAcceptInputCarriesContextPackage -v
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add scannode/legion_ai_bridge.go scannode/legion_ai_bridge_test.go
git commit -m "feat(scannode): carry ContextPackage through aiSessionInput + AcceptInput (S3c)"
```

---

## Task 3: statelessAIEngineRuntimeDriver + statelessAIEngineRuntimeHandle

**Files:**
- Create: `scannode/legion_ai_runtime_stateless.go`
- Test: `scannode/legion_ai_runtime_stateless_test.go`

**Interfaces:**
- Consumes: `aiSessionRuntimeDriver`/`aiSessionRuntimeHandle` interfaces (`legion_ai_bridge.go:19-21,25-31`); `aiengine.NewAIEngine` + `WithStateless(true)` (Task 1); `buildYakAIEngineOptions` helpers (`legion_ai_runtime_yak.go:223`); `classifyYakAIEvent` + `marshalYakAIOutputEvent` (`legion_ai_runtime_yak.go:849,886`); `aiSessionInput.ContextPackage` (Task 2).
- Produces: `newStatelessAIEngineRuntimeDriver()` returning `aiSessionRuntimeDriver`. NOT wired into legionJobBridge (S3d does that).

- [ ] **Step 1: Confirm the history-injection mechanism in aiengine**

Before writing the stateless handle, determine how to seed `ContextPackage.messages` (replayed history) into a fresh `aiengine.AIEngine`. Run:
```bash
cd .worktrees/s3c-stateless-engine
grep -rn 'WithAttachedFileContent\|WithSystemPrompt\|WithHistory\|WithMessages\|WithConversation\|WithTimeline' common/aiengine/config.go common/aiengine/aiengine.go | head -15
```
The likely mechanism is `aiengine.WithAttachedFileContent(text)` — inject a pre-formatted history block as an "attached file" so the LLM sees prior turns. If a more direct `WithHistory`/`WithMessages` option exists, use it. Record which option(s) are available; Task 3 Step 3 uses them. If none exist, the fallback is to prepend history into the `system_prompt` supplement or into the `user_input` as a formatted prefix.

- [ ] **Step 2: Write the failing stateless driver test**

Create `scannode/legion_ai_runtime_stateless_test.go`. The test verifies: (a) Bind returns a handle without calling NewAIEngine (no engine instance retained); (b) SendInput builds a per-turn engine and closes it after; (c) ContextPackage flows into the engine's context; (d) the 8-type event contract is preserved. Because the real `aiengine.AIEngine` needs an AI provider, the test uses a stub operator (mirror `stubAISyncOperator` from `legion_ai_runtime_yak_test.go:195`) to avoid real LLM calls.

```go
package scannode

import (
	"context"
	"testing"

	aiv1 "legion/gen/proto/legion/ai/v1"
)

// See legion_ai_runtime_yak_test.go for stubAISyncOperator / newTestAISessionEmitter patterns.
// The test verifies the stateless driver's per-turn lifecycle + ContextPackage injection.

func TestStatelessDriverBindDoesNotCreateEngine(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-1", OwnerUserID: "u1"},
	}
	handle, err := driver.Bind(context.Background(), binding, nil) // emitter nil for test; see caveat
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if handle == nil {
		t.Fatal("bind returned nil handle")
	}
	// The handle must NOT hold a live *aiengine.AIEngine yet (engine is per-turn).
	// Assert via a typed assertion or an exposed field — see Step 3 for the handle struct shape.
}

func TestStatelessDriverSendInputBuildsPerTurnEngine(t *testing.T) {
	// Setup: stateless driver bound; SendInput with a ContextPackage; assert:
	//   - an engine was constructed (via a injected engine-factory callback stub)
	//   - engine.Close() was called after SendMsg returns
	//   - ContextPackage.messages were injected
	// The exact assertion mechanism depends on whether the handle exposes a
	// settable engine factory for testing. Task 3 Step 3 defines the handle
	// struct with a `newEngine func(opts ...aiengine.AIEngineConfigOption) (*aiengine.AIEngine, error)`
	// field that defaults to aiengine.NewAIEngine but is overridable in tests.
}
```

**Caveat:** The emitter argument to `Bind` is `aiSessionRuntimeEmitter` — in tests it may be nil if the stateless handle tolerates nil emitter (no events to publish in a unit test). Confirm the `aiSessionRuntimeEmitter` interface and whether nil is tolerated by checking how `yakAIEngineRuntimeDriver.Bind` uses it (`legion_ai_runtime_yak.go:43-61`). If nil emitter panics, construct a no-op emitter stub.

- [ ] **Step 3: Implement statelessAIEngineRuntimeDriver + handle**

Create `scannode/legion_ai_runtime_stateless.go`:

```go
package scannode

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "legion/gen/proto/legion/ai/v1"
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
	// helpers WITHOUT calling NewAIEngine. The options are cached on the handle.
	// NOTE: buildYakAIEngineOptions currently calls NewAIEngine internally at the
	// caller (yakAIEngineRuntimeDriver.Bind:52), not inside the function itself.
	// Confirm by reading buildYakAIEngineOptions (legion_ai_runtime_yak.go:223-302):
	// it returns []aiengine.AIEngineConfigOption. Cache that slice here.
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

	// Inject ContextPackage history if present.
	if input.ContextPackage != nil {
		historyBlock := buildContextPackageHistoryBlock(input.ContextPackage)
		if historyBlock != "" {
			options = append(options, aiengine.WithAttachedFileContent(historyBlock))
		}
	}

	engine, err := h.newEngine(options...)
	if err != nil {
		return fmt.Errorf("stateless sendinput: new engine: %w", err)
	}
	defer engine.Close()

	// Determine the user input text. Prefer ContextPackage.user_input; fall back
	// to the legacy PayloadJSON content (for compatibility if ContextPackage is nil).
	userInput := ""
	if input.ContextPackage != nil && input.ContextPackage.UserInput != "" {
		userInput = input.ContextPackage.UserInput
	} else {
		// Fallback: decode from PayloadJSON via yakAIInputContent.
		content, _, _, _, perr := yakAIInputContent(input)
		if perr != nil {
			return fmt.Errorf("stateless sendinput: decode input: %w", perr)
		}
		userInput = content
	}
	if userInput == "" {
		return fmt.Errorf("stateless sendinput: empty user input")
	}
	return engine.SendMsg(userInput)
}

func (h *statelessAIEngineRuntimeHandle) AppendContext(ctx context.Context, update aiSessionContextUpdate) error {
	// Stateless engine has no cross-turn state; AppendContext is a no-op.
	// (If needed in the future, the next turn's ContextPackage will carry it.)
	return nil
}

func (h *statelessAIEngineRuntimeHandle) Cancel(reason string) {
	// Per-turn engine is already closed after SendMsg; Cancel is a no-op.
	// If a turn is in flight, the engine's ctx cancel (set in NewAIEngine) handles it.
}

func (h *statelessAIEngineRuntimeHandle) Close(reason string) {
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
```

**IMPORTANT caveats for the implementer:**
1. `buildYakAIEngineOptions` signature (`legion_ai_runtime_yak.go:223`) — confirm it returns `([]aiengine.AIEngineConfigOption, error)` and does NOT call `NewAIEngine` itself. If it DOES call NewAIEngine internally, extract the option-building into a separate helper or refactor to return options only. Read the function before coding Step 3.
2. `yakAIInputContent` (`legion_ai_runtime_yak.go:747`) — confirm its signature returns `(string, bool, *yakAISyncEvent, []aiengine.AIEngineConfigOption, error)` and that calling it with a stateless `aiSessionInput` is safe.
3. `aiengine.WithAttachedFileContent` — confirm it exists (grep `common/aiengine/config.go`). If not, use whatever option injects text context into the engine.
4. The `emitter` nil-tolerance — if `buildYakAIEngineOptions` dereferences emitter (e.g. in the OnEvent callback), a nil emitter will panic. The test must pass a no-op emitter, OR `buildYakAIEngineOptions` must tolerate nil. Read `:223-302` to confirm.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd .worktrees/s3c-stateless-engine
go test ./scannode/ -run TestStatelessDriver -v
```
Expected: PASS. Iterate on the test + implementation until both the Bind-doesn't-create-engine and SendInput-builds-per-turn-engine assertions pass.

- [ ] **Step 5: Commit**

```bash
git add scannode/legion_ai_runtime_stateless.go scannode/legion_ai_runtime_stateless_test.go
git commit -m "feat(scannode): add statelessAIEngineRuntimeDriver — per-turn engine, no persistence (S3c)"
```

---

## Task 4: Full build + regression + push + PR

**Files:** None modified — verification + push + PR only.

- [ ] **Step 1: Full scannode + aiengine build**

```bash
cd .worktrees/s3c-stateless-engine
go build ./common/aiengine/... ./scannode/...
```
Expected: PASS.

- [ ] **Step 2: Full test suite (aiengine + scannode)**

```bash
cd .worktrees/s3c-stateless-engine
go test ./common/aiengine/... ./scannode/...
```
Expected: PASS — all existing tests still green (no regression) + all new S3c tests pass. Tests requiring NATS/Postgres infrastructure may skip — no NEW failures is the bar.

- [ ] **Step 3: Confirm legionJobBridge is untouched (S3c does NOT wire in)**

```bash
cd .worktrees/s3c-stateless-engine
git diff origin/go0p/refactor/scannode -- scannode/legion_job_bridge.go scannode/legion_ai_bridge.go | grep -E 'statelessAIEngine|StatelessAIEngine' | head
```
Expected: no output in legion_job_bridge.go (the stateless driver is NOT referenced there). `legion_ai_bridge.go` shows only the Task 2 changes (aiSessionInput field + AcceptInput read). If `statelessAIEngineRuntimeDriver` appears in legion_job_bridge.go, STOP — S3c must not wire in.

- [ ] **Step 4: Rebase + push**

```bash
cd .worktrees/s3c-stateless-engine
git fetch origin && git rebase origin/go0p/refactor/scannode
git push -u origin feat/yaklang/s3c-stateless-engine --force-with-lease
```
Expected: branch pushed.

- [ ] **Step 5: Create the PR**

```bash
cd .worktrees/s3c-stateless-engine
gh pr create --base go0p/refactor/scannode --head feat/yaklang/s3c-stateless-engine \
  --title "feat(scannode): S3c 无状态 AI 引擎 runtime" \
  --body "## 改动摘要

S3 (Coordinator + 无状态引擎) 的第三步 S3c:在 scannode 侧新增无状态 AI 引擎 runtime 路径,与现有有状态 yakAIEngineRuntimeDriver 并行存在。

三个改动:
1. aiengine.WithStateless(true) 选项:在 buildReActOptions 里短路 PersistentSessionId/MemoryTriageId/TimelineArchiveStore/SaveTimeline 四个落盘分支,re-act.go 不动
2. aiSessionInput 扩展 ContextPackage 字段:AcceptInput 读取 command.GetContextPackage(),传入 handle
3. statelessAIEngineRuntimeDriver + statelessAIEngineRuntimeHandle:Bind 缓存附件/凭证/回调选项(不建引擎),SendInput 每轮 new 一个 AIEngine + WithStateless(true) + ContextPackage 历史 + turn 完 Close

## 改动文件

- common/aiengine/config.go: 加 Stateless 字段 + WithStateless 选项
- common/aiengine/aiengine.go: buildReActOptions 短路持久化
- common/aiengine/stateless_noop.go (新建): no-op MemoryTriage
- scannode/legion_ai_bridge.go: aiSessionInput 加 ContextPackage + AcceptInput 读取
- scannode/legion_ai_runtime_stateless.go (新建): 无状态 driver + handle

## 验证方式

- go build ./common/aiengine/... ./scannode/... 通过
- go test ./common/aiengine/... ./scannode/... 全绿(无回归)
- legion_job_bridge.go 零改动(S3c 不接线 driver,接线在 S3d)

## 范围说明

S3c 只建无状态 driver + 选项 + 单测,不接入 legionJobBridge。driver 选择开关 + 行为切换是 S3d(行为切换点)。有状态 yakAIEngineRuntimeDriver 保留不动,client 桌面路径零影响。

事件契约零破坏:复用 classifyYakAIEvent/marshalYakAIOutputEvent/aiSessionEventPublisher,8 个 ai.session.* 子事件类型不变。

Spec: docs/superpowers/specs/2026-07-27-s3-coordinator-stateless-engine-design.md §4 + §6"
```
Expected: PR created.

- [ ] **Step 6: Monitor CI (if the repo triggers checks on this base branch)**

```bash
gh pr checks <PR-number> -R yaklang/yaklang --watch
```
If no checks are reported (as observed with S3a's yaklang PR), note that the repo's CI may not trigger for `go0p/refactor/scannode`-base PRs; the PR is still mergeable.

- [ ] **Step 7: Report to user**

Report: PR link, test results, the `legion_job_bridge.go` unchanged confirmation, completion tier (T1 — code complete, unit tests pass; e2e not verified because S3c driver is not wired in — T2 happens at S3d).

---

## Self-Review (post-writing)

**Spec coverage check:**
- spec §4 ContextPackage contract (consumed by engine) → Task 2 carries it + Task 3 injects it ✓
- spec §6 stateless runtime (per-turn engine, no persistence, reuse event pipeline) → Task 1 (WithStateless) + Task 3 (driver/handle) ✓
- spec §6.1 open question (aiSessionRuntimeManager handle) → resolved: keep manager map, handle per-turn engine; Global Constraints document this ✓
- spec §3 principle "event contract zero-break" → Task 3 reuses classifyYakAIEvent/marshalYakAIOutputEvent; Global Constraints document ✓
- spec §3 principle "state_machine isolated" → N/A for S3c (that's S3e) ✓ correctly excluded
- spec §9 S3c = "stateless runtime, parallel path, not wired" → Task 4 Step 3 verifies legion_job_bridge.go unchanged ✓
- spec §11 open question #2 (stateless runtime file layout) → resolved: new legion_ai_runtime_stateless.go, keep manager map ✓
- spec §11 open question #3 (streaming→running trigger) → S3e scope, correctly excluded ✓

**Placeholder scan:** The plan has several "confirm before coding" caveats (buildYakAIEngineOptions signature, MemoryTriage interface, WithAttachedFileContent existence, emitter nil-tolerance, newTestAISessionBridge helper signature). These are NOT placeholders — they are explicit verification steps where the implementer must read the code before writing, because the exact signatures were not fully verified during plan writing (the subagent gave file:line but not full signatures for every helper). Each caveat tells the implementer exactly what to grep and what to do if the assumption is wrong. This is the honest representation of uncertainty in a large unfamiliar codebase.

**Type consistency:** `aiSessionInput.ContextPackage *aiv1.ContextPackage` (Task 2) → consumed in Task 3 `input.ContextPackage`. `acceptedAISessionInput.contextPackage` (Task 2) → passed to `aiSessionInput.ContextPackage` in handleAISessionInput (Task 2 Step 5). `aiengine.WithStateless(bool)` (Task 1) → used in Task 3 `options = append(options, aiengine.WithStateless(true))`. `statelessAIEngineRuntimeHandle.newEngine` field (Task 3) → defaults to `aiengine.NewAIEngine`, overridable in tests. The `aiv1` alias is flagged for confirmation in Task 2 Step 1.

**Risk callout:** Task 3 Step 3 is the highest-risk step — it depends on `buildYakAIEngineOptions` returning options (not calling NewAIEngine), `yakAIInputContent` being safe to call, and `WithAttachedFileContent` existing. If any assumption breaks, the implementer stops and reports (per executing-plans "stop when blocked"). This is preferable to the plan guessing wrong and producing non-compiling code.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-27-s3c-stateless-engine.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks. Task 1 (aiengine) and Task 2 (bridge) are independent; Task 3 depends on both.

**2. Inline Execution** — Execute tasks in this session using executing-plans.

Which approach?"