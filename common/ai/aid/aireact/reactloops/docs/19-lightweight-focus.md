# Lightweight focus overlays

A lightweight focus runs the ordinary assistant loop with fixed context. It keeps the same tool dispatch, planning, review and completion behavior. Use a separate custom loop only when a mode needs a different execution state machine.

```go
err := loop_default.RegisterLightweightFocus("project-assistant", []reactloops.ReActLoopOption{
    reactloops.WithPinnedSkills("project-guide"),
    reactloops.WithPinnedTools("query_project"),
    reactloops.WithStaticInstruction("Prefer the selected project. Query before answering."),
}, reactloops.WithVerboseName("Project Assistant"))
```

Supply custom Skills through the existing config loader or `WithSkillLoader`. Select the registered name through the existing `WithFocus`, task focus or frontend focus field. `loop_default.NewLoop(name, runtime, options...)` also creates an ordinary named loop directly. These context options work on other Go loops as well.

- `WithPinnedSkills(names...)`: load the complete `SKILL.md` and file tree once per loop instance. Referenced resources remain on demand through the same loader. Missing built-in Skills can be supplied without automatic discovery or extracting files to disk. Custom loaders and custom versions of a built-in Skill take precedence.
- `WithPinnedTools(names...)`: load full parameter schemas and protocol-specific usage once. Every named tool must already be enabled and available. A missing or disabled tool fails construction; pinning grants no execution permission and makes no tool call. Pending MCP stubs must finish initialization before being pinned.
- `WithStaticInstruction(text)`: add trusted host instructions to the high-static system section. A later option replaces the earlier focus instruction, preserving the core system instructions. Pass user attachments and retrieved content through their existing data channels.

The overlay belongs to its loop instance. Timeline compression, explicit retirement of ordinary Skill records, tool-cache LRU pressure and the speed-priority history projection cannot remove its definitions. They remain byte-stable throughout execution; the ordinary assistant does not inherit the overlay after leaving the mode. Session restore or re-entry reconstructs it from the selected mode's current preset.

The combined pinned definitions and static instruction have a fixed 16,384-token ceiling. An oversized preset fails rather than truncating a required contract. Select only the Skills and tools needed by the mode; resource files and tool results retain their ordinary budgets. Duplicate root definitions are omitted from the focus prompt's Timeline projection without changing its journal, frozen boundaries, referenced resource windows or other tool results.

`knowledge_enhance` uses this mechanism. It retains its public mode name and Chinese display name, pins `yaklang-system`, and pins whichever of `manage_knowledge_base` and `query_knowledge_base` the caller permits. A read-only caller can expose only query. It performs no automatic search or model-driven selection during initialization. Attached names and raw notes guide the model's explicit scope choice; an auto-select flag means list and choose, while all-base queries require an explicit user request. Query token and result limits remain mandatory.

The generic focus registration disables the legacy automatic RAG action. The ordinary default loop and unrelated legacy callers retain their existing behavior until their separate migration.
