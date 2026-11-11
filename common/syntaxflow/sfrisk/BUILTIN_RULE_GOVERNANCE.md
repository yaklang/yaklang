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

## Mode contract

| Mode | Use it for | Do not use it for |
| --- | --- | --- |
| `source` | A fixed-form match whose hit is itself the finding: vendor credential formats, `verify=False`, `privileged: true`, `acl = "public-read"`, compatibility hints. | API/class/method names, call points, anything that needs type resolution. |
| `struct` | Syntax-level call/type/structure matching on the compile unit without dataflow: `ProcessBuilder(* as $cmd)`, `ObjectInputStream(* as $in)`, `eval(* as $code)`, `TypeNameHandling.All`. | Any query that needs to know where an argument came from. |
| `ssa` | Dataflow from a request source to a sink, across functions and branches. | Sink-only matches and literal/API-shape checks; those belong to `struct`. |

A call-point rule in `source` mode is a classification error even when its regex is
specific: the scanner would report the name without resolving the symbol. Convert
it to `struct` (or drop it when an equivalent struct rule already exists).

## Naming

`desc(mode)` is the single source of truth for the verification path. The file
name says what the rule looks for, not how it runs.

| Element | Rule | Example |
| --- | --- | --- |
| File name | `<language>-<behavior>.sf` | `python-eval.sf`, `go-path-traversal-read.sf` |
| File name (mode collision) | when a `source` and a `struct` rule share the same language and behavior, keep the mode prefix on both files of that directory | `source-js-eval.sf`, `struct-js-eval.sf` |
| `title` / `title_zh` | the rule's own wording; a new rule does not add a mode marker | `Go 路径穿越：请求参数拼接文件路径` |
| `level` | one of `critical`, `high`, `middle`, `low`, `info`; quoted; never `mid`/`medium`/`warning` | `level: "middle"` |
| CWE directory | lowercase `cwe-<id>-<slug>` | `cwe-295-certificate-validation` |

Changing a rule's `mode` does not rename its file unless the directory also holds a
rule of the other mode with the same behavior. `TestBuiltinRuleTitlesAreUnique`
rejects duplicate titles, so keep the wording unique. Existing titles are kept as
they are even when they still carry an old `源码匹配：` / `结构匹配：` / `数据流：`
marker.

## File filters

`${...}` selects the files a `source` rule reads.

| Filter | Use it for |
| --- | --- |
| `${*}` | credential and token shapes only, because a secret can sit in `.env`, `.json`, `.yaml` or a log |
| `${*.py}` / `${/\.(js|mjs|cjs)$/}` | code and config patterns; name every extension the pattern can appear in |
| `${/^(?:Dockerfile(?:\.[\w.-]+)?|[\w.-]+\.dockerfile)$/}` | tool-specific files |

Documentation never reaches a rule: the project walk (compile and live source
stage) drops documentation directories (`doc`, `docs`, `documentation`) and
documentation entry files (`README*`, `CHANGELOG*`, `LICENSE*`, `CONTRIBUTING*`,
and the rest of the list in `ssaconfig.IsDocumentationFilePath`). Ordinary
markdown stays in the pool, so document rules that target `SKILL.md` still work.

`${*}` means code and configuration, not manifests. A blanket filter skips
dependency manifests (`package.json`, `pom.xml`, `Cargo.toml`, `*.csproj`, and
the rest of `sfpattern`'s manifest list): a `password` key in `package.json`
names a dependency, it is not a credential finding. A rule that really reads a
manifest lists it (`${*package.json}`), and that filter still matches.

Do not use `${*}` for API-shape or code-shape matches: it reports the same line from
config, logs and license text. Do not keep a single-extension filter when sibling
extensions carry the same syntax (`*.js` misses `.mjs`, `.cjs` and `.jsx`; `*.c`
misses headers; `*.yml` misses `.yaml`).

## Severity by mode

Severity answers how much proof the detector carries, not how damaging the CWE
would be if the code were reachable. Each mode uses its own scale.

| Mode | `high` / `critical` | `middle` | `low` / `info` |
| --- | --- | --- | --- |
| `source` | live credential format, private key block, literal that disables a control (`verify=False`, `InsecureSkipVerify: true`, `privileged: true`) | fixed-form dangerous pattern without enough context (`csrf_exempt`, `debug=True`, `echo $_GET`) | compatibility hints and style checks |
| `struct` | code/command execution, deserialization, TLS/JWT verification disabled, dynamic class/assembly loading, hardcoded key/password | injection sinks that still need attacker input (SQL, XXE, XPath, LDAP, SSRF, path, XSS), weak crypto, reflection, file operations | cookie flags, debug leftovers, CORS, hardening gaps |
| `ssa` | proven source-to-sink flow with no guard on the path | guarded, partial, or audit-only flow; rule whose fixture does not reproduce the finding | informational traces |

`type: audit` rules never carry `high`/`critical`: they flag an API or structure
for a human reviewer and cannot prove exploitability on their own. Cap them at
`middle` (or `low` when the pattern is only a hint) and give them a positive
fixture only.

`high` and `critical` rules must embed at least one positive fixture and one
near-miss negative fixture. `applyHighSeverityVerifyDefaults` forces POS+NEG
verification for every rule that can emit high/critical, and
`TestBuiltinHighSeverityRules_HaveFixtures` fails the corpus when such a rule has
no fixture. A rule that cannot pass both sides is downgraded to middle until the
fixtures and the query agree. Library rules (`AllowIncluded`) define sinks and
are verified through their importers.

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
