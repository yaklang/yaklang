"""Independent raw-byte checks of native IS-IS entry associations.

No reference normalization or route/adjacency state. Complements repeated-field
comparisons: correct multisets alone cannot prove metric/prefix/ID association.
"""
import base64
import collections
import ipaddress


def system_id(b):
    text = '.'.join(b[i:i+2].hex() for i in range(0, 6, 2))
    if len(b) > 6:
        text += '.'+b[6:7].hex()
    if len(b) > 7:
        text += '-'+b[7:8].hex()
    return text


def verify(w, session):
    checks, differences = collections.Counter(), []

    def equal(name, got, want):
        checks['wire_entry_association'] += 1
        if got != want or isinstance(want, bool) and not isinstance(got, bool):
            address_types = (ipaddress.IPv4Address, ipaddress.IPv6Address, ipaddress.IPv4Network, ipaddress.IPv6Network)
            differences.append({'association': name, 'native': str(got) if isinstance(got, address_types) else got, 'wire': str(want) if isinstance(want, address_types) else want})

    def span(row, key, start, size):
        equal(key+' byte association', row.get('Wire Field Ranges', {}).get(key), [start, start+size])

    def metrics(row, pos, ip):
        for i, name in enumerate(('Default', 'Delay', 'Expense', 'Error')):
            v = w[pos+i]
            equal(name+' metric', row.get(name+' Metric'), v & 63)
            equal(name+' metric scope', row.get(name+' Metric External'), bool(v & 64))
            span(row, name+' Metric', pos+i, 1)
            if i:
                equal(name+' metric support', row.get(name+' Metric Supported'), not bool(v & 128))
            else:
                equal('default metric high-bit meaning', row.get('Up Down' if ip else 'Default Metric Reserved Bit'), bool(v & 128))
                equal('default metric is not optional support', 'Default Metric Supported' in row, False)

    pdu = w[4] & 31
    hello, lsp, snp = pdu in (15, 16, 17), pdu in (18, 20), pdu in (24, 25, 26, 27)
    for tlv in session.get('TLVs') or []:
        t, n = tlv['Type'], tlv['Length']
        at = tlv['Wire Field Ranges']['Type'][0]
        equal('TLV type at its own envelope', t, w[at])
        equal('TLV length at its own envelope', n, w[at+1])
        pos, stop = at+2, at+2+n
        if tlv.get('TLV Name') == 'Malformed TLV value':
            equal('malformed value retains literal bytes', base64.b64decode(tlv.get('Opaque Value') or '').hex(), w[pos:stop].hex())
            equal('malformed value has no fabricated neighbors', 'Neighbors' in tlv, False)
            equal('malformed value has no fabricated reachability', 'Reachability Entries' in tlv, False)
            continue
        if t == 6 and pdu in (15, 16):
            rows = tlv.get('IS Neighbors') or []
            equal('SNPA count', len(rows), n//6)
            for row, start in zip(rows, range(pos, stop, 6)):
                equal('SNPA identity', row.get('SNPA'), ':'.join('%02x' % v for v in w[start:start+6]))
                equal('SNPA is not System ID', 'System ID' in row, False)
                span(row, 'SNPA', start, 6)
        elif t == 9 and snp:
            rows = tlv.get('LSP Entries') or []
            equal('reported LSP count', len(rows), n//16)
            for row, start in zip(rows, range(pos, stop, 16)):
                for key, off, size in (('Remaining Lifetime Seconds', 0, 2), ('Sequence Number', 10, 4), ('Reported Checksum', 14, 2)):
                    equal('same reported LSP '+key, row.get(key), int.from_bytes(w[start+off:start+off+size], 'big'))
                    span(row, key, start+off, size)
                equal('reported LSP identity', row.get('LSP ID'), system_id(w[start+2:start+10]))
                span(row, 'LSP ID', start+2, 8)
        elif t in (2, 3) and lsp:
            if t == 2:
                key = 'Virtual Flag' if pdu == 18 else 'Reserved Neighbor Octet'
                equal('neighbor header meaning', tlv.get(key), bool(w[pos] & 1) if pdu == 18 else w[pos])
                pos += 1
            step, width, key = (11, 7, 'Neighbor Node ID') if t == 2 else (10, 6, 'System ID')
            rows = tlv.get('Neighbors') or []
            equal('legacy neighbor count', len(rows), (stop-pos)//step)
            for row, start in zip(rows, range(pos, stop, step)):
                metrics(row, start, False)
                equal('metric belongs to this neighbor', row.get(key), system_id(w[start+4:start+4+width]))
                span(row, key, start+4, width)
        elif t in (128, 130) and lsp:
            rows = tlv.get('Reachability Entries') or []
            equal('legacy IPv4 entry count', len(rows), n//12)
            for row, start in zip(rows, range(pos, stop, 12)):
                metrics(row, start, True)
                for key, off in (('IPv4 Prefix', 4), ('IPv4 Mask', 8)):
                    equal('metric belongs to '+key, row.get(key), str(ipaddress.IPv4Address(w[start+off:start+off+4])))
                    span(row, key, start+off, 4)
        elif t in (135, 236) and lsp:
            rows = tlv.get('Reachability Entries') or []
            consumed = 0
            for row in rows:
                start = pos
                v6 = t == 236
                fixed, width, bit = (6, 16, 32) if v6 else (5, 4, 64)
                control = w[start+4]
                bits = w[start+5] if v6 else control & 63
                count = (bits+7)//8
                encoded = w[start+fixed:start+fixed+count]
                literal = ipaddress.ip_address(encoded+b'\0'*(width-count))
                prefix = ipaddress.ip_network((literal, bits), strict=False)
                equal('prefix metric association', row.get('Metric'), int.from_bytes(w[start:start+4], 'big'))
                span(row, 'Metric', start, 4)
                equal('prefix up/down association', row.get('Up Down'), bool(control & 128))
                equal('prefix sub-TLV presence', row.get('Sub-TLVs Present'), bool(control & bit))
                equal('prefix length association', row.get('Prefix Length'), bits)
                equal('literal encoded prefix', base64.b64decode(row.get('Prefix Bytes') or '').hex(), encoded.hex())
                equal('literal expanded prefix', ipaddress.ip_address(row['Prefix Address']), literal)
                equal('canonical prefix association', ipaddress.ip_network(row['Canonical Prefix']), prefix)
                equal('canonical prefix address', ipaddress.ip_address(row['Canonical Prefix Address']), prefix.network_address)
                span(row, 'Prefix Bytes', start+fixed, count)
                if v6:
                    equal('IPv6 prefix external bit', row.get('External'), bool(control & 64))
                pos = start+fixed+count
                if control & bit:
                    size = w[pos]
                    equal('prefix sub-TLV block length', row.get('Sub-TLV Length'), size)
                    span(row, 'Sub-TLV Length', pos, 1)
                    pos += 1+size
                consumed += 1
            equal('complete prefix-entry block', pos, stop)
            equal('prefix-entry count', len(rows), consumed)
        elif t == 22 and lsp:
            rows = tlv.get('Neighbors') or []
            for row in rows:
                size = w[pos+10]
                equal('extended neighbor and metric', row.get('Neighbor Node ID'), system_id(w[pos:pos+7]))
                span(row, 'Neighbor Node ID', pos, 7)
                equal('extended metric association', row.get('Metric'), int.from_bytes(w[pos+7:pos+10], 'big'))
                span(row, 'Metric', pos+7, 3)
                equal('link sub-TLV block association', row.get('Sub-TLV Length'), size)
                span(row, 'Sub-TLV Length', pos+10, 1)
                pos += 11+size
            equal('complete extended-neighbor block', pos, stop)
    return checks, differences
