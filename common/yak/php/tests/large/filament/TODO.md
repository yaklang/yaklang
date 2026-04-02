# Filament Deferred Large Fixtures

- `docs-assets__app__app__Livewire__TablesDemo.php`
  - Current status: AST is correct, but in the full default syntax suite it drifts just over the 30s budget at about 30.3s.
  - Why deferred: this file is close to the budget edge and stays stable only in isolated runs, so keeping it in the always-on suite causes flake-level regressions.
- `tests__src__Forms__Components__SelectTest.php`
  - Current status: AST is correct, but in the full default syntax suite it still lands just over the 30s budget at about 30.7s.
  - Why deferred: this file is another edge-budget Filament test that passes cleanly in isolation but is not stable enough for the default suite under full regression load.
- `tests__src__Tables__ColumnTest.php`
  - Current status: AST is correct, but in the full default syntax suite it still lands over the 30s budget at about 31.3s.
  - Why deferred: this file is stable in focused runs but not stable enough under the full default regression load, so it belongs with the other deferred-large Filament tests for now.
- `tests__src__Tables__Filters__QueryBuilderTest.php`
  - Current status: AST is correct, but standalone frontend parse is still about 1m24s.
  - Why deferred: this file is the dominant remaining Filament parser hotspot and needs dedicated profiling instead of blocking the 10-project regression loop.

Follow-up:

- Keep this file out of the default `syntax/filament_slow` 30s suite for now.
- Revisit parser/runtime profiling later and move them back once they can stay within the normal budget.
