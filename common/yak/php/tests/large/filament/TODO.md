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
- `packages__actions__src__Action.php`
  - Current status: AST is correct, but isolated parse is still about `57.03s`.
- `packages__actions__src__ActionGroup.php`
  - Current status: AST is correct, but isolated parse is still about `47.16s`.
- `packages__actions__src__Concerns__InteractsWithActions.php`
  - Current status: AST is correct, but isolated parse is still about `47.04s`.
- `packages__forms__src__Components__Builder.php`
  - Current status: AST is correct, but isolated parse is still about `32.96s`.
- `packages__forms__src__Components__Concerns__CanBeValidated.php`
  - Current status: AST is correct, but isolated parse is still about `37.10s`.
- `packages__forms__src__Components__ModalTableSelect.php`
  - Current status: AST is correct, but isolated parse is still about `40.30s`.
- `packages__infolists__src__Components__TextEntry.php`
  - Current status: AST is correct, but isolated parse is still about `32.22s`.
- `packages__panels__src__Commands__MakePageCommand.php`
  - Current status: AST is correct, but isolated parse is still about `59.74s`.
- `packages__panels__src__Commands__MakeRelationManagerCommand.php`
  - Current status: AST is correct, but isolated parse is still about `30.63s`.
- `packages__tables__src__Columns__SelectColumn.php`
  - Current status: AST is correct, but isolated parse is still about `54.68s`.
- `tests__src__Tables__Filters__SelectFilterTest.php`
  - Current status: AST is correct, but isolated parse is still about `40.22s`.

Follow-up:

- Keep these files out of the default `syntax/filament_slow` 30s suite for now.
- The following Filament files are still over budget but must stay in the normal project path because they are below `20KB`:
  - `packages/panels/src/Auth/Pages/EditProfile.php`
  - `packages/support/src/SupportServiceProvider.php`
  - `packages/widgets/src/Commands/MakeWidgetCommand.php`
  - `packages/support/src/helpers.php`
- Revisit parser/runtime profiling later and move them back once they can stay within the normal budget.
