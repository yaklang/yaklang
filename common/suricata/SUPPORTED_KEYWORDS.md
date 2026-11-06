# Suricata rule compatibility

The matcher targets Suricata 8 rule syntax while preserving the legacy underscore aliases already used by yaklang rules. Syntax acceptance is not treated as execution support. `rule.KeywordCapabilities()` is the source of truth and `rule.ValidateRule` reports every constraint that cannot be enforced.

## Execution policy

- `match.CompileRule` and `Group.LoadRuleWithError` are fail-closed: unsupported or stateful-only semantics reject the rule.
- `rule.Parse` remains tolerant for compatibility and migration tooling. A malformed physical rule line with unbalanced quotes is isolated so it cannot swallow the following rules.
- `rule.ParseStrictRule` rejects lexer, parser, and visitor errors.
- `match.New` remains a compatibility wrapper; new rule-loading code should use `CompileRule`.

## Supported now

- Header: `alert`, `->`, `<>`, IPv4/IPv6/CIDR/list/negation variables, ports/ranges/lists/variables.
- Protocols: IP, TCP, `tcp-pkt`, UDP, ICMPv4, ICMPv6, DNS, HTTP, TLS (including JA3/JA3S and certificate fields), and TCP-payload compatibility protocols SMTP, FTP, FTP-data, SMB, SSH, DCERPC and `tcp-stream`.
- Payload: `content`, negative content (`content:!"..."`), negative PCRE, `nocase`, `offset`, `depth`, `distance`, `within`, `startswith`, `endswith`, `isdataat`, `fast_pattern`, `bsize`, `dsize`, `rawbytes`, `dotprefix`, `byte_test`, `byte_jump`, `byte_extract` variables, `base64_decode`/`base64_data`, `file_data`/`file.data` (HTTP body, TCP payload or SMTP DATA body), and the 8-bit PCRE2 backend (including lookarounds and backreferences, `/B` raw packet bytes, `/G` ungreedy and `/O` match limits).
- IPv4/IPv6 packet fields: `ttl` (IPv6 Hop Limit), `sameip`, `ip_proto`; IPv4-only `id`, `fragbits`, `fragoffset`, `tos`, `ipopts`.
- Header sticky buffers: `ipv4.hdr`/`ipv4_hdr`, `ipv6.hdr`/`ipv6_hdr`, `tcp.hdr`/`tcp_hdr`, `udp.hdr`/`udp_hdr`, `icmpv4.hdr`/`icmpv4_hdr` and `icmpv6.hdr`/`icmpv6_hdr`.
- TCP: `flags` including `!`, `*`, `+`, ignored-mask syntax, `seq`, `ack`, `window`, and `tcp.mss`.
- ICMP: `itype`, `icode`, `icmp_id`, `icmp_seq` (ICMPv4), and `itype`/`icode` for ICMPv6.
- DNS: `dns.query`, `dns.opcode`, `dns.rcode`, `dns.rrtype`.
- HTTP: the registered request/response buffers, including legacy aliases, `http.accept_lang`, `app-layer-protocol:http`, and the `http.invalid_transfer_encoding_value_in_request` application event.
- TLS: `tls.sni`, `ja3.hash`, `ja3.string`, `ja3s.hash`, `ja3s.string`, `tls.cert_subject`, `tls.cert_issuer`, `tls.cert_serial`, `tls.cert_fingerprint`, `tls.certs`, and `tls.version`.
- SSH/FTP: the `ssh.proto`/`ssh_proto` version-banner buffer and `ftpbounce` source-address comparison.
- ASN.1: `asn1` with `oversize_length`, `bitstring_overflow`, `double_overflow`, `absolute_offset` and `relative_offset` over recursively decoded BER objects.
- Metadata: `sid`, `rev`, `gid`, `msg`, `classtype`, `reference`, `priority`, `metadata`, and `target` (`src_ip`/`dest_ip`, alert metadata only).

## Unified engine

`engine.StatefulEngine` is the primary API. It embeds the stateless matcher per rule and adds flow state, alerts, capture binding and pooling:

- Stateless rules without flow or payload constraints run on the packet view.
- Stateless TCP rules can also run on the ordered stream view.
- `tcp-pkt` rules are packet-scoped by definition; they never use stream view even when a flow is present.
- Stateful TCP rules add `flow`, `flowbits`, `threshold` and FIN/RST/generation cleanup.
- Stateful HTTP rules run on the ordered TCP stream, including split requests and responses; HTTP buffers are parsed from the reassembled payload. The engine waits for complete headers and a declared `Content-Length`/chunked body, and advances a per-direction message cursor so keep-alive requests are matched one by one.
- Stateful TLS rules run on the ordered TCP stream so ClientHello, ServerHello and Certificate records split across segments are still parsed. SNI, JA3/JA3S and certificate fields are matched from the direction stream; handshake matches are deduplicated per rule and direction.
- Stateful UDP/DNS rules run on a bounded UDP pseudo-flow: first observed direction is client-to-server, `established` requires datagrams in both directions, and idle flows are evicted. A reused 5-tuple gets a new generation after eviction, so flowbits/threshold state does not leak.
- `MatchPacket`, `MatchRaw`, `MatchStream` and `MatchHTTPFlow` expose the matcher's direct evaluation API on the engine.
- `engine.OpenPcapFile`/`StartSniff` and `pcapx.pcap_suricata(engine)` bind the engine without a session object.

