# Protocol sample health check

Material availability only. This does **not** mark YAML dissectors complete.

Generated against `ProtocolRoadmap` (616 names) and the vendored corpus in this directory.

## Snapshot

| Metric | Before this branch | After this branch |
| --- | ---: | ---: |
| Capture files | 174 | 385 |
| Packets | 41828 | 58003 |
| Unique roadmap protocols with a capture | 156 | 338 |
| Outside-roadmap candidate captures | 10 | 24 |
| Locally generated captures | 0 | 162 |
| Roadmap `done` / `todo` | 211 / 405 | 211 / 405 (unchanged) |

Sources: nDPI, Wireshark `test/captures`, tcpdump tests, Google CTF, Scapy, ITI ICS-Security-Tools, mrhenrike/PCAPTrafficAnalysis, mgadelha/Sampled_Values, and **local Scapy synthesis** (`generated-local`, CC0). Identification captures are Scapy `wrpcap` files kept only when `tshark` recorded a protocol token that names the roadmap entry. HTTP/TLS/RPC/RMI/STP/NBNS/data-only frames are not remapped to a more specific missing name.

## Family coverage (after)

| Family | Roadmap | With capture | Note |
| --- | ---: | ---: | --- |
| `cn-vendor` | 33 | **0** | still private/Colasoft-only |
| `microsoft` | 23 | **2** | WPAD + LLMNR-MDNS collision; generic DCE/RPC bind mapped to `MSRPC` (mq-rpc), not a named pipe |
| `service-tools` | 66 | **23** | + RMI, IPMI RMCP+, NTLM v1/v2 (tshark `ntlmssp`); HTTP-only Consul/Jenkins/etc. dropped |
| `link` | 49 | **40** | + Ethernet 802.2, CFM, TRILL, E-LMI, Fibre Channel, VNTAG, IEEE 802.1ah PBB, EOAM, Linux SLL2 |
| `ics` | 46 | **29** | + J1939, Powerlink |
| `internet` | 40 | **35** | + IPcomp, GTPv2, MIPv6 |
| `longtail` | 40 | **19** | + X11-adjacent longtail: TIPC, AllJoyn, AARP, VINES, SNA, SliMP3, SIGCOMP, swIPe, WSMP, DECnet, SEBEK, XNS |
| `routing` | 24 | **20** | + IGRP, DVMRP, MSDP, IS-IS, LACP-marker |
| `database` | 20 | **12** | + ETCD, MariaDB, Redis (RESP) |
| `mgmt` | 20 | **13** | + Zabbix |
| `storage` | 10 | **5** | AoE, MinIO/S3, iSCSI |
| `file` | 21 | **18** | + SMB2/SMB3, CIFS, 9P, Portmap, Mount |
| `auth` | 16 | **13** | + TACACS+, NTLMSSP, SPNEGO |
| `carrier` | 19 | **6** | + Diameter Cx/Dx, UCP/EMI |
| `voip` | 23 | **16** | + Megaco, ICE (`stun`) |
| `remote` | 17 | **10** | + Rlogin, X11 |
| `transport` | 10 | **10** | complete |

## Locally generated (162 captures / 338 unique mapped)

Batch 4 replaces HTTP/RPC/RMI/STP/NBNS floods with tshark-named frames. New names: X11, AllJoyn, AARP, Fibre Channel, MIPv6, VINES, SNA, SliMP3, SIGCOMP, swIPe, WSMP, VNTAG, IEEE 802.1ah PBB, EOAM, LACP-marker, Diameter Cx/Dx, UCP/EMI, DECnet, SEBEK, NTLM v1/v2, XNS, ICE, TIPC, IS-IS, Linux SLL2.

Kept honest batch-3 names include Ethernet 802.2, CFM, TRILL, E-LMI, NVGRE, NAT-T, Mobile IP, IGRP, DVMRP, MSDP, Rsync, NCP, NTLM, GSS-API, Rlogin, RMI, IIOP Locate, IPMI RMCP+, Megaco/H.248, CMPP, Quake2, Quake3, iSCSI, AppleTalk, LAT, NetNTLMv2.

Scapy recipes in `tools/generate-local/generate.py`. Dropped recipes (no tshark hit) are listed in `generated-index.json` `failed`.

## ICS remaining without a capture

Modbus RTU/ASCII, IEC 60870-5-101, OPC DA, PROFIBUS, SERCOS III, CC-Link IE, CODESYS, FF HSE, LonTalk, DALI, UDS, VARAN, AES50, Z-Wave, IEC 62056, M-Bus, DALI-2.

## How to grow further

1. `PATH` must include tshark. `./tools/generate-local/.venv/bin/python generate.py` then `go run ./tools/generate`.
2. Keep one small capture per new roadmap name; require a tshark filter hit.
3. Named Microsoft pipes need a tshark `epm`/`samr`/`srvsvc` label, not a generic `dcerpc` bind.
