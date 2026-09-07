# Derived QUIC stream fixtures

These are decrypted stream bytes exported from the two unchanged public source
captures, not plaintext bytes stored on their packet wire. Wireshark performs
decryption using each pcapng file's embedded Decryption Secrets Block. The
extractor explicitly disables the external TLS key-log setting. Bin-parser does
not perform QUIC decryption or transport reassembly in this workflow.

Each JSON file pins its source SHA-256 and records the extraction command, tool
version, stream direction, stream ID, source frame numbers, fragment offsets,
FIN flags, fragment digests and assembled stream digest. Repeated data is kept
as retransmission evidence; ICMP quotations are excluded. Conflicting overlaps,
gaps, inconsistent FIN positions and streams over one MiB reject extraction.
Offsets refer to the decrypted stream, not to original packet-wire positions.

| Source | Stream evidence | Boundary |
| --- | --- | --- |
| `wireshark-http3-qpack` | Six fragments, four streams of 53, 46, 1505 and 33 bytes | The request has FIN; control and encoder streams are contiguous observed prefixes |
| `ndpi-doq` | Eight fragments, two streams of 40 and 56 bytes, including six response retransmissions | Both directions have observed FIN |

The DoQ capture uses the historical `doq-i00` mapping: raw DNS without the
two-octet length introduced by the modern RFC 9250 mapping. It must use the
explicit draft entry. Modern length-prefixed controls are independent inputs,
not modifications of the captured draft messages.

To reproduce from the stored captures, run from the repository root:

```sh
node common/bin-parser/testdata/protocol-corpus/tools/extract-quic-streams.mjs --tshark /path/to/tshark
```

Add `--check` to compare without rewriting output. `--only ndpi-doq` or
`--only wireshark-http3-qpack` selects one source. A newer tool-version string is
allowed during comparison; all source identities, command options, stream
bytes, boundaries and frame mappings must still match. Go protocol-specific
tests consume the stored JSON and do not require Wireshark at runtime.

These derived files are separate from the capture count and from independently
generated companion packets. Their existence alone does not establish passing
application decoding, successful exchanges or endpoint identity.
