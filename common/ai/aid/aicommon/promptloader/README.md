# AI prompt resources

Shared fixed AI prompt text and its associated response schemas live below `prompts/` and are read through this package. Paths passed to `MustLoad` and `ReadFile` are relative to that directory. `agents/security/` and `agents/coding/` own identity, default-loop instructions/examples, verification instructions/examples, and plan/task review instructions/examples. `AgentPrompts(AgentMode)` is the only selector; an unspecified mode uses security. `mainloop/` contains shared section/cache wrappers; only sections that differ by protocol have `textstream/` and `functioncall/` variants. `ai/`, `aiforge/`, and `inline/` retain the originating package in their paths to avoid name collisions.

Coordinator prompts and the Yakit plan-review schema live here alongside the shared ReAct resources.

Edit the source files directly. Ordinary builds embed them without a generation step. Release builds use `gzip_embed`; `scripts/generate-compressed-assets.sh` regenerates the single `prompts.tar.gz` archive from the same source tree before compiling. `TestCompressedPromptResources` compares every loaded resource with its source in both build modes and fails if the archive is stale.

## Agent content and assembly

`prompt_mainloop.go` defines the content set and mode selector. `prompt_security.go` and `prompt_coding.go` explicitly list each mode's resource paths; only the Coding loader composes its fixed fragments. `prompt_shared.go` loads common protocol/cache wrappers. All resources are loaded at package initialization, without session data or dynamic path construction.

Callers use `ypb.AgentModeSecurity` / `ypb.AgentModeCoding` for agent selection and `PromptProtocolTextStream` / `PromptProtocolFunctionCall` for template selection. `PromptMaterials.Protocol()` converts the existing request flag at the assembly boundary; template selectors do not accept bare booleans.

- `agents/<mode>/high_static/`: immutable identity and protocol-specific system text. Coding methodology and platform rules are composed once at resource load. No project paths, timestamps or Git queries belong here.
- `agents/<mode>/default_loop/`: next-action instructions and examples. Specialized loops keep their explicit overrides.
- `agents/<mode>/verification/`: caller-specific completion criteria and examples; response schema/dynamic input remain in `ai/aid/aireact/prompts/verification/`.
- `agents/<mode>/plan_review/` and `task_review/`: mode-specific review instructions and examples. Review schemas, dynamic input templates and decision handling remain shared in `ai/aid/aicommon/prompts/review/` and `do_wait_agree_dynamic_planning.go`. Coding reviews assess implementation and verification; security review content is preserved byte-for-byte.
- `agents/coding/workspace_context.txt`: session facts only. `prompt_workspace.go` prepares ordered directory mappings and worktree creation metadata using `encoding/json`. Templates do not format or escape paths.
- `mainloop/workspace.txt`: existing security workspace rendering, preserved unchanged.

All full-prefix callers use `ApplyAgentPromptContext` when constructing materials, including main decisions, answers, verification, plan review and task review. A caller keeps its own task instruction, schema and output protocol. Workspace facts enter SemiDynamic1; live Git state is obtained through tools, not prompt rendering. Rendering does not scan directories, create artifacts or change process cwd.

Security source text and fixed-input assembly are pinned by `TestSecurityPromptBaseline` (only the per-process projection nonce is normalized). `TestSecurityPlanningReviewPromptBaseline` pins complete plan/task review requests in Chinese and English, normalizing only envelope nonces, the current time and host OS/architecture. Mode/context tests cover JSON round trips, cache boundaries and helper callers. The FreeInput integration test creates a real Git worktree and dispatches the built-in file tool under both textstream and functioncall protocols.

To inspect complete first-turn requests from that integration fixture (scripted model responses, real engine assembly), run:

```sh
CODING_PROMPT_EXPORT_DIR=/absolute/output/directory go test ./common/ai/aid/aireact/sessionruntime -run '^TestCodingRuntimeFreeInputWritesOnlyManagedWorktree$' -count=1
```

Each protocol exports the complete prompt plus projected messages/action tools. Only the internal projection nonce is redacted. Temporary project paths refer to the test fixture, not a user session; the test deletes its fixture on completion. Ordinary test runs do not export files.
