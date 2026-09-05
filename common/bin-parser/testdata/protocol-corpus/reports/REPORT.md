# Protocol corpus evidence report

This report is generated from `sources.json`, the pinned capture bytes and `protocol_roadmap.go`. It reports material availability only; it does not promote any roadmap status.

- Roadmap: **616** protocols; 211 `done`, 0 `partial`, 405 `todo`.
- Corpus: **178 capture files**, **24126 packets**, **5512994 bytes**.
- Direct roadmap material: **156 unique protocols**; outside-roadmap candidates: **10 captures**.
- Evidence classes: 162 positive upstream and 16 negative/boundary upstream captures.

## Source distribution

| Source | Captures | Packets |
| --- | ---: | ---: |
| `ndpi` | 151 | 23435 |
| `tcpdump` | 17 | 55 |
| `wireshark-tests` | 10 | 636 |

## Roadmap family distribution

| Family | Roadmap protocols | With collected capture | Capture files |
| --- | ---: | ---: | ---: |
| `service-tools` | 66 | 5 | 5 |
| `link` | 49 | 5 | 7 |
| `cn-app` | 48 | 6 | 6 |
| `ics` | 46 | 17 | 17 |
| `internet` | 40 | 16 | 17 |
| `longtail` | 40 | 3 | 3 |
| `cn-vendor` | 33 | 0 | 0 |
| `mq-rpc` | 31 | 11 | 11 |
| `routing` | 24 | 8 | 9 |
| `microsoft` | 23 | 0 | 0 |
| `voip` | 23 | 11 | 11 |
| `file` | 21 | 7 | 8 |
| `database` | 20 | 9 | 9 |
| `mgmt` | 20 | 6 | 7 |
| `carrier` | 19 | 3 | 3 |
| `name-config` | 19 | 11 | 14 |
| `remote` | 17 | 8 | 8 |
| `auth` | 16 | 5 | 7 |
| `web` | 16 | 10 | 11 |
| `game` | 10 | 2 | 2 |
| `storage` | 10 | 1 | 1 |
| `transport` | 10 | 5 | 5 |
| `mail` | 9 | 6 | 6 |
| `finance` | 6 | 1 | 1 |

![Protocol material distribution. Every row prints collected protocol count over the full roadmap family count.](protocol-material-distribution.svg)

**Figure 1 | Authoritative capture material by roadmap family.** Blue marks unique roadmap protocols with at least one collected file; the gray extent is the full family backlog. Exact values are printed, so color is not the only encoding.

## Interpretation limits

A capture mapped to a roadmap item establishes available test material, not complete protocol coverage. A single PCAP may exercise only one PDU, direction or version. Negative captures are kept separately because malformed input and classification-boundary handling are part of parser robustness. `outside-roadmap-candidates.csv` records useful discoveries without pretending they were already among the 616 items.
