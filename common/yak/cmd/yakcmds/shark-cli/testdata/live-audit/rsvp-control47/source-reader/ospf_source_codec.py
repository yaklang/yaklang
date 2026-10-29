"""Independent RFC wire facts, not a candidate-native expected object.

All scalar fields carry source spans. Only complete original IPv6-declared
OSPFv3 bytes are parsed. No LSDB/neighbor/SA/key/replay or authentication result
is inferred. Reserved fields and unknown bodies remain source observations.
"""
import hashlib, ipaddress, struct

TYPE_NAMES = {1: "Hello", 2: "Database Description", 3: "Link State Request", 4: "Link State Update", 5: "Link State Acknowledgement"}
LSA_NAMES = {0x2001: "Router", 0x2002: "Network", 0x2003: "Inter-Area-Prefix", 0x2004: "Inter-Area-Router", 0x4005: "AS-External", 0x2007: "NSSA", 0x0008: "Link", 0x2009: "Intra-Area-Prefix"}

def digest(raw):
    return hashlib.sha256(raw).hexdigest()

def checksum(raw):
    raw += bytes(len(raw) & 1)
    value = sum(struct.unpack("!%dH" % (len(raw) // 2), raw))
    while value >> 16:
        value = (value & 65535) + (value >> 16)
    return (~value) & 65535

def fletcher(raw):
    a = b = 0
    for value in raw[2:]:
        a = (a + value) % 255
        b = (b + a) % 255
    return dict(c0=a, c1=b, valid=a == b == 0, age_excluded=True)

class View:
    def __init__(self, raw, origin):
        self.raw, self.origin = raw, origin

    def span(self, at, size):
        assert at >= 0 and size >= 0 and at + size <= len(self.raw), (at, size, len(self.raw))
        return dict(physical_range=[self.origin + at, self.origin + at + size], logical_PDU_range=[at, at + size],
                    source_SHA256=digest(self.raw[at:at + size]))

    def number(self, at, size, signed=False):
        value = self.span(at, size)
        value["value"] = int.from_bytes(self.raw[at:at + size], "big", signed=signed)
        return value

    def identifier(self, at):
        value = self.number(at, 4)
        value["dotted_router_identifier"] = str(ipaddress.IPv4Address(value["value"]))
        return value

    def ipv6(self, at):
        value = self.span(at, 16)
        value["value"] = str(ipaddress.IPv6Address(self.raw[at:at + 16]))
        return value

    def prefixes(self, at, end, count, trailer_kind):
        assert 0 <= count <= (end - at) // 4
        result = []
        for index in range(count):
            assert at + 4 <= end
            bits = self.raw[at]
            assert bits <= 128
            rounded = ((bits + 31) // 32) * 4
            assert at + 4 + rounded <= end
            stored = self.raw[at + 4:at + 4 + rounded]
            padded = stored + bytes(16 - rounded)
            network = ipaddress.IPv6Network((ipaddress.IPv6Address(padded), bits), strict=False)
            row = dict(index=index, prefix_length=self.number(at, 1), prefix_options=self.number(at + 1, 1),
                       encoded_address_prefix=self.span(at + 4, rounded), rounded_bytes=rounded,
                       normalized_prefix=str(network), source_padded_address=str(ipaddress.IPv6Address(padded)),
                       host_or_padding_bits_nonzero=int.from_bytes(padded, "big") != int(network.network_address),
                       next_logical_PDU_offset=at + 4 + rounded)
            row[trailer_kind] = self.number(at + 2, 2)
            result.append(row)
            at += 4 + rounded
        return result, at

    def lsa_header(self, at):
        assert at + 20 <= len(self.raw)
        typ = int.from_bytes(self.raw[at + 2:at + 4], "big")
        age = int.from_bytes(self.raw[at:at + 2], "big")
        sequence = int.from_bytes(self.raw[at + 12:at + 16], "big")
        return dict(header=self.span(at, 20), age=self.number(at, 2), age_without_DNA=age & 0x7fff,
                    DoNotAge=bool(age & 0x8000), LS_type=self.number(at + 2, 2), LS_type_hex="0x%04x" % typ,
                    U_bit=bool(typ & 0x8000), scope_bits=(typ >> 13) & 3, function_code=typ & 0x1fff,
                    base_RFC5340_body_name=LSA_NAMES.get(typ), unknown_to_supported_base_body_types=typ not in LSA_NAMES,
                    link_state_ID=self.identifier(at + 4), advertising_router_ID=self.identifier(at + 8),
                    sequence=self.number(at + 12, 4), signed_sequence=sequence if sequence < 0x80000000 else sequence - (1 << 32),
                    checksum=self.number(at + 16, 2), length=self.number(at + 18, 2))

    def lsa(self, at, enclosing_end):
        head = self.lsa_header(at)
        length = head["length"]["value"]
        assert length >= 20 and at + length <= enclosing_end
        end, typ = at + length, head["LS_type"]["value"]
        row = dict(header_fields=head, complete_wire=self.span(at, length), body=self.span(at + 20, length - 20),
                   full_LSA_checksum=fletcher(self.raw[at:end]), checksum_source_range=self.span(at + 2, length - 2),
                   complete_original_LSA=True, fields={}, prefixes=[])
        fields, pos = row["fields"], at + 20
        if typ == 0x2001:
            assert end - pos >= 4 and (end - pos - 4) % 16 == 0
            fields.update(flags=self.number(pos, 1), options=self.number(pos + 1, 3))
            links = []
            for offset in range(pos + 4, end, 16):
                links.append(dict(type=self.number(offset, 1), reserved=self.number(offset + 1, 1),
                                  metric=self.number(offset + 2, 2), interface_ID=self.number(offset + 4, 4),
                                  neighbor_interface_ID=self.number(offset + 8, 4), neighbor_router_ID=self.identifier(offset + 12)))
            fields["links"] = links
            pos = end
        elif typ == 0x2002:
            assert end - pos >= 4 and (end - pos - 4) % 4 == 0
            fields.update(reserved=self.number(pos, 1), options=self.number(pos + 1, 3),
                          attached_router_IDs=[self.identifier(offset) for offset in range(pos + 4, end, 4)])
            pos = end
        elif typ == 0x2003:
            assert end - pos >= 8
            fields.update(reserved_metric=self.number(pos, 1), metric=self.number(pos + 1, 3))
            row["prefixes"], pos = self.prefixes(pos + 4, end, 1, "reserved")
        elif typ == 0x2004:
            assert end - pos == 12
            fields.update(reserved_options=self.number(pos, 1), options=self.number(pos + 1, 3),
                          reserved_metric=self.number(pos + 4, 1), metric=self.number(pos + 5, 3),
                          destination_router_ID=self.identifier(pos + 8))
            pos = end
        elif typ in (0x4005, 0x2007):
            assert end - pos >= 8
            flags = self.raw[pos]
            reference = int.from_bytes(self.raw[pos + 6:pos + 8], "big")
            fields.update(flags=self.number(pos, 1), E_bit=bool(flags & 4), F_bit=bool(flags & 2), T_bit=bool(flags & 1),
                          reserved_flag_bits=flags & 0xf8, metric=self.number(pos + 1, 3))
            row["prefixes"], pos = self.prefixes(pos + 4, end, 1, "referenced_LS_type")
            if flags & 2:
                assert pos + 16 <= end
                fields["forwarding_address"] = self.ipv6(pos)
                pos += 16
            if flags & 1:
                assert pos + 4 <= end
                fields["external_route_tag"] = self.number(pos, 4)
                pos += 4
            if reference:
                assert pos + 4 <= end
                fields["referenced_link_state_ID"] = self.identifier(pos)
                pos += 4
        elif typ == 0x0008:
            assert end - pos >= 24
            count = int.from_bytes(self.raw[pos + 20:pos + 24], "big")
            fields.update(router_priority=self.number(pos, 1), options=self.number(pos + 1, 3),
                          link_local_address=self.ipv6(pos + 4), prefix_count=self.number(pos + 20, 4))
            row["prefixes"], pos = self.prefixes(pos + 24, end, count, "reserved")
        elif typ == 0x2009:
            assert end - pos >= 12
            count = int.from_bytes(self.raw[pos:pos + 2], "big")
            fields.update(prefix_count=self.number(pos, 2), referenced_LS_type=self.number(pos + 2, 2),
                          referenced_link_state_ID=self.identifier(pos + 4), referenced_advertising_router_ID=self.identifier(pos + 8))
            row["prefixes"], pos = self.prefixes(pos + 12, end, count, "metric")
        else:
            row["body_semantics"] = "opaque unrecognized base body; bounded raw source retained"
            pos = end
        assert pos == end, ("known LSA body has residual bytes", typ, at, pos, end)
        return row, end

def message(raw, physical_origin, ipv6_source, ipv6_destination, checksum_destination_known=True):
    view = View(raw, physical_origin)
    assert len(raw) >= 16 and raw[0] == 3
    length = int.from_bytes(raw[2:4], "big")
    assert length >= 16 and length <= len(raw)
    v = View(raw[:length], physical_origin)
    typ = raw[1]
    result = dict(message_name=TYPE_NAMES.get(typ), header=dict(version=v.number(0, 1), type=v.number(1, 1),
                  packet_length=v.number(2, 2), router_ID=v.identifier(4), area_ID=v.identifier(8),
                  checksum=v.number(12, 2), instance_ID=v.number(14, 1), reserved=v.number(15, 1)),
                  complete_message=v.span(0, length), original_parent_remainder=view.span(length, len(raw) - length),
                  message_scalar_source="independent wire parser", fields={}, prefixes=[], full_LSAs=[], header_only_LSAs=[], requests=[])
    p, options = 16, None
    if typ == 1:
        assert length >= 36 and (length - 36) % 4 == 0
        options = int.from_bytes(raw[21:24], "big")
        result["fields"] = dict(interface_ID=v.number(16, 4), router_priority=v.number(20, 1), options=v.number(21, 3),
                                 hello_interval=v.number(24, 2), router_dead_interval=v.number(26, 2),
                                 designated_router_ID=v.identifier(28), backup_designated_router_ID=v.identifier(32),
                                 neighbor_router_IDs=[v.identifier(offset) for offset in range(36, length, 4)])
    elif typ == 2:
        assert length >= 28 and (length - 28) % 20 == 0
        options = int.from_bytes(raw[17:20], "big")
        result["fields"] = dict(reserved_1=v.number(16, 1), options=v.number(17, 3), interface_MTU=v.number(20, 2),
                                 reserved_2=v.number(22, 1), flags=v.number(23, 1), DD_sequence=v.number(24, 4))
        result["header_only_LSAs"] = [v.lsa_header(offset) for offset in range(28, length, 20)]
    elif typ == 3:
        assert (length - 16) % 12 == 0
        result["requests"] = [dict(reserved=v.number(offset, 2), LS_type=v.number(offset + 2, 2),
                                    link_state_ID=v.identifier(offset + 4), advertising_router_ID=v.identifier(offset + 8))
                                for offset in range(16, length, 12)]
    elif typ == 4:
        assert length >= 20
        count = int.from_bytes(raw[16:20], "big")
        assert count <= (length - 20) // 20
        result["fields"]["LSA_count"] = v.number(16, 4)
        p = 20
        for _ in range(count):
            lsa, p = v.lsa(p, length)
            result["full_LSAs"].append(lsa)
        assert p == length
    elif typ == 5:
        assert (length - 16) % 20 == 0
        result["header_only_LSAs"] = [v.lsa_header(offset) for offset in range(16, length, 20)]
    else:
        result["message_semantics"] = "unrecognized packet type; no body semantics inferred"
    result["options_AT_bit"] = None if options is None else bool(options & 0x400)
    result["options_LLS_bit"] = None if options is None else bool(options & 0x200)
    # Never treat arbitrary trailing bytes as authenticated AT or neighbor state.
    result["authentication_trailer_context"] = "observed AT option" if result["options_AT_bit"] else "no observed AT option on this packet; neighbor/configuration state not inferred"
    pseudo = ipaddress.IPv6Address(ipv6_source).packed + ipaddress.IPv6Address(ipv6_destination).packed + length.to_bytes(4, "big") + b"\x00\x00\x00\x59"
    residual = checksum(pseudo + raw[:length])
    zeroed = bytearray(raw[:length]); zeroed[12:14] = b"\x00\x00"
    result["source_checksum"] = dict(computed_checksum=checksum(pseudo + bytes(zeroed)), residual=residual,
                                    mathematical_match=residual == 0, destination_context_known=checksum_destination_known,
                                    pseudoheader_upper_layer_length=length, pseudoheader_next_header=89,
                                    validation_applicability="omitted under RFC7166" if result["options_AT_bit"] else "base RFC5340 mathematical check only; no authenticated neighbor state",
                                    authentication_verified=False)
    for row in result["header_only_LSAs"]:
        row["LSA_checksum_validation"] = "not computed: only LSA header is present"
    return result
