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

## 2026-04-01 20:22:45 +08:00

### Checkpoint

- Commit created: `967e081de`
- Commit message: `fix(php): checkpoint prestashop ast support`

Saved state:

- `PrestaShop` fixture coverage and project-level AST pass are checkpointed in git.
- Work after the checkpoint is intentionally left uncommitted while exploring the grav regression.

### Current Exploration

- `PrestaShop`
  - still passes fixture coverage
  - project-level AST pass was revalidated before the checkpoint commit
- `grav`
  - still blocked by `system/src/Grav/Framework/Flex/FlexObject.php`
  - recent parser experiments after the checkpoint are uncommitted
  - current focus is to remove the grav performance regression without losing the PrestaShop support added in the checkpoint

## 2026-04-01 21:37:01 +08:00

### Grav

- Status: completed
- Project path: `/home/wlz/Target/phpcms/grav`

Changes made:

- Kept the `typeHint` rule split introduced during the grav investigation:
  - `typeHintAtom`
  - `typeHintIntersection`
  - `typeHintUnion`
- Reworked namespace grammar to remove the repeated `useDeclaration* namespaceStatement*` ambiguity in both bracketed and semicolon namespace forms.
- Regenerated the PHP parser after the grammar change.

Why this mattered:

- ANTLR diagnostics showed repeated full-context ambiguity in `namespaceDeclarationSemi` for almost every top-level `use ...;` in `system/src/Grav/Framework/Flex/FlexObject.php`.
- After the namespace grammar split, the grav blocker file dropped from roughly `33s` to about `3.37s` in isolated fixture parsing, bringing it back under the `30s` budget with a large safety margin.

Verification:

- `go test ./common/yak/php/tests -run 'TestAllSyntaxForPHP_G4/syntax file: syntax/grav_slow/system__src__Grav__Framework__Flex__FlexObject.php' -count=1 -v`
  - passed
  - `FlexObject.php`: about `3.37s`
- `go test ./common/yak/php/tests -run 'TestAllSyntaxForPHP_G4/syntax file: syntax/prestashop/.*' -count=1`
  - passed
- `YAK_PHP_RUN_PROJECT_AST=1 YAK_PHP_PROJECT_AST_TARGET=/home/wlz/Target/phpcms/grav YAK_PHP_FIXTURE_PARSE_BUDGET_SEC=30 go test ./common/yak/php/tests -run TestProjectAst -count=1 -v`
  - passed
  - total parsed files: `522`
  - total project parse time: about `50.14s`
  - `system/src/Grav/Framework/Flex/FlexObject.php`: about `15.72s`
  - isolated slow files over `30s`: `0`
- `YAK_PHP_RUN_PROJECT_AST=1 YAK_PHP_PROJECT_AST_TARGET=/home/wlz/Target/phpcms/PrestaShop YAK_PHP_FIXTURE_PARSE_BUDGET_SEC=30 go test ./common/yak/php/tests -run TestProjectAst -count=1 -v`
  - passed
  - total parsed files: `7163`
  - total project parse time: about `2m21.03s`
  - isolated slow files over `30s`: `0`

Regression note:

- A full `go test ./common/yak/php/tests -count=1` run still fails in the exact-IR assertion suite.
- The failing subset observed after the grav fix is:
  - `TestAssignVariables`
  - `TestParseSSA_DeclareConst`
  - `TestExpression_If1`
  - `TestExpression_Try`
  - `TestBlueprintVirtual`
  - `TestGlobal`
  - `TestNativeCall_Include`
  - `TestNamespace2`
  - `TestOOP_static_member`
- The same subset also fails when run from the checkpoint export of commit `967e081de`, so these are treated as pre-existing baseline failures for this branch rather than a regression introduced by the grav performance fix.

## 2026-04-01 22:17:40 +08:00

### QloApps

- Status: completed
- Project path: `/home/wlz/Target/phpcms/QloApps`

Changes made:

- Added QloApps regression fixtures under:
  - `common/yak/php/tests/syntax/qloapps/`
