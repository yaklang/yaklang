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

## Unified engine

`engine.StatefulEngine` is the primary API. It embeds the stateless matcher per rule and adds flow state, alerts, capture binding and pooling:

- Stateless TCP/HTTP/DNS/TLS/UDP/ICMP rules run on the packet view.
- Stateless TCP rules can also run on the ordered stream view.
- Stateful TCP rules add `flow`, `flowbits`, `threshold` and FIN/RST/generation cleanup.
- Stateful HTTP rules run on the ordered TCP stream, including split requests and responses; HTTP buffers are parsed from the reassembled payload. The engine waits for complete headers and a declared `Content-Length`/chunked body, and advances a per-direction message cursor so keep-alive requests are matched one by one.
- Stateful UDP/DNS rules run on a bounded UDP pseudo-flow: first observed direction is client-to-server, `established` requires datagrams in both directions, and idle flows are evicted. A reused 5-tuple gets a new generation after eviction, so flowbits/threshold state does not leak.
- `MatchPacket`, `MatchRaw`, `MatchStream` and `MatchHTTPFlow` expose the matcher's direct evaluation API on the engine.
- `engine.OpenPcapFile`/`StartSniff` and `pcapx.pcap_suricata(engine)` bind the engine without a session object.

The engine executes:

- `flow:to_server/to_client,established/not_established,stateless,only_stream/no_stream` (TCP/HTTP; UDP/DNS except `only_stream`)
- `flowbits:set/unset/toggle/isset/isnotset/noalert` (TCP/HTTP/UDP/DNS)
- `threshold` (`limit`, `threshold`, `both`) and `detection_filter` (TCP/HTTP/UDP/DNS)
- capture-time windows, TCP FIN/RST cleanup, UDP idle cleanup, and generation isolation
- packet/stream separation, retransmission deduplication, bounded stream storage and fail-closed diagnostics

The bundled ET scan corpus has 284 rules. The legacy stateless matcher executes 28; the engine executes 275 and accepts every rule the matcher accepts. Of the 55 TCP rules, 52 compile statefully; the three `byte_test` rules remain rejected. The remaining engine rejections are 2 SSH, 2 FTP, 1 SMTP and 1 `file_data` rule. UDP/DNS stateful rules are supported through the pseudo-flow; other non-TCP application protocols remain stateless until their transaction buffers are implemented.

HTTP stateful matching uses the TCP stream and therefore expects a visible handshake for `flow:established`; mid-stream captures can use `flow:stateless`/`flow:no_stream` rules. Responses without `Content-Length` or chunked framing are matched at header completion; HTTP/2 and HTTP/3 transaction buffers remain unsupported.

The AST and bounded state store already model `flowint`, `xbits`, `stream_size`, `flow.age`, `flow.pkts`, and `flow.bytes`, but M1 compilation deliberately rejects these until their runtime predicates/effects are connected. Protocol transactions are emitted as owned `ProtocolEvent` inspection events; incomplete/malformed/resource-limited events produce diagnostics, while transaction-backed application rules remain a later milestone.

`match.Matcher`/`match.Group` and the yak `NewSuricataMatcher`/`NewSuricataMatcherGroup` exports remain as compatibility APIs. They are stateless and reject stateful-required options; new code should use `StatefulEngine`.

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
