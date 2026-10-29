# ESP original-source audit

These scripts verify the two pinned public Weber/Sharon captures. Input captures,
Wireshark references, native/CLI exports, binaries and output evidence belong
outside the repository. They require Python 3 and the existing adjacent
`vxlan_endpoint_projection_audit.py` capture reader and its live-audit imports.

`esp_source_inventory.py` runs an explicitly supplied TShark executable and writes
an external per-source inventory. Supply `--tshark`, `--capture-weber`,
`--capture-sharon` and a new `--out` directory. It preserves original section,
interface, nanosecond clock and wire-length facts and records capture, executable
and output hashes.

`esp_public_source_audit.py` requires `--native`, `--CLI`, `--baseline`, `--bins`,
`--source`, `--reference`, both capture arguments and a new `--out` directory.
Each export root has `preservation/weber` and `preservation/sharon` subdirectories.
Native inputs contain `events.jsonl`, `flows.json` and their producer receipts;
CLI inputs contain `yak.jsonl` and its producer receipt. The binary directory
contains events/flows/yak executables and corresponding build receipts. The
reference directory is the output of the inventory script. Existing finalized
inventories with the same source schema can also be supplied.

When these files are installed beside the live-audit readers no path adjustment
is needed. For external staging, `--audit-modules` selects that existing reader
directory. It defaults to the script directory. No repository, user or temporary
root is hardcoded.

The independent expected helpers construct every ESP native event and packet
CLI row from original wire bytes. All types, arrays, source references, ranges,
SPI/sequence words, opaque bytes and metadata are compared. The two additional
invalid-parent observations have full source-built expected objects. Existing
NAT-T and rejected-parent native observations are preserved as whole objects
with only the source-bound own PDU ID translated. A separate full-SDK certificate
must establish the global generated-ID bijection. Packet/flow/close objects and
capture API error results remain literal; counter differences are listed.

This scope covers observed clear ESP headers and an opaque remainder. It does
not establish an SA, algorithm, decryption, peer authentication, reconstructed
ESN or replay verdict. UDP4500 NAT-T remains an independent UDP profile. The
literal interpreter is scoped to the frozen original capture profiles and is
not an arbitrary tunnel dissector.

`esp_source_audit_controls.py` takes `--capture`, `--native-jsonl`, `--CLI-jsonl`,
`--public-proof`, `--corpus` and a new output file. It source-binds a real positive
and rejects ten native plus seven CLI whole-object semantic corruptions. These
are audit negative controls; they are not additional codec executions.

Run any entry point with `--help` for its complete argument list. The pure
expected helper files preserve the original certified bytes; the portable
wrapper changes input plumbing and records its actual imported reader hash.

`ip_refusal_source_audit.py` independently enumerates all original Sharon source
frames before selecting 443 direct IPv4 parent refusals. It then requires exactly
one source-built complete native object and no admitted child per source. Run
`--help` for the capture, native input, helper and new external output arguments.
The decoder's literal zero IPv4 lengths are preserved as zero facts.

`whole_cli_generation_diff.py` compares all 37 closed CLI executions, including
complete producer/capture bindings, row order, native field projections and EOF
publications. Its changed ledger is diagnostic; changed paths are not waivers.

`ipv4_eol_cli_source_audit.py` consumes the frozen 121792-to-02d2 differential,
the original Sharon capture, the adjacent reader directory and a new external
output directory. It constructs complete expected rows for 16,039 of 16,342 EOL
changes and rejects twelve semantic corruptions. The other 303 nested/child/domain
projections are explicitly listed as uncovered, never silently accepted.

`icmp_alias_cli_projection_audit.py` consumes the 02d2-to-fef CLI differential and
a new output directory. It checks all 10,971 alias changes as complete expected
rows and rejects twelve semantic corruptions. Real repeated network layers and
all original native field projections remain literal. This is a presentation
certificate, not additional protocol or message support.

`icmpv6_full_sdk_bridge.py`, `icmpv6_expected.py` and `literal_own_header.py`
compare the 37 complete native generations before and after the ICMPv6 body
fix. Supply `--before`, `--after`, `--capture-sharon`, `--audit-modules`, `--bins`,
`--source-before`, `--source-after` and a new `--out`. Every unchanged export line
must remain byte-identical; the 63 replacements have complete raw-source-built
expected objects, with nine object-corruption controls. The two complete flow
files remain literal. Four previously decoded malformed/incomplete messages and
59 previous IP diagnostics are separately classified. The literal parser records
cross-family and zero-IP-length compatibility as limited observations; it never
infers capture loss, offload cause or a valid on-wire IP extent from those bytes.

