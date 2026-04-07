# CMS Deferred Large Fixtures

- `src__Assets__Asset.php`
  - Current status: AST is correct, but isolated parse still lands around `1m04s`.
- `src__Fieldtypes__Bard.php`
  - Current status: AST is correct, but isolated parse still lands around `53.20s`.
- `src__Http__Controllers__CP__Collections__CollectionsController.php`
  - Current status: AST is correct, but isolated parse still lands around `47.27s`.
- `src__Modifiers__CoreModifiers.php`
  - Current status: AST is correct, but isolated parse still lands around `2m17.95s`.
- `src__Providers__AddonServiceProvider.php`
  - Current status: AST is correct, but isolated parse still lands around `38.13s`.
- `tests__Assets__AssetContainerTest.php`
  - Current status: AST is correct, but isolated parse still lands around `54.37s`.
- `tests__CP__Navigation__NavPreferencesTest.php`
  - Current status: AST is correct, but isolated parse still lands around `31.64s`.
- `tests__Data__Entries__CollectionTest.php`
  - Current status: AST is correct, but isolated parse still lands around `43.66s`.
- `tests__Data__Entries__EntryTest.php`
  - Current status: AST is correct, but isolated parse still lands around `3m27.70s`.
- `tests__Data__Taxonomies__TermQueryBuilderTest.php`
  - Current status: AST is correct, but isolated parse still lands around `32.01s`.
- `tests__FrontendTest.php`
  - Current status: AST is correct, but isolated parse still lands around `30.39s`.

Follow-up:

- Keep these files out of the normal `30s` project-budget path for now.
- `src__Fieldtypes__Entries.php` and `src__Fieldtypes__Terms.php` moved back to the normal project-budget path after the SLL->LL fallback reuse optimization; keep watching them in the main `cms` project run instead of `large/`.
- Revisit parser/runtime profiling later and move them back once they can stay within the standard budget again.
