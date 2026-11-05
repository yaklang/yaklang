# Suricata rule compatibility

The matcher targets Suricata 8 rule syntax while preserving the legacy underscore aliases already used by yaklang rules. Syntax acceptance is not treated as execution support. `rule.KeywordCapabilities()` is the source of truth and `rule.ValidateRule` reports every constraint that cannot be enforced.

## Execution policy

- `match.CompileRule` and `Group.LoadRuleWithError` are fail-closed: unsupported or stateful-only semantics reject the rule.
- `rule.Parse` remains tolerant for compatibility and migration tooling.
- `rule.ParseStrictRule` rejects lexer, parser, and visitor errors.
- `match.New` remains a compatibility wrapper; new rule-loading code should use `CompileRule`.

## Supported now

- Header: `alert`, `->`, `<>`, IPv4/IPv6/CIDR/list/negation variables, ports/ranges/lists/variables.
- Protocols: TCP, UDP, ICMPv4, DNS, HTTP, and the existing partial TLS/JA3 buffers.
- Payload: `content`, negative content, `nocase`, `offset`, `depth`, `distance`, `within`, `startswith`, `endswith`, `isdataat`, `fast_pattern`, `bsize`, `dsize`, and the implemented PCRE subset.
- IPv4/IPv6 packet fields: `ttl` (IPv6 Hop Limit), `sameip`, `ip_proto`; IPv4-only `id`, `fragbits`, `fragoffset`, `tos`, `ipopts`.
- TCP: `flags` including `!`, `*`, `+`, ignored-mask syntax, `seq`, `ack`, `window`, and `tcp.mss`.
- ICMPv4: `itype`, `icode`, `icmp_id`, `icmp_seq`.
- DNS: `dns.query`, `dns.opcode`, `dns.rcode`, `dns.rrtype`.
- HTTP: the registered request/response buffers, including legacy aliases and `http.accept_lang`.
- Metadata: `sid`, `rev`, `gid`, `msg`, `classtype`, `reference`, `priority`, and `metadata`.

## Stateful TCP engine

`engine.StatefulEngine` and `engine.PcapAdapter` compose the public `pcaputil` callbacks without modifying the capture or protocol packages. TCP flow identity is capture domain plus reassembly generation; ordered bytes come from pcaputil's accepted stream callback, while header predicates use the associated real packet.

M1 executes:

- `flow:to_server/to_client,established/not_established,stateless,only_stream/no_stream`
- `flowbits:set/unset/toggle/isset/isnotset/noalert`
- `threshold` (`limit`, `threshold`, `both`) and `detection_filter`
- capture-time windows and FIN/RST/inactive/generation cleanup
- packet/stream separation, retransmission deduplication, bounded stream storage and fail-closed diagnostics

The bundled ET scan corpus currently has 55 TCP rules: 52 compile into the stateful engine; the remaining 3 require `byte_test` and remain rejected.

The AST and bounded state store already model `flowint`, `xbits`, `stream_size`, `flow.age`, `flow.pkts`, and `flow.bytes`, but M1 compilation deliberately rejects these until their runtime predicates/effects are connected. Protocol transactions are emitted as owned `ProtocolEvent` inspection events; incomplete/malformed/resource-limited events produce diagnostics, while application protocol rule providers remain a later milestone.

The stateless `match.CompileRule` and legacy `Group` still reject every stateful-required option. Callers must explicitly choose `engine.CompileStatefulRule` / `engine.NewStatefulEngine`.

## Recognized but unsupported

Major families include:

- `geoip`, reputation, Lua, datasets/datarep
- `byte_test`, `byte_math`, `byte_extract`, `byte_jump`
- transforms and base64 buffer pipelines
- general file transaction properties and hashes
- `replace`, RPC, application-layer events
- TLS certificate/SNI/JA3S fields that are not yet backed by extracted data
- SSH and other application protocol keyword families
- IPS side effects such as non-alert actions, `tag`, and `filestore`

Use `rule.KeywordCapabilities()` for the complete machine-readable list. Unknown keywords are reported as unsupported; they are never silently considered executable by `CompileRule`.
