# SyntaxFlow risk type display taxonomy

`taxonomy.json` is the producer-owned dictionary for built-in rule risk types.
It separates stable display-bucket names, Chinese labels, navigation categories,
and explicitly reviewed legacy aliases. It does not rewrite rule content,
`RiskType`, rule UUIDs, finding hashes, or execution snapshots.

The rule archive exports this dictionary in `meta.json.risk_type_taxonomy` with
schema `yaklang_risk_type_taxonomy.v1`. Consumers can group aliases for display
and query their original values. Older importers can ignore the additive field;
unknown custom types must retain their raw label and query value.

## Adding or maintaining a built-in type

1. Check the compiled rule-level type and every alert override. Do not infer a
   type from the filename, severity, language, CWE, or an arbitrary substring.
2. Reuse a reviewed canonical name for new rules when its meaning matches.
   Add a new type only when it expresses a distinct finding or audit purpose.
3. Use a lowercase kebab-case `name`, a concise Chinese `display_name`, and one
   product navigation category. Established abbreviations such as SQL, XSS,
   SSRF, and TLS can remain in the Chinese label.
4. Add an alias only after checking equivalent semantics. Command execution,
   command injection, code execution, and code injection remain distinct;
   hardcoded passwords, credentials, and cryptographic keys also remain distinct.
5. Keep existing raw values and aliases stable. Changing their persisted value
   needs a separate identity/baseline migration. Bump `version` when changing
   this dictionary; change `schema_version` only for a structural contract change.
6. Use `review_required` for known ambiguous or misplaced values. For example,
   legacy `Moderate` / `中等` are marked for review instead of inventing a
   vulnerability type or changing their severity.

## Validation

Run from an isolated Yaklang checkout. Use a task-owned `YAKIT_HOME` when tests
need local databases. The compiled-corpus check covers all nonempty rule types
and alert overrides; missing metadata is not filled by this display migration.

```bash
go test ./common/syntaxflow/sfrisk -count=1
go test ./common/schema -run '^TestSSARiskTaxonomyPreservesIdentity$' -count=1
go test ./common/syntaxflow/sfdb -run '^TestRuleExportEmbedsTaxonomyAndRemainsImportCompatible$' -count=1
go test ./common/syntaxflow/sfbuildin -run '^TestBuiltinRiskTypeTaxonomy$' -count=1
go run ./common/utils/gzip_embed/gzip-embed -cache --source ./common/syntaxflow/sfbuildin/buildin --gz ./common/syntaxflow/sfbuildin/buildin.tar.gz --no-embed
go test -tags gzip_embed ./common/syntaxflow/sfbuildin -run '^TestBuiltinRiskTypeTaxonomy$' -count=1
```

The essential-tests workflow includes the dictionary, export, and compiled
source-corpus packages. The gzip command regenerates the ignored archive from
the current source; an old local archive is not evidence of current coverage.

## Design references

- [CodeQL query metadata](https://codeql.github.com/docs/writing-codeql-queries/metadata-for-codeql-queries/): stable rule IDs and separate display names.
- [SonarQube rules](https://docs.sonarsource.com/sonarqube/latest/user-guide/rules): language, rule type, severity, and tags are separate facets.
- [CWE mapping guidance](https://cwe.mitre.org/documents/cwe_usage/mapping_navigation.html): product navigation categories do not substitute for precise weakness mappings.
