# Built-in rule risk type maintenance

`taxonomy.json` is the producer-owned vocabulary for SyntaxFlow risk types. A
type `name` is a stable machine key; `display_name` is Chinese product copy and
`aliases` contains reviewed historical spellings. Rule identity, alert identity,
severity, CWE mappings and scan-batch behavior are outside this contract.

## Authoring

Use an active lowercase kebab-case `name` from the registry for every built-in
`risk` or `risk_type` value. Do not write a translated label, legacy alias,
severity, CWE ID, or generic placeholder. When no existing key describes a new
detector, review its actual behavior, add a precise type and Chinese label, place
it in one product category, and bump the taxonomy version.

A rule-level type is the default inherited by its alerts. Mixed-type rules can
omit that default when each emitted alert declares a canonical type. Reusable
libraries may expose intermediate values without classifying them as findings.

```syntaxflow
desc(
    risk: "sql-injection",
    severity: "high",
)

alert $sink for {
    risk: "sql-injection",
}
```

## Semantic review

Canonicalization is based on the detector's behavior, not a global translation.
Keep command execution, command injection, code execution, and code injection
separate. Keep hardcoded passwords, credentials, and cryptographic keys separate.
Values such as `Moderate`, `中等`, `Security`, and `信息` require semantic review;
they must not be converted from spelling alone. Product categories are navigation
groups and do not replace CWE or OWASP mappings.

## Checks

`TestBuiltinRiskTypeTaxonomy` compiles every embedded rule and rejects unknown,
legacy, review-required, or missing effective risk types. Negative fixtures cover
rule defaults, alert overrides, mixed-alert rules, libraries, and last-assignment
semantics. The CI workflow regenerates gzip resources and runs the same gate in
both embedding modes. The parity test compares embedded paths and file bytes with
the checkout so a partial or stale archive cannot pass.

Run focused checks in an isolated worktree:

```sh
go test -mod=readonly ./common/syntaxflow/sfrisk ./common/syntaxflow/sfbuildin -run 'TestRiskType|TestBuiltinRisk' -count=1
```

Refresh `rule_versions.json` and embedded resources with the repository's existing
generation commands after changing rule source. Rule content hashes and versions
must reflect the real content. Historical results are not rewritten by this
governance rule.
