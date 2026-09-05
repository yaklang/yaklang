# Protocol sample health check

Material availability only. This does **not** mark YAML dissectors complete.

Generated against `ProtocolRoadmap` (616 names) and the vendored corpus in this directory.

## Snapshot

| Metric | Before this branch | After this branch |
| --- | ---: | ---: |
| Capture files | 174 | 334 |
| Packets | 41828 | 57910 |
| Unique roadmap protocols with a capture | 156 | 287 |
| Outside-roadmap candidate captures | 10 | 24 |
| Locally generated captures | 0 | 111 |
| Roadmap `done` / `todo` | 211 / 405 | 211 / 405 (unchanged) |

Sources: nDPI, Wireshark `test/captures`, tcpdump tests, Google CTF, Scapy, ITI ICS-Security-Tools, mrhenrike/PCAPTrafficAnalysis, mgadelha/Sampled_Values, and **local Scapy synthesis** (`generated-local`, CC0). Docker Desktop was used for a Samba named-pipe attempt; identification captures themselves are Scapy `wrpcap` files kept only when `tshark` matched the recorded filter.

## Family coverage (after)

| Family | Roadmap | With capture | Note |
| --- | ---: | ---: | --- |
| `cn-vendor` | 33 | **0** | still private/Colasoft-only |
| `microsoft` | 23 | **2** | WPAD + LLMNR-MDNS collision; generic DCE/RPC bind mapped to `MSRPC` (mq-rpc), not a named pipe |
| `service-tools` | 66 | **18** | + Kubernetes API, WinRM HTTP, Redfish SSDP, Java serialization, JDWP, LLMNR response |
| `link` | 49 | **31** | + STP/RSTP, SNAP, PAP, Linux SLL, WEP, Slow Protocols, FCoE, HomePlug AV |
| `ics` | 46 | **29** | + J1939, Powerlink |
| `internet` | 40 | **31** | + IPcomp, GTPv2 |
| `database` | 20 | **12** | + ETCD, MariaDB, Redis (RESP) |
| `mgmt` | 20 | **13** | + Zabbix |
| `storage` | 10 | **4** | AoE, MinIO/S3 |
| `file` | 21 | **16** | + SMB2/SMB3, CIFS, 9P, Portmap, Mount |
| `auth` | 16 | **11** | + TACACS+, NTLMSSP, SPNEGO |
| `transport` | 10 | **10** | complete |

## Locally generated (111 captures / 287 unique mapped)

Batch 2 adds STP/RSTP, Ethernet SNAP, PAP, Linux SLL, WEP, DCCP, NBT SS, Redis RESP, TACACS+, Portmap/Mount, Bonjour, ACMEv2, Kubernetes API, WinRM HTTP, Submission, NTLMSSP/SPNEGO, SMB3/CIFS, 9P, FCoE, TZSP, Quake, Powerlink, IPcomp, HomePlug AV, Java serialization, JDWP, GTPv2, OLSR, NHRP, Zabbix, H.225.

Scapy recipes in `tools/generate-local/generate.py`. Dropped recipes (no tshark hit) are listed in `generated-index.json` `failed`.

Notable new names: Ethernet II, IEEE 802.1Q, QinQ, RARP, LACP, EAPOL, IEEE 802.11, CDP, ICMPv6 NDP, IPv6 Hop-by-Hop/Routing/Fragment/DstOpt, IGMP, MPLS, Geneve, RIP, RIPng, OSPFv3, EIGRP, LLMNR, DHCPv6, NetFlow v5, SNMPv3, LDAP/CLDAP, SOCKS4, Memcache binary, Docker API, AoE, MinIO/S3, JDWP handshake, JSON-RPC 2.0, ONC RPC, LPD, FTP-DATA, SDP, J1939, SSL, XML-RPC, WPAD, MSRPC, **SMB2**, **MariaDB**.

## ICS remaining without a capture

Modbus RTU/ASCII, IEC 60870-5-101, OPC DA, PROFIBUS, Powerlink, SERCOS III, CC-Link IE, CODESYS, FF HSE, LonTalk, DALI, UDS, VARAN, AES50, Z-Wave, IEC 62056, M-Bus, DALI-2.

## How to grow further

1. `PATH` must include tshark. `./tools/generate-local/.venv/bin/python generate.py` then `go run ./tools/generate`.
2. Keep one small capture per new roadmap name; require a tshark filter hit.
3. Named Microsoft pipes need a tshark `epm`/`samr`/`srvsvc` label, not a generic `dcerpc` bind.
