# Protocol sample corpus

This directory is test and experiment material for the 616-item
`ProtocolRoadmap`. It deliberately keeps evidence collection separate from
claims that a Yaklang dissector is complete.

The current snapshot contains 178 capture files, 24,126 packets and direct
material for 156 unique roadmap protocols. The source and family breakdown is
generated in `reports/REPORT.md`; `reports/UPSTREAM_INDEX.md` records the wider
authoritative collections and selection policy.

Each capture has all of the following:

- a source repository and immutable upstream commit;
- the upstream path, source URL, license and independently pinned source
  SHA-256;
- packet count and link type read from the capture itself;
- a representative full-frame hex dump selected with a recorded Wireshark
  display filter;
- an exact roadmap mapping, or an explicit outside-roadmap marker.

`upstream-positive` means the upstream project uses the capture as positive
protocol material. `upstream-negative` means malformed, truncated or
classification-boundary regression material.
None of these labels proves that the current Yaklang YAML rule parses every
packet correctly.

## Executable validation

`protocol_corpus_parse_test.go` closes that gap with an explicit validation
path for every committed capture. The current snapshot requires exactly one
path for each of the 178 files: 159 positive captures are parsed, 14 malformed
captures must be rejected, 2 container/link-type cases are checked
structurally, and 3 proprietary formats are checked against the byte
invariants published by their pinned nDPI detectors.

The 156 roadmap names are also checked independently. The current split is 143
field-level parsers, 10 cases where the available capture exposes only an
enclosing or observable outer format, and 3 proprietary formats for which no
public field grammar is available. These categories are intentionally kept
separate so a signature match cannot be reported as field-level parsing.

Representative positive parses must return a non-empty tree, include declared
protocol-specific fields, consume the bounded protocol input exactly, and stay
within the input range. The integrity pass reads all 24,126 packet records and
checks every capture hash, count, link type and representative frame. A second,
independent all-packet pass sends every non-empty record through a bounded,
structured link- or network-envelope rule, requires an exact terminal-byte
cover and verifies that no bounded input remains unread. The current snapshot
passes all 24,126 records through bin-parser. The sample whose link type is not
decoded is still bound to its real classic-pcap record header and payload rather
than bypassing the parser. This envelope pass also includes the complete USB
container and the deliberately truncated Ethernet record instead of silently
skipping them.

Envelope coverage is not reported as application-level decoding. Semantic
parsing remains scoped to each recorded representative message; whole-capture
checks are added where a format or classifier needs flow-level evidence. The
new DHCP, HTTP, IPX/RIP, USB HID, RADIUS, LLDP and RadioTap captures also have
dedicated checks that visit every record appropriate to their contracts.
Selected fields have independent exact-value assertions, and all registered
malformed inputs must fail with their declared failure class. The ten-record
RADIUS boundary capture additionally pins each record's discriminator and
specific rejection reason instead of accepting an arbitrary parser error.

The inventory is fail-closed in both directions: every capture, representative
hex file and license on disk must be referenced exactly once, and every source
entry must match the generated manifest field-for-field. A new capture is not
accepted until it has a schema-valid source record, a fixed source digest, a
representative frame when non-empty, and exactly one executable validation
category. Its complete packet inventory is then included automatically in the
all-packet envelope pass. Classifier entries additionally require a registered
whole-capture audit.

## Rebuild

The inputs are declarative in `sources.json`; the two JSON Schemas describe the
input and generated manifest. To download the pinned inputs and regenerate the
manifest, representative frame hex, CSV tables and SVG figures:

```bash
go run ./common/bin-parser/testdata/protocol-corpus/tools/generate -fetch
```

Requirements: Go, `tshark` and network access. No service in a capture is
contacted. The generator only downloads immutable raw files from the listed
GitHub repositories.

To regenerate from already downloaded captures:

```bash
go run ./common/bin-parser/testdata/protocol-corpus/tools/generate
```

The local test verifies pinned source hashes, the closed artifact inventory,
packet counts, representative frame hex, source/manifest parity, roadmap
mappings and executable validation categories without requiring Wireshark or
network access.
The SVG chart is the canonical generated figure; the adjacent PNG is a rendered
review copy.

## Data handling and licensing

The committed corpus is passive data consumed by local parser tests. Reading a
capture does not contact any service.

Upstream license texts are stored under `licenses/`. Captures from Wireshark's
official repository are pinned and vendored under its project license.
Wireshark SampleCaptures is retained separately as a research index in
`reports/UPSTREAM_INDEX.md`; historical wiki attachments are not vendored
because attachment-level redistribution terms are not uniform. This corpus
vendors only captures from repositories with an explicit project license at
the pinned commit.
