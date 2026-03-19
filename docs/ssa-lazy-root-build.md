# SSA Lazy Root Build

## Goal

Project compile no longer keeps a `file -> AST -> second generic traversal` loop in `common/yak/ssaapi/ssa_compile_fs.go`.

The compile pipeline is now:

1. **PreHandler single AST traversal**
   - parse each file once
   - register `Function` / `Blueprint` lazy builders
   - register a small set of controlled root build nodes for top-level work
   - release the file AST reference from the project scanner immediately after registration
2. **Root build launch**
   - `Program.RunRootBuilds()` only launches registered SSA-owned nodes
   - no second pass over the project file list
3. **Lazy expansion**
   - top-level nodes trigger function / blueprint builds on demand
   - `Program.Finish()` completes the remaining lazy graph

## Root build node types

Current root launchers are:

- `function`: existing `Function.Build()`
- `blueprint`: existing `Blueprint.Build()`
- `top-level`: controlled top-level builder used for file-scope work
- `helper`: metadata-only root tasks such as extra file/editor recording

The important constraint is:

- AST can still be captured **inside lazy builders**
- AST must **not** be owned by the project-level second pass

## Top-level builder

`ssa.TopLevelBuilder` is the bridge for file-scope logic that is not itself a normal function or blueprint.

Responsibilities:

- owns the AST closure for one top-level build unit
- restores editor context before executing the closure
- records file/editor metadata through the normal program editor stack
- runs exactly once through `LazyBuilder`

This keeps the root phase node-driven while avoiding a generic `prog.Build(rootAST, ...)` loop in the orchestrator.

## Language-specific notes

- **Go / Java / TypeScript / PHP / Python**
  - keep their existing prehandler/front-loading logic
  - root phase launches registered top-level nodes with the normal builder/editor context
- **C**
  - prehandler now only registers function/type front information
  - runtime root phase skips re-registering function definitions and only handles non-function top-level work
  - this avoids duplicate lazy builders and duplicate SSA output

## Additional guardrails

- `Program.VisitAst()` is now a compatibility no-op; root completion no longer depends on scanner-held AST bookkeeping
- `Program.Finish()` snapshots upstream programs before recursion
  - this avoids deadlocks when lazy build creates new libraries while finish is traversing upstream programs

## Expected follow-up

This refactor gives the required foundation for later concurrency work:

- AST lifetime is shorter
- root launch is explicit and controllable
- lazy build dependencies are represented by SSA-owned nodes instead of a second filesystem/AST loop
