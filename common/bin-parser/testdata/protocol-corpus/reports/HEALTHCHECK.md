# Protocol sample health check

Material availability only. This does **not** mark YAML dissectors complete.

Generated against `ProtocolRoadmap` (616 names) and the vendored corpus in this directory.

## Snapshot

| Metric | Before this branch | After this branch |
| --- | ---: | ---: |
| Capture files | 174 | 212 |
| Packets | 41828 | 46024 |
| Unique roadmap protocols with a capture | 156 | 165 |
| Outside-roadmap candidate captures | 10 | 24 |
| Roadmap `done` / `todo` | 211 / 405 | 211 / 405 (unchanged) |

Sources: nDPI, Wireshark `test/captures`, tcpdump tests, Google CTF, **Scapy** (GPL-2.0). Wiki SampleCaptures and ICS-pcap LFS dumps were **indexed, not vendored** (no uniform attachment license).

## Family coverage (after)

Worst licensed-sample deserts:

| Family | Roadmap | With capture | Note |
| --- | ---: | ---: | --- |
| `cn-vendor` | 33 | **0** | Huawei/H3C/Sangfor/Hikvision/GB28181 etc. are `srcPrivate`; no licensed public PCAP in nDPI/Wireshark tests |
| `microsoft` | 23 | **0** | MSRPC subsets exist under `mq-rpc`/`auth`; named Microsoft product protocols still empty |
| `service-tools` | 66 | 5 | Jenkins/Zabbix/… mostly still missing |
| `longtail` | 40 | 3 | IPX now has a second capture |
| `ics` | 46 | **21** | was 17; added GE EGD, LoRa/Meshtastic, **DoIP**, **Zigbee** (Scapy) |

## ICS remaining without a licensed capture

Still `missing` after nDPI 629 default pcaps, Wireshark 136 test captures, and Scapy 17 test pcaps:

Modbus RTU/ASCII, COTP, TPKT, IEC 60870-5-101, IEC 61850 GOOSE, IEC 61850 SV, OPC DA, Profinet DCP, PROFIBUS, EtherCAT, Powerlink, SERCOS III, CC-Link IE, CODESYS, FF HSE, LonTalk, DALI, J1939, UDS, VARAN, AES50, Z-Wave, IEC 62056, M-Bus, DALI-2.

They do not appear as named files in the two licensed trees. Do not invent hex or pull unlicensed wiki attachments.

## ICS / rare captures added this round

From nDPI (LGPL-3.0, commit `4cae778e`):

- EtherNet/IP CIP **explicit** (`ethernet_ip-cip.pcap`) in addition to existing CIP I/O
- Schneider **UMAS**, Triconex **TriStation**, IEEE **C37.118**, **TRDP**, EtherS-Bus, EtherSIO, **HiSLIP**, Veeder-Root **ATG**, GE **EGD** (mapped to GE SRTP with an honest note)
- Matter, Meshtastic (LoRa mesh; mapped to LoRaWAN with an honest note)
- XMPP/Jabber, eDonkey, Weibo, Aliyun, Xiaomi IoT, Tuya

From Wireshark tests (GPL-2.0-or-later):

- KNX/IP DataSec / SecureWrapper / TimerNotify, OPC UA signed, NFS, Git daemon, SIP, TFTP, NVMe-oF discovery, IPX RIP, DHCP, NTP

From Scapy (GPL-2.0, commit `6f158c0c`):

- **DoIP** ACK, **Zigbee** join + SKKE, IPFIX, NetFlow v9, PFCP

## Chinese-app / vendor

Public licensed material exists for WeChat (already in corpus), DingTalk (already), Weibo, Aliyun, Xiaomi, Tuya, Line/Kakao (Kakao not added this round). **cn-vendor (33)** still has zero vendorable files; those names are private/Colasoft-only by roadmap source tags.

## How to grow further without writing dissectors

1. Keep using `sources.json` + `go run ./tools/generate -fetch`.
2. Prefer nDPI `tests/cfgs/default/pcap` and Wireshark `test/captures` (explicit repo license).
3. Skip Wireshark wiki SampleCaptures and automayt/ICS-pcap until each file has a redistributable license.
4. Filter: one small capture per new roadmap name; skip duplicate HTTP/TLS/QUIC floods.
