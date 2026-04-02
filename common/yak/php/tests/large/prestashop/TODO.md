# PrestaShop Deferred Large Fixtures

- `classes/controller/AdminController.php`
  - Current status: AST is correct, but with the current CMS-compatible grammar it remains slightly above the 30s syntax budget.
  - Why deferred: narrowing CMS `expr::member` / `app(...)[...] = ...` support kept `PrestaShop` stable overall, but this one file still hovers around 32s.
  - Follow-up: profile the remaining `phpBlock` / top-level `use` ambiguity and class-constant initializer hotspots without widening CMS support again.