`icmpv6_public_fields_audit.py` runs the supplied TShark on the original captures
and compares literal ARP/ICMP/ICMPv6 fields, native projections and source clocks/
lengths. It consumes an explicit census, the source ledger and the complete SDK
bridge, and binds their hashes, constructor/parser hashes and producer binary.
The current certificate covers 11,085 observations and 40,785 key scalars. Its
390 other tunnel/parent profiles remain explicitly uncovered.

`icmpv6_partial_cli_audit.py` constructs the 60 complete changed CLI rows from
the 63 original-source failure objects and verifies every PDU field projection.
Supply the original `--capture`, `--SDK-proof`, the 49b0-to-4c023 `--diff`,
`--audit-modules` and a new external output directory. Its six semantic negative
controls cannot alter a protocol, status, source, PDU, layer or value type.


## ICMPv4, typed network fields and complete TCP diagnostics

The `icmp4_expected_source.py` constructor and `icmpv4_whole_source_plan.py`
rebuild original before/after ICMPv4 objects. `icmpv4_whole_sdk_bridge.py` and
`partial_cli_bridge.py` verify entire generations, including every unchanged
field and byte source. Their `--help` arguments select external captures,
producer folders, binary receipts and a fresh external output directory.

`network23_values.py` and `network23_carrier_source.py` locate and type the
23 aliases from original bytes. `network23_whole_bridge.py` checks all37 SDK
and CLI exports. `network23_ws_source_positions.py` binds actual TShark scalar
values and raw positions to each proven own header. Supply this directory for
the audit/constructor/helper directory arguments; no packet-value search is used.

`bounded_etherip_refusal_expected.py` requires child namespace payloads to fit
their declared current input. `bounded_etherip_whole_sdk_bridge.py` checks the
37 SDK exports and an explicit, injective mapping of known generated references.
Flow counters are reported separately, never normalized to force equality.
`bounded_etherip_TCP_whole_cli_bridge.py` combines those original-source parent
rows with the fresh705 TCP oracle and checks all384,518 CLI rows. Frame, flow,
domain and ordinary numeric values are not reference substitutions.

The portable `tcp-source/` directory contains a frozen full145 constructor,
source-only discovery, the fresh705 row/worker constructor and its freeze/verify
entry points. Use that same folder for `--source-parser` and `--contract145`.
Discovery only supplies frame numbers and source hashes. It does not supply
expected values, fields or ranges. The705 plan is explicitly post-discovery;
the separate145 pre-candidate plan is not retroactively enlarged. Worker checks
compare entire responses, require Source0 ranges within the actual frame, and
reject invented fields, Source1 and connection state. A separate external quick
API is unavailable; targeted Go and real console pages check quick/deep behavior.

## Membership, original lengths and actual console ranges

`membership-source/` retains the certified helper basenames and versioned folders.
The IGMP `source-prep-05` and MLD `MLD-whole-native-prep-03` modules construct
complete typed expectations from literal wire bytes and declared input extents.
They do not run a candidate parser to obtain expected values. The frozen source
ledgers and plans are external inputs, alongside the original public captures.

`membership-whole-bridge-prep-04/bridge.py` has separate `prepare` and `verify`
modes. It pairs retained events in actual publisher order, source-binds new IGMP
events and checks entire objects under an explicit injective generated-PDU
reference map. It does not translate physical frame, flow, domain or ordinary
numeric values. Its `controls.py` and `literal_identity_controls.py` reject
source, type, order and identity corruption. The separate complete flow exports
must still be checked literally; an event bridge alone is not a flow certificate.

`membership-CLI-prep-01/prepare.py` freezes full row expectations before the new
CLI exports exist. `verify.py` checks all37 closed executions and requires those
frozen expectations, the actual native bridge, binary/build receipts and original
captures. `controls02.py` tests source and whole-object corruption independently
of a candidate CLI export. Run each script with `--help` for its external input
folders; supply this live-audit folder for `--audit-modules`. Supply the retained
version subfolders for the IGMP/MLD module arguments. A new run records its own
paths, hashes and provenance; copying an auditor does not create a new execution
or retroactively make an earlier post-discovery plan pre-candidate.