The engine executes:

- `flow:to_server/to_client,established/not_established,stateless,only_stream/no_stream` (TCP-based protocols including HTTP, TLS, SMTP, FTP, SMB, SSH and DCERPC; `tcp-pkt` packet-only; UDP/DNS except `only_stream`; IP directional flow remains unsupported)
- `flowbits:set/unset/toggle/isset/isnotset/noalert` (TCP-based protocols, `tcp-pkt`, UDP/DNS; IP rules when the packet has a TCP/UDP flow)
- `threshold` (`limit`, `threshold`, `both`) and `detection_filter` (TCP-based protocols, `tcp-pkt`, UDP/DNS, and IP/ICMP/ICMPv6 host/pair thresholds)
- `xbits:set/unset/toggle/isset/isnotset` with `ip_src`/`ip_dst`/`ip_pair` scope and `expire`
- `flowint` set/add/subtract and comparisons (`==`, `!=`, `<`, `>`, `<=`, `>=`, `isset`, `isnotset`)
- `tag:session,<count>,packets|seconds` tagged alerts on the same flow
- `stream_size:client|server|both|either` (TCP/HTTP/TLS stream view and `tcp-pkt` packet counters)
- `target:src_ip`/`target:dest_ip` on alerts
- `rawbytes` packet-header matching and TCP rules with TLS sticky buffers
- capture-time windows, TCP FIN/RST cleanup, UDP idle cleanup, and generation isolation
- packet/stream separation, retransmission deduplication, bounded stream storage and fail-closed diagnostics

The bundled ET scan corpus has 284 rules. The legacy stateless matcher executes 28; the engine executes all 284 and accepts every rule the matcher accepts. All 55 TCP rules compile statefully.

The ET Pro corpus used for the latest audit has 69,451 active alert rules. The parser recovers all 69,451 rules and the engine compiles all 69,451 (100%). Four archived `.rules` files contain only comments (license text, deleted-rule history, or disabled examples); they are accepted as empty rule sets rather than parse failures.

HTTP stateful matching uses the TCP stream and therefore expects a visible handshake for `flow:established`; mid-stream captures can use `flow:stateless`/`flow:no_stream` rules. Responses without `Content-Length` or chunked framing are matched at header completion; HTTP/2 and HTTP/3 transaction buffers remain unsupported.

`alert tcp`/`tcp-pkt` rules that select HTTP sticky buffers are automatically evaluated through the HTTP parser and message cursor; they are not limited to the literal `http` header protocol. SMTP rules with HTTP/file buffers still require an SMTP MIME/file transaction layer and remain fail-closed.

`app-layer-protocol` currently recognizes HTTP negotiation only; other protocol names fail closed. `app-layer-event` currently evaluates `http.invalid_transfer_encoding_value_in_request` using libhtp's rule (the value must be exactly `chunked`, and chunked framing requires HTTP/1.1 or newer). Other application events remain unsupported.

`file_data`/`file.data` use the current HTTP request or response body when HTTP parsing is active, the TCP payload for plain `tcp`/`tcp-pkt` rules, or the SMTP DATA body for `smtp` rules. Chunked HTTP bodies are decoded before matching; gzip and MIME transfer encodings are not decompressed. This is a payload/body approximation: multipart HTTP files and MIME attachments are not exposed as individual decoded parts, and FTP/SMB file transactions remain unsupported.

TLS stateful matching also expects a visible handshake and complete TLS records in the reassembled direction stream. `tls.certs` currently exposes the leaf certificate DER rather than iterating every certificate in the chain, and TLS handshake matches are emitted once per rule and direction. `tls.cert_chain_len`, `tls.cert_expired` and `tls.cert_valid` remain unsupported.

`dotprefix` is implemented as a sticky-buffer transform for `dns.query`, TLS buffers, and HTTP host buffers. It prepends a literal `.` before content matching, preserving label boundaries without matching `notexample.com`. Transforms apply only to the buffer that follows them; switching to a different sticky buffer clears the transform.

