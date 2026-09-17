#!/usr/bin/env python3
"""Build deterministic positive companions without replacing original data.

The original malformed records remain under generated-local and are tested as
boundary inputs. These companions use the same documented wire formats with
correct nested lengths. No endpoint is contacted.
"""

from pathlib import Path
import argparse
import base64
import hashlib
import hmac
import struct
import zlib

from scapy.all import Ether, Dot3, LLC, SNAP, STP, IP, IPv6, ARP, ICMPv6EchoRequest, UDP, TCP, Raw, EAPOL, EAP, wrpcap
from scapy.contrib.cdp import CDPv2_HDR, CDPMsgDeviceID, CDPMsgSoftwareVersion, CDPMsgAddr, CDPAddrRecordIPv4
from scapy.contrib.rsvp import RSVP, RSVP_Object, RSVP_HOP

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "captures" / "generated-validated"
MAC_A = "02:00:00:00:00:01"
MAC_B = "02:00:00:00:00:02"


def eth(payload):
    return Ether(src=MAC_A, dst=MAC_B) / payload


def udp(port, payload):
    return eth(IP(src="192.0.2.1", dst="192.0.2.2") / UDP(sport=40101, dport=port) / Raw(payload))


def tcp(payload, port=443):
    a = IP(src="192.0.2.1", dst="192.0.2.2")
    b = IP(src="192.0.2.2", dst="192.0.2.1")
    return [
        eth(a / TCP(sport=40100, dport=port, flags="S", seq=1)),
        eth(b / TCP(sport=port, dport=40100, flags="SA", seq=1000, ack=2)),
        eth(a / TCP(sport=40100, dport=port, flags="A", seq=2, ack=1001)),
        eth(a / TCP(sport=40100, dport=port, flags="PA", seq=2, ack=1001) / Raw(payload)),
    ]


def tlv(tag, value):
    assert len(value) < 128
    return bytes([tag, len(value)]) + value


