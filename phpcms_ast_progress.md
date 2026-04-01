# PHPCMS AST Progress

## 2026-04-01 19:57:59 +08:00

### PrestaShop

- Status: completed
- Project path: `/home/wlz/Target/phpcms/PrestaShop`
- Branch: `fix/ssa/php-project-ast-perf`

Changes made:

- Renamed the branch from `fix/php/grav_project_ast_perf` to `fix/ssa/php-project-ast-perf` without changing the worktree directory.
- Added real-project AST regression coverage for PrestaShop fixtures under:
  - `common/yak/php/tests/syntax/prestashop/`
  - `common/yak/php/tests/syntax/prestashop_slow/`
  - `common/yak/php/tests/perfdata/prestashop/`
- Extended PHP grammar / parser support for PrestaShop-driven syntax:
  - chained assignment targets from static method call results
  - static-property increment and dynamic member assignment
  - nullsafe member access
  - arrow functions with declared return types
  - intersection type hints
  - `new Class::$dynamicName()`
  - enum constants such as `public const ...`
- Extended `common/yak/php/tests/perf_test.go` with PrestaShop slow-file benchmarks.
- Updated `TestProjectAst` to re-check candidate slow files in isolation after the parallel pass, so queueing time in the concurrent reducer does not get mistaken for single-file parser time.

Verification:

- `go test ./common/yak/php/tests -run 'TestAllSyntaxForPHP_G4/syntax file: syntax/prestashop/.*' -count=1`
  - passed
- `YAK_PHP_RUN_PROJECT_AST=1 YAK_PHP_PROJECT_AST_TARGET=/home/wlz/Target/phpcms/PrestaShop YAK_PHP_FIXTURE_PARSE_BUDGET_SEC=30 go test ./common/yak/php/tests -run TestProjectAst -count=1 -v`
  - passed after isolated slow-file recheck
  - parser errors: 0
  - isolated slow files over 30s: 0

Notes:

- The concurrent reducer still reports some large `file parse:` durations for big PrestaShop files, but the isolated recheck confirmed those were not true single-file parser regressions.

### Grav

- Status: regression found during required rerun
- Project path: `/home/wlz/Target/phpcms/grav`

Current finding:

- `system/src/Grav/Framework/Flex/FlexObject.php` regressed above the 30s AST budget during grav rerun after the PrestaShop parser work.
- Work is in progress to recover grav performance before moving to the next project.
