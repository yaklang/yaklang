# Protocol corpus evidence report

This report is generated from `sources.json`, the pinned capture bytes and `protocol_roadmap.go`. It reports material availability only; it does not promote any roadmap status.

- Roadmap: **616** protocols; 211 `done`, 0 `partial`, 405 `todo`.
- Corpus: **552 capture files**, **58533 packets**, **10511216 bytes**.
- Direct roadmap material: **341 unique protocols**; outside-roadmap candidates: **29 captures**.
- Evidence classes: 215 positive upstream, 16 negative/boundary upstream, 232 positive generated, 88 negative/boundary generated, 1 generated identification-only.

## Source distribution

| Source | Captures | Packets |
| --- | ---: | ---: |
| `generated-local` | 72 | 131 |
| `generated-pr5023` | 168 | 311 |
| `generated-validated` | 81 | 337 |
| `google-samples` | 3 | 17747 |
| `iti-ics` | 4 | 18 |
| `mgadelha-sv` | 1 | 10161 |
| `mrhenrike-pcap` | 1 | 986 |
| `ndpi` | 177 | 27762 |
| `scapy` | 6 | 264 |
| `tcpdump` | 17 | 55 |
| `wireshark-tests` | 22 | 761 |

## Roadmap family distribution

| Family | Roadmap protocols | With collected capture | Capture files |
| --- | ---: | ---: | ---: |
| `service-tools` | 66 | 23 | 41 |
| `link` | 49 | 44 | 75 |
| `cn-app` | 48 | 8 | 8 |
| `ics` | 46 | 27 | 35 |
| `internet` | 40 | 36 | 58 |
| `longtail` | 40 | 22 | 41 |
| `cn-vendor` | 33 | 0 | 0 |
| `mq-rpc` | 31 | 14 | 17 |
| `routing` | 24 | 20 | 33 |
| `microsoft` | 23 | 2 | 4 |
| `voip` | 23 | 15 | 19 |
| `file` | 21 | 18 | 28 |
| `database` | 20 | 12 | 15 |
| `mgmt` | 20 | 13 | 22 |
| `carrier` | 19 | 5 | 7 |
| `name-config` | 19 | 16 | 25 |
| `remote` | 17 | 10 | 11 |
| `auth` | 16 | 13 | 23 |
| `web` | 16 | 13 | 19 |
| `game` | 10 | 7 | 8 |
| `storage` | 10 | 5 | 10 |
| `transport` | 10 | 10 | 16 |
| `mail` | 9 | 7 | 7 |
| `finance` | 6 | 1 | 1 |

![Protocol material distribution. Every row prints collected protocol count over the full roadmap family count.](protocol-material-distribution.svg)

**Figure 1 | Authoritative capture material by roadmap family.** Blue marks unique roadmap protocols with at least one collected file; the gray extent is the full family backlog. Exact values are printed, so color is not the only encoding.

## Interpretation limits

A capture mapped to a roadmap item establishes available test material, not complete protocol coverage. A single PCAP may exercise only one PDU, direction or version. Negative captures are kept separately because malformed input and classification-boundary handling are part of parser robustness. `outside-roadmap-candidates.csv` records useful discoveries without pretending they were already among the 616 items.
