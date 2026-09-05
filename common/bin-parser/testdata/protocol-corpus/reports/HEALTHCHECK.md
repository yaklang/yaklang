# Protocol sample health check

Material availability only. This does **not** mark YAML dissectors complete.

Generated against `ProtocolRoadmap` (616 names) and the vendored corpus in this directory.

## Snapshot

| Metric | Before this branch | After this branch |
| --- | ---: | ---: |
| Capture files | 174 | 223 |
| Packets | 41828 | 57695 |
| Unique roadmap protocols with a capture | 156 | 176 |
| Outside-roadmap candidate captures | 10 | 24 |
| Roadmap `done` / `todo` | 211 / 405 | 211 / 405 (unchanged) |

Sources: nDPI, Wireshark `test/captures`, tcpdump tests, Google CTF, Scapy, **ITI ICS-Security-Tools** (CC-BY-4.0), **mrhenrike/PCAPTrafficAnalysis** (MIT), **mgadelha/Sampled_Values** (README only). Wiki SampleCaptures and automayt/ICS-pcap LFS dumps stay indexed, not vendored (LFS pointers, not packet bytes).

## Family coverage (after)

Worst sample deserts:

| Family | Roadmap | With capture | Note |
| --- | ---: | ---: | --- |
| `cn-vendor` | 33 | **0** | Huawei/H3C/Sangfor/Hikvision/GB28181 etc. are `srcPrivate`; no public PCAP in the selected trees |
| `microsoft` | 23 | **0** | DCE/RPC exists under `mq-rpc`; named Microsoft product protocols (SAMR/LSARPC/WMI/…) still empty |
| `service-tools` | 66 | 6 | added **Checkmk**; Jenkins/Nagios/Docker/… still missing |
| `longtail` | 40 | 4 | added **NetBEUI** |
| `ics` | 46 | **27** | was 21; added GOOSE, SV, EtherCAT, Profinet DCP, TPKT, COTP |

## ICS remaining without a capture

Still `missing` after this round:

Modbus RTU/ASCII, IEC 60870-5-101, OPC DA, PROFIBUS, Powerlink, SERCOS III, CC-Link IE, CODESYS, FF HSE, LonTalk, DALI, J1939, UDS, VARAN, AES50, Z-Wave, IEC 62056, M-Bus, DALI-2.

automayt/ICS-pcap `ETHERCAT/ethercat.pcap` and `POWERLINK/epl.pcap` are 131-byte Git LFS pointers, not packet captures. Do not invent hex.

## Captures added this round (identification only)

From nDPI (LGPL-3.0, commit `4cae778e`):

- **Checkmk**, **IAX2**, **Steam**, **miHoYo/HoYoverse** (Genshin Impact), **NetBEUI** (Win98 SMB/NetBEUI)

From ITI ICS-Security-Tools (CC-BY-4.0, commit `9b826091`):

- **IEC 61850 GOOSE** (`GOOSE.pcap`, tshark `goose`)
- **Profinet DCP** (`ChangeIPUsingDCP.pcap`, tshark `pn_dcp`)
- **TPKT** / **COTP** from the smallest Snap7 S7 setup/readVar frames (tshark `tpkt`/`cotp`/`s7comm`)

From mrhenrike/PCAPTrafficAnalysis (MIT, commit `216566e9`):

- **EtherCAT** (`ICS-Ethercat-001.pcap`, tshark `ecat`)

From mgadelha/Sampled_Values (README only, commit `0d6760c7`):

- **IEC 61850 SV** (`SV_Normal_Traffic.cap`, 1.4MB / 10161 packets; 5–8MB variants skipped)

Earlier on this branch: nDPI CIP explicit / UMAS / TriStation / C37.118 / TRDP / GE EGD / Meshtastic; Wireshark KNX/OPC UA/NFS/Git; Scapy DoIP/Zigbee/IPFIX.

## Chinese-app / vendor

Public material exists for WeChat, DingTalk, Weibo, Aliyun, Xiaomi, Tuya. **cn-vendor (33)** still has zero files; those names are private/Colasoft-only by roadmap source tags.

## How to grow further without writing dissectors

1. Keep using `sources.json` + `go run ./tools/generate -fetch`.
2. Prefer one small capture per new roadmap name; skip duplicate HTTP/TLS/QUIC floods and Git LFS pointer files.
3. Keep the upstream LICENSE or README next to the capture (`licenses/`).
4. Microsoft named RPC interfaces still need a capture that tshark labels as those DCE/RPC pipes, not a generic `dcerpc` frame already mapped to DCE/RPC.
