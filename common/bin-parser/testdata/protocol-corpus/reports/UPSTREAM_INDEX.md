# Authoritative upstream capture index

This index separates material that is safe to vendor from useful collections
that still need attachment-level license review. Counts below are exact for the
recorded commits, not estimates of distinct protocols.

## Vendored sources

| Source | Pinned commit | Capture files upstream | Selected here | Why it is useful |
| --- | --- | ---: | ---: | --- |
| [nDPI regression corpus](https://github.com/ntop/nDPI/tree/4cae778e7e8f846b34f11d4f8392504cdebd3db8/tests/cfgs) | `4cae778e7e8f846b34f11d4f8392504cdebd3db8` | 743 | 114 | Positive classifications, version variants, false-positive cases and malformed inputs used by a maintained traffic classifier. |
| [tcpdump tests](https://github.com/the-tcpdump-group/tcpdump/tree/007db68e28a14a0e8231bd71db9bc6cf8ba37874/tests) | `007db68e28a14a0e8231bd71db9bc6cf8ba37874` | 831 | 14 | Small parser boundaries: truncation, invalid lengths, unsupported link types and historical memory-safety regressions. |
| [Google educational challenge archive](https://github.com/google/google-ctf/tree/067421eb7e918c29e39f187fac5a0f0d72a6ab83) | `067421eb7e918c29e39f187fac5a0f0d72a6ab83` | 3 | 3 | Official exercise traffic with a real reverse-engineering objective rather than a synthetic one-packet fixture. |

The selected set is deliberately smaller than the upstream inventory. Duplicate
application-classification captures, captures that require secrets to decrypt,
and large files that do not add a new roadmap protocol or boundary were left in
the upstream index. `sources.json` is the reviewable allow-list.

## Official educational exercises

| Capture | Challenge context | Packets | Useful exercise |
| --- | --- | ---: | --- |
| `google-challenge-ascii-art` | 2017 qualification reverse-engineering exercise | 60 | Recover an application exchange carried by HTTP form traffic and distinguish transport parsing from application semantics. |
| `google-challenge-engraver` | 2022 qualification hardware exercise | 860 | Decode USB HID reports and reconstruct device actions from a capture with a non-Ethernet link type. |
| `google-challenge-sc` | 2019 game-traffic exercise | 16827 | Handle LLC/IPX traffic and recover state from a long bidirectional game trace. |

These are passive challenge artifacts. Solving them is intentionally out of
scope for the corpus verifier; the verifier checks provenance and bytes, not a
published flag.

## Indexed, not vendored

- [Wireshark SampleCaptures](https://wiki.wireshark.org/SampleCaptures) is the
  best broad manual index for protocol-oriented PCAPs. It covers common L2/L3,
  routing, file sharing, VoIP, databases, SCTP and industrial protocols. The
  wiki page does not provide one uniform redistribution license for every
  historical attachment, so those files are candidates for local experiments,
  not committed inputs, until each attachment is cleared.
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