def integer(value):
    raw = value.to_bytes(max(1, (value.bit_length() + 7) // 8), "big")
    if raw[0] & 128:
        raw = b"\x00" + raw
    return tlv(2, raw)


def checksum(data):
    if len(data) % 2:
        data += b"\x00"
    total = sum(struct.unpack("!%dH" % (len(data) // 2), data))
    while total >> 16:
        total = (total & 0xffff) + (total >> 16)
    return (~total & 0xffff) or 0xffff


def netbios_name(text, suffix=0x20):
    raw = text.encode("ascii").ljust(15, b" ") + bytes([suffix])
    assert len(raw) == 16
    encoded = bytearray()
    for octet in raw:
        encoded.extend((0x41 + (octet >> 4), 0x41 + (octet & 0x0f)))
    return b"\x20" + bytes(encoded) + b"\x00"


def ntlm_field(length, offset):
    return struct.pack("<HHI", length, length, offset)


def ntlm_http(token, header=b"Authorization"):
    return b"GET / HTTP/1.1\r\nHost: lab\r\n" + header + b": NTLM " + base64.b64encode(token) + b"\r\n\r\n"


def fixtures():
    # Complete LLC/STP configuration BPDU, not a two-byte STP prefix.
    yield "gen-llc-valid", [Dot3(src=MAC_A, dst="01:80:c2:00:00:00") / LLC(dsap=0x42, ssap=0x42, ctrl=3) / STP()]
    # A complete IEEE 802.1X identity response.
    yield "gen-ieee8021x-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x888e) / EAPOL(version=2, type=0) / EAP(code=2, id=1, type=1, identity=b"sample")]
    # CDP address records belong inside an Address TLV.
    yield "gen-cdp-valid", [Dot3(src=MAC_A, dst="01:00:0c:cc:cc:cc") / LLC(dsap=0xaa, ssap=0xaa, ctrl=3) / SNAP(OUI=0xc, code=0x2000) / CDPv2_HDR() / CDPMsgDeviceID(val=b"lab-switch") / CDPMsgSoftwareVersion(val=b"1.0") / CDPMsgAddr(addr=[CDPAddrRecordIPv4(addr="192.0.2.1")])]
    # RSVP HOP is nested in a length-bearing object.
    yield "gen-rsvp-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=46) / RSVP(Flags=0) / RSVP_Object(Length=12, Class=3, C_Type=1) / RSVP_HOP(neighbor="192.0.2.1"))]
    # RFC 3412/3414 noAuthNoPriv discovery request with a complete scoped PDU.
    header = tlv(0x30, integer(1) + integer(65507) + tlv(4, b"\x04") + integer(3))
    usm = tlv(0x30, tlv(4, b"") + integer(0) + integer(0) + tlv(4, b"") * 3)
    pdu = tlv(0xa0, integer(1) + integer(0) + integer(0) + tlv(0x30, b""))
    scoped = tlv(0x30, tlv(4, b"") + tlv(4, b"") + pdu)
    yield "gen-snmpv3-valid", [udp(161, tlv(0x30, integer(3) + header + tlv(4, usm) + scoped))]
    # TLS 1.0 ClientHello with computed record and handshake lengths.
    hello = b"\x03\x01" + bytes(range(32)) + b"\x00\x00\x02\x00\x2f\x01\x00"
    handshake = b"\x01" + len(hello).to_bytes(3, "big") + hello
    yield "gen-ssl-valid", tcp(b"\x16\x03\x01" + struct.pack("!H", len(handshake)) + handshake)
    # L2TPv2 zero-length-body acknowledgement: T/L/S set, full control header.
    yield "gen-l2tp-valid", [udp(1701, struct.pack("!6H", 0xc802, 12, 1, 0, 0, 0))]
    # RARP is EtherType 0x8035, distinct from ordinary ARP (0x0806).
    request = Ether(src=MAC_A, dst="ff:ff:ff:ff:ff:ff", type=0x8035) / ARP(op=3, hwsrc=MAC_A, psrc="0.0.0.0", hwdst=MAC_A, pdst="0.0.0.0")
    response = Ether(src=MAC_B, dst=MAC_A, type=0x8035) / ARP(op=4, hwsrc=MAC_B, psrc="192.0.2.2", hwdst=MAC_A, pdst="192.0.2.1")
    yield "gen-rarp-valid", [request, response]
    # Historical 6to4 prefix encodes the outer documentation IPv4 addresses.
    yield "gen-6to4-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=41) / IPv6(src="2002:c000:0201::1", dst="2002:c000:0202::2") / ICMPv6EchoRequest(data=b"sample"))]
    # RFC 3828 uses protocol 136 in the pseudo-header, not UDP's 17.
    lite = struct.pack("!4H", 1234, 4321, 12, 0) + b"lite"
    pseudo = bytes.fromhex("c0000201c00002020088") + struct.pack("!H", len(lite))
    lite = lite[:6] + struct.pack("!H", checksum(pseudo + lite)) + lite[8:]
    yield "gen-udplite-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=136) / Raw(lite))]
    # Connectionless LDAP search in the LDAPMessage envelope recognized by
    # Wireshark; the RFC 1798 user-field variant is separately unit-tested.
    search = tlv(4, b"") + tlv(10, b"\x00") + tlv(10, b"\x00") + integer(10) + integer(2) + tlv(1, b"\x00")
    search += tlv(0x87, b"objectClass") + tlv(0x30, tlv(4, b"namingContexts") + tlv(4, b"supportedLDAPVersion"))
    yield "gen-cldap-valid", [udp(389, tlv(0x30, integer(1) + tlv(0x63, search)))]
    # Command 1 carries an eight-octet Query Config Information header.
    def aoe(flags, major, minor, config):
        header = struct.pack("!BBHBBI", 0x10 | flags, 0, major, minor, 1, 0x12345678)
        query = struct.pack("!HHBBH", 16, 0x1234, 2, 0x10, len(config)) + config
        return Ether(src=MAC_A if flags == 0 else MAC_B, dst=MAC_B if flags == 0 else MAC_A, type=0x88a2) / Raw(header + query)
    yield "gen-aoe-valid", [aoe(0, 0xffff, 0xff, b""), aoe(8, 7, 2, b"sample-volume")]
    # RFC 1002 session-request names are first-level encoded 16-octet names.
    names = netbios_name("CALLED") + netbios_name("CALLING")
    yield "gen-nbt-ss-valid", tcp(b"\x81\x00" + struct.pack("!H", len(names)) + names, 139)
    # Complete Java Serialization stream containing one TC_NULL value.
    yield "gen-java-ser-valid", tcp(bytes.fromhex("aced000570"), 1099)
    # [MS-NLMP] NEGOTIATE_MESSAGE: flags plus both empty field descriptors.
    negotiate = b"NTLMSSP\x00" + struct.pack("<II", 1, 0x00000201) + ntlm_field(0, 32) * 2
    yield "gen-ntlmssp-valid", tcp(ntlm_http(negotiate), 80)
    # Complete CHALLENGE_MESSAGE with empty target fields and an 8-byte challenge.
    challenge = b"NTLMSSP\x00" + struct.pack("<I", 2) + ntlm_field(0, 48)
    challenge += struct.pack("<I", 0x00000201) + bytes.fromhex("0102030405060708") + b"\x00" * 8 + ntlm_field(0, 48)
    yield "gen-ntlm-valid", tcp(ntlm_http(challenge, b"WWW-Authenticate"), 80)
    # AUTHENTICATE_MESSAGE whose NT response field contains a complete
    # 44-octet NTLMv2 response/blob boundary used by the NetNTLMv2 rule.
    blob = bytes.fromhex("11" * 16 + "0101000000000000" + "0000000000000000" + "0908070605040302" + "00000000")
    authenticate = b"NTLMSSP\x00" + struct.pack("<I", 3)
    authenticate += ntlm_field(0, 108) + ntlm_field(len(blob), 64)
    authenticate += ntlm_field(0, 108) * 4 + struct.pack("<I", 0x00088201) + blob
    yield "gen-netntlmv2-valid", tcp(ntlm_http(authenticate), 80)
    # The PR #5023 NTLM-v2 material ends before the mandatory Type 3 flags.
    # Keep that record as a negative boundary and provide this complete alias
    # companion so the roadmap entry is exercised by the same bounded parser.
    yield "gen-ntlm-v2-valid", tcp(ntlm_http(authenticate), 80)
    # RFC 4178 initialContextToken with a required NegTokenInit/mechTypes list.
    spnego = bytes.fromhex("601c06062b0601050502a0123010a00e300c060a2b06010401823702020a")
    http = b"GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate " + base64.b64encode(spnego) + b"\r\n\r\n"
    yield "gen-spnego-valid", tcp(http, 80)
    # Echo Request without a TEID uses the four-octet sequence/spare header.
    yield "gen-gtpv2-valid", [udp(2123, bytes.fromhex("4001000400000000"))]
    # The PR #5023 request declares seven body octets but carries eight.
    winrm_body = b"<s:Env/>"
    winrm = b"POST /wsman HTTP/1.1\r\nHost: winrm.lab\r\nContent-Type: application/soap+xml\r\nContent-Length: 8\r\n\r\n" + winrm_body
    yield "gen-winrm-http-valid", tcp(winrm, 5985)
    # Quake control packets include the complete datagram length in the low
    # 31 bits of the first word.
    quake = b"QUAKE\x00"
    yield "gen-quake-valid", [udp(26000, struct.pack("!I", 0x80000000 | (4 + len(quake))) + quake)]
    # An empty OLSR packet consists of the packet header alone.
    yield "gen-olsr-valid", [udp(698, struct.pack("!HH", 4, 1))]
    # MSDP type four is a header-only KeepAlive message.
    yield "gen-msdp-valid", tcp(bytes.fromhex("040003"), 639)
    # X11 setup with no authorization data is exactly the fixed 12-byte
    # little-endian prefix.
    yield "gen-x11-valid", tcp(bytes.fromhex("6c000b000000000000000000"), 6000)
    # The iSCSI login BHS advertises the carried 16-byte data segment.
    bhs = bytearray(48)
    bhs[0] = 0x43
    bhs[7] = 16
    yield "gen-iscsi-valid", tcp(bytes(bhs) + b"InitiatorName=iq", 3260)
    # A complete UCP envelope. LEN covers the characters between STX and ETX;
    # checksum 7E is the low octet of all characters through the final slash.
    yield "gen-ucp-valid", tcp(b"\x0200/00020/O/30/1.0/7E\x03", 3027)
    # RMCP+ Open Session Request with the 12-byte session header and three
    # eight-byte algorithm payloads.
    open_session = bytes.fromhex(
        "01000400" "78563412"
        "0000000801000000"
        "0100000801000000"
        "0200000801000000"
    )
    rmcp_plus = bytes.fromhex("0600ff07" "06100000000000000000") + struct.pack("<H", len(open_session)) + open_session
    yield "gen-ipmi-rmcpplus-valid", [udp(623, rmcp_plus)]
    # RFC 7637 mandates the GRE key: 24-bit VSID plus an 8-bit Flow ID.
    # The incoming fixture omitted this key and is only transparent GRE.
    inner = bytes(Ether(src=MAC_A, dst=MAC_B) / IP(src="192.0.2.1", dst="192.0.2.2") / UDP(sport=12000, dport=12001) / Raw(b"sample"))
    nvgre = struct.pack("!HHI", 0x2000, 0x6558, 0x12345678) + inner
    yield "gen-nvgre-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=47) / Raw(nvgre))]
    # RFC 2394 DEFLATE is a complete raw stream, without a zlib wrapper.
    # Compress a UDP datagram with a zero IPv4 UDP checksum and repetitive data.
    plain = struct.pack("!HHHH", 12000, 12001, 520, 0) + b"protocol sample " * 32
    compressor = zlib.compressobj(level=6, wbits=-15)
    compressed = compressor.compress(plain) + compressor.flush()
    ipcomp = struct.pack("!BBH", 17, 0, 2) + compressed
    yield "gen-ipcomp-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=108) / Raw(ipcomp))]
    # RFC 5944 registration request and a complete Mobile-Home extension.
    # The key is deterministic fixture data, not an endpoint credential.
    request = struct.pack("!BBHIIIQ", 1, 0x20, 60, 0xc0000201, 0xc0000202, 0xc0000201, 1)
    extension = struct.pack("!BBI", 32, 20, 256)
    value = hmac.new(b"protocol-sample-key", request + extension, hashlib.md5).digest()
    yield "gen-mip-valid", [udp(434, request + extension + value)]
    # RFC 6275 direct IPv6 Mobility Headers: Home Test Init plus a complete
    # Binding Update with Nonce Indices and final 96-bit Binding Data option.
    # Tokens and derived values are fixture-only; no live binding is implied.
    source = bytes.fromhex("20010db8000000000000000000000001")
    destination = bytes.fromhex("20010db8000000000000000000000002")
    def mobility(message):
        pseudo = source + destination + struct.pack("!I3xB", len(message), 135)
        message = message[:4] + struct.pack("!H", checksum(pseudo + message)) + message[6:]
        return eth(IPv6(src="2001:db8::1", dst="2001:db8::2", nh=135) / Raw(message))
    init = struct.pack("!BBBBHHQ", 59, 1, 1, 0, 0, 0, 0x0102030405060708)
    update = struct.pack("!BBBBHHHH", 59, 3, 5, 0, 0, 1, 0x8000, 60)
    update += struct.pack("!BBHH", 4, 4, 1, 2)
    update += bytes([5, 12])
    binding_key = hashlib.sha1(b"homekey1" + b"carekey1").digest()
    binding_data = hmac.new(binding_key, source + destination + update, hashlib.sha1).digest()[:12]
    update += binding_data
    yield "gen-mipv6-valid", [mobility(init), mobility(update)]
    # MEF 16 version-one STATUS ENQUIRY with mandatory Report Type,
    # Sequence Numbers and Data Instance information elements.
    elmi = bytes.fromhex("0175 010101 02020102 03050000000001")
    yield "gen-elmi-valid", [Ether(src=MAC_A, dst="01:80:c2:00:00:07", type=0x88ee) / Raw(elmi)]
    # IEEE 802.3 information OAMPDU: local and remote information followed by
    # an organization-specific TLV, a terminator and Ethernet padding.
    local_info = bytes.fromhex("011001002a001f05ee02000001020304")
    remote_info = bytes.fromhex("0210010007000905ee02000005060708")
    eoam = bytes.fromhex("03005000") + local_info + remote_info + bytes.fromhex("fe06020000aa00")
    eoam += b"\x00" * (46 - len(eoam))
    yield "gen-eoam-valid", [Ether(src=MAC_A, dst="01:80:c2:00:00:02", type=0x8809) / Raw(eoam)]
    # MRP messages carry fixed-size first values plus a base-six event vector.
    # Exercise both MMRP attribute types and more than one packed event byte.
    mmrp = bytes.fromhex("00 0101 0002 00 06 0000 0206 0003 020000000010 08 0000 0000")
    yield "gen-mmrp-valid", [Ether(src=MAC_A, dst="01:80:c2:00:00:20", type=0x88f6) / Raw(mmrp)]
    mvrp = bytes.fromhex("00 0102 0004 0064 086c 0000 0000")
    yield "gen-mvrp-valid", [Ether(src=MAC_A, dst="01:80:c2:00:00:21", type=0x88f5) / Raw(mvrp)]
    stream_id = bytes.fromhex("0200000000011234")
    talker = stream_id + bytes.fromhex("91e0f0000001") + struct.pack("!HHHBI", 100, 1500, 1, 0x70, 12345)
    failed = talker + bytes.fromhex("020000000002567801")
    def msrp_message(kind, first, events, count=3, declarations=b""):
        vector = struct.pack("!H", count) + first + events + declarations + b"\x00\x00"
        return bytes([kind, len(first)]) + struct.pack("!H", len(vector)) + vector
    msrp = b"\x00" + msrp_message(1, talker, b"\x08") + msrp_message(2, failed, b"\x08")
    msrp += msrp_message(3, stream_id, b"\x08", declarations=b"\x6c")
    msrp += msrp_message(4, bytes.fromhex("06030064"), b"\x06", count=2) + b"\x00\x00"
    yield "gen-msrp-valid", [Ether(src=MAC_A, dst="01:80:c2:00:00:0e", type=0x22ea) / Raw(msrp)]
    # EPSG DS 301 core SoC/PReq/PRes. Keep the incoming zero-node SoC
    # separately as a negative; valid messages use MN 240 and CN 42.
    soc = bytes([1, 255, 240, 0, 0xc0, 0]) + struct.pack("<IIQ", 0x12345678, 123456789, 0x0102030405060708)
    preq = bytes.fromhex("03 2a f0 00 25 00 12 00 03 00 ab cd ef")
    pres = bytes.fromhex("04 ff 2a fd 31 1a 21 00 03 00 de ad be")
    yield "gen-powerlink-valid", [
        Ether(src=source, dst=destination, type=0x88ab) / Raw(payload.ljust(46, b"\x00"))
        for source, destination, payload in [(MAC_A, "01:11:1e:00:00:01", soc), (MAC_A, MAC_B, preq), (MAC_B, "01:11:1e:00:00:02", pres)]
    ]
    # ISO 10589 / RFC 1195 LAN IIH: complete Area Addresses, supported NLPIDs,
    # IPv4 interfaces and one length-bounded unrecognized extension.
    isis = bytes.fromhex("831b01000f01000001010001000100001e00350101000100010001010403490001810381cc8e8408c0000201c6336402fa03aabbcc")
    yield "gen-isis-valid", [Dot3(src=MAC_A, dst="01:80:c2:00:00:14") / LLC(dsap=0xfe, ssap=0xfe, ctrl=3) / Raw(isis)]
    # RFC 4340 sections 5 and 9: all ten packet types, plus the three
    # short-sequence variants; exact header offsets and IPv4 pseudo-checksums.
    # These are independent datagrams, not a negotiated connection trace.
    dccp_packets = []
    for kind in range(10):
        for extended in [False, True]:
            if not extended and kind not in (2, 3, 4):
                continue
            header = bytearray(struct.pack("!HHBBH", 4000, 80, 0, 0xa0, 0))
            header += bytes([(kind << 1) | extended])
            header += bytes.fromhex("00010203040506" if extended else "040506")
            if kind not in (0, 2):
                header += bytes.fromhex("0000060504030201" if extended else "00030201")
            if kind in (0, 1):
                header += b"test"
            if kind == 7:
                header += bytes([5, 41, 0xab, 0xcd])
            header += bytes.fromhex("2906010203040000")  # Timestamp + padding
            header[4] = len(header) // 4
            data = bytes(header) + b"sample"
            pseudo = bytes.fromhex("c0000201c00002020021") + struct.pack("!H", len(data))
            data = data[:6] + struct.pack("!H", checksum(pseudo + data)) + data[8:]
            dccp_packets.append(eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=33) / Raw(data)))
    yield "gen-dccp-valid", dccp_packets
    # Linux v6.12 struct elapaarp and __aarp_send_query/aarp_send_reply/
    # aarp_send_probe: independently encoded Ethernet/AppleTalk fields.
    # https://github.com/torvalds/linux/blob/v6.12/include/linux/atalk.h
    # https://github.com/torvalds/linux/blob/v6.12/net/appletalk/aarp.c
    # Linux uses 000000/80f3 SNAP: request/probe multicast, reply unicast.
    # The explicit 802.3 length excludes ten bytes of Ethernet padding.
    aarp_packets = []
    for function, source, destination, source_net, source_node, target_mac, target_net, target_node in [
        (1, MAC_A, "09:00:07:ff:ff:ff", 0x1234, 0x2a, bytes(6), 0x1234, 0x56),
        (2, MAC_B, MAC_A, 0x1234, 0x56, bytes.fromhex("020000000001"), 0x1234, 0x2a),
        (3, MAC_A, "09:00:07:ff:ff:ff", 0x2345, 0x67, bytes(6), 0x2345, 0x67),
    ]:
        body = struct.pack("!HHBBH", 1, 0x809b, 6, 4, function)
        body += bytes.fromhex(source.replace(":", "")) + struct.pack("!BHB", 0, source_net, source_node)
        body += target_mac + struct.pack("!BHB", 0, target_net, target_node)
        assert len(body) == 28
        aarp_packets.append(Dot3(src=source, dst=destination, len=36) / LLC(dsap=0xaa, ssap=0xaa, ctrl=3) / SNAP(OUI=0, code=0x80f3) / Raw(body + bytes(10)))
    yield "gen-aarp-valid", aarp_packets

    # Independent extended DDP encoding. Linux SNAP uses OUI 080007, PID809b;
    # the declared 802.3 length excludes retained Ethernet padding.
    # https://github.com/apple-oss-distributions/xnu/blob/xnu-1228.15.4/bsd/netat/ddp.h
    # https://github.com/torvalds/linux/blob/v6.12/net/appletalk/ddp.c
    ddp_packets = []
    for index, encoded in enumerate([
        "0017af9512342345562a04810401a1b2c3007f80fedc55",
        "0c170000234512342a5681040402a1b2c3007f80fedc55",
        "241460e9abcd1234785691a3fe00137f80a5feff",
    ]):
        body = bytes.fromhex(encoded)
        source, destination = (MAC_B, MAC_A) if index == 1 else (MAC_A, MAC_B)
        packet = Dot3(src=source, dst=destination, len=8+len(body)) / LLC(dsap=0xaa, ssap=0xaa, ctrl=3) / SNAP(OUI=0x080007, code=0x809b) / Raw(body+bytes(60-22-len(body)))
        ddp_packets.append(packet)
    yield "gen-appletalk-valid", ddp_packets

    # RFC 2332 fixed/mandatory parts: all seven message types, one independent
    # Client Information Entry, and a responder extension on Resolution Reply.
    # https://www.rfc-editor.org/rfc/rfc2332.html
    nhrp_packets = []
    cie = bytes.fromhex("0020000005dc012c04000407c6336402c0000202")
    for kind in range(1, 8):
        header = bytearray(bytes.fromhex("0001080000000000001000000000000001000400"))
        header[17] = kind
        mandatory = bytes.fromhex("0404000012345678c6336401c0000201c0000202")
        if kind == 1:
            mandatory = mandatory[:2]+bytes.fromhex("0800")+mandatory[4:]
        if kind == 7:
            mandatory = mandatory[:4]+bytes.fromhex("0007000a")+mandatory[8:]+bytes(20)
        else:
            client = bytearray(cie)
            if kind in (5, 6):
                client[4:8] = bytes(4)
                client[11] = 0
            mandatory += client
        body = header+mandatory
        if kind == 2:
            body[14:16] = struct.pack("!H", len(body))
            responder = cie[:1]+bytes(1)+cie[2:]
            body += struct.pack("!HH", 0x8003, len(responder))+responder+bytes.fromhex("80000000")
        body[10:12] = struct.pack("!H", len(body))
        body[12:14] = struct.pack("!H", checksum(bytes(body)))
        nhrp_packets.append(eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=54) / Raw(bytes(body))))
    yield "gen-nhrp-valid", nhrp_packets

    # Independent CFM CCM fields/TLVs, cross-checked against Wireshark 4.4.8
    # packet-cfm.c. Retain the original incomplete PR #5023 CCM separately.
    # https://gitlab.com/wireshark/wireshark/-/blob/v4.4.8/epan/dissectors/packet-cfm.c
    cfm_frame = bytes.fromhex("0180c200003302000000000189026001c546102030401234040441434d4502054d412d303100000000000000000000000000000000000000000000000000000000000000000000000102030411223344556677880000000002000102030003aabbcc040001031f0006001b2107deadfa0002123400")
    yield "gen-cfm-valid", [Ether(cfm_frame)]

    # Cisco IGRP 12-byte header and three 14-byte route-vector classes.
    # Request retains the documented zero-checksum convention; the separate
    # update records validate nonzero complete-message checksums.
    # https://www.cisco.com/c/en/us/support/docs/ip/interior-gateway-routing-protocol-igrp/26825-5.html
    igrp_bodies = [
        "11351234000100010001bdd500010000007b0003e805dcf11702ac1000000456004c4b1234de3105c63364ffffff123456abcd807f09",
        "120012340000000000000000",
        "11a61234000000000000dc25",
    ]
    yield "gen-igrp-valid", [eth(IP(src="10.0.0.1", dst="255.255.255.255", ttl=1, proto=9) / Raw(bytes.fromhex(body))) for body in igrp_bodies]

    # Xerox XSIS 028112 IDP: declared Length 37 excludes the nonzero garbage
    # octet, but the checksum covers all 38 wire octets. Echo data is opaque.
    xns_body = bytes.fromhex("541100250b021234567802aabbccddee045190abcdef0213243546578ace0001dead178039a7")
    yield "gen-xns-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x0600) / Raw(xns_body)]

    # RFC 6325 six-byte base header and required inner C-tag; RFC 7179 flags
    # word in the second independent record. No forwarding-state assertions.
    trill_bodies = [
        "0025123456780200000000200200000000108100a12308004500001c0001000040017cde7f0000017f0000010800f7ff00000000",
        "0849234567890000000001005e00000102000000001081005abc88b51020304050",
    ]
    yield "gen-trill-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x22f3) / Raw(bytes.fromhex(body)) for body in trill_bodies]

    # RFC 1075 version-one Response/Request/NMR/Cancel. All tagged commands
    # and Internet checksums are independently asserted in the Go fixtures.
    # These four datagrams are not a captured multicast-routing exchange.
    dvmrp_bodies = [
        "1301ea350202040206100301ffffff0005c107028002fbe78002ec02",
        "1302f3c4020203000802c0000200c6336400",
        "1303140400aa020205ff0902e002030100000014e005040600000028",
        "130407e602020a02e0070805ef010203",
    ]
    yield "gen-dvmrp-valid", [eth(IP(src="10.0.0.1", dst="224.0.0.4", ttl=1, proto=2) / Raw(bytes.fromhex(body))) for body in dvmrp_bodies]

    # Digital AA-X435A-TK section 10: counted short/long routing data,
    # padded verification, level-one route update and endnode hello.
    # These independent messages are not a recorded routing/session exchange.
    decnet_bodies = [
        "0900020104020400240100",
        "1800060000aa00040001040000aa000400020400000000240100",
        "0700820003020401aa",
        "0c00070204000100010001040404",
        "22000d020000aa0004000a04033240000000000000000000aa00040000000a000002aaaa",
    ]
    yield "gen-decnet-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x6003) / Raw(bytes.fromhex(body)) for body in decnet_bodies]

    # Linux v6.12 net/tipc/{msg.h,msg.c,socket.c,group.c}: payload users 0..3,
    # connected/direct/named/multicast and group message types 5..7. Unknown
    # application bytes and future header words remain explicit, not decoded.
    tipc_bodies = [
        "40c4001d000013572468369c010020031020304050607080d1001780f9",
        "43040025600013572468369c0100200310203040506070800100200401002005d1031780f9",
        "4544002d401013572468369c01002003102030405060708001002004010020050102030410203040d1021780f9",
        "47680031201013572468369c010020031020304000000000010020040000000001020304102030401020304fd1011780f9",
        "43680031a01013572468369c0100200310203040000000000100200400000000010203040000000035a70001d1051780f9",
        "45680031c01013572468369c0100200310203040000000000100200400000000010203041020304035a70001d1061780f9",
        "47680031e01013572468369c0100200310203040506070800100200401002005010203040000000035a70000d1071780f9",
        "43e40041401013572468369c01002003102030405060708001002004010020050102030410203040a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3d1021780f9",
    ]
    yield "gen-tipc-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x88ca) / Raw(bytes.fromhex(body)) for body in tipc_bodies]


    # Original Slim Devices/SlimServer layouts, not a recorded playback
    # session: discovery request/reply, infrared, display and MPEG data.
    slimp3_bodies = [
        "640001110000000000000000001122334455",
        "4400c00002090d9b00000000000000000000",
        "6900000f4240ff100000f732001122334455",
        "6c20202020202020202020202020202020200348036902010005",
        "6d0300000000123400000102000000000000fffb9064",
    ]
    yield "gen-slimp3-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / UDP(sport=3483, dport=3483) / Raw(bytes.fromhex(body))) for body in slimp3_bodies]

    # Official asterisk/dahdi-linux 276c914ea7d58bc2c19478f17835ed833e8f66d4:
    # dynamic Ethernet prefix + samples=8 header, optional ceil(ch/4)*2
    # signaling bytes, and exactly channels*8 sample bytes.
    tdmoe_bodies = [
        "12340800010200010001020304050607",
        "00fe0803fffe00054321000510111213141516172021222324252627303132333435363740414243444546475051525354555657",
        "beef08fd80000002a0a1a2a3a4a5a6a7b0b1b2b3b4b5b6b7",
    ]
    yield "gen-tdmoe-valid", [Ether(src=MAC_A, dst=MAC_B, type=0xd00d) / Raw(bytes.fromhex(body)) for body in tdmoe_bodies]

    # IEEE 802.1AE clear SecTAG layouts, also checked against Linux v6.12.
    # These are field-boundary controls with arbitrary, unverified ICV bytes.
    macsec_bodies = [
        "400601020304080001020304000102030405060708090a0b0c0d0e0f",
        "2f08ffffffff0200000000011234a0a1a2a3a4a5a6a7101112131415161718191a1b1c1d1e1f",
        "12000000000086dd000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d202122232425262728292a2b2c2d2e2f",
        "24040000002a020000000001000110203040303132333435363738393a3b3c3d3e3f01020304",
    ]
    yield "gen-macsec-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x88e5) / Raw(bytes.fromhex(body)) for body in macsec_bodies]

    # IEEE 1609.3 v2 and ISO16460:2021 wire-equivalent v3 short messages:
    # variable PSID, network/transport extensions and ITS-port transport.
    wsmp_bodies = [
        "0220800003616263",
        "0280020f01ac10010c0401148100058102616263",
        "02e00000018200020102",
        "03002003616263",
        "0b050f01ac10010c040114170155fe02aabb00c0000103616263",
        "0301800201fd02beef02cafe",
        "03021234567803010203",
        "0b0003123456780000",
    ]
    yield "gen-wsmp-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x88dc) / Raw(bytes.fromhex(body)) for body in wsmp_bodies]

    # RFC 3948 keepalive, ESP, and marked IKEv1/v2 datagrams. Clear fields
    # and lengths are reproducible; opaque bodies are not verified/decrypted.
    def natt_ike(next_payload, version, flags, body):
        header = bytearray.fromhex("000000001122334455667788887766554433221100202508010203040000001c")
        header[20], header[21], header[23] = next_payload, version, flags
        header[28:32] = struct.pack("!I", 28 + len(body))
        return bytes(header) + body
    natt_bodies = [
        b"\xff",
        bytes.fromhex("1122334401020304a0a1a2a3a4a5a6a7a8a9aaabacadaeaf"),
        natt_ike(41, 0x20, 8, bytes.fromhex("280000080000402e00000014000102030405060708090a0b0c0d0e0f")),
        natt_ike(13, 0x10, 0, bytes.fromhex("0000000c1122334455667788")),
        natt_ike(8, 0x10, 1, b"\xa5" * 16),
        natt_ike(46, 0x20, 8, bytes.fromhex("23800018000102030405060708090a0b0c0d0e0f10111213")),
        natt_ike(53, 0x20, 8, bytes.fromhex("2300001c00010002000102030405060708090a0b0c0d0e0f10111213")),
        natt_ike(53, 0x20, 8, bytes.fromhex("0000001c00020002000102030405060708090a0b0c0d0e0f10111213")),
        natt_ike(41, 0x20, 8, bytes.fromhex("000000100304400911223344aabbccdd")),
        natt_ike(34, 0x20, 8, bytes.fromhex("0000000c0013000001020304")),
    ]
    yield "gen-nat-t-valid", [udp(4500, body) for body in natt_bodies]

    # Official AJNS legacy IP name service v0/v1, core-alljoyn commit
    # 103b0833801f8e36e648a6d66313518356b0218d, IpNsProtocol.cc.
    # A v0 question, a v0 dual-address answer (IPv4 precedes IPv6), and a
    # v1 question with separate TCP/UDP answers. These independent discovery
    # layouts neither replace the original trailing-data negative nor imply
    # observed service availability or message-bus/session decoding.
    alljoyn_bodies = [
        "000100008901126f72672e6578616d706c652e53656e736f72",
        "000001787b0126e3c000020a20010db8000000000000000000000010203030313132323333343435353636373738383939616162626363646465656666126f72672e6578616d706c652e53656e736f72",
        "210102ff8401126f72672e6578616d706c652e53656e736f727a010004c000020a26e320010db800000000000000000000001026e3203030313132323333343435353636373738383939616162626363646465656666126f72672e6578616d706c652e53656e736f7275010100c633641426e220010db800000000000000000000002026e2203030313132323333343435353636373738383939616162626363646465656666116f72672e6578616d706c652e436c6f636b",
    ]
    yield "gen-alljoyn-valid", [udp(9956, bytes.fromhex(body)) for body in alljoyn_bodies]

    # Modern FC-BB-5 FCoE and independently bounded FC-2 layouts, from Linux
    # adc218676eef25575469234709c2d87185ca223a and pinned Wireshark fields.
    # CRC32 is retained as literal evidence, not calculated by the parser.
    # Frame 1 is the shared FCoE/FC positive control; no duplicate companion
    # is manufactured for the inner protocol name. Data remains uninterpreted.
    fcoe_bodies = [
        "000000000000000000000000002e0401020300040506ff380000070000001234ffff00000000deadbeef4a1916e142000000",
        "000000000000000000000000002e800102030004050600380000070000001234ffff000000007e1a47b142000000",
        "000000000000000000000000002e0401020300040506ff380000072000001234ffff0000000000112233445566778899aabbccddeeff01020304643e522d42000000",
        "000000000000000000000000002e0401020300040506ff380001070000001234ffff00000000aabbcc7f30b2407742000000",
        "000000000000000000000000002e0401020300040506ff380002070000001234ffff00000000aabb12343d996fda42000000",
        "000000000000000000000000002e5003a247091122330401020300040506ff380003072000001234ffff0000000000112233445566778899aabbccddeeff1011121314a1a2a379b87e9a42000000",
        "000000000000000000000000002e0401020300040506ff380000073100001234ffff00000000000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f6d6da71e42000000",
    ]
    yield "gen-fcoe-valid", [Ether(dst="0e:fc:00:01:02:03", src="0e:fc:00:04:05:06", type=0x8906) / Raw(bytes.fromhex(body)) for body in fcoe_bodies]

    # CORBA 3.2 chapter 9: four version-specific LocateRequest layouts,
    # followed by a complete 1.2 Request and Reply with empty context lists.
    # Keep the original zero-body Request plus unexplained suffix separately.
    giop_messages = []
    for minor in range(4):
        little = minor % 2
        order = "<" if little else ">"
        body = struct.pack(order + "I", minor + 1)
        if minor >= 2:
            body += struct.pack(order + "H", 0) + b"\x00\x00"
        body += struct.pack(order + "I", 11) + b"NameService"
        giop_messages.append(b"GIOP" + bytes([1, minor, little, 3]) + struct.pack(order + "I", len(body)) + body)
    giop_messages += [
        bytes.fromhex("47494f50010200000000002000000001030000000000000000000000000000065f69735f6100000000000000"),
        bytes.fromhex("47494f50010201010c000000010000000000000000000000"),
    ]
    giop_packets = []
    for index, message in enumerate(giop_messages):
        packets = tcp(message, 2809)
        for packet in packets:
            if packet[TCP].sport == 40100:
                packet[TCP].sport += index
            else:
                packet[TCP].dport += index
        giop_packets += packets
    yield "gen-giop-valid", giop_packets

    # DMTF DSP0266 1.23.1 section 12.4, UDA 2.0 search and compatible 1.0
    # notification layouts. Independent text controls; no REST endpoint or
    # remote UUID is contacted/validated. The malformed redfish (without -rest)
    # target in the original PR #5023 record is retained as a strict negative.
    redfish_messages = [
        b'M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: "ssdp:discover"\r\nST: urn:dmtf-org:service:redfish-rest:1\r\nMX: 2\r\n\r\n',
        b'HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nST: urn:dmtf-org:service:redfish-rest:1\r\nUSN: uuid:92384634-2938-2342-8820-489239905423::urn:dmtf-org:service:redfish-rest:1\r\nAL: https://192.0.2.50/redfish/v1/\r\nEXT:\r\n\r\n',
        b'NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: http://192.0.2.50/description.xml\r\nNT: urn:dmtf-org:service:redfish-rest:1\r\nNTS: ssdp:alive\r\nSERVER: SampleOS/1.0 UPnP/1.0 Discovery/1.0\r\nUSN: uuid:92384634-2938-2342-8820-489239905423::urn:dmtf-org:service:redfish-rest:1\r\n\r\n',
        b'NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: urn:dmtf-org:service:redfish-rest:1\r\nNTS: ssdp:byebye\r\nUSN: uuid:92384634-2938-2342-8820-489239905423::urn:dmtf-org:service:redfish-rest:1\r\n\r\n',
        b'M-SEARCH * HTTP/1.1\r\nHOST: 192.0.2.50:1901\r\nMAN: "ssdp:discover"\r\nST: urn:dmtf-org:service:redfish-rest:1\r\n\r\n',
        b'HTTP/1.1 200 OK\r\ncache-control:\tmax-age = 3600\r\nst: urn:dmtf-org:service:redfish-rest:1:23\r\nusn: uuid:92384634-2938-2342-8820-489239905423::urn:dmtf-org:service:redfish-rest:1:23\r\nal: https://[2001:db8::50]:8443/redfish/v1/\r\next:\t\r\nsample.example.com: first\r\nsample.example.com: second\r\n\r\n',
    ]
    redfish_packets = []
    for index, message in enumerate(redfish_messages):
        packet = udp(1900, message)
        if index in (0, 2, 3):
            packet[IP].dst = "239.255.255.250"
            packet[Ether].dst = "01:00:5e:7f:ff:fa"
        elif index == 4:
            packet[IP].dst, packet[UDP].dport = "192.0.2.50", 1901
        else:
            packet[UDP].sport, packet[UDP].dport = 1900, 40101
        redfish_packets.append(packet)
    yield "gen-redfish-ssdp-valid", redfish_packets

    # Novell NCP SDK connection, function 20, 23/26, 33 and acknowledgement
    # records. The original PR #5023 malformed create-connection datagram is
    # retained separately. Replies have no inferred request/session context.
    ncp_messages = [
        "111100ff000000",
        "111100ffa5b6c7",
        "55550201345678",
        "22220101010014",
        "222202fe03801700051a78563412",
        "222203010100210400",
        "2222040101007e010203",
        "3333000100000000",
        "33330101010000007e09060c223803",
        "9999a50112c3d4e5",
    ]
    yield "gen-ncp-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / UDP(sport=524, dport=524) / Raw(bytes.fromhex(message))) for message in ncp_messages]

    # Packed SEBEK v2/v3 record layouts, with an explicitly configured Magic
    # and raw v2 body / v3 IPv4 endpoint fields. Not a recorded event stream.
    sebek_messages = [
        "a1b2c3d40002000101020304102030400001e24000000008000000090000000a7265636f72642d6c6162656c000000034100ff",
        "a1b2c3d40003000201020304102030400001e2400000000700000008000000090000000a0000000b7265636f72642d6c6162656c0000000fc00002141f90c633640ac001000311",
    ]
    yield "gen-sebek-valid", [udp(1101, bytes.fromhex(message)) for message in sebek_messages]

    # Digital AA-NL26A-TE virtual-circuit layouts: zero-slot Run, both Start
    # directions, Stop and a Run containing all six common slot headers.
    # Capture containers omit Ethernet padding/FCS; these are exact LAT PDUs,
    # not a claim that an on-wire Ethernet frame may be shorter than minimum.
    lat_messages = [
        "010034127856feff",
        "0600000078560100ee050501100208143412110105534c415645064d4153544552008002dead00",
        "040034127856010040020501100200003412030105534c415645064d41535445520000",
        "08003412000009070203627965",
        "010634127856090801021e93011f7f03545459044e4f444501020180020234120403505f318002dead0001020305616263e1010201a299e2010201b081e3010001c244e4010001d355e5",
    ]
    yield "gen-lat-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x6004) / Raw(bytes.fromhex(message)) for message in lat_messages]

    # RFC 2896 VIP/IPC/SPP offsets and pinned Wireshark ARP/RTP/ICP layouts.
    # Checksum is a retained field, not a verified integrity result. Capture
    # frames omit padding/FCS so the exact VIP length remains explicit.
    vines_messages = [
        "ffff001b0f011020304080015060708000011234567800006100ff",
        "ffff00252f0110203040800150607080000112345678012001020304000500060009010203",
        "ffff00221e011020304080015060708000011234567802200102030400050006009d",
        "ffff00224d0210203040800150607080000112345678052001020304000500061234",
        "ffff001a0f041020304080015060708000010003123456788002",
        "ffff00200f04102030408001506070800001010312345678800201020304000a",
        "ffff00207f04ffffffffffff5060708000010100000000000000000000000000",
        "ffff00223f05ffffffffffff5060708000010202000010203040000510203040ffff",
        "ffff00190f0510203040800150607080000100010102010001",
        "ffff00300f05102030408001506070800001000102020100026012340000123456780005506070800006010203040000",
        "ffff00180f05102030408001506070800001000104020100",
        "ffff00280f0610203040800150607080000100010005ffff00120fee102030408001506070800001",
    ]
    yield "gen-vines-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x0bad) / Raw(bytes.fromhex(message)) for message in vines_messages]

    # RFC 3320 datagram headers (all partial-ID lengths, feedback forms and
    # uploaded bytecode) plus RFC 4077 NACK v1. These are field-layout vectors,
    # not a claim of resolved state, executed bytecode or successful decoding.
    sigcomp_messages = [
        "f9010203040506aabb",
        "fe7f010203040506070809cc",
        "ff8210200102030405060708090a0b0cdd",
        "f8001223ee",
        "f8000101000000000102030405060708090a0b0c0d0e0f10111213010203040506",
    ]
    yield "gen-sigcomp-valid", [udp(5060, bytes.fromhex(message)) for message in sigcomp_messages]

    # RFC 3525 Annex B text layouts, without SDP/package/session semantics.
    # Literal CRLF, escaped closing brace and trailing comments are preserved.
    megaco_messages = [
        'MEGACO/1 [192.0.2.10]:2944 Transaction=1{Context=-{ServiceChange=ROOT{Services{Method=Restart,Reason="901 restart",Profile=demo/1,Version=1}}}}',
        'MEGACO/1 <mg.example> Transaction=42{Context=${Priority=7,Add=rtp/${Media{Stream=1{LocalControl{Mode=SendReceive,ReservedGroup=OFF,foo/bar=[1,2]},Local{v=0\r\ns=example\\}text\r\n}}},Signals{tonegen/play{Duration=100,SignalType=Brief}}},Modify=rtp/1{Events=7{al/of{Stream=1}},EventBuffer{al/of},DigitMap=digits,Audit{Media,Signals}}},Context=1{Subtract=rtp/2{Audit{}},AuditValue=rtp/3{Audit{Media,Statistics}}}}',
        '!/1 mg1 T=3{C=1{O-W-A=rtp/${M{O{MO=SR}}},MV=aaln/1,AC=rtp/1{AT{M,SG}},N=aaln/1{OE=9{20260906T12345678:al/of{ST=1,foo="x,y"}}}}}',
        'MEGACO/1 mg1 Reply=42{ImmAckRequired,Context=1{Add=rtp/1{Media{LocalControl{Mode=SendReceive}},Statistics{rtp/ps=10},Packages{root-1}},Modify=rtp/2,Notify=aaln/1,ServiceChange=ROOT{Services{Version=1,Profile=demo/1}}}}',
        'MEGACO/1 [2001:db8::1] Pending=7{} TransactionResponseAck{1,3-7}',
        'MEGACO/1 mg1 Error = 500 {"unavailable context"}',
        '; leading\r\nmEgAcO/01 ; sender\r\n[001.002.003.004]:00080 ; body\nT=0{C=-{S=ROOT}} ; tail\r\n',
    ]
    yield "gen-megaco-valid", [udp(2944, message.encode("ascii")) for message in megaco_messages]

    # Historical draft-ipsec-swipe-01: four-byte plain header, clear sequence
    # only for type one, and encrypted header regions for types two/three.
    # Control packets expose only their specified common header. These are
    # structural vectors, with no key/policy/integrity validation implied.
    swipe_messages = [
        "0001000145000017010200003f110000c000020ac6336414646174",
        "0103123401020304deadbeef45000017010200003f110000c000020ac63364146461740000",
        "0202123411223344aabbccdd",
        "030312341122334455667788aabbccddeeff0011",
        "10010001010203",
        "3f021234aabbccdd",
    ]
    yield "gen-swipe-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=53) / Raw(bytes.fromhex(message))) for message in swipe_messages]

    # IBM GA27-3136-20 SNA fields under a length-bounded Ethernet/LLC wrapper.
    # The LLC-only controls and opaque LU/session/enciphered bodies are not
    # semantic positives for a PIU or application message.
    sna_messages = [
        "001000040404072c00010201020380c00100ff",
        "000c000404032c0001020102838000",
        "0011000404032c00010201028790000801123431",
        "000d000404032d00010201026b8000a0",
        "0011000404030c0012345678010200044b800084",
        "001200040403180012345678010200050380000801",
        "000b000404032000010201020102",
        "000b000404032400010201020304",
        "000a000404033cc50300004040",
        "000c000404032c0001020102830100",
        "000f000404032d0001020102830100600010",
        "0011000404032c0001020102034000110004c440",
        "0011000404032c0001020102034004110004c440",
        "0006000404af8103fe",
        "0004000404010b",
        "000f000404032c00010201020b8000010201",
    ]
    yield "gen-sna-valid", [Ether(src=MAC_A, dst=MAC_B, type=0x80d5) / Raw(bytes.fromhex(message)) for message in sna_messages]

    # ITU H.225 aligned-PER GatekeeperRequest: sequence one, version-four
    # protocol OID and IPv4 RAS transport address. No session is inferred.
    yield "gen-h225-valid", [udp(1719, bytes.fromhex("00000000060008914a0004007f00000106b70000"))]

    # Pinned XYPLEX UDP registration fields. Replies are distinguished by
    # transport direction, not by guessing from type or payload size.
    xyplex_messages = [
        (40101, 173, "010000071f900000"),
        (173, 40101, "01000000"),
        (173, 40101, "01000005"),
        (40101, 173, "a5ff1234c00155aa00ff61"),
        (173, 40101, "fe807fff616200ff"),
    ]
    yield "gen-xyplex-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / UDP(sport=sport, dport=dport) / Raw(bytes.fromhex(message))) for sport, dport, message in xyplex_messages]

    # XTP 4.0 wire VER=001. Fixed independently serialized layouts/checksums;
    # NOCHECK in record 4 skips only the body, not the header checksum.
    xtp_messages = [
        "800102030405060700000020000000004d811234010203040102030405060708",
        "8001020304050607000000200000000523b512340102030401020304050607086162636465",
        "8001020304050607000100200000000b49a61234010203040102030405060708112233445566778878797a",
        "800102030405060740000020000000050d7c12340102030401020304050607086162636465",
        "80010203040506070000002100000014a8bd12340102030401020304050607080102030405060708111213141516171821222324",
        "8001020304050607000000220000002d4ccd123401020304010203040506070800102401c0000202c00002010b7e9ca500180401000005dc000186a000000fa000030d4000001f4048454c4c4f",
        "80010203040506070001002200000019cfff123401020304010203040506070800082400000000000008040000000000112233445566778844",
        "800102030405060700000025000000289287123401020304010203040506070801020304050607081112131415161718212223240000000081828384858687880008040000000000",
        "80010203040506070800002700000048717a1234010203040102030405060708010203040506070811121314151617182122232400000000818283848586878800102401c0000202c00002010b7e9ca500180401000005dc000186a000000fa000030d4000001f40",
        "80010203040506070000002800000013004a123401020304010203040506070800000001000000016e6f206c697374656e6572",
    ]
    yield "gen-xtp-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2", proto=36) / Raw(bytes.fromhex(message))) for message in xtp_messages]

    # Oracle RMI transport/serialization grammars: a complete SingleOp Call,
    # including the 34-byte primitive block and a string-valued argument.
    rmi_message = bytes.fromhex("4a524d4900024c50aced0005772200000000000000000000000000000000000000000000ffffffff010203040506070874000673616d706c65")
    yield "gen-rmi-valid", tcp(rmi_message, port=1099)

    # [MS-CIFS] 2.2.4.52.1: request WordCount=0; exact ByteCount counts each
    # BufferFormat=2 plus its NUL-terminated OEM dialect string.
    cifs_message = bytearray(35)
    cifs_message[:5] = b"\xffSMB\x72"
    cifs_message[9] = 0x18
    cifs_message[10:12] = struct.pack("<H", 0xc853)
    cifs_message[26:28] = struct.pack("<H", 0x1234)
    cifs_message[30:32] = struct.pack("<H", 7)
    for dialect in (b"PC NETWORK PROGRAM 1.0", b"NT LM 0.12"):
        cifs_message += b"\x02" + dialect + b"\x00"
    cifs_message[33:35] = struct.pack("<H", len(cifs_message)-35)
    yield "gen-cifs-valid", tcp(struct.pack("!I", len(cifs_message)) + cifs_message, port=445)

    # MS-SMB2 negotiated-format layouts: 3.0/3.0.2/3.1.1 requests and
    # responses, including mandatory preauthentication and typed contexts.
    # Each record is independent; no successful session is implied.
    smb3_messages = [
        "fe534d4240000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000024000200010000007f000000000102030405460788090a0b0c0d0e0f000000000000000002020003",  # request-3.0
        "fe534d4240000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000024000200010000007f000000000102030405460788090a0b0c0d0e0f000000000000000000030203",  # request-3.0.2
        "fe534d4240000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000024000300010000007f000000000102030405460788090a0b0c0d0e0f70000000070000000003020311030000000000000200060000000000020002000100000001000900000000000100030001003141590000000000000003000c0000000000020000000100000001000300000000000600040000000000000000000000000007000c0000000000020000000000000001000200000000000800060000000000020002000100000005000400000000006e007300",  # request-3.1.1-contexts
        "fe534d424000000000000000000001000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004100010000030000000102030405460788090a0b0c0d0e0f3f000000000001000000020000000400080706050403020100000000000000008000000000000000",  # response-3.0
        "fe534d424000000000000000000001000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004100010002030000000102030405460788090a0b0c0d0e0f3f000000000001000000020000000400080706050403020100000000000000008000000000000000",  # response-3.0.2
        "fe534d424000000000000000000001000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004100010011030600000102030405460788090a0b0c0d0e0f3f0000000000010000000200000004000807060504030201000000000000000080000000800000000200040000000000010002000000000001000900000000000100030001003141590000000000000003000c0000000000020000000100000001000300000000000600040000000000000000000000000007000c000000000002000000000000000100020000000000080004000000000001000200",  # response-3.1.1-contexts
        "fe534d4240000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000024000100010000007f000000000102030405460788090a0b0c0d0e0f6800000001000000110300000100090000000000010003000100314159",  # request-3.1.1-minimal
    ]
    smb3_packets = []
    for message in smb3_messages:
        payload = bytes.fromhex(message)
        reply = bool(payload[16] & 1)
        sport, dport = (445, 40100) if reply else (40100, 445)
        src, dst = ("192.0.2.2", "192.0.2.1") if reply else ("192.0.2.1", "192.0.2.2")
        smb3_packets.append(eth(IP(src=src, dst=dst) / TCP(sport=sport, dport=dport, flags="PA", seq=2, ack=1) / Raw(struct.pack("!I", len(payload)) + payload)))
    yield "gen-smb3-valid", smb3_packets

    # Namespace-qualified SOAP 1.2 Identify, separate from the earlier
    # HTTP-length-only companion whose <s:Env/> is not a SOAP message.
    winrm_body = b'<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:i="http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd"><s:Header/><s:Body><i:Identify/></s:Body></s:Envelope>'
    winrm_message = b"POST /wsman HTTP/1.1\r\nHost: wsman.example\r\nContent-Type: application/soap+xml;charset=UTF-8\r\nContent-Length: " + str(len(winrm_body)).encode("ascii") + b"\r\n\r\n" + winrm_body
    yield "gen-winrm-identify-valid", tcp(winrm_message, port=5985)

    # RFC 2743/4178 DER structures carried by RFC 4559 HTTP Negotiate.
    # The final record is a tokenless challenge control; it does not provide
    # token-field evidence. Mechanism/MIC values are opaque, not usable sessions.
    gssapi_tokens = [
        "601b06062b0601050502a011300fa00d300b06092a864886f712010202",
        "602706062b0601050502a01d301ba019301706092a864886f712010202060a2b06010401823702020a",
        "602e06062b0601050502a0243022a00d300b06092a864886f712010202a10403020640a2050403616263a3040402dead",
        "a121301fa0030a0101a10b06092a864886f712010202a2050403010203a3040402beef",
        "a1073005a0030a0100",
        "a1073005a0030a0102",
        "a1073005a0030a0103",
        "a1023000",
        "a1063004a2020400",
    ]
    gssapi_messages = []
    for i, token in enumerate(gssapi_tokens):
        encoded = base64.b64encode(bytes.fromhex(token))
        if i < 3:
            message = b"GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate " + encoded + b"\r\n\r\n"
        else:
            status = b"401 Unauthorized" if i in (3, 5, 6) else b"200 OK"
            message = b"HTTP/1.1 " + status + b"\r\nWWW-Authenticate: Negotiate " + encoded + b"\r\nContent-Length: 0\r\n\r\n"
        gssapi_messages.append((i >= 3, message))
    gssapi_messages.append((True, b"HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Negotiate\r\nContent-Length: 0\r\n\r\n"))
    gssapi_packets = []
    for response, message in gssapi_messages:
        src, dst = ("192.0.2.2", "192.0.2.1") if response else ("192.0.2.1", "192.0.2.2")
        sport, dport = (80, 40100) if response else (40100, 80)
        gssapi_packets.append(eth(IP(src=src, dst=dst) / TCP(sport=sport, dport=dport, flags="PA", seq=2, ack=1) / Raw(message)))
    yield "gen-gssapi-valid", gssapi_packets

    # Docker Engine v1.41 ContainerList request codec examples. Each record
    # is independent; filters/booleans follow the pinned Moby v20.10.0 codec.
    docker_messages = [
        b"GET /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\n\r\n",
        b"GET /v1.41/containers/json?all=true&size=1&limit=25&filters=%7B%22status%22%3A%5B%22running%22%5D%2C%22label%22%3A%5B%22group%3Ddemo%22%5D%7D HTTP/1.1\r\nHost: localhost:2375\r\nContent-Length: 0\r\n\r\n",
        b"GET /v1.41/containers/json?all=NO&all=true&size=anything&limit=-1&since=older&before=newer&x=one+two&x=%2B&filters=%7B%22name%22%3A%7B%22demo%22%3Afalse%7D%7D HTTP/1.0\r\n\r\n",
        b"GET /v1.41/containers/json?all=&size=NONE&limit=&filters=null HTTP/1.1\r\nhOsT: [2001:db8::1]:2375\r\nX-Note: one\r\nX-Note: two\r\n\r\n",
    ]
    yield "gen-docker-api-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / TCP(sport=40100, dport=2375, flags="PA", seq=2, ack=1) / Raw(message)) for message in docker_messages]

    # Kubernetes v1 namespace list request with documented ListOptions. The
    # selectors and resource version are retained, not evaluated by a server.
    k8s_message = b"GET /api/v1/namespaces?limit=2&timeoutSeconds=30&labelSelector=env%3Ddemo&fieldSelector=metadata.name%3Dsample&resourceVersion=0&allowWatchBookmarks=false HTTP/1.1\r\nHost: api.example\r\nAccept: application/json\r\n\r\n"
    yield "gen-k8s-list-options-valid", tcp(k8s_message, port=6443)

    # RFC 8555 outer JWS structure, not verified signatures or usable account
    # requests. Empty payload means POST-as-GET; other object schemas are raw.
    acme_protected = b'{"alg":"ES256","nonce":"AQIDBA","url":"https://acme.example/acme/order/1","kid":"https://acme.example/acme/account/1"}'
    acme_packets = []
    for payload in (b"", b"{}", b'{"identifiers":[{"type":"dns","value":"example.test"}]}'):
        body = b'{"protected":"' + base64.urlsafe_b64encode(acme_protected).rstrip(b"=") + b'","payload":"' + base64.urlsafe_b64encode(payload).rstrip(b"=") + b'","signature":"' + base64.urlsafe_b64encode(bytes(64)).rstrip(b"=") + b'"}'
        message = b"POST /acme/order/1 HTTP/1.1\r\nHost: acme.example\r\nUser-Agent: binparser-fixture/1\r\nContent-Type: application/jose+json\r\nContent-Length: " + str(len(body)).encode("ascii") + b"\r\n\r\n" + body
        acme_packets.append(eth(IP(src="192.0.2.1", dst="192.0.2.2") / TCP(sport=40100, dport=443, flags="PA", seq=2, ack=1) / Raw(message)))
    yield "gen-acme-jws-valid", acme_packets

    # etcd /version (not /v3/version), plus the documented v3.6 response shape.
    etcd_request = b"GET /version HTTP/1.1\r\nHost: etcd.example:2379\r\nAccept: application/json\r\n\r\n"
    etcd_body = b'{"etcdserver":"3.6.0","etcdcluster":"3.6.0","storage":"3.5.2"}'
    etcd_response = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " + str(len(etcd_body)).encode("ascii") + b"\r\n\r\n" + etcd_body
    etcd_packets = tcp(etcd_request, port=2379)
    etcd_packets.append(eth(IP(src="192.0.2.2", dst="192.0.2.1") / TCP(sport=2379, dport=40100, flags="PA", seq=1001, ack=2+len(etcd_request)) / Raw(etcd_response)))
    yield "gen-etcd-version-valid", etcd_packets

    # S3 Signature V4 structural controls. Except the AWS public worked
    # example, signatures are inert zeros, not usable authorization results.
    s3_signature = b"0" * 64
    s3_empty_hash = b"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    s3_messages = [
        b"GET /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIA/20200101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=" + s3_signature + b"\r\nx-amz-date: 20200101T000000Z\r\nx-amz-content-sha256: " + s3_empty_hash + b"\r\n\r\n",
        b"GET /test.txt HTTP/1.1\r\nHost: examplebucket.s3.amazonaws.com\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,SignedHeaders=host;range;x-amz-content-sha256;x-amz-date,Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41\r\nRange: bytes=0-9\r\nx-amz-content-sha256: " + s3_empty_hash + b"\r\nx-amz-date: 20130524T000000Z\r\n\r\n",
        b"PUT /bucket/a//b%20c?partNumber=1&uploadId=x%2Fy HTTP/1.1\r\nHost: [2001:db8::1]:9000\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=content-type;host;x-amz-date;x-amz-security-token,Signature=" + s3_signature + b"\r\nContent-Type: application/octet-stream\r\nContent-Length: 3\r\nx-amz-security-token: EXAMPLE-SESSION\r\nx-amz-date: 20200101T010203Z\r\nx-amz-content-sha256: UNSIGNED-PAYLOAD\r\n\r\nabc",
        b"HEAD /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=date;host,Signature=" + s3_signature + b"\r\nDate: Wed, 01 Jan 2020 00:00:00 GMT\r\nx-amz-content-sha256: " + s3_empty_hash + b"\r\nContent-Length: 0\r\n\r\n",
        b"DELETE /bucket/object?versionId=one&versionId=two HTTP/1.1\r\nhOsT:\tminio.local \t\r\nAuThOrIzAtIoN: AWS4-HMAC-SHA256  Signature = " + s3_signature + b" ,\tCredential = example/20200101/local-region/s3/aws4_request , SignedHeaders = host;x-amz-date\r\nx-amz-date: 20200101T010203Z\r\nDate: overridden raw value\r\nx-amz-content-sha256: UNSIGNED-PAYLOAD\r\n\r\n",
        b"GET /bucket/./object//?acl&prefix=a+b HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=host;x-amz-date;x-amz-meta-color,Signature=" + s3_signature + b"\r\nx-amz-meta-color: blue\r\nx-amz-meta-color: green\r\nx-amz-date: 20200101T000000Z\r\nx-amz-content-sha256: " + s3_empty_hash + b"\r\n\r\n",
    ]
    yield "gen-minio-s3-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / TCP(sport=40100, dport=9000, flags="PA", seq=2, ack=1) / Raw(message)) for message in s3_messages]

    # DMTF Redfish service-root request structures. HTTP plaintext does not
    # establish TLS. GET bodies are framed/preserved, not resource semantics.
    redfish_messages = [
        b"GET /redfish/v1/ HTTP/1.1\r\nHost: bmc.lab\r\nOData-Version: 4.0\r\n\r\n",
        b"GET /redfish/v1?excerpt=&OEM-Example-View=summary&x=a+b&x=%2B HTTP/1.1\r\nHost: [2001:db8::1]:8443\r\nAccept: application/json\r\nIf-None-Match: W/\"fixture\"\r\n\r\n",
        b"GET /redfish/v1/?$expand=*($levels=2)&$select=RedfishVersion,Links/Sessions&includeoriginofcondition HTTP/1.1\r\nHost: bmc.example:443\r\nOData-MaxVersion: 4.0\r\nContent-Type: application/json\r\nContent-Length: 2\r\nUser-Agent: binparser-fixture/1.0\r\n\r\n{}",
        b"GET /redfish/v1/?$expand=~&$select=Name,Links HTTP/1.1\r\nhOsT:\tbmc.lab \t\r\nodata-version: 4.0\r\nTransfer-Encoding: chunked\r\nContent-Type: application/octet-stream\r\n\r\n3\r\nabc\r\n0\r\nX-End: yes\r\n\r\n",
    ]
    yield "gen-redfish-valid", [eth(IP(src="192.0.2.1", dst="192.0.2.2") / TCP(sport=40100, dport=443, flags="PA", seq=2, ack=1) / Raw(message)) for message in redfish_messages]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--only", action="append", default=[], metavar="FIXTURE",
                        help="write only this companion; repeat for several names")
    args = parser.parse_args()
    selected = set(args.only)
    records = list(fixtures())
    unknown = selected - {name for name, _ in records}
    if unknown:
        parser.error("unknown companion(s): " + ", ".join(sorted(unknown)))
    OUT.mkdir(parents=True, exist_ok=True)
    for name, packets in records:
        if selected and name not in selected:
            continue
        for i, packet in enumerate(packets):
            packet.time = 1_700_000_000 + i
        wrpcap(str(OUT / (name + ".pcap")), packets)
        print(name, len(packets))


if __name__ == "__main__":
    main()
