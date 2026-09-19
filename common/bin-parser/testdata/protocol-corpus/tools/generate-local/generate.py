#!/usr/bin/env python3
"""Synthesize tiny identification PCAPs; keep only tshark-positive frames."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

from scapy.all import (
    ARP,
    CookedLinux,
    CookedLinuxV2,
    DNS,
    DNSQR,
    DNSRR,
    Dot1Q,
    EAP,
    EAPOL,
    Ether,
    GRE,
    ICMP,
    IP,
    IPOption_EOL,
    IPerror,
    IPv6,
    LLC,
    Loopback,
    NTP,
    Padding,
    Raw,
    SNAP,
    STP,
    TCP,
    UDP,
    conf,
    wrpcap,
)
from scapy.layers.inet6 import (
    ICMPv6EchoRequest,
    ICMPv6MLReport,
    ICMPv6ND_NA,
    ICMPv6ND_NS,
    ICMPv6ND_RA,
    ICMPv6NDOptSrcLLAddr,
    IPv6ExtHdrDestOpt,
    IPv6ExtHdrFragment,
    IPv6ExtHdrHopByHop,
    IPv6ExtHdrRouting,
)
from scapy.layers.l2 import Dot3
from scapy.packet import Packet

conf.verb = 0

HERE = Path(__file__).resolve().parent
CORPUS = HERE.parents[1]
OUT_DIR = CORPUS / "captures" / "generated-local"
INDEX_PATH = HERE / "generated-index.json"
TSHARK = os.environ.get("TSHARK", "tshark")
MAC_A = "02:00:00:00:00:01"
MAC_B = "02:00:00:00:00:02"
MAC_STP = "01:80:c2:00:00:00"
MAC_SLOW = "01:80:c2:00:00:02"
IP_A = "10.0.0.1"
IP_B = "10.0.0.2"
IP6_A = "2001:db8::1"
IP6_B = "2001:db8::2"

RECIPES: list[tuple[str, str, str, object]] = []


def recipe(cap_id: str, roadmap: str, display_filter: str):
    def deco(fn):
        RECIPES.append((cap_id, roadmap, display_filter, fn))
        return fn

    return deco


def eth(payload: Packet) -> Packet:
    return Ether(src=MAC_A, dst=MAC_B) / payload


def tcp_talk(dport: int, payload: bytes, sport: int = 40100) -> list[Packet]:
    syn = eth(IP(src=IP_A, dst=IP_B) / TCP(sport=sport, dport=dport, flags="S", seq=1))
    synack = eth(IP(src=IP_B, dst=IP_A) / TCP(sport=dport, dport=sport, flags="SA", seq=1000, ack=2))
    ack = eth(IP(src=IP_A, dst=IP_B) / TCP(sport=sport, dport=dport, flags="A", seq=2, ack=1001))
    data = eth(
        IP(src=IP_A, dst=IP_B)
        / TCP(sport=sport, dport=dport, flags="PA", seq=2, ack=1001)
        / Raw(payload)
    )
    return [syn, synack, ack, data]


def udp(dport: int, payload: bytes | Packet, sport: int = 40101) -> Packet:
    inner = payload if isinstance(payload, Packet) else Raw(payload)
    return eth(IP(src=IP_A, dst=IP_B) / UDP(sport=sport, dport=dport) / inner)


def tshark_first(pcap: Path, display_filter: str) -> tuple[int, list[str]]:
    cmd = [TSHARK, "-r", str(pcap), "-Y", display_filter, "-T", "fields", "-e", "frame.number", "-e", "frame.protocols"]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    if proc.returncode != 0 and not proc.stdout.strip():
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or "tshark failed")
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t", 1)
        number = int(parts[0])
        protos = parts[1].split(":") if len(parts) == 2 and parts[1] else []
        return number, protos
    return 0, []


# --- link ---

@recipe("gen-ethernet-ii", "Ethernet II", "eth.type == 0x0800")
def ethernet_ii():
    return [eth(IP(src=IP_A, dst=IP_B) / ICMP())]


@recipe("gen-ethernet-8023", "Ethernet 802.3", "llc")
def ethernet_8023():
    return [Dot3(src=MAC_A, dst=MAC_B) / LLC(dsap=0xAA, ssap=0xAA, ctrl=3) / SNAP() / Raw(b"snap")]


@recipe("gen-ethernet-snap", "Ethernet SNAP", "snap")
def ethernet_snap():
    return [Dot3(src=MAC_A, dst=MAC_B) / LLC(dsap=0xAA, ssap=0xAA, ctrl=3) / SNAP(OUI=0, code=0x0800) / IP() / ICMP()]


@recipe("gen-ieee8021q", "IEEE 802.1Q", "vlan")
def ieee8021q():
    return [Ether(src=MAC_A, dst=MAC_B) / Dot1Q(vlan=100) / IP(src=IP_A, dst=IP_B) / ICMP()]


@recipe("gen-qinq", "IEEE 802.1ad QinQ", "vlan.id == 200")
def qinq():
    return [Ether(src=MAC_A, dst=MAC_B) / Dot1Q(vlan=100, type=0x8100) / Dot1Q(vlan=200) / IP() / ICMP()]


@recipe("gen-rarp", "RARP", "arp.opcode == 3")
def rarp():
    return [Ether(src=MAC_A, dst="ff:ff:ff:ff:ff:ff") / ARP(op=3, hwsrc=MAC_A, psrc=IP_A, hwdst="00:00:00:00:00:00", pdst=IP_A)]


@recipe("gen-llc", "LLC", "llc")
def llc():
    return [Dot3(src=MAC_A, dst=MAC_B) / LLC(dsap=0x42, ssap=0x42, ctrl=3) / Raw(b"\x00\x00")]


@recipe("gen-snap", "SNAP", "snap")
def snap():
    return [Dot3(src=MAC_A, dst=MAC_B) / LLC(dsap=0xAA, ssap=0xAA, ctrl=3) / SNAP(OUI=0x00c000, code=0x2000) / Raw(b"cdp")]


@recipe("gen-stp", "STP", "stp")
def stp():
    return [Ether(src=MAC_A, dst=MAC_STP, type=None) / LLC(dsap=0x42, ssap=0x42, ctrl=3) / STP()]


@recipe("gen-lacp", "LACP", "lacp")
def lacp():
    from scapy.contrib.lacp import LACP, SlowProtocol

    return [Ether(src=MAC_A, dst=MAC_SLOW) / SlowProtocol() / LACP()]


@recipe("gen-eapol", "EAPOL", "eapol")
def eapol():
    return [Ether(src=MAC_A, dst=MAC_B, type=0x888E) / EAPOL(version=1, type=0) / EAP(code=1, type=1)]


@recipe("gen-eap", "EAP", "eap")
def eap():
    return [Ether(src=MAC_A, dst=MAC_B, type=0x888E) / EAPOL() / EAP(code=2, type=1, identity=b"user")]


@recipe("gen-ieee8021x", "IEEE 802.1X", "eapol")
def ieee8021x():
    return [Ether(src=MAC_A, dst=MAC_B, type=0x888E) / EAPOL(type=3) / Raw(b"\x01\x00\x00\x00")]


@recipe("gen-loopback", "Loopback", "frame")
def loopback():
    return [Loopback() / IP(src="127.0.0.1", dst="127.0.0.1") / ICMP()]


@recipe("gen-linux-sll", "Linux SLL", "sll")
def linux_sll():
    return [CookedLinux(pkttype=0, lladdr="02:00:00:00:00:01") / IP(src=IP_A, dst=IP_B) / ICMP()]


@recipe("gen-linux-sll2", "Linux SLL2", "sll2")
def linux_sll2():
    return [CookedLinuxV2(proto=0x0800, ifindex=1, lladdr="02:00:00:00:00:01") / IP(src=IP_A, dst=IP_B) / ICMP()]


@recipe("gen-ieee80211", "IEEE 802.11", "wlan")
def ieee80211():
    from scapy.layers.dot11 import Dot11, Dot11Beacon, Dot11Elt, RadioTap

    return [
        RadioTap()
        / Dot11(type=0, subtype=8, addr1="ff:ff:ff:ff:ff:ff", addr2=MAC_A, addr3=MAC_A)
        / Dot11Beacon(cap=0x1104)
        / Dot11Elt(ID="SSID", info=b"lab")
    ]


@recipe("gen-wep", "WEP", "wep")
def wep():
    from scapy.layers.dot11 import Dot11, Dot11WEP, RadioTap

    return [RadioTap() / Dot11(type=2, addr1=MAC_B, addr2=MAC_A, addr3=MAC_A) / Dot11WEP(iv=b"\x00\x00\x01", wepdata=b"\x00" * 20, icv=0)]


@recipe("gen-cdp", "CDP", "cdp")
def cdp():
    from scapy.contrib.cdp import CDPAddrRecordIPv4, CDPMsgDeviceID, CDPMsgSoftwareVersion, CDPv2_HDR

    return [
        Ether(src=MAC_A, dst="01:00:0c:cc:cc:cc")
        / LLC(dsap=0xAA, ssap=0xAA, ctrl=3)
        / SNAP(OUI=0x00000C, code=0x2000)
        / CDPv2_HDR()
        / CDPMsgDeviceID(val=b"lab-switch")
        / CDPMsgSoftwareVersion(val=b"1.0")
        / CDPAddrRecordIPv4(addr="10.0.0.1")
    ]


@recipe("gen-slow-protocols", "IEEE 802.3 Slow Protocols", "slow")
def slow_protocols():
    from scapy.contrib.lacp import LACPMarkerProtocol, SlowProtocol

    return [Ether(src=MAC_A, dst=MAC_SLOW) / SlowProtocol(subtype=2) / LACPMarkerProtocol()]


@recipe("gen-macsec", "MACSec", "macsec")
def macsec():
    from scapy.contrib.macsec import MACsecSA, MACsec

    pkt = Ether(src=MAC_A, dst=MAC_B, type=0x88E5) / MACsec(an=0, PN=1, SCI=b"\x02\x00\x00\x00\x00\x01\x00\x01") / Raw(b"enc")
    return [pkt]


# --- internet ---

@recipe("gen-icmpv6", "ICMPv6", "icmpv6")
def icmpv6():
    return [eth(IPv6(src=IP6_A, dst=IP6_B) / ICMPv6EchoRequest(data=b"ping"))]


@recipe("gen-icmpv6-ndp", "ICMPv6 NDP", "icmpv6.type == 135")
def icmpv6_ndp():
    return [eth(IPv6(src=IP6_A, dst="ff02::1:ff00:2") / ICMPv6ND_NS(tgt=IP6_B) / ICMPv6NDOptSrcLLAddr(lladdr=MAC_A))]


@recipe("gen-icmpv6-mld", "ICMPv6 MLD", "icmpv6.type == 131")
def icmpv6_mld():
    return [eth(IPv6(src=IP6_A, dst="ff02::16") / ICMPv6MLReport())]


@recipe("gen-ipv6-hbh", "IPv6 Hop-by-Hop", "ipv6.nxt == 0")
def ipv6_hbh():
    return [eth(IPv6(src=IP6_A, dst=IP6_B) / IPv6ExtHdrHopByHop() / ICMPv6EchoRequest())]


@recipe("gen-ipv6-routing", "IPv6 Routing Header", "ipv6.nxt == 43")
def ipv6_routing():
    return [eth(IPv6(src=IP6_A, dst=IP6_B) / IPv6ExtHdrRouting(addresses=[IP6_B]) / ICMPv6EchoRequest())]


@recipe("gen-ipv6-frag", "IPv6 Fragment", "ipv6.nxt == 44")
def ipv6_frag():
    return [eth(IPv6(src=IP6_A, dst=IP6_B) / IPv6ExtHdrFragment(offset=0, m=1, id=1) / Raw(b"A" * 32))]


@recipe("gen-ipv6-dstopts", "IPv6 Destination Options", "ipv6.nxt == 60")
def ipv6_dstopts():
    return [eth(IPv6(src=IP6_A, dst=IP6_B) / IPv6ExtHdrDestOpt() / ICMPv6EchoRequest())]


@recipe("gen-igmp", "IGMP", "igmp")
def igmp():
    from scapy.contrib.igmp import IGMP

    return [eth(IP(src=IP_A, dst="224.0.0.1", proto=2, ttl=1) / IGMP(type=0x11))]


@recipe("gen-ipip", "IPIP", "ip.proto == 4")
def ipip():
    return [eth(IP(src=IP_A, dst=IP_B, proto=4) / IP(src="192.0.2.1", dst="192.0.2.2") / ICMP())]


@recipe("gen-mpls", "MPLS", "mpls")
def mpls():
    from scapy.contrib.mpls import MPLS

    return [Ether(src=MAC_A, dst=MAC_B, type=0x8847) / MPLS(label=16, s=1) / IP() / ICMP()]


@recipe("gen-geneve", "Geneve", "geneve")
def geneve():
    from scapy.contrib.geneve import GENEVE

    return [udp(6081, GENEVE(vni=100) / IP(src="192.0.2.1", dst="192.0.2.2") / ICMP())]


@recipe("gen-l2tp", "L2TP", "l2tp")
def l2tp():
    from scapy.layers.l2tp import L2TP

    return [udp(1701, L2TP())]


@recipe("gen-6to4", "6to4", "ip.proto == 41")
def sixto4():
    return [eth(IP(src="192.88.99.1", dst="192.88.99.2", proto=41) / IPv6(src=IP6_A, dst=IP6_B) / ICMPv6EchoRequest())]


@recipe("gen-icmp-ts", "ICMP Timestamp", "icmp.type == 13")
def icmp_timestamp():
    return [eth(IP(src=IP_A, dst=IP_B) / ICMP(type=13, ts_ori=1, ts_rx=2, ts_tx=3))]


# --- routing ---

@recipe("gen-rip", "RIP", "rip")
def rip():
    from scapy.layers.rip import RIP, RIPEntry

    return [udp(520, RIP(cmd=2, version=2) / RIPEntry(addr="10.1.0.0", mask="255.255.255.0", nextHop=IP_A))]


@recipe("gen-ripng", "RIPng", "ripng")
def ripng():
    from scapy.layers.rip import RIP

    # RIPng is UDP/521 with IPv6; scapy RIP over IPv6
    pkt = Ether(src=MAC_A, dst=MAC_B) / IPv6(src=IP6_A, dst="ff02::9") / UDP(sport=521, dport=521) / RIP(cmd=1, version=1)
    return [pkt]


@recipe("gen-ospfv3", "OSPFv3", "ospf")
def ospfv3():
    from scapy.contrib.ospf import OSPFv3_Hdr, OSPFv3_Hello

    return [eth(IPv6(src=IP6_A, dst="ff02::5") / OSPFv3_Hdr() / OSPFv3_Hello())]


@recipe("gen-eigrp", "EIGRP", "eigrp")
def eigrp():
    from scapy.contrib.eigrp import EIGRP, EIGRPParam, EIGRPSwVer

    return [eth(IP(src=IP_A, dst="224.0.0.10", proto=88, ttl=2) / EIGRP(opcode=5, asn=100) / EIGRPParam() / EIGRPSwVer())]


@recipe("gen-isis", "IS-IS", "isis")
def isis():
    from scapy.contrib.isis import ISIS_CommonHdr, ISIS_Hello, ISIS_P2P_Hello

    return [
        Ether(src=MAC_A, dst="01:80:c2:00:00:14", type=None)
        / LLC(dsap=0xFE, ssap=0xFE, ctrl=3)
        / ISIS_CommonHdr(pdutype=17)
        / ISIS_P2P_Hello(sourceid="0100.0100.0100.00")
    ]


@recipe("gen-carp", "CARP", "carp")
def carp():
    # CARP shares VRRP IP proto 112; craft VRRP-like with carp ethertype if needed
    from scapy.layers.vrrp import VRRPv3

    return [eth(IP(src=IP_A, dst="224.0.0.18", proto=112, ttl=255) / VRRPv3(vrid=1, priority=100, addrlist=[IP_A]))]


@recipe("gen-rsvp", "RSVP", "rsvp")
def rsvp():
    from scapy.contrib.rsvp import RSVP, RSVP_Object, RSVP_HOP

    return [eth(IP(src=IP_A, dst=IP_B, proto=46, tos=0xC0) / RSVP() / RSVP_HOP())]


@recipe("gen-olsr", "OLSR", "olsr")
def olsr():
    from scapy.contrib.olsr import OLSR, OLSR_Hello, OLSR_HelloMsg

    return [udp(698, OLSR() / OLSR_Hello() / OLSR_HelloMsg(iface_addr=IP_A))]


@recipe("gen-babel", "Babel", "babel")
def babel():
    from scapy.contrib.babel import Babel, BabelHello

    return [Ether(src=MAC_A, dst=MAC_B) / IPv6(src=IP6_A, dst="ff02::1:6") / UDP(sport=6696, dport=6696) / Babel() / BabelHello()]


# --- transport / name ---

@recipe("gen-dccp", "DCCP", "dccp")
def dccp():
    from scapy.layers.dccp import DCCP

    return [eth(IP(src=IP_A, dst=IP_B, proto=33) / DCCP(dport=80, sport=4000, dataofs=6))]


@recipe("gen-udplite", "UDP-Lite", "udplite")
def udplite():
    from scapy.layers.inet import UDP

    # UDP-Lite is proto 136
    return [eth(IP(src=IP_A, dst=IP_B, proto=136) / UDP(sport=1234, dport=1234) / Raw(b"lite"))]


@recipe("gen-nbns", "NBNS", "nbns")
def nbns():
    from scapy.layers.netbios import NBNSHeader, NBNSQueryRequest

    return [udp(137, NBNSHeader() / NBNSQueryRequest(QUESTION_NAME=b"WORKSTATION"))]


@recipe("gen-nbt-ns", "NBT NS", "nbns")
def nbt_ns():
    from scapy.layers.netbios import NBNSHeader, NBNSQueryRequest

    return [udp(137, NBNSHeader() / NBNSQueryRequest(QUESTION_NAME=b"FILESHARE"))]


@recipe("gen-nbt-ns-resp", "NBT-NS response", "nbns.flags.opcode == 0")
def nbt_ns_resp():
    from scapy.layers.netbios import NBNSHeader, NBNSQueryResponse

    return [udp(137, NBNSHeader(FLAGS=0x8500) / NBNSQueryResponse(RR_NAME=b"HOST", ADDR=IP_A), sport=137)]


@recipe("gen-nbt-dg", "NBT DG", "nbdgm")
def nbt_dg():
    from scapy.layers.netbios import NBTDatagram

    return [udp(138, NBTDatagram(SourceName=b"SRC", DestinationName=b"DST") / Raw(b"hello"))]


@recipe("gen-nbt-ss", "NBT SS", "nbss")
def nbt_ss():
    # NetBIOS session message header: type 0x00, length
    payload = bytes([0x81, 0x00, 0x00, 0x44]) + b"\x20" + b"ENAME           " + b"\x00" + b"\x20" + b"CALLED          " + b"\x00"
    return tcp_talk(139, payload)


@recipe("gen-llmnr", "LLMNR", "llmnr")
def llmnr():
    return [udp(5355, DNS(rd=1, qd=DNSQR(qname="host.local")))]


@recipe("gen-llmnr-resp", "LLMNR response", "llmnr.flags.response == 1")
def llmnr_resp():
    q = DNSQR(qname="host.local")
    return [udp(5355, DNS(id=1, qr=1, aa=1, qd=q, an=DNSRR(rrname="host.local", ttl=30, rdata=IP_A)), sport=5355)]


@recipe("gen-llmnr-mdns", "LLMNR-MDNS collision", "llmnr or mdns")
def llmnr_mdns():
    llm = udp(5355, DNS(rd=1, qd=DNSQR(qname="conflict.local")))
    mdn = eth(IP(src=IP_A, dst="224.0.0.251") / UDP(sport=5353, dport=5353) / DNS(rd=0, qd=DNSQR(qname="conflict.local")))
    return [llm, mdn]


@recipe("gen-dhcpv6", "DHCPv6", "dhcpv6")
def dhcpv6():
    from scapy.layers.dhcp6 import DHCP6_Solicit, DHCP6OptClientId, DUID_LL

    pkt = (
        Ether(src=MAC_A, dst="33:33:00:01:00:02")
        / IPv6(src=IP6_A, dst="ff02::1:2")
        / UDP(sport=546, dport=547)
        / DHCP6_Solicit(trid=1)
        / DHCP6OptClientId(duid=DUID_LL(lladdr=MAC_A))
    )
    return [pkt]


@recipe("gen-dhcpv6-reply", "DHCPv6 server exchange", "dhcpv6.msgtype == 7")
def dhcpv6_reply():
    from scapy.layers.dhcp6 import DHCP6_Reply, DHCP6OptClientId, DUID_LL

    pkt = (
        Ether(src=MAC_B, dst=MAC_A)
        / IPv6(src=IP6_B, dst=IP6_A)
        / UDP(sport=547, dport=546)
        / DHCP6_Reply(trid=1)
        / DHCP6OptClientId(duid=DUID_LL(lladdr=MAC_A))
    )
    return [pkt]


@recipe("gen-ipv6-ra", "IPv6 RA", "icmpv6.type == 134")
def ipv6_ra():
    return [eth(IPv6(src=IP6_A, dst="ff02::1") / ICMPv6ND_RA(prf=1, routerlifetime=1800))]


@recipe("gen-sntp", "SNTP", "ntp")
def sntp():
    return [udp(123, NTP(mode=3))]


# --- mgmt / auth / db / storage / tools ---

@recipe("gen-netflow-v5", "NetFlow v5", "cflow")
def netflow_v5():
    from scapy.layers.netflow import NetflowHeader, NetflowHeaderV5, NetflowRecordV5

    return [udp(2055, NetflowHeader(version=5) / NetflowHeaderV5(count=1) / NetflowRecordV5(src=IP_A, dst=IP_B, nexthop=IP_A, srcport=1234, dstport=80))]


@recipe("gen-snmpv3", "SNMPv3", "snmp")
def snmpv3():
    # SNMPv3 header: version=3
    # RFC 3412: version 3 then globalData
    raw = bytes(
        [
            0x30, 0x3A, 0x02, 0x01, 0x03, 0x30, 0x0F, 0x02, 0x01, 0x00, 0x02, 0x02, 0x00, 0xFF,
            0x04, 0x01, 0x00, 0x02, 0x01, 0x03, 0x04, 0x0C, 0x04, 0x00, 0x02, 0x01, 0x00, 0x02,
            0x01, 0x00, 0x04, 0x00, 0x04, 0x00, 0x04, 0x00, 0x30, 0x16, 0x04, 0x00, 0x04, 0x00,
            0xA0, 0x10, 0x02, 0x01, 0x01, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x30, 0x05, 0x30,
            0x03, 0x06, 0x01, 0x00,
        ]
    )
    return [udp(161, raw)]


@recipe("gen-tacacs-plus", "TACACS+", "tacacs")
def tacacs_plus():
    # TACACS+ header: version 0xc0, type auth=1, seq=1, flags=0, session, length
    hdr = bytes([0xC0, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x08]) + b"\x00" * 8
    return tcp_talk(49, hdr)


@recipe("gen-socks4", "SOCKS4", "socks")
def socks4():
    # VER=4, CMD=1 connect, port 80, ip 1.2.3.4, userid
    payload = bytes([0x04, 0x01, 0x00, 0x50, 1, 2, 3, 4]) + b"user\x00"
    return tcp_talk(1080, payload)


@recipe("gen-ldap", "LDAP", "ldap")
def ldap():
    # LDAP BindRequest anonymous: SEQUENCE
    # 30 0c 02 01 01 60 07 02 01 03 04 00 80 00
    payload = bytes.fromhex("300c020101600702010304008000")
    return tcp_talk(389, payload)


@recipe("gen-cldap", "CLDAP", "cldap")
def cldap():
    payload = bytes.fromhex("300c020101600702010304008000")
    return [udp(389, payload)]


@recipe("gen-redis", "Redis", "redis")
def redis():
    return tcp_talk(6379, b"*1\r\n$4\r\nPING\r\n")


@recipe("gen-memcache-bin", "Memcache binary", "memcache")
def memcache_bin():
    # magic 0x80 request, opcode GET=0x00, keylen 3, extras 0, datatype 0, status 0, bodylen 3
    key = b"foo"
    hdr = bytes([0x80, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0, 0]) + key
    return tcp_talk(11211, hdr)


@recipe("gen-zookeeper", "ZooKeeper", "zookeeper")
def zookeeper():
    # connect request: proto=0, zxid=0, timeout=10000, session=0, passwd=16zeros, len prefix
    body = (b"\x00\x00\x00\x00" + b"\x00" * 8 + (10000).to_bytes(4, "big") + b"\x00" * 8 + b"\x00" * 16)
    pkt = len(body).to_bytes(4, "big") + body
    return tcp_talk(2181, pkt)


@recipe("gen-etcd", "ETCD", "http")
def etcd():
    req = b"GET /v3/version HTTP/1.1\r\nHost: 127.0.0.1:2379\r\nUser-Agent: etcdctl\r\n\r\n"
    return tcp_talk(2379, req)


@recipe("gen-iscsi", "iSCSI", "iscsi")
def iscsi():
    # iSCSI login request BHS: opcode 0x03, version max/min, isid, tsih, itt, cid
    bhs = bytearray(48)
    bhs[0] = 0x43  # immediate login
    bhs[4] = 0x00
    bhs[5] = 0x00
    data = bytes(bhs) + b"InitiatorName=iqn.2024-01.lab:cli\x00TargetName=iqn.2024-01.lab:tgt\x00"
    # AHS+data length in bytes 5-7
    dlen = len(data) - 48
    data = bytearray(data)
    data[5:8] = dlen.to_bytes(3, "big")
    return tcp_talk(3260, bytes(data))


@recipe("gen-nbd", "NBD", "nbd")
def nbd():
    # newstyle handshake: "NBDMAGIC" + "IHAVEOPT"
    return tcp_talk(10809, b"NBDMAGIC" + b"IHAVEOPT" + b"\x00\x03")


@recipe("gen-aoe", "AoE", "aoe")
def aoe():
    # AoE query config: ethertype 0x88A2
    hdr = bytes([0x10, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00])
    return [Ether(src=MAC_A, dst="ff:ff:ff:ff:ff:ff", type=0x88A2) / Raw(hdr)]


@recipe("gen-minio-s3", "MinIO/S3", "http.request.uri contains \"amazonaws\" or http.request.uri contains \"/bucket\"")
def minio_s3():
    req = b"GET /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIA/20200101/us-east-1/s3/aws4_request\r\n\r\n"
    return tcp_talk(9000, req)


@recipe("gen-docker-api", "Docker API", "http.request.uri contains \"/containers\"")
def docker_api():
    req = b"GET /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\n\r\n"
    return tcp_talk(2375, req)


@recipe("gen-jdwp", "JDWP handshake", "tcp.payload contains \"JDWP-Handshake\"")
def jdwp():
    return tcp_talk(5005, b"JDWP-Handshake")


@recipe("gen-qmp", "QEMU QMP", "json")
def qmp():
    return tcp_talk(4444, b'{"QMP": {"version": {"qemu": {"major": 8}}, "capabilities": []}}\r\n')


@recipe("gen-nrpe", "Nagios NRPE", "nrpe")
def nrpe():
    # NRPE packet version 3/2: version=2, type=1 query, crc, result, buffer
    import struct, zlib

    buf = b"_NRPE_CHECK\x00" + b"\x00" * (1024 - 12)
    pkt = struct.pack("!hhIh", 2, 1, 0, 0) + buf
    crc = zlib.crc32(pkt) & 0xFFFFFFFF
    pkt = struct.pack("!hhIh", 2, 1, crc, 0) + buf
    return tcp_talk(5666, pkt)


@recipe("gen-java-ser", "Java serialization", "tcp.payload[0:2] == 0xaced")
def java_ser():
    return tcp_talk(1099, bytes.fromhex("aced0005757200135b4c6a6176612e6c616e672e537472696e673b"))


@recipe("gen-pickle", "Python pickle", "tcp")
def pickle_pkt():
    import pickle

    return tcp_talk(11311, pickle.dumps({"k": "v"}, protocol=4))


@recipe("gen-php-ser", "PHP serialize", "tcp")
def php_ser():
    return tcp_talk(9001, b'a:1:{s:3:"foo";s:3:"bar";}')


@recipe("gen-jsonrpc", "JSON-RPC 2.0", "json")
def jsonrpc():
    body = b'{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}'
    http = b"POST /rpc HTTP/1.1\r\nHost: lab\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n" % len(body) + body
    return tcp_talk(8080, http)


@recipe("gen-mqttsn", "MQTT-SN", "mqttsn or mqtt")
def mqttsn():
    # MQTT-SN CONNECT: len, msgtype=0x04, flags, protoid, duration, clientid
    cid = b"lab"
    pkt = bytes([6 + len(cid), 0x04, 0x00, 0x01, 0x00, 0x1E]) + cid
    return [udp(1883, pkt)]


@recipe("gen-portmap", "Portmap/Rpcbind", "rpc or portmap")
def portmap():
    from scapy.layers.rpc import RPCcall, RPCcallHeader

    # SUNRPC CALL GETPORT
    try:
        from scapy.layers.rpc import RPC

        return [udp(111, RPC(xid=1, mtype=0) / RPCcall(rpcvers=2, prog=100000, vers=2, proc=3))]
    except Exception:
        # xid, msgtype=call, rpcvers=2, program=100000, vers=2, proc=3, creds/verf null
        raw = (
            b"\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x02"
            + (100000).to_bytes(4, "big")
            + b"\x00\x00\x00\x02\x00\x00\x00\x03"
            + b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
        )
        return [udp(111, raw)]


@recipe("gen-onc-rpc", "ONC RPC", "rpc")
def onc_rpc():
    raw = (
        b"\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00\x02"
        + (100003).to_bytes(4, "big")  # nfs
        + b"\x00\x00\x00\x03\x00\x00\x00\x00"
        + b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
    )
    return [udp(2049, raw)]


@recipe("gen-rpc", "RPC", "rpc")
def rpc_generic():
    raw = (
        b"\x00\x00\x00\x03\x00\x00\x00\x00\x00\x00\x00\x02"
        + (100005).to_bytes(4, "big")  # mountd
        + b"\x00\x00\x00\x03\x00\x00\x00\x01"
        + b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
    )
    return [udp(20048, raw)]


@recipe("gen-lpd", "LPD", "lpd")
def lpd():
    return tcp_talk(515, b"\x02printer\n")


@recipe("gen-ftp-data", "FTP-DATA", "ftp-data")
def ftp_data():
    return tcp_talk(20, b"file-bytes-from-retr\n")


@recipe("gen-turn", "TURN", "turn or stun")
def turn():
    # STUN/TURN Allocate request: type 0x0003, length, magic cookie
    msg = bytes.fromhex("00030000000000002112a442") + os.urandom(12)
    return [udp(3478, msg)]


@recipe("gen-sdp", "SDP", "sdp")
def sdp():
    body = b"v=0\r\no=- 0 0 IN IP4 10.0.0.1\r\ns=Talk\r\nc=IN IP4 10.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"
    sip = (
        b"INVITE sip:b@10.0.0.2 SIP/2.0\r\nVia: SIP/2.0/UDP 10.0.0.1\r\nFrom: <sip:a@10.0.0.1>;tag=1\r\n"
        b"To: <sip:b@10.0.0.2>\r\nCall-ID: 1@lab\r\nCSeq: 1 INVITE\r\nContent-Type: application/sdp\r\n"
        b"Content-Length: %d\r\n\r\n" % len(body)
        + body
    )
    return [udp(5060, sip)]


@recipe("gen-modbus-rtu", "Modbus RTU/ASCII", "mbrtu or modbus")
def modbus_rtu():
    # RTU: addr=1, func=3, start=0, qty=1, crc
    import struct

    pdu = bytes([0x01, 0x03, 0x00, 0x00, 0x00, 0x01])

    def crc16(data: bytes) -> int:
        crc = 0xFFFF
        for b in data:
            crc ^= b
            for _ in range(8):
                crc = (crc >> 1) ^ 0xA001 if crc & 1 else crc >> 1
        return crc

    frame = pdu + struct.pack("<H", crc16(pdu))
    return [eth(IP() / UDP(sport=1502, dport=1502) / Raw(frame))]


@recipe("gen-j1939", "J1939", "j1939 or can")
def j1939():
    from scapy.layers.can import CAN

    # 29-bit J1939 id: PGN 0xF004 (EEC1)
    canid = 0x18F00400
    return [CAN(identifier=canid, flags="extended", length=8, data=b"\x00" * 8)]


@recipe("gen-uds", "UDS", "uds or isotp")
def uds():
    from scapy.contrib.automotive.uds import UDS, UDS_DSC

    try:
        pkt = eth(IP() / UDP(sport=13400, dport=13400) / UDS() / UDS_DSC(diagnosticSessionType=1))
        return [pkt]
    except Exception:
        # UDS tester present 0x3E 0x00 over ISO-TP single frame
        return [eth(IP() / UDP(sport=13400, dport=13400) / Raw(b"\x02\x3E\x00"))]


@recipe("gen-mbus", "M-Bus", "mbus")
def mbus():
    # wireless M-Bus or serial start 0x10 / 0x68
    frame = bytes.fromhex("1040fe16")
    return [eth(IP() / UDP(sport=10000, dport=10000) / Raw(frame))]


@recipe("gen-lontalk", "LonTalk", "lon")
def lontalk():
    return [Ether(src=MAC_A, dst=MAC_B, type=0x9000) / Raw(b"\x00\x01" + b"lon" * 8)]


@recipe("gen-codesys", "CODESYS", "codesys or tcp.port == 2455")
def codesys():
    return tcp_talk(2455, bytes.fromhex("0000010153532d4d4f44452d54435000"))


@recipe("gen-wpad", "WPAD", "http.request.uri contains \"wpad\"")
def wpad():
    req = b"GET /wpad.dat HTTP/1.1\r\nHost: wpad.lab.local\r\n\r\n"
    return tcp_talk(80, req)


@recipe("gen-wpad-proxy", "WPAD proxy", "http.request.uri contains \"wpad.dat\"")
def wpad_proxy():
    req = b"GET http://wpad/wpad.dat HTTP/1.1\r\nHost: wpad\r\n\r\n"
    return tcp_talk(80, req)


@recipe("gen-prometheus", "Prometheus exposition", "http.request.uri contains \"/metrics\"")
def prometheus():
    body = b"# TYPE cpu_usage gauge\ncpu_usage 0.2\n"
    resp = b"HTTP/1.1 200 OK\r\nContent-Type: text/plain; version=0.0.4\r\nContent-Length: %d\r\n\r\n" % len(body) + body
    syn = tcp_talk(9090, b"GET /metrics HTTP/1.1\r\nHost: lab\r\n\r\n")
    data = eth(
        IP(src=IP_B, dst=IP_A)
        / TCP(sport=9090, dport=40100, flags="PA", seq=1001, ack=2 + 40)
        / Raw(resp)
    )
    return syn + [data]


@recipe("gen-redfish", "Redfish", "http.request.uri contains \"/redfish\"")
def redfish():
    req = b"GET /redfish/v1/ HTTP/1.1\r\nHost: bmc.lab\r\nOData-Version: 4.0\r\n\r\n"
    return tcp_talk(443, req)


@recipe("gen-xmlrpc", "XML-RPC", "http.content_type contains \"xml\" or xml")
def xmlrpc():
    body = b'<?xml version="1.0"?><methodCall><methodName>sys.methodHelp</methodName></methodCall>'
    req = b"POST /RPC2 HTTP/1.1\r\nHost: lab\r\nContent-Type: text/xml\r\nContent-Length: %d\r\n\r\n" % len(body) + body
    return tcp_talk(80, req)


@recipe("gen-grpc", "gRPC", "grpc or http2")
def grpc_pkt():
    # HTTP/2 preface + SETTINGS
    preface = b"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
    settings = b"\x00\x00\x00\x04\x00\x00\x00\x00\x00"
    return tcp_talk(443, preface + settings)


@recipe("gen-ssl", "SSL", "ssl or tls")
def ssl_pkt():
    # TLS 1.0 ClientHello-ish handshake record
    rec = bytes.fromhex("16030100410100003d0301" + "00" * 32 + "0000020002002f0100")
    return tcp_talk(443, rec)


@recipe("gen-msrpc-epm", "MSRPC EPM", "epm or dcerpc")
def msrpc_epm():
    # DCERPC bind to EPM UUID e1af8308-5d1f-11c9-91a4-08002b14a0fa
    try:
        from scapy.layers.dcerpc import DceRpc5, DceRpc5Bind, DceRpc5BindContext, DceRpc5Interface

        iface = DceRpc5Interface(uuid="e1af8308-5d1f-11c9-91a4-08002b14a0fa", version_major=3, version_minor=0)
        pkt = DceRpc5(ptype=11) / DceRpc5Bind() / DceRpc5BindContext(ctx_id=0, iface=iface)
        return tcp_talk(135, bytes(pkt))
    except Exception:
        uuid = bytes.fromhex("0883afe11f5dc91191a408002b14a0fa")
        # simplified bind
        hdr = bytes([0x05, 0x00, 0x0B, 0x03, 0x10, 0x00, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00])
        rest = b"\xd0\x16\xd0\x16\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x01\x00" + uuid + b"\x03\x00\x00\x00" + b"\x04\x5d\x88\x8a\xeb\x1c\xc9\x11\x9f\xe8\x08\x00\x2b\x10\x48\x60" + b"\x02\x00\x00\x00"
        return tcp_talk(135, hdr + rest)


@recipe("gen-srvsvc", "SRVSVC", "srvsvc or dcerpc")
def srvsvc():
    uuid = bytes.fromhex("c84f324b7016d30112785a47bf6ee188")
    hdr = bytes([0x05, 0x00, 0x0B, 0x03, 0x10, 0x00, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00])
    rest = b"\xd0\x16\xd0\x16\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x01\x00" + uuid + b"\x03\x00\x00\x00" + b"\x04\x5d\x88\x8a\xeb\x1c\xc9\x11\x9f\xe8\x08\x00\x2b\x10\x48\x60" + b"\x02\x00\x00\x00"
    return tcp_talk(445, hdr + rest)


@recipe("gen-pppoe-session", "PPPoE Session", "pppoes")
def pppoe_session():
    from scapy.layers.ppp import PPPoE, PPP

    return [Ether(src=MAC_A, dst=MAC_B, type=0x8864) / PPPoE(sessionid=1) / PPP() / IP() / ICMP()]


@recipe("gen-lcp", "LCP", "lcp or ppp")
def lcp():
    from scapy.layers.ppp import PPP, PPP_LCP, PPPoE

    return [Ether(src=MAC_A, dst=MAC_B, type=0x8864) / PPPoE(sessionid=1) / PPP() / PPP_LCP(code=1)]


@recipe("gen-pap", "PAP", "pap")
def pap():
    from scapy.layers.ppp import PPP, PPP_PAP, PPPoE

    return [Ether(src=MAC_A, dst=MAC_B, type=0x8864) / PPPoE(sessionid=1) / PPP() / PPP_PAP(code=1)]


@recipe("gen-chap", "CHAP", "chap")
def chap():
    from scapy.layers.ppp import PPP, PPP_CHAP, PPPoE

    return [Ether(src=MAC_A, dst=MAC_B, type=0x8864) / PPPoE(sessionid=1) / PPP() / PPP_CHAP(code=1)]


@recipe("gen-smb2", "SMB2", "smb2")
def smb2():
    from scapy.layers.smb2 import SMB2_Header, SMB2_Negotiate_Protocol_Request

    body = bytes(SMB2_Header(Command=0) / SMB2_Negotiate_Protocol_Request(Dialects=[0x0202, 0x0210, 0x0300]))
    nbt = bytes([0x00, (len(body) >> 16) & 0xFF, (len(body) >> 8) & 0xFF, len(body) & 0xFF]) + body
    return tcp_talk(445, nbt)


@recipe("gen-mariadb", "MariaDB", "mysql")
def mariadb():
    # Greeting captured from docker.io/library/mariadb:10.11; tshark uses the mysql dissector.
    greet = Path(__file__).with_name("mariadb-greeting.bin").read_bytes()
    syn, synack, ack, _ = tcp_talk(3306, b"\x00")
    data = eth(IP(src=IP_B, dst=IP_A) / TCP(sport=3306, dport=40100, flags="PA", seq=1001, ack=2) / Raw(greet))
    return [syn, synack, ack, data]


def distinctive(protos: list[str], filt: str) -> bool:
    if not protos:
        return False
    joined = ":".join(protos)
    if joined in {"eth:ethertype:ip:tcp", "eth:ethertype:ip:udp", "eth:ethertype:ip:tcp:data", "eth:ethertype:ip:udp:data"}:
        # allow if filter is payload-specific
        if "payload" in filt or "http" in filt or "uri" in filt:
            return True
        return False
    return True


def main() -> int:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    kept = []
    failed = []
    for cap_id, roadmap, filt, fn in RECIPES:
        pcap_path = OUT_DIR / f"{cap_id}.pcap"
        try:
            pkts = fn()
            if not pkts:
                raise RuntimeError("no packets")
            wrpcap(str(pcap_path), pkts)
            number, protos = tshark_first(pcap_path, filt)
            if number <= 0:
                # fallback already handled by corpus generator; here we require a hit
                raise RuntimeError(f"filter {filt!r} matched nothing (protos from frame: {protos})")
            if not distinctive(protos, filt) and "payload" not in filt and "http" not in filt:
                raise RuntimeError(f"non-distinctive hierarchy {protos}")
            kept.append(
                {
                    "id": cap_id,
                    "roadmap_name": roadmap,
                    "display_filter": filt,
                    "file": str(pcap_path.relative_to(CORPUS)),
                    "frame_protocols": protos,
                    "frame": number,
                    "packets": len(pkts),
                }
            )
            print(f"KEEP {cap_id:24} {roadmap:28} {protos}")
        except Exception as exc:
            failed.append({"id": cap_id, "roadmap_name": roadmap, "error": str(exc)})
            print(f"DROP {cap_id:24} {roadmap:28} {exc}")
            if pcap_path.exists():
                pcap_path.unlink()
    INDEX_PATH.write_text(json.dumps({"kept": kept, "failed": failed}, indent=2) + "\n")
    print(f"\nkept {len(kept)} dropped {len(failed)} -> {INDEX_PATH}")
    return 0 if len(kept) >= 20 else 1


if __name__ == "__main__":
    sys.exit(main())
