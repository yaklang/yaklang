# Authoritative upstream capture index

This index separates material that is safe to vendor from useful collections
that still need attachment-level license review. Counts below are exact for the
recorded commits, not estimates of distinct protocols.

## Vendored sources

| Source | Pinned commit | Capture files upstream | Selected here | Why it is useful |
| --- | --- | ---: | ---: | --- |
| [nDPI regression corpus](https://github.com/ntop/nDPI/tree/4cae778e7e8f846b34f11d4f8392504cdebd3db8/tests/cfgs) | `4cae778e7e8f846b34f11d4f8392504cdebd3db8` | 743 | 151 | Positive classifications, version variants, classification-boundary cases and malformed inputs used by a maintained traffic classifier. |
| [Wireshark test captures](https://github.com/wireshark/wireshark/tree/4f63ea0eae68cf6facea31604994f1a339e43640/test/captures) | `4f63ea0eae68cf6facea31604994f1a339e43640` | 136 | 10 | Official dissector regression material for ARP, DHCP, HTTP, ICMP, IKEv1, IKEv2, IPX, mDNS, USB HID and HTTP/3, stored in the licensed source repository. |
| [tcpdump tests](https://github.com/the-tcpdump-group/tcpdump/tree/007db68e28a14a0e8231bd71db9bc6cf8ba37874/tests) | `007db68e28a14a0e8231bd71db9bc6cf8ba37874` | 831 | 17 | Small parser samples and boundaries: positive LLDP, RADIUS and Radiotap inputs plus truncation, invalid lengths, unsupported link types and historical parser regressions. |

The selected set is deliberately smaller than the upstream inventory. Duplicate
application-classification captures, captures that require additional secrets
to decode, and large files that do not add a new roadmap protocol or boundary
were left in the upstream index. `sources.json` is the reviewable allow-list.

## Selected neutral fixtures

| Capture | Sample context | Packets | Useful parser validation |
| --- | --- | ---: | --- |
| `wireshark-http` | HTTP request fixture | 1 | Parse a complete application request and verify method, path, version, headers and exact input consumption. |
| `wireshark-usb-hid` | USB HID input-report fixture | 10 | Decode a complete four-byte input report from a capture with a non-Ethernet link type while keeping descriptor-dependent data opaque. |
| `wireshark-ipx-rip` | IPX routing-response fixture | 1 | Parse the complete IPX packet, exclude Ethernet padding and preserve the routing payload as exact bytes. |

These are passive sample artifacts from the same immutable Wireshark source
used elsewhere in the corpus. The verifier checks provenance, exact bytes and
bounded parser behavior.

## Indexed, not vendored

- [Wireshark SampleCaptures](https://wiki.wireshark.org/SampleCaptures) is a
  broad manual index distinct from the vendored files in Wireshark's official
  Git repository. It covers common L2/L3, routing, file sharing, VoIP,
  databases, SCTP and industrial protocols. The wiki page does not provide one
  uniform redistribution license for every historical attachment, so those
  files are candidates for local experiments, not committed inputs, until each
  attachment is cleared.
- [Zeek testing](https://github.com/zeek/zeek-testing) is authoritative for
  analyzer regression behavior, but the current repository tree does not carry
  the PCAP payloads directly. It is therefore not treated as a vendorable
  capture source here.

## Reproduce the inventory counts

With GitHub CLI authentication, replace `OWNER/REPO` and `COMMIT` below using
the table above:

```bash
gh api 'repos/OWNER/REPO/git/trees/COMMIT?recursive=1' \
  --jq '[.tree[] | select(.type=="blob") | select(.path|test("\\.(pcap|pcapng|cap)$";"i"))] | length'
```

The generator performs a stronger check for the selected files: it downloads
only immutable raw URLs, computes SHA-256, reads every packet locally, and
extracts a representative full-frame hex value using the recorded display
filter.
