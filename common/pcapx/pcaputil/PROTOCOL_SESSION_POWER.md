# Power-industry ProtocolSession candidates

These SCADA/utility protocols are **not** already on ProtocolSession except
Modbus TCP (already admitted). Catalog / roadmap rows stay `partial` / `new` /
`todo`. This file does not mark anything `done`. Standard ports and EtherTypes
are not protocol truth.

| Protocol | First-version profile | Spec | Must-have | Not complete |
|---|---|---|---|---|
| IEC 60870-5-104 | APDU `0x68` + length | IEC 60870-5-104 | U-format STARTDT/STOPDT/TESTFR; I-format TypeID, COT, IOA, send/recv sequence | IEC 101/102/103 serial; file transfer; balanced/unbalanced 101 |
| DNP3 | IEEE 1815 link `05 64` | IEEE 1815 | Link dest/src, header CRC fail-closed, application request/response pairing | Secure Authentication; UDP/broadcast; full object parsing |
| IEEE C37.118 | Synchrophasor frames | IEEE C37.118-2005 / C37.118.2-2011 | SYNC, framesize, CRC; CMD; CFG-2 or DATA; DATA without CFG is ContextRequired | CFG-3; scaled engineering units; live PMU validity |
| IEC 61850 GOOSE | L2 GOOSE PDU | IEC 61850-8-1 | APPID/length, stNum/sqNum, dataset identity, captured BOOLEAN/BIT STRING | MMS/SCL object model; SV; protection-trip timing as network truth |

Modbus TCP remains the previously admitted companion protocol and is not reimplemented here.
