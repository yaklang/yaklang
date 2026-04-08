# pfSense Deferred Large Fixtures

- `filter.inc`
  - Current status: AST is correct, but the default syntax suite still pushes isolated parse to about `42.76s`.
- `interfaces.inc`
  - Current status: AST is correct, but the default syntax suite still pushes isolated parse to about `49.18s`.
- `services.inc`
  - Current status: AST is correct, but the default syntax suite still pushes isolated parse to about `32.46s`.
- `util.inc`
  - Current status: AST is correct, but the default syntax suite still pushes isolated parse to about `34.68s`.

Follow-up:

- These fixtures are all far above the `20KB` large-fixture threshold, ranging from about `176KB` to `242KB`.
- Keep them out of the always-on `30s` default suite for now and continue validating them through the deferred `large/` AST and build passes.
