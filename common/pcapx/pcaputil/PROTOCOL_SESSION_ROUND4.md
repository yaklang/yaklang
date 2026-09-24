# Protocol session review, round 4 — 2026-09-18

Maintenance starts from PR #5013 head `62adbbd73f`, after the previous squash.
This round changes four existing profiles and the shared snapshot helper; it
adds no new protocol or completeness claim. The refreshed
[next ten candidates](PROTOCOL_SESSION_CANDIDATES.md) distinguish packet rules
from live sessions and identify concrete sample gaps.

## Fixes

- **SMTP:** outstanding commands now obey `MaxCollectionElements`. The FIFO
  clears consumed references and reuses storage across exchanges. Compaction
  preserves PIPELINING order. Close-time string-list snapshots are owned copies,
  with their string bytes included in history accounting.
- **IMAP:** `{n}` and `{n+}` now share separate header/literal framing, including
  zero-length literals and trailing CRLF. Bare `+` continuations are accepted.
  Oversized declarations fail before waiting for their payload. Pending tags are
  bounded, duplicate active tags cannot overwrite association, and retained tags
  and command names no longer keep entire command lines alive.
- **CoAP:** request MID and Token indexes include the initiating direction.
  Opposite endpoints may reuse identical IDs/Tokens without overwriting one
  another. ACK/RST and responses consult the opposite endpoint's index. Both
  indexes update in constant time, replacing linear scans on MID reuse/reset.
  Retransmissions/replacements remain allowed at the pending-request limit;
  options, including options not exported as fields, obey the collection budget.
- **Modbus TCP:** read responses must return the byte count implied by the
  matched request's quantity; write responses must echo address and value/count.
  Exceptions from the known client direction are rejected. Invalid replies do
  not consume pending requests. New transaction IDs obey the collection budget;
  same-direction retransmissions remain accepted.

All four profiles reserve conservative state capacity before admitting data;
IMAP includes retained string bytes. These are bounded accounting estimates,
not measurements of exact Go heap size. Existing capture-wide byte ceilings and
close-time release remain active.

## Evidence

`protocol_session_round4_test.go` covers pending-limit overflow and release via
public `ProtocolSession.Feed`, equal CoAP tokens/MIDs in both directions,
replacement/reset/index consistency and option bounds, IMAP full/deferred
0/1/7-byte splits for both literal forms and zero-length payloads, declaration
limits and duplicate tags, Modbus mismatched read/write responses and exception
direction, plus SMTP FIFO reuse/snapshot isolation.

The four core regression groups were also run against an isolated checkout of
`62adbbd73f`; pending budgets, bidirectional CoAP, IMAP literal framing and
Modbus response contracts each fail there and pass after these changes.

The complete pcaputil and CLI suites pass, including all eight original upstream
capture regressions from round 3. Their event/byte expectations and capture bytes
are unchanged. The full pcaputil race suite also passes.

Commands:

```sh
go test ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect -count=1 -timeout=5m
go test -race ./common/pcapx/pcaputil -count=1 -timeout=5m
go test ./common/pcapx/pcaputil -run '^$' -bench '^BenchmarkRound4SMTPCommandReply$' -benchmem -benchtime=500ms -count=3
```

On macOS arm64 / Go 1.22.12, the SMTP state-machine command/reply microbenchmark
changed from **764 B / 12 allocations** to **748 B / 11 allocations** per pair.
Three timing samples were 460.4/448.0/450.2 ns before and 470.7/418.4/430.6 ns
after. Timing ranges overlap; the reliable result is one fewer allocation, not
a claim about whole-capture throughput. Rule YAML and its generated archive are
unchanged. Final remote acceptance still requires the pushed head's complete
Essential Tests Gate and Diff-Code-Check.

## Remaining boundaries

SMTP AUTH/BDAT and general IMAP multi-literal command continuation/IDLE/extensions
remain incomplete. CoAP capture decoding is still per-datagram: this round fixes
association inside explicit caller sessions, not persistent UDP flow tracking.
Observe, blockwise aggregation, retransmission timers, OSCORE and full option
semantics remain outside the profile. Modbus functions beyond 1–6/15/16, RTU,
TLS and ambiguous response-only starts remain outside it. More production mail,
CoAP and multi-vendor industrial captures are needed; the new edge cases are
deterministic regression inputs, not newly acquired real captures.

References: [IMAP RFC 3501](https://www.rfc-editor.org/rfc/rfc3501.html#section-2.2),
[non-synchronizing literals RFC 7888](https://www.rfc-editor.org/rfc/rfc7888.html),
[CoAP response matching RFC 7252](https://www.rfc-editor.org/rfc/rfc7252.html#section-5.3.2),
[Modbus application protocol](https://www.modbus.org/file/secure/modbusprotocolspecification.pdf).

## CI scanner input repair

Diff-Code-Check run `35365603486` selected Yak `1.4.8-beta19` and failed before
source compilation with `root path is not a directory: .` when given `fs.zip`.
The downloaded artifact passes ZIP CRC checks and contains 1,580 files,
including 621 Go files and the protocol fixes. The workflow now materializes
that same snapshot in a fresh directory and scans the directory. Every extracted
file was checked byte-for-byte against the archive; traversal/symlink entries
are rejected. Scanner rules, exclusions, risk thresholds and error handling are
unchanged. Actionlint adds no new diagnostics (six existing action-version and
expression warnings remain). This fixes scanner input compatibility; it does not
turn scanner errors into successful or skipped checks.