`byte_test` evaluates extracted values with Suricata operators (`=`, `!=`, `<`, `>`, `<=`, `>=`, `&`, `^`, and their `!` forms), relative offsets, big/little endian, string bases, and bitmask shifting. `byte_jump` moves the detection pointer using relative/from_beginning/from_end modes, multiplier, align, post_offset, string extraction, and bounds checks; subsequent content and PCRE matches start from the jumped position. `byte_extract` stores extracted values in per-match variables; those variables can drive `byte_test`/`byte_jump` width or values and the `offset`/`depth`/`distance`/`within` bounds.

`base64_decode` decodes at most `bytes` input characters from the current buffer, optionally after an absolute or `relative` offset; decoding stops at the first non-base64 character. `base64_data` exposes the decoded bytes as a sticky buffer for subsequent content. Base64 options are parsed fail-closed, and `fast_pattern` is evaluated against the decoded buffer when present.

PCRE parsing normalizes escaped rule delimiters (`\/`) and preserves the rule-level backslashes that ET uses to match literal backslash sequences. Quotes inside a PCRE expression are escaped only for the grammar and restored before compilation. Matching is byte-oriented PCRE2, so `\xff` matches the raw `0xff` byte rather than a UTF-8 code point. `/G` enables ungreedy matching, `/x` enables extended mode, `/B` selects raw packet bytes, `/E` uses `PCRE2_DOLLAR_ENDONLY`, and `/O` accepts Suricata's match-limit marker while the backend always enforces configured match/depth limits. Numeric ranges accept whitespace after operators such as `dsize:< 32`.

`stream_size` is enforced against reassembled per-direction TCP byte counts for stream rules and against sequence-deduplicated packet byte counters for `tcp-pkt` rules. `xbits` are stored in the bounded state store with capture-time expiry and host/pair scoping.

IP rules evaluate the network header and IP payload; transport-less packets such as ICMP are published through the normalized lane. IP rules without a transport flow support threshold counters scoped to source/destination hosts. Flowbits on IP rules apply when the packet belongs to a TCP/UDP flow; transport-less flowbits fail closed with a diagnostic.

SMTP, FTP, FTP-data, SMB, SSH, DCERPC and `tcp-stream` are executed over the TCP payload/stream compatibility lane. This covers their common content/flow/threshold rules. SMTP `file_data` exposes the raw DATA body; FTP/SMB file transactions and their protocol-specific buffers remain fail-closed. TCP rules that select TLS certificate/SNI/JA3 buffers use the TLS parser even when the header protocol is `tcp`.

`ssh.proto`/`ssh_proto` exposes the SSH version-banner line (for example `SSH-2.0-PUTTY`) as a sticky buffer; content can match either the protocol or software portion of that banner. `ftpbounce` parses `PORT` command arguments and alerts when the requested IPv4 address differs from the FTP client's source address.

`asn1` recursively decodes BER objects from the packet payload and applies `oversize_length`, `bitstring_overflow` and `double_overflow` with Suricata's semantics. `absolute_offset` selects a fixed payload offset; `relative_offset` is measured from the end of each candidate match of the preceding content rule. Decoding is bounded to 30 top-level frames and stops at the first malformed object, retaining objects decoded before the error.

`flowint` effects are committed atomically with flowbits inside the same flow-state transaction; comparison and existence predicates run before effects. `tag:session` records a per-flow tag on the alerting rule and emits tagged alerts for subsequent normalized packets, bounded by packet count or capture-time seconds; tagged alerts carry the original rule identity and `Tagged=true`.

The AST and bounded state store already model `flow.age`, `flow.pkts`, and `flow.bytes`, but the stateful compiler deliberately rejects these until their runtime predicates/effects are connected. Protocol transactions are emitted as owned `ProtocolEvent` inspection events; incomplete/malformed/resource-limited events produce diagnostics, while transaction-backed application rules remain a later milestone.

`match.Matcher`/`match.Group` and the yak `NewSuricataMatcher`/`NewSuricataMatcherGroup` exports remain as compatibility APIs. They are stateless and reject stateful-required options; new code should use `StatefulEngine`.

## Recognized but unsupported

Major families include:

- `geoip`, reputation, Lua, datasets/datarep
- `byte_math`
- other buffer transforms
- file hashes/magic/name and protocol-specific file transactions beyond the HTTP body approximation
- `replace`, RPC, and application-layer events other than `http.invalid_transfer_encoding_value_in_request`
- `app-layer-protocol` values other than `http`
- TLS certificate-chain length and certificate validity/expiry fields
- SSH keywords other than `ssh.proto` and protocol-specific FTP/SMB buffers
- IPS side effects such as non-alert actions and `filestore`

Use `rule.KeywordCapabilities()` for the complete machine-readable list. Unknown keywords are reported as unsupported; they are never silently considered executable by `CompileRule`.
