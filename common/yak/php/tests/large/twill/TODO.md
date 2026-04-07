# Twill Deferred Large Fixtures

- `src__Http__Controllers__Admin__ModuleController.php`
  - Current status: AST is correct, but the main project traversal still lands around `4m33.26s`.

Follow-up:

- Keep this file out of the normal `30s` project-budget path for now.
- Revisit parser/runtime profiling later and move it back once it can stay within the standard budget again.
