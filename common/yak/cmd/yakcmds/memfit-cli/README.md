# Memfit CLI

`memfit` is the local agent CLI backed by the Yaklang ReAct engine. The
interactive frontend and the AI engine run in separate processes so terminal
rendering, cancellation, and diagnostics cannot corrupt the model protocol.

## Entrypoints

```bash
yak memfit                         # general-purpose local agent
yak memfit-code                    # coding-only agent
yak memfit-code --print "fix tests" # one-shot coding task
yak memfit-code --workdir ./backend --review ai
```

`memfit-code` is also available as `yak code-agent`. It is a fixed coding
profile layered on the same runtime:

- focus mode is always `coding`;
- the general intent router cannot switch the task into a security or traffic
  analysis focus;
- the coding distribution composes its own ToolManager from the existing tool
  catalog, so coding tools are enabled by default without reimplementing any
  tool callbacks;
- the default tool disclosure starts with coding tools, code navigation tools,
  Git, and enabled MCP tools;
- the default skill disclosure starts with `coding-workflow`,
  `yaklang-system`, `fuzztag`, and `code-review`;
- default disclosure is presentation-only. Every other built-in tool, profile
  plugin, MCP tool, Forge, Plan, sub-agent, and Skill remains searchable and
  callable through `search_capabilities`, `require_tool`, and
  `directly_call_tool`;
- the coding loop keeps the full repository-aware ReAct capabilities: TODO
  tracking, file inspection, incremental editing, shell/build/test execution,
  Git inspection, attached files, cancellation, and session history.

## Architecture

```text
yak memfit-code
  -> memfit CLI command + TUI/plain renderer
  -> spawn `yak memfit-worker`
  -> aiengine.NewAIEngine(focus=coding)
  -> composed coding ToolManager + standard MCP loader
  -> ReActLoop coding -> loop_default execution machinery
  -> default disclosure policy + coding-workflow skill
  -> read/modify/bash/git actions against the selected workdir
```

The worker protocol is versioned and framed. The parent can only send start,
input, interactive reply, review policy, cancel, and shutdown frames. The child
returns presentation-safe events plus terminal turn status.
