# Protocol sample corpus

This directory is test and experiment material for the 616-item
`ProtocolRoadmap`. It deliberately keeps evidence collection separate from
claims that a Yaklang dissector is complete.

The current snapshot contains 384 capture files, 58,044 packets and direct
material for 337 unique roadmap protocols. The source and family breakdown is
generated in `reports/REPORT.md`; `reports/HEALTHCHECK.md` is the coverage
gap survey; `reports/UPSTREAM_INDEX.md` records the wider authoritative
collections and selection policy.

Each capture has all of the following:

- a source repository and immutable upstream commit;
- the upstream path, source URL, license and local SHA-256;
- packet count and link type read from the capture itself;
- a representative full-frame hex dump selected with a recorded Wireshark
  display filter;
- an exact roadmap mapping, or an explicit outside-roadmap marker.

`upstream-positive` means the upstream project uses the capture as positive
protocol material. `upstream-negative` means malformed, truncated or
false-positive regression material. `educational-challenge` is an official
exercise capture.
None of these labels proves that the current Yaklang YAML rule parses every
packet correctly.

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

The ordinary Go test verifies local hashes, packet counts, representative
frame hex and roadmap mappings without requiring Wireshark or network access.
The SVG chart is the canonical generated figure; the adjacent PNG is a rendered
review copy.

## Data handling and licensing

The committed corpus is passive data consumed by local parser tests. Reading a
capture does not contact any service. Educational captures may contain exercise
solutions by design.

Upstream license texts (or README when a repo has no LICENSE file) are stored
under `licenses/`. GitHub captures are pinned to an upstream commit. Locally
generated captures (`generated-local`) are synthesized with Scapy; their
`source_url` is `generated://scapy/<recipe-sha1>/<id>` and the recipe lives
in `tools/generate-local/`. Wireshark SampleCaptures and automayt/ICS-pcap
LFS pointer files remain indexed in `reports/UPSTREAM_INDEX.md` rather than
vendored.
