# Suricata differential audit report

## Scope and environment

- Yaklang commit: `e4d0968ed2f0`
- Official Suricata: 8.0.8 RELEASE, Homebrew arm64
- Snort: 3.13.2.0, Homebrew arm64
- Platform: macOS arm64
- Mode: offline PCAP replay only
- Audit date: 2026-11-10

The audit compared normalized `(gid, sid, rev)` alert multisets. Every generated
case records deterministic rule/PCAP hashes and verifies how many rules
Suricata loaded. Snort was only used for the common rule subset; dialect
rejections were recorded as execution failures instead of zero alerts.

## Resolved issue: DNS response exposed the request-only `dns.query` buffer

Severity: medium. Type: false positive and alert-count inflation. Status: fixed by
this branch; the tables below preserve the pre-fix audit baseline.

Yaklang matches a `dns.query` rule against the Question section of a DNS
response. Official Suricata treats `dns.query` as a request-direction sticky
buffer and does not match the response. When a UDP pseudo-flow begins with the
response, Yaklang also treats that first packet as `flow:to_server`.

Two deterministic controls reproduce the behavior:

| Case | Rule SID | Yaklang | Suricata |
| --- | ---: | ---: | ---: |
| `dns-response-first-direction` | 9901014 | 1 | 0 |
| `dns-query-buffer-on-response` | 9901016 | 1 | 0 |

Both rules loaded successfully in both engines. The shared generated PCAP has
SHA-256:

```text
463fd8d2cb9e14461e8e0bcdc4ce68b2ef82fdc1eefd5071885b762b2d7d9ace
```

TShark independently reports the packet as a valid DNS response:

```text
src=198.51.100.53:53 dst=192.0.2.10:53000
response=True query=response-first.test udp.checksum.status=2
```

The result was repeated five times: Yaklang alerted five out of five; Suricata
alerted zero out of five.

The same behavior appears in the repository's hash-pinned nDPI DNS capture:

```text
common/bin-parser/testdata/protocol-corpus/captures/ndpi/ndpi-dns.pcap
SHA-256 61276b4f63f00eba469b7cab626bca8a7f820f60a698169ca603fd73d40fe362
```

That PCAP contains two queries and two responses for `www.github.com`. With SID
9902001 using `dns.query; content:"www.github.com"`:

| Engine | Alerts |
| --- | ---: |
| Yaklang | 4 |
| Suricata | 2 |

This doubled the alert count for a normal query rule. The implementation
boundary was the UDP pseudo-flow direction policy in
`common/suricata/engine/udp.go` together with DNS buffer selection in
`common/suricata/match/dns.go`.

The fix derives the semantic UDP client direction from the DNS QR bit when the
decoded DNS layer belongs to the current UDP payload. It preserves the numeric
packet `Direction` and first-observed endpoints, rejects the whole match when a
response is evaluated against the request-only query buffer, and keeps the
existing first-packet fallback for non-DNS UDP. It also accepts the Suricata 8
`dns.query.name` spelling and rejects its combination with `flow:to_client` at
compile time.

After the fix, all 14 generated cases agree with Suricata 8.0.8, including alert
counts and rule-load counts. Both generated DNS controls now produce zero
alerts in both engines. The pinned nDPI capture produces two Yaklang alerts,
matching Suricata's two query alerts instead of the pre-fix count of four.

## Cases without a Yaklang/Suricata difference

The following controls were equal, including alert counts:

- case-sensitive `content` and `nocase`
- bidirectional `<>` reverse flow
- `$EXTERNAL_NET` excluding `$HOME_NET`
- `$HTTP_PORTS` negative control
- TCP `flags` and `dsize` negative controls
- content split across in-order TCP segments
- out-of-order TCP stream reconstruction
- exact retransmission de-duplication
- `flowbits` setter with `noalert` and dependent checker
- midstream capture with `flow:established`

These results confirm that the stateful engine added before this audit closes
many limitations of the legacy packet matcher.

## Reference-engine limitations observed

- Snort 3 without a full inspection configuration did not inspect the
  server-to-client payload in the reverse-flow control, even with an explicit
  reverse header. That case is excluded from the default Snort comparison.
- Snort rejected Suricata's `dns.query` and `nocase` forms in this bare rules
  mode, and had no `$HTTP_PORTS` variable. These are recorded as rule-dialect
  failures, not detection differences.
- Suricata rejects `dns.query` combined with `flow:to_client` as conflicting
  directions. This was useful corroboration that `dns.query` is request-only.

## Reproduction

Run the complete generated matrix:

```sh
go run ./common/suricata/cmd/suricata-diff \
  -work-dir /tmp/yak-suricata-audit > /tmp/yak-suricata-audit.json
```

The expected command exit is 0 after the fix. To inspect any future difference,
use:

```sh
jq '.[] | select(.comparison.equal == false)' /tmp/yak-suricata-audit.json
```

Run a custom capture with:

```sh
go run ./common/suricata/cmd/suricata-diff \
  -rules /path/to/rules.rules \
  -pcap /path/to/capture.pcap
```
