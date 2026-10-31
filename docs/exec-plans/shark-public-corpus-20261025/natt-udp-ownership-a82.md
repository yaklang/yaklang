# UDP encapsulated ESP and datagram ownership

Runtime source: `a82acd0fff97ae50dfc1bbb403c16af359d7bf8f`. This is a bounded
header-observation and datagram-ownership checkpoint, not TOP30 certification.
The [machine-readable summary](natt-udp-ownership-a82-summary.json) retains
separate producer, input and result hashes. Captures, keys, complete expected
objects, exports, logs, executables and console buffers remain outside Git.

## Behavior and limits

Seven complete Weber UDP4500 datagrams previously remained unknown UDP. They
now expose observed ESP SPI, sequence and opaque payload through the shared
bin-parser header codec, native API, private fields and progressive CLI view.
The source-selected envelope is 12..65527 bytes with a nonzero SPI. It does not
infer an SA, cipher, authenticated peer or decrypted content. Direct IP50 ESP
keeps its separate carrier contract; IKE's four-zero marker and one-byte NAT
keepalive are not treated as ESP. See [RFC3948](https://www.rfc-editor.org/rfc/rfc3948.html).

Complete native and session grammars run before automatic NAT-T fallback.
Explicit `esp-natt` decode-as remains terminal and rejects invalid input.
An existing TFTP request no longer owns every datagram sharing its client port
and server IP. A candidate must match the original endpoint, a confirmed or
previously observed peer, or a TFTP opcode prefix before changing ID, clock,
LRU, allocation or state. Refused current candidates and retired generations
have different states in one charged, bounded peer history. Repeated evidence
does not allocate again; new generations preserve retired-peer refusal.

This distinction preserves both existing no-stitch cases: residual UDP bytes
cannot complete an earlier partial OACK/DATA, while a later complete OACK from
the same current candidate may recover. Count/byte exhaustion retains existing
typed resource causes rather than claiming an absent field or new session.

## Closed executions

| Execution | Result | Scope |
|---|---:|---|
| Full five-package regression | 285542 pass, 24 skip, 0 fail | Every run has a unique terminal result |
| Focused ownership and fields | 1992 pass | Separate execution; not added to coverage |
| Public native replay | 37 inputs, 39 exports | 306455 event objects and 2 flow objects, separately counted |
| Public CLI replay | 37 inputs, 384518 rows | Entire files, including capture-close publications |
| API and ProtocolStats | 7 complete objects, all 16 stats fields | Exact source-built correction and unchanged remainder |
| Shared UDP / QUIC ownership fuzz | 28973 and 143059 executions | Two separate 60-second targets; 0 failures |

Seven complete source-built replacements were frozen before the corrected
producer. All remaining public objects, IDs, references, array order and both
flow files remain literal. There is no general ID translation or JSON type
normalization. Weber gains seven messages and 672 classified/limited bytes;
unknown decreases by seven and unclassified bytes by 672. No flow or SA is
invented. Unchanged objects are preservation evidence, not proof that every
unchanged field was already correct.

The independent C8 control interleaves complete IPv4/IPv6 STUN, CoAP, TFTP and
RADIUS on client port4500. Eight physical packets produce ten native objects
and ten CLI rows, including two actual capture-close publications. Whole-object
comparison is exact, including both new RADIUS flows and original EOF IDs.
Default Wireshark labels its two CoAP packets ESP on4500; complete CoAP grammar
is verified with explicit reference decode-as. This deliberate difference is
retained, not counted as default-label parity. The earlier RADIUS fixture lacked
NAS identity and remains recorded as an invalid fixture, not a parser failure.

## Actual Windows page

The a82 executable replayed the original 51328-frame Weber capture with the
default 20000 packet cache and 64MiB history. Actual native-console cells select
frame46978/PDU37903 and distinguish Source0's 138-byte physical frame from
Source1's 96-byte ESP message. The selected SPI maps to `[0,4)` in Source1 and
`[42,46)` physically; sequence is a separate `[4,8)` field. Only the SPI selection
was exercised. Source cycling, resizing, normal exit and CP936 restoration
passed. Root reviewed the rendered actual cells; these are not Windows Terminal
pixel screenshots. Screen buffers and PNGs remain external.

The truthful global partial-filter footer remains because unrelated history
has been evicted. An earlier auditor rejected that footer despite valid selected
ESP fields; the failed audit is preserved and the corrected checker scopes its
assertion to the selected detail pane. No budget was raised to hide eviction.

## Provenance and remaining work

The [frozen live audit archive](../../../common/yak/cmd/yakcmds/shark-cli/testdata/live-audit.zip)
retains `live-audit/natt-source/`, containing 38 exact historical Python/Go sources and their original registration
hashes. Their hard-coded paths and source-specific plans retain their original
scope; running an adapter is a new execution, not a replacement certificate.
The earlier priority, scope-gate, fixture, EOF-controller and auditor failures
remain linked in the machine summary. None were overwritten as successful runs.

All four a82 gates passed:
[Essential](https://github.com/yaklang/yaklang/actions/runs/40827559951),
[explicit full-SHA traffic](https://github.com/yaklang/yaklang/actions/runs/40827567724),
[Diff](https://github.com/yaklang/yaklang/actions/runs/40827559946), and
[path case](https://github.com/yaklang/yaklang/actions/runs/40827559934).
The draft-triggered skipped traffic run is not a pass.

OSPFv3's 22 missing legacy views remain open: source checks cover all417 v3
packets, 182 complete LSA checksums, 128 prefix encodings and109 AH headers,
but those source checks are not a corrected native execution. Broader TLS/QUIC
loss/reuse, the original investigation lists and other TOP30 profiles remain
separate obligations. No claim of core-protocol completeness is made here.