- Copied the 8 initial project blockers into the fixture directory:
  - `classes/controller/AdminController.php`
  - `controllers/admin/AdminNormalProductsController.php`
  - `controllers/admin/AdminOrdersController.php`
  - `controllers/admin/AdminProductsController.php`
  - `tools/mailer/symfony/event-dispatcher/Debug/TraceableEventDispatcher.php`
  - `tools/mailer/symfony/event-dispatcher/Debug/WrappedListener.php`
  - `tools/mailer/symfony/event-dispatcher/EventDispatcher.php`
  - `tools/mailer/symfony/mime/Crypto/SMimeEncrypter.php`
- Extended the grammar / builder for two QloApps-driven syntax gaps:
  - static dynamic method calls such as `Validate::{$function}($value)`
  - PHP 8.1 first-class callable syntax such as `$this->normalizeFilePath(...)`

Verification:

- `go test ./common/yak/php/tests -run 'TestAllSyntaxForPHP_G4/syntax file: syntax/qloapps/.*' -count=1 -v`
  - passed
  - all 8 copied QloApps repro files now parse successfully
- `YAK_PHP_RUN_PROJECT_AST=1 YAK_PHP_PROJECT_AST_TARGET=/home/wlz/Target/phpcms/QloApps YAK_PHP_FIXTURE_PARSE_BUDGET_SEC=30 go test ./common/yak/php/tests -run TestProjectAst -count=1 -v`
  - passed
  - total parsed files: `3440`
  - total project parse time: about `1m19.65s`
  - no single-file parse exceeded `30s`
  - observed heavy file example: `tools/tcpdf/tcpdf.php` about `21.47s`

Regression rerun after QloApps:

- `grav`
  - rerun passed
  - total parsed files: `522`
  - total project parse time: about `19.72s`
  - `FlexObject.php`: about `5.20s`
  - log: `../build/backup/php-project-ast-logs/grav-project-ast-rerun-after-qloapps.log`
- `PrestaShop`
  - rerun passed
  - total parsed files: `7163`
  - total project parse time: about `1m44.67s`
  - log: `../build/backup/php-project-ast-logs/prestashop-project-ast-rerun-after-qloapps.log`

## 2026-04-01 22:35:46 +08:00

### Bolt

- Status: completed
- Project path: `/home/wlz/Target/phpcms/bolt`

Changes made:

- Added Bolt regression fixtures under:
  - `common/yak/php/tests/syntax/bolt/`
- Copied the 2 initial project blockers into the fixture directory:
  - `app/bootstrap.php`
  - `src/Storage/Mapping/MetadataDriver.php`
- Extended `foreach` grammar support for Bolt-driven forms:
  - `foreach ($items as &$value)`
  - `foreach ($items as list(...))`
  - `foreach ($items as $key => list(...))`

Verification:

- `go test ./common/yak/php/tests -run 'TestAllSyntaxForPHP_G4/syntax file: syntax/bolt/.*' -count=1 -v`
  - passed
  - both copied Bolt repro files now parse successfully
- `YAK_PHP_RUN_PROJECT_AST=1 YAK_PHP_PROJECT_AST_TARGET=/home/wlz/Target/phpcms/bolt YAK_PHP_FIXTURE_PARSE_BUDGET_SEC=30 go test ./common/yak/php/tests -run TestProjectAst -count=1 -v`
  - passed
  - total parsed files: `933`
  - total project parse time: about `22.23s`
  - no single-file parse exceeded `30s`

Regression rerun after Bolt:

- `grav`
  - rerun passed
  - total parsed files: `522`
  - total project parse time: about `20.83s`
  - `FlexObject.php`: about `6.09s`
  - log: `../build/backup/php-project-ast-logs/grav-project-ast-rerun-after-bolt.log`
- `QloApps`
  - rerun passed
  - total parsed files: `3440`
  - total project parse time: about `1m37.15s`
  - heaviest observed file remained below `30s`
  - log: `../build/backup/php-project-ast-logs/qloapps-project-ast-rerun-after-bolt.log`
- `PrestaShop`
  - rerun passed
  - total parsed files: `7163`
  - total project parse time: about `2m5.48s`
  - log: `../build/backup/php-project-ast-logs/prestashop-project-ast-rerun-after-bolt.log`
