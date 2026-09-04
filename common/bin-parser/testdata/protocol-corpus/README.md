# Protocol sample corpus

This directory is test and experiment material for the 616-item
`ProtocolRoadmap`. It deliberately keeps evidence collection separate from
claims that a Yaklang dissector is complete.

The current snapshot contains 131 capture files, 34,885 packets and direct
material for 113 unique roadmap protocols. The source and family breakdown is
generated in `reports/REPORT.md`; `reports/UPSTREAM_INDEX.md` records the wider
authoritative collections and selection policy.

Each capture has all of the following:

- a source repository and immutable upstream commit;
- the upstream path, source URL, license and local SHA-256;
- packet count and link type read from the capture itself;
- a representative full-frame hex dump selected with a recorded Wireshark
  display filter;
- an exact roadmap mapping, or an explicit outside-roadmap marker.

`upstream-positive` means the upstream project uses the capture as positive
protocol material. `upstream-negative` means malformed, truncated or
false-positive regression material. `ctf` is an official challenge capture.
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

## Safety and licensing

The committed corpus is passive data. Do not replay it on a network you do not
own or administer. CTF captures can contain challenge secrets by design.

Upstream license texts are stored under `licenses/`. Wireshark SampleCaptures
is retained as a research index in `reports/UPSTREAM_INDEX.md`, but those wiki
attachments are not vendored because attachment-level redistribution terms are
not uniform. This corpus vendors only captures from repositories with an
explicit project license at the pinned commit.
