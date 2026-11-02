# Captured-layer reference joins

TShark `-T fields -E occurrence=a` can return several `udp.stream` values for
one physical frame. They belong to different encapsulation layers. A comparison
must not join that flattened list to the native outer tuple or call it an
application-session merge.

`udp-projection` matches the exact captured IPv4 source/destination positions and
all four UDP header fields to one PDML protocol occurrence. Matching ports, a
preferred occurrence index, or a shared stream number are insufficient. Inner
reference headers remain in the parsed packet; they are not reclassified as an
outer connection. The current profile supports direct unfragmented IPv4 → UDP.
IPv6, fragments and other selected carrier paths require separate profiles.

Generate PDML from the complete original capture with an explicit TShark version,
configuration and filter. Retain `frame`, `ip` and `udp` field trees, for example
`-T pdml -J "frame ip udp"`. Preserve stderr, command/configuration, input capture
SHA256 and output SHA256. Reference dissector warnings and default TSO length
inference remain evidence; a successful process exit does not waive them.

Build an independent JSONL evidence ledger from the same capture. Each record has:

- `frame`: original physical frame number, starting at one.
- `interface`, `section`, `encapsulation`: original capture domain. Section and
  encapsulation are carried source metadata; PDML interface is checked directly.
- `ip_offset`: independently verified packet-relative outer IPv4 header offset.
- `captured`: complete captured frame bytes, JSON base64. These private records
  and the full PDML stay outside the repository and fixture archives.

Run from the repository root:

```text
go run ./internal/trafficoracle/cmd/udp-projection -capture-sha256 <original-capture-sha256> -evidence <private-evidence.jsonl> -pdml <private-reference.pdml> -output <new-private-result.jsonl>
```

The command verifies every selected frame before creating an output. Missing or
duplicate frames, ambiguous occurrences, mismatched bytes/ranges/domains and
unsupported carriers fail explicitly. Extra reference frames are counted
separately. Existing output files are refused. Scope limits are 64 MiB of captured
evidence, 100,000 selected frames and 1 GiB of streaming PDML; split larger scopes
explicitly rather than silently omitting frames.

Outputs retain original domain and captured SHA256. The summary pins the capture,
evidence and PDML hashes. `extent_consistent` describes only literal IP/UDP length
compatibility. It does not validate a checksum, establish an application session,
infer a connection generation, or certify all protocol semantics. Aggregate by
the complete source domain and selected carrier; never discard VLAN/section
identity or compare an inner stream set to the outer tuple.

The actual command and adversarial joins run in `go test ./scripts/ci`, which is
already part of the full traffic CI. They require no downloaded capture and never
write samples into the source tree.
