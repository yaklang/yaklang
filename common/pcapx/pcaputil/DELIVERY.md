# PR #5013 first-batch T18–T21 delivery

This records the implemented profile boundaries and local evidence for the
SNMP, Syslog, media and enterprise protocol work. The companion acceptance
report in Downloads records the exact pushed commit, GitHub run IDs and final
check states; this file deliberately does not treat an earlier branch SHA as
the final PR head.

## Scope delivered

- **T18 SNMP:** v1/v2c community PDUs and bounded v3 USM/ScopedPDU metadata;
  request/response direction and ID correlation; Get/GetNext/GetBulk/Set,
  Trap/Inform/Report, OID and typed VarBind values. Encrypted scoped PDUs are
  identified as opaque and are not presented as decrypted or authenticated.
- **T19 Syslog:** RFC 5424 and RFC 3164 message fields over UDP and TCP, with
  octet-counted framing and LF framing, including embedded LF within a
  counted message. Explicit decode-as remains separate from heuristic
  admission.
- **T20 media:** SIP transaction and forked-dialog state, SDP offer/answer
  evidence, RTP/RTCP sequencing and reports, STUN/TURN method validation,
  RTSP interleaved framing and SETUP channel mapping, and UDP RTP association
  with SIP/RTSP signaling evidence.
- **T21 enterprise:** bounded SMB2 compound/session/tree/file operations,
  SMB3 transform envelope recognition, LDAP operations/StartTLS boundaries,
  Kerberos v5 UDP and TCP record framing, and DCE/RPC connection-oriented v5
  Bind/BindAck/context/call/fragment/Fault handling.

The profile/version matrix, event evidence model, fixture provenance, and
per-protocol limits are documented in [PROTOCOL_SESSION_S29.md](PROTOCOL_SESSION_S29.md)
and in the checked-in sample manifests. Live packet evidence is explicitly
distinguished from upstream corpus and deterministic synthetic sessions.

## Passed

All required top-level acceptance tests were present and passed with no skip:

| Required test | Passing test nodes | Skipped | Failed |
| --- | ---: | ---: | ---: |
| `TestFirstBatchT18` | 27 | 0 | 0 |
| `TestFirstBatchT19` | 34 | 0 | 0 |
| `TestFirstBatchT20` | 52 | 0 | 0 |
| `TestFirstBatchT21` | 36 (34 `pcaputil` + 2 `stream_parser`) | 0 | 0 |

Across both packages the four required suites produced 149 passing test
nodes.

Commands and results:

- `go test ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser -list '^TestFirstBatchT(18|19|20|21)$'` — all required entry points listed.
- `go test -json ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser -count=1 -timeout=20m` — passed. The taskbook checker reported all four required tests present, with no skips or failures.
- `go test -race ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser -count=1 -timeout=25m` — both packages passed.
- `go vet ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser` — passed.
- `go test ./common/bin-parser/rules -count=1 -timeout=3m` — passed after regenerating the DCE/RPC rule archive.
- `git diff --check` — passed.
- Bounded fuzzing, 30 seconds per target, passed: `FuzzSNMPBoundedBER` (306,430 executions), `FuzzSyslogMessageBounded` (241,332), `FuzzProtocolSessionSIPMediaStateSequence` (16,930), and `FuzzRound5ProtocolBoundaries` (176,278). `FuzzLiveProtocolSessions` also passed (49,702 executions); its current seeds primarily cover existing HTTP/2 and MySQL paths, so it is not counted as T18–T21-specific coverage.
- Capture replay benchmark passed for eight sample profiles, three decode modes and worker counts 1/2/4, with 100 repetitions per combination (`GOMAXPROCS=8`, Go 1.22.12, macOS arm64 / Apple M1 Max). It asserts the expected protocol events exist; event counts remained stable across the compared modes.
- The sample/oracle manifest hash and size check validated 22 referenced artifacts with zero mismatch.

The macOS linker printed an `LC_DYSYMTAB` warning while linking both race-test
binaries; both test processes exited successfully.

Test JSON, race output, per-target fuzz logs, and the raw benchmark output are
included in the companion evidence bundle. The exact final PR head is checked
separately by GitHub CI; a green prior head is not reused as evidence for a
later push.

## Performance interpretation

The benchmark compares full decode, deferred event capture, and deferred
on-demand field projection on the current implementation. It is not a
before/after speedup claim because these profile implementations did not have
an equivalent supported baseline. On one worker, deferred capture reduced
median allocations on the SIP/RTP replay from 16,290 to 14,161 and bytes/op
from about 2.43 MB to 1.70 MB; the live SNMP replay from 1,687 to 1,539 and
417 KB to 386 KB; and the Kerberos replay from 18,142 to 1,577 and 2.20 MB to
394 KB. LDAP stayed near 4,140 allocations and 588 KB in both full and
deferred capture; the small SMB2 replay also showed no material allocation
reduction. Deferred on-demand decoding restores most full-field materialization
costs, as intended.

These are capture-replay allocations and latency on a single machine. CPU
profiles, heap profiles, process RSS, production-rate throughput and a
same-semantics before/after baseline were not measured; no broader throughput
gain is claimed. Increasing replay workers to 2/4 did not establish a
single-stream speedup and increased aggregate bytes/op in these short captures.

## Not run in this environment

- Full-repository Essential Tests and Diff-Code-Check are GitHub checks, not
  local package tests. Their final exact-head states and run IDs are in the
  companion acceptance report.
- Live servers were not recaptured during this validation pass. The checked-in
  live pcaps and independent rsyslog/TShark oracles were hashed and validated;
  their original server/client versions and capture methods are in the
  manifests.
- The benchmark did not collect CPU/heap/RSS profiles or run a long-duration
  throughput/load test.

## Out of profile / explicitly unsupported

- SNMPv3 authentication digests are observed metadata, not cryptographically
  verified; privacy-protected ScopedPDU is opaque without keys.
- TLS application data, LDAP SASL credentials and decrypted StartTLS traffic
  are not inferred from encrypted bytes. LDAP has no identity/authentication
  verification claim.
- SIP IMS-specific extensions and SRTP decryption are unsupported; SDP media
  mappings are observed signaling evidence, not proof of authenticated media
  identity.
- SMB Direct, SMB3 encrypted body decryption, credential validation and share
  authorization are unsupported; the Samba capture is guest traffic.
- Kerberos tickets/checksums are not decrypted or validated, and realm or
  principal strings are observations rather than verified identities.
- DCE/RPC unknown stubs remain opaque; general NDR schemas, auth trailers,
  AlterContext reuse and RPC application identity are unsupported. The full
  BindAck/call capture is deterministic generated data, not live server
  interoperability evidence.
