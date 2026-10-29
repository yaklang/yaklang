"""Source-first IPv6 NextHeader89 census for the frozen 37 public inputs.

No native output is read. Link/IP/extension/carrier bounds are checked against
original records. IPv6 fragments are retained as source facts, never parsed
as independent complete OSPF messages. Only selected original frames are saved
outside Git; unknown encapsulations and invalid parents stay explicit.
"""
from pathlib import Path
import argparse, collections, hashlib, importlib.util, ipaddress, json, struct, sys, traceback, zlib
from ospf_source_codec import message, digest

def sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

class Census:
    def __init__(self, raw, record, frame, counters):
        self.w, self.record, self.frame, self.counts = raw, record, frame, counters
        self.candidates, self.fragments, self.refusals = [], [], []

    def require(self, start, size, end):
        if not 0 <= start <= start + size <= end <= len(self.w):
            raise ValueError("source header exceeds bounded enclosing bytes")

    def u(self, start, size=2):
        return int.from_bytes(self.w[start:start + size], "big")

    def enter(self, kind, start, end, path, depth=0):
        if depth > 8:
            self.counts["encapsulation_depth_refused"] += 1
            return
        if kind == "ethernet":
            self.require(start, 14, end)
            typ, at = self.u(start + 12), start + 14
            layers = path + [dict(kind="Ethernet", range=[start, at])]
            for _ in range(8):
                if typ not in (0x8100, 0x88a8, 0x9100, 0x9200):
                    break
                self.require(at, 4, end)
                layers += [dict(kind="VLAN", range=[at, at + 4], TCI=self.u(at), outer_type=typ)]
                typ, at = self.u(at + 2), at + 4
            else:
                raise ValueError("source VLAN depth exceeds bounded audit profile")
            if typ <= 1500:
                self.require(at, typ, end)
                end = at + typ
                if self.w[at:at + 6] == b"\xaa\xaa\x03\x00\x00\x00" and at + 8 <= end:
                    layers += [dict(kind="LLC-SNAP", range=[at, at + 8])]
                    typ, at = self.u(at + 6), at + 8
                else:
                    self.counts["non_IP_8023_LLC"] += 1
                    return
            return self.ether_type(typ, at, end, layers, depth)
        if kind == "ppp":
            at = start
            if self.w[at:at + 2] == b"\xff\x03":
                at += 2
            self.require(at, 1, end)
            size = 1 if self.w[at] & 1 else 2
            self.require(at, size, end)
            proto = self.u(at, size)
            if proto in (0x21, 0x57):
                self.enter("ipv4" if proto == 0x21 else "ipv6", at + size, end,
                           path + [dict(kind="PPP", range=[start, at + size], protocol=proto)], depth + 1)
            else:
                self.counts["PPP_other_protocol_0x%x" % proto] += 1
            return
        if kind == "ipv4":
            self.require(start, 20, end)
            if self.w[start] >> 4 != 4:
                raise ValueError("declared IPv4 carrier version differs")
            ihl, total = (self.w[start] & 15) * 4, self.u(start + 2)
            if ihl < 20 or total < ihl:
                raise ValueError("invalid IPv4 parent declared length")
            self.require(start, ihl, end)
            limit = min(start + total, end)
            proto, frag = self.w[start + 9], self.u(start + 6)
            if frag & 0x3fff:
                self.counts["IPv4_fragment_protocol_%d" % proto] += 1
                if proto in (4, 41, 47):
                    self.counts["IPv4_fragmented_tunnel_needs_reassembly"] += 1
                return
            layers = path + [dict(kind="IPv4", range=[start, start + ihl], declared_end=start + total, available_end=limit)]
            self.upper(proto, start + ihl, limit, layers, depth)
            return
        if kind != "ipv6":
            raise ValueError("unhandled source entry kind")
        self.require(start, 40, end)
        if self.w[start] >> 4 != 6:
            raise ValueError("declared IPv6 carrier version differs")
        payload_length, nxt = self.u(start + 4), self.w[start + 6]
        at, declared_end = start + 40, start + 40 + payload_length
        limit = min(end, declared_end)
        src, dst = str(ipaddress.IPv6Address(self.w[start + 8:start + 24])), str(ipaddress.IPv6Address(self.w[start + 24:start + 40]))
        self.counts["IPv6_source_headers"] += 1
        self.counts["IPv6_initial_NextHeader_%d" % nxt] += 1
        layers = path + [dict(kind="IPv6", range=[start, start + 40], declared_end=declared_end, available_end=limit,
                             source=src, destination=dst, initial_next_header=nxt, payload_length=payload_length)]
        extension_rows = []
        destination_known = True
        checksum_destination = dst
        if payload_length == 0:
            self.counts["IPv6_zero_payload_or_unhandled_jumbo"] += 1
            if nxt == 89:
                self.candidates.append(dict(carrier=layers, extensions=[], ipv6_source=src, ipv6_destination=dst,
                                           payload_range=[at, limit], parent_declared_end=declared_end,
                                           parent_complete=declared_end <= end, zero_payload=True,
                                           source_boundary_refusal="zero declared IPv6 payload; no complete child admitted"))
            return
        for _ in range(16):
            if nxt not in (0, 43, 44, 51, 60):
                break
            self.require(at, 2, limit)
            old, next_header = nxt, self.w[at]
            if old == 44:
                self.require(at, 8, limit)
                flags = self.u(at + 2)
                ext = dict(kind="Fragment", range=[at, at + 8], next_header=next_header,
                           fragment_offset=(flags >> 3) * 8, more=bool(flags & 1),
                           reserved_octet=self.w[at + 1], reserved_bits=(flags >> 1) & 3, identification=self.u(at + 4, 4))
                extension_rows.append(ext)
                self.counts["IPv6_fragment_headers"] += 1
                if ext["fragment_offset"] or ext["more"]:
                    self.fragments.append(dict(carrier=layers, extensions=extension_rows, ipv6_source=src, ipv6_destination=dst,
                                               domain=[self.record["section"], self.record["interface"]],
                                               fragment=ext, fragment_payload_range=[at + 8, limit],
                                               parent_complete=declared_end <= end, payload_SHA256=digest(self.w[at + 8:limit])))
                    self.counts["IPv6_fragment_payload_NextHeader_%d" % next_header] += 1
                    return
                self.counts["IPv6_atomic_fragment_headers"] += 1
                size = 8
            else:
                size = (self.w[at + 1] + (2 if old == 51 else 1)) * (4 if old == 51 else 8)
                self.require(at, size, limit)
                if old == 51 and size < 12:
                    raise ValueError("AH header shorter than fixed header")
                ext = dict(kind={0: "Hop-by-Hop", 43: "Routing", 51: "AH", 60: "Destination"}[old],
                           range=[at, at + size], next_header=next_header, length_octet=self.w[at + 1])
                if old == 51:
                    ext.update(reserved=self.u(at + 2), SPI=self.u(at + 4, 4), sequence=self.u(at + 8, 4),
                               ICV_range=[at + 12, at + size], ICV_SHA256=digest(self.w[at + 12:at + size]), authentication_verified=False)
                if old == 43:
                    ext.update(routing_type=self.w[at + 2], segments_left=self.w[at + 3])
                    if self.w[at + 3]:
                        if self.w[at + 2] in (0, 2) and size >= 24 and (size - 8) % 16 == 0:
                            checksum_destination = str(ipaddress.IPv6Address(self.w[at + size - 16:at + size]))
                        else:
                            destination_known = False
                extension_rows.append(ext)
            at, nxt = at + size, next_header
        else:
            raise ValueError("extension depth exceeds bounded audit profile")
        self.counts["IPv6_terminal_NextHeader_%d" % nxt] += 1
        if nxt == 89:
            value = dict(carrier=layers, extensions=extension_rows, ipv6_source=src, ipv6_destination=dst,
                         checksum_destination=checksum_destination, checksum_destination_context_known=destination_known,
                         payload_range=[at, limit], parent_declared_end=declared_end, parent_complete=declared_end <= end,
                         frame_tail_outside_IP_range=[declared_end, end] if declared_end < end else None,
                         zero_payload=False)
            self.candidates.append(value)
        else:
            self.upper(nxt, at, limit, layers + extension_rows, depth)

    def ether_type(self, typ, at, end, path, depth):
        if typ in (0x0800, 0x86dd):
            return self.enter("ipv4" if typ == 0x0800 else "ipv6", at, end, path, depth + 1)
        if typ in (0x8847, 0x8848):
            begin, labels = at, []
            for _ in range(16):
                self.require(at, 4, end)
                value = self.u(at, 4)
                labels.append(value)
                at += 4
                if value & 0x100:
                    break
            else:
                raise ValueError("MPLS depth exceeds audit bound")
            path += [dict(kind="MPLS", range=[begin, at], labels=labels)]
            if at < end and self.w[at] >> 4 in (4, 6):
                return self.enter("ipv4" if self.w[at] >> 4 == 4 else "ipv6", at, end, path, depth + 1)
            self.counts["MPLS_non_direct_IP_not_decoded"] += 1
        elif typ == 0x8864:
            self.require(at, 6, end)
            if self.w[at] != 0x11 or self.w[at + 1] != 0:
                self.counts["PPPoE_non_session_data"] += 1
                return
            length = self.u(at + 4)
            self.require(at + 6, length, end)
            self.enter("ppp", at + 6, at + 6 + length, path + [dict(kind="PPPoE", range=[at, at + 6])], depth + 1)
        else:
            self.counts["other_EtherType_0x%04x" % typ] += 1

    def upper(self, proto, at, end, path, depth):
        if proto in (4, 41):
            self.enter("ipv4" if proto == 4 else "ipv6", at, end, path + [dict(kind="IP-in-IP", range=[at, at])], depth + 1)
        elif proto == 47:
            self.require(at, 4, end)
            flags, typ = self.u(at), self.u(at + 2)
            if flags & 7 or flags & 0x4000:
                self.counts["GRE_version_or_routing_not_decoded"] += 1
                return
            size = 4 + (4 if flags & 0x8000 else 0) + (4 if flags & 0x2000 else 0) + (4 if flags & 0x1000 else 0)
            self.require(at, size, end)
            entry = path + [dict(kind="GRE", range=[at, at + size], protocol=typ, flags=flags)]
            if typ == 0x6558:
                self.enter("ethernet", at + size, end, entry, depth + 1)
            elif typ == 0x880b:
                self.enter("ppp", at + size, end, entry, depth + 1)
            else:
                self.ether_type(typ, at + size, end, entry, depth + 1)
        elif proto == 17:
            if at + 8 > end:
                return
            src, dst, length = self.u(at), self.u(at + 2), self.u(at + 4)
            if length < 8 or at + length > end:
                self.counts["UDP_parent_invalid_not_decapsulated"] += 1
                return
            begin, limit = at + 8, at + length
            if 4789 in (src, dst) and begin + 8 <= limit and self.w[begin] & 8:
                self.enter("ethernet", begin + 8, limit, path + [dict(kind="UDP-VXLAN", range=[at, begin + 8])], depth + 1)
            elif 6081 in (src, dst) and begin + 8 <= limit and self.w[begin] >> 6 == 0:
                size, typ = 8 + (self.w[begin] & 63) * 4, self.u(begin + 2)
                self.require(begin, size, limit)
                entry = path + [dict(kind="UDP-Geneve", range=[at, begin + size], protocol=typ)]
                if typ == 0x6558:
                    self.enter("ethernet", begin + size, limit, entry, depth + 1)
                else:
                    self.ether_type(typ, begin + size, limit, entry, depth + 1)

    def run(self):
        link, w = self.record["link"], self.w
        if link == 1:
            self.enter("ethernet", 0, len(w), [])
        elif link == 274:
            at = 1 if w and w[0] == 0x50 else 0
            while at < min(65, len(w)) and w[at] == 0x55:
                at += 1
            if at >= len(w) or w[at] != 0xd5 or len(w) < at + 1 + 14 + 4:
                raise ValueError("mPacket preamble/SFD boundary unavailable")
            at += 1
            fcs = zlib.crc32(w[at:-4]) == int.from_bytes(w[-4:], "little")
            if not fcs:
                self.counts["mPacket_FCS_mismatch"] += 1
            self.enter("ethernet", at, len(w) - 4, [dict(kind="mPacket", preamble_range=[0, at], FCS_range=[len(w) - 4, len(w)], FCS_valid=fcs)])
        elif link == 0:
            self.require(0, 4, len(w))
            family = int.from_bytes(w[:4], "little")
            if family == 2:
                self.enter("ipv4", 4, len(w), [dict(kind="Loopback", range=[0, 4], family=family)])
            elif family in (10, 23, 24, 28, 30):
                self.enter("ipv6", 4, len(w), [dict(kind="Loopback", range=[0, 4], family=family)])
            else:
                self.counts["loopback_unknown_family_%d" % family] += 1
        elif link == 113:
            self.require(0, 16, len(w))
            self.ether_type(self.u(14), 16, len(w), [dict(kind="Linux-SLL", range=[0, 16])], 0)
        elif link == 101:
            self.require(0, 1, len(w))
            if w[0] >> 4 in (4, 6):
                self.enter("ipv4" if w[0] >> 4 == 4 else "ipv6", 0, len(w), [])
            else:
                self.counts["raw_unknown_IP_version"] += 1
        else:
            self.counts["unsupported_link_%d" % link] += 1

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("inputs", "reader", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args()
    a.out.mkdir(exist_ok=False)
    doc = json.loads(a.inputs.read_bytes())
    assert doc["input_count"] == 37
    sys.path.insert(0, str(a.reader.parent))
    spec = importlib.util.spec_from_file_location("source_capture_reader", a.reader)
    reader = importlib.util.module_from_spec(spec); spec.loader.exec_module(reader)
    rows = []
    for source in doc["inputs"]:
        capture = Path(source["capture"])
        assert sha(capture) == source["capture_SHA256"]
        folder = a.out / ("input-%02d" % source["input"]); folder.mkdir()
        raw_folder = folder / "selected-original-frames"; raw_folder.mkdir()
        counts, observations, fragments, refusals = collections.Counter(), [], [], []
        for number, record in enumerate(reader.capture_records(capture), 1):
            visitor = Census(record["wire"], record, number, counts)
            try:
                visitor.run()
            except (AssertionError, ValueError, IndexError) as error:
                refusals.append(dict(frame=number, reason=str(error), wire_SHA256=digest(record["wire"]), link=record["link"]))
            for fragment in visitor.fragments:
                fragments.append(dict(frame=number, frame_wire_SHA256=digest(record["wire"]), **fragment))
            for index, candidate in enumerate(visitor.candidates):
                filename = "frame-%d.bin" % number
                (raw_folder / filename).write_bytes(record["wire"])
                item = dict(input=source["input"], frame=number, section=record["section"], interface=record["interface"],
                            link=record["link"], time_ns=record["time_ns"], captured_length=record["captured_length"],
                            original_length=record["original_length"], frame_wire_SHA256=digest(record["wire"]),
                            original_frame_file=str(raw_folder / filename), ingress=candidate,
                            raw_NextHeader89_candidate_index=index, source_fragment_reassembly_used=False)
                lo, hi = candidate["payload_range"]
                item["source_payload_SHA256"] = digest(record["wire"][lo:hi])
                if not candidate.get("source_boundary_refusal"):
                    try:
                        item["OSPF_source"] = message(record["wire"][lo:hi], lo, candidate["ipv6_source"],
                                                       candidate["checksum_destination"], candidate["checksum_destination_context_known"])
                    except (AssertionError, ValueError, IndexError) as error:
                        item["OSPF_source_refusal"] = dict(reason=str(error), independent_wire_parser=True)
                observations.append(item)
        for name, value in (("NextHeader89-source-facts.json", observations), ("IPv6-fragment-source-facts.json", fragments), ("parent-refusals.json", refusals)):
            (folder / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
        item = dict(input=source["input"], capture=str(capture), capture_SHA256=source["capture_SHA256"],
                    physical_frames=source["physical_frames"], counters=dict(counts),
                    NextHeader89_candidates=len(observations), complete_parsed_v3_messages=sum("OSPF_source" in row for row in observations),
                    candidates_with_source_refusal=sum("OSPF_source_refusal" in row for row in observations),
                    IPv6_non_atomic_fragments=len(fragments), fragments_directly_declaring_89=sum(row["fragment"]["next_header"] == 89 for row in fragments),
                    parent_refusals=len(refusals), source_facts_file=str(folder / "NextHeader89-source-facts.json"),
                    source_facts_SHA256=sha(folder / "NextHeader89-source-facts.json"),
                    fragment_file=str(folder / "IPv6-fragment-source-facts.json"), fragment_SHA256=sha(folder / "IPv6-fragment-source-facts.json"))
        rows.append(item)
        print(json.dumps(dict(input=source["input"], frames=source["physical_frames"], NH89=item["NextHeader89_candidates"], parsed=item["complete_parsed_v3_messages"], IPv6_fragments=len(fragments))), flush=True)
    result = dict(scope=__doc__, inputs_inventory_SHA256=sha(a.inputs), driver_SHA256=sha(Path(__file__)),
                  independent_codec_SHA256=sha(Path(__file__).with_name("ospf_source_codec.py")), input_count=37,
                  captures=rows, total_NextHeader89_candidates=sum(row["NextHeader89_candidates"] for row in rows),
                  total_complete_parsed_v3=sum(row["complete_parsed_v3_messages"] for row in rows),
                  no_native_output_or_key_read=True, no_expected_objects_generated=True,
                  limitations=["Reference comparison and full fragment-group reconstruction are separate follow-up checks; this file is a literal source census, not final ingress completeness proof.",
                               "Unrecognized encapsulation counters and parent refusals remain explicit; payload bytes are never searched heuristically for an OSPF signature."])
    target = a.out / "summary.json"; target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), NH89=result["total_NextHeader89_candidates"], parsed=result["total_complete_parsed_v3"])))

if __name__ == "__main__":
    main()
