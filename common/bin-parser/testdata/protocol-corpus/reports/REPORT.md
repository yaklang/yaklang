# Protocol corpus evidence report

This report is generated from `sources.json`, the pinned capture bytes and `protocol_roadmap.go`. It reports material availability only; it does not promote any roadmap status.

- Roadmap: **616** protocols; 210 `done`, 0 `partial`, 406 `todo`.
- Corpus: **131 capture files**, **34885 packets**, **5398203 bytes**.
- Direct roadmap material: **113 unique protocols**; outside-roadmap candidates: **10 captures**.
- Evidence classes: 112 positive upstream, 16 negative/boundary upstream, 3 official CTF.

## Source distribution

| Source | Captures | Packets |
| --- | ---: | ---: |
| `google-ctf` | 3 | 17747 |
| `ndpi` | 114 | 17125 |
| `tcpdump` | 14 | 13 |

## Roadmap family distribution

| Family | Roadmap protocols | With collected capture | Capture files |
| --- | ---: | ---: | ---: |
| `pentest` | 66 | 4 | 4 |
| `link` | 49 | 3 | 3 |
| `cn-app` | 48 | 4 | 4 |
| `ics` | 46 | 16 | 16 |
| `internet` | 40 | 11 | 12 |
| `longtail` | 40 | 1 | 1 |
| `cn-vendor` | 33 | 0 | 0 |
| `mq-rpc` | 31 | 9 | 9 |
| `routing` | 24 | 7 | 8 |
| `microsoft` | 23 | 0 | 0 |
| `voip` | 23 | 6 | 6 |
| `file` | 21 | 5 | 6 |
| `database` | 20 | 8 | 8 |
| `mgmt` | 20 | 6 | 7 |
| `carrier` | 19 | 1 | 1 |
| `name-config` | 19 | 6 | 8 |
| `remote` | 17 | 6 | 6 |
| `auth` | 16 | 4 | 5 |
| `web` | 16 | 6 | 7 |
| `game` | 10 | 0 | 0 |
| `storage` | 10 | 1 | 1 |
| `transport` | 10 | 4 | 4 |
| `mail` | 9 | 4 | 4 |
| `finance` | 6 | 1 | 1 |

![Protocol material distribution. Every row prints collected protocol count over the full roadmap family count.](protocol-material-distribution.svg)

**Figure 1 | Authoritative capture material by roadmap family.** Blue marks unique roadmap protocols with at least one collected file; the gray extent is the full family backlog. Exact values are printed, so color is not the only encoding.

## Interpretation limits

A capture mapped to a roadmap item establishes available test material, not complete protocol coverage. A single PCAP may exercise only one PDU, direction or version. Negative captures are kept separately because malformed input and false-positive resistance are part of parser authentication. `outside-roadmap-candidates.csv` records useful discoveries without pretending they were already among the 616 items.
