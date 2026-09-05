# Protocol sample health check

Material availability only. This does **not** mark YAML dissectors complete.

Generated against `ProtocolRoadmap` (616 names) and the vendored corpus in this directory.

## Snapshot

| Metric | Before this branch | After this branch |
| --- | ---: | ---: |
| Capture files | 174 | 295 |
| Packets | 41828 | 57826 |
| Unique roadmap protocols with a capture | 156 | 248 |
| Outside-roadmap candidate captures | 10 | 24 |
| Locally generated captures | 0 | 72 |
| Roadmap `done` / `todo` | 211 / 405 | 211 / 405 (unchanged) |

Sources: nDPI, Wireshark `test/captures`, tcpdump tests, Google CTF, Scapy, ITI ICS-Security-Tools, mrhenrike/PCAPTrafficAnalysis, mgadelha/Sampled_Values, and **local Scapy synthesis** (`generated-local`, CC0). Docker Desktop was used for a Samba named-pipe attempt; identification captures themselves are Scapy `wrpcap` files kept only when `tshark` matched the recorded filter.

## Family coverage (after)

| Family | Roadmap | With capture | Note |
| --- | ---: | ---: | --- |
| `cn-vendor` | 33 | **0** | still private/Colasoft-only |
| `microsoft` | 23 | **2** | WPAD + LLMNR-MDNS collision; generic DCE/RPC bind mapped to `MSRPC` (mq-rpc), not a named pipe |
| `service-tools` | 66 | **12** | Docker API, Memcache binary, JDWP handshake, IPv6 RA, DHCPv6 server, WPAD proxy |
| `link` | 49 | **21** | Ethernet II/802.3, VLAN/QinQ, RARP, LLC, LACP, EAPOL/EAP/802.1X, 802.11, CDP, Loopback, PPPoE Session, LCP, CHAP |
| `ics` | 46 | **28** | + J1939 |
| `internet` | 40 | **29** | IPv6 extension headers, NDP/MLD, IGMP, MPLS, Geneve, L2TP, IPIP, 6to4 |
| `database` | 20 | **11** | + ETCD, MariaDB (live docker greeting) |
| `mgmt` | 20 | **12** | + NetFlow v5, SNMPv3, ICMP Timestamp, Prometheus, Redfish |
| `storage` | 10 | **4** | + AoE, MinIO/S3 |

## Locally generated this round (72 names / 16 families)

Scapy recipes in `tools/generate-local/generate.py`. Dropped recipes (no tshark hit) are listed in `generated-index.json` `failed`.

Notable new names: Ethernet II, IEEE 802.1Q, QinQ, RARP, LACP, EAPOL, IEEE 802.11, CDP, ICMPv6 NDP, IPv6 Hop-by-Hop/Routing/Fragment/DstOpt, IGMP, MPLS, Geneve, RIP, RIPng, OSPFv3, EIGRP, LLMNR, DHCPv6, NetFlow v5, SNMPv3, LDAP/CLDAP, SOCKS4, Memcache binary, Docker API, AoE, MinIO/S3, JDWP handshake, JSON-RPC 2.0, ONC RPC, LPD, FTP-DATA, SDP, J1939, SSL, XML-RPC, WPAD, MSRPC, **SMB2**, **MariaDB**.

## ICS remaining without a capture

Modbus RTU/ASCII, IEC 60870-5-101, OPC DA, PROFIBUS, Powerlink, SERCOS III, CC-Link IE, CODESYS, FF HSE, LonTalk, DALI, UDS, VARAN, AES50, Z-Wave, IEC 62056, M-Bus, DALI-2.

## How to grow further

1. `PATH` must include tshark. `./tools/generate-local/.venv/bin/python generate.py` then `go run ./tools/generate`.
2. Keep one small capture per new roadmap name; require a tshark filter hit.
3. Named Microsoft pipes need a tshark `epm`/`samr`/`srvsvc` label, not a generic `dcerpc` bind.
