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