`membership_live_source_recheck.py` is explicitly a post-execution independent
Wireshark source recheck of the six owned Windows reports. The original36-field
pre-execution freeze remains distinct from the later45-field reference check.
`verify_fresh_membership_runtime.py` actually executes the supplied SDK binary
and checks the complete selected semantic tree, typed values and byte ranges.
These sources establish captured fields, not checksum validity or listener state.

`membership_windows_cells_audit.py` reads closed native-console harness results.
It checks actual captured/native source captions and each hex-cell background
for a selected repeated field, in addition to its literal source bytes and range.
It does not operate a desktop app or generate a design mockup. Console renders
remain distinct from native Windows Terminal pixel screenshots.

`closed_protocol_label_audit.py` reruns an explicit TShark executable with an
isolated personal configuration. It checks every original physical frame's
number, lengths, interface and nanosecond clock before counting literal labels.
EOF publications and native messages remain separate. Its no-explicit-keylog
reference mode and explicit TCP out-of-order setting are recorded; label counts
are inspection leads and do not establish protocol correctness.

`decode_error_source_audit.py` closes the three historical accounting changes
using source replay and an optional explicitly byte-differenced counterfactual.
`eol_remaining_source_inventory.py` classifies the original303 EOL investigation
sources without claiming a whole-row certificate for every remaining profile.
All captures, keys, complete exports, build programs, images and logs stay outside
the repository; only these auditor sources and compact evidence metadata belong
in Git.

## Whole CLI continuity and actual Windows transport sources

`public_cli_literal_continuity.py --before OLD --after NEW --out FRESH` verifies
all37 closed CLI executions, build/binary/capture bindings and every full export.
It requires literal byte equality and accepts no generated-ID translations.
Use this for display-only changes; a semantic correction instead needs an
independent source-frozen whole-object expectation.

`transport_windows_source_audit.py --manifest CASES --runtime CLOSED_WINDOWS
--audit-modules THIS_FOLDER --out FRESH` consumes actual console cells. It binds
the full frame, bounded unadmitted payload preview and observed UDP header to
their separate exact source bytes, titles, ranges and original metadata. It
also checks real loopback address text, TCP option indentation, and rejects
range/SHA/source-order mutations. It neither drives the desktop nor substitutes
a generated mockup for a run. A new run keeps its own provenance; earlier
failed harness/alias runs remain distinct external evidence.

## PIM prefixes, ranges and protected-transport recovery

`pim-source/source-manifest.json` lists16 literal source constructors and strict
whole native verifiers. Each stage has its own producer and full expected
objects; the prefix oracle's explicit API batch-order correction is retained.
`pim_cli_boundary_audit.py`, `pim_cli_prefix_audit.py` and
`pim_cli_ranges_audit.py` expose `prepare` and `verify`; prepare an independent
source plan before running a new CLI producer, then verify all37 complete files.
No ID translation, type coercion or array sorting is an accepted shortcut.

`pim_windows_cells_audit.py` consumes closed console executions and independently
locates Source1 as the PIM body, not the entire Source0 frame. It verifies the
selected occurrence by native cell attributes, including two equal addresses
at distinct offsets, and keeps opaque extracted values explicitly unmapped.
Its CLI is `--manifest CASES --runtime CLOSED --audit-modules THIS_FOLDER --out FRESH`.

`public_native_literal_continuity.py --before OLD --after NEW --out FRESH`
binds37 event files and two complete flow files to their original execution,
build and capture receipts. Literal continuity proves preservation, not blanket
correctness of unchanged records.

`tls-quic-source/recovery-02/source-manifest.json` binds13 deployed entry points,
their exact original proof sources in `frozen/`, explicit path-only adapters,
CLI commands and external result hashes. The four adapted sources have distinct
producer hashes; do not relabel older certificates as their executions.
The independent AEAD/frame verifiers retain TCP holes, missing STREAM intervals,
QPACK blocks and malformed tags. Source-only correction of the first wrong
bridge expectation remains separate from its preserved failure.

Always use `python -B`, a new external output directory, the declared original
capture/key-provider/reference settings, and the complete native/CLI producer.
The assertions bind the selected owned capture profiles described in the
manifests; they are not generalized TOP30 certification. Original captures,
keys, PDML, full expectations/exports, plaintext, binaries and console images
remain outside Git.

