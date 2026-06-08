# SSA Deferred Build

## Goal

Project compile no longer keeps a `file -> AST -> second generic traversal` loop in `common/yak/ssaapi/ssa_compile_fs.go`.

The compile pipeline is now:

1. **PreHandler single AST traversal**
   - parse each file once
   - emit file-level SSA skeletons where the language front end supports it
   - register `Function` / `Blueprint` lazy builders and small deferred helper tasks
   - release the file AST root immediately after registration
2. **Deferred task launch**
   - `Program.RunDeferredBuilds()` only launches registered SSA-owned tasks
   - no second pass over the project file list
3. **Lazy expansion**
   - `Program.Finish()` completes the remaining lazy graph

## Deferred Build Types

Current launchers are:

- `function`: existing `Function.Build()`
- `blueprint`: existing `Blueprint.Build()`
- `file`: file-scope work that still needs the normal builder/editor context
- `helper`: metadata-only tasks such as extra file/editor recording

The important constraint is:

- AST can still be captured inside lazy builders after `ssa.DetachAST`
- AST must not be owned by a project-level second pass

## Language Notes

- **Go / Java / TypeScript / PHP / Python / C**
  - keep their existing prehandler/front-loading logic
  - pre-handler emits skeletons and schedules any remaining file work through deferred build tasks
  - the shared pipeline does not register a whole-file-AST closure for these languages

## Guardrails

- `Program.VisitAst()` is a compatibility no-op; completion no longer depends on scanner-held AST bookkeeping
- `Program.Finish()` snapshots upstream programs before recursion
  - this avoids deadlocks when lazy build creates new libraries while finish is traversing upstream programs

## Expected Follow-Up

This refactor gives the required foundation for later concurrency work:

- AST lifetime is shorter
- deferred launch is explicit and controllable
- lazy build dependencies are represented by SSA-owned tasks instead of a second filesystem/AST loop