## Compact history and Windows recovered QUIC views

`native_replay_literal_continuity.py --before OLD --after NEW --plan PLAN
--out FRESH` binds each entire replay file to both original execution receipts,
the original input plan, captures, keys, binary and build. It compares actual
bytes without PDU renumbering or JSON normalization. Preservation does not
certify the interpretation of unchanged fields.

`quic_windows_recovery_cells_audit.py --manifest CASES --runtime CLOSED
--native CLOSED_NATIVE --requirements FROZEN_REQUIREMENTS
--audit-modules THIS_FOLDER --out FRESH` checks the actual default-budget
Windows console cells against physical wire, encrypted PDU, independently
authenticated STREAM bytes and extracted wire remainder. A short viewport
certifies its visible prefix only; the full source is separately hash-bound.

`quic_windows_context_cells_audit.py --manifest CASES --runtime CLOSED
--source-proof FOUR_SOURCES_SUMMARY --audit-modules THIS_FOLDER --out FRESH`
checks key-update recovery facts and QPACK refusal, including actual selection
attributes. Derived counts must not acquire an invented ciphertext interval or
hex highlight. These two auditors explicitly bind the historical `24ca25d1`
producer. New producer verification needs its own source checkpoint; do not
relabel those historical console executions.

## Contributor identity and bounded-detail availability

`contributor-identity/source-manifest.json` preserves the literal external Go
replay probe, its build/run controller and the separately frozen Windows v2
driver. The Go probe uses actual callbacks from the selected immutable capture.
It keeps full snapshots only as a reference; its partial witness contains only
IDs returned by the inspector's source-reference index, never omitted fields.
It checks same-frame Initial/padding after actual detail eviction and the
global1MiB replay's actual PDU299/frame294 association. No production API was
added to work around Yak's inability to construct that Go snapshot value.

Build/run with `contributor-identity/run_mixed_identity.py build|run --repo ROOT
--source FULL_SHA --out FRESH`, supplying the closed build `--producer` for run
and explicit `--capture`/`--keylog` locations where needed. These are selected
source-profile proofs, not arbitrary-capture assertions. The Windows v2 driver
additionally freezes small source files and hashes all inputs before/after.
Its console helper retains its original producer independently of the current
application. The earlier page's successful identity query still had visual
caption/PDU0 defects; its historical execution is not rewritten as a UI pass.

`history_windows_cells_audit.py --runtime CLOSED --source FULL_SHA
--audit-modules THIS_FOLDER --out FRESH` independently verifies that the
unavailable-detail page keeps its observed caption, prints no fake PDU0, and
does not duplicate the reason or manufacture plaintext. It checks every visible
physical-frame octet and five negative mutations. The strengthened duplicate
reason control preserves the heading and byte source, so it fails on the
intended condition rather than a missing unrelated heading.

`protocol_label_cohorts.py --census CLOSED_LABEL_SUMMARY --out FRESH` verifies
both margins of the exact label-pair matrices and renders reference TOP30
cohorts. It preserves every label and each physical frame's single membership;
it does not turn spelling, visibility or conservative refusal into a score.

## NAT-T header source and exact datagram ownership

`natt-source/source-manifest.json` preserves38 literal historical source files,
with their original paths, hashes and external certificate links. See its README
before adapting any entry point. Complete source-built native/API/CLI objects,
all stats fields and the eight-packet interleaving control retain actual EOF
publications and distinct flow identities. The source verifier accepts no
generic ID translation, label aliases or type normalization. Default Wireshark's
CoAP-on4500 disagreement remains explicit.

The actual Windows checker binds separate physical-frame and ESP sources and
the selected SPI interval. Its corrected detail-scoped assertion allows the
truthful global partial-history footer; old failed auditor and producer runs
remain separate evidence. This is a UDP4500 bounded-header and ownership scope,
not SA authentication, native keepalive completeness or OSPFv3 certification.

The later636/685 OSPFv3 checkpoint and independent MLD recheck are indexed in
[the scoped report](../../../../../../../docs/exec-plans/shark-public-corpus-20261025/ospfv3-636-685.md).
Its39 exact historical source files are in `ospfv3-mld-source36/`; prior NATT
helpers remain hash references. Full packets, keys, plans, expected objects and
outputs are excluded. This checkpoint does not certify TOP30 or core completeness.
