"""Original-byte classification of the historical EOL source gap.

This is a source inventory and explicit compositional proof index, not a blanket
whole-row or application parity certificate.  Known whole-row plans are rebound
to original records; byte-field-only plans remain clearly byte-field-only.
"""
import argparse
import collections
import hashlib
import importlib.util
import ipaddress
import json
from pathlib import Path
import sys


def sha(p):
    h = hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda: f.read(1048576), b''): h.update(b)
    return h.hexdigest()


def rows(p):
    with Path(p).open(encoding='utf-8-sig') as f:
        for line in f: yield json.loads(line)


def same(a, b):
    if type(a) is not type(b): return False
    if isinstance(a, dict): return a.keys() == b.keys() and all(same(a[k], b[k]) for k in a)
    if isinstance(a, list): return len(a) == len(b) and all(same(x, y) for x, y in zip(a, b))
    return a == b


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def digest(v):
    return hashlib.sha256(json.dumps(v, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()).hexdigest()


def outer(r):
    w = r['wire']
    assert r['link'] == 1 and len(w) >= 34 and w[12:14] == b'\x08\x00'
    at = 14
    assert w[at] >> 4 == 4
    n = (w[at] & 15) * 4
    total = int.from_bytes(w[at+2:at+4], 'big')
    fragment = int.from_bytes(w[at+6:at+8], 'big')
    assert n >= 20 and at + n <= len(w) and (total == 0 or total >= n)
    assert fragment & 0x3fff == 0, 'fragment profile not included in this uncovered list'
    options = []
    pos = at + 20
    eol = None
    while pos < at + n:
        typ = w[pos]
        if typ == 0:
            options.append(dict(kind=0, offset=pos, length=1));eol = pos;break
        if typ == 1:
            options.append(dict(kind=1, offset=pos, length=1));pos += 1;continue
        assert pos + 2 <= at + n and w[pos+1] >= 2 and pos + w[pos+1] <= at + n
        options.append(dict(kind=typ, offset=pos, length=w[pos+1]));pos += w[pos+1]
    end = len(w) if total == 0 else min(len(w), at + total)
    return dict(network_offset=at, own_offset=at+n, own_end=end,
                network_header_length=n, declared_total_length=total,
                declared_child_protocol=w[at+9], source=str(ipaddress.ip_address(w[at+12:at+16])),
                destination=str(ipaddress.ip_address(w[at+16:at+20])),
                source_range=[at+12, at+16], destination_range=[at+16, at+20],
                options=options, EOL_offset=eol, padding_range=[eol+1, at+n] if eol is not None else None,
                EOL_scope='outer-options' if eol is not None else 'no-outer-EOL; nested-child-or-observer-effect-requires-separate-proof',
                extent='legacy-zero-length-observation' if total == 0 else 'incomplete-declared-extent' if at+total > len(w) else 'complete-declared-extent')


def bounded_child(w, p):
    """Only explicit common headers/envelopes. No flow or subtype success claims."""
    start, end = p['own_offset'], p['own_end']
    proto = p['declared_child_protocol']
    b = w[start:end]
    n = len(b)
    out = dict(declared_protocol_number=proto, offset=start, end=end, available=n,
               scope='Observed header/envelope facts only; no checksum, authentication, subtype or connection validation')
    if proto == 6:
        out.update(kind='TCP-declared-but-no-captured-body' if n == 0 else 'TCP-header-facts', minimum_fixed=20)
        if n >= 13: out['declared_header_bytes'] = (b[12] >> 4) * 4
    elif proto == 17:
        out.update(kind='UDP', minimum_fixed=8)
        if n >= 8:
            length = int.from_bytes(b[4:6], 'big')
            out.update(source_port=int.from_bytes(b[:2], 'big'), destination_port=int.from_bytes(b[2:4], 'big'),
                       declared_length=length, envelope='below-fixed-header' if length < 8 else 'exceeds-captured-parent-extent' if length > n else 'complete',
                       child_dissection_selected=False)
    elif proto == 132:
        out.update(kind='SCTP common/chunk envelopes', minimum_fixed=12, chunks=[])
        if n < 12: out['boundary'] = 'common-header-short';return out
        out.update(source_port=int.from_bytes(b[:2], 'big'), destination_port=int.from_bytes(b[2:4], 'big'),
                   verification_tag=int.from_bytes(b[4:8], 'big'))
        pos = 12
        while pos < n:
            if pos + 4 > n: out['boundary'] = 'chunk-header-short';out['failure_offset'] = start+pos;break
            size = int.from_bytes(b[pos+2:pos+4], 'big')
            chunk = dict(offset=start+pos, type=b[pos], flags=b[pos+1], declared_length=size, available=n-pos)
            out['chunks'].append(chunk)
            if size < 4: out['boundary'] = 'chunk-length-below-four';break
            if size > n-pos: out['boundary'] = 'chunk-exceeds-captured-parent-extent';break
            rounded = (size+3) & ~3
            if rounded > n-pos: out['boundary'] = 'final-chunk-padding-not-captured';break
            pos += rounded
        else: out['boundary'] = 'common-chunk-envelopes-fit; subtype-semantics-unverified'
    elif proto == 103:
        out.update(kind='PIM common header only', minimum_fixed=4)
        if n >= 4:
            out.update(observed_version=b[0] >> 4, observed_type=b[0] & 15,
                       body_scope='PIM message-specific body not validated by this classifier')
    elif proto == 2:
        out.update(kind='IGMP common/report envelopes', minimum_fixed=8)
        if n < 8: out['boundary'] = 'base-header-short';return out
        out['type'] = b[0]
        if b[0] != 0x22: out['boundary'] = 'non-v3-report-body-not-validated';return out
        count = int.from_bytes(b[6:8], 'big');pos = 8
        out.update(declared_group_records=count, records=[])
        for _ in range(count):
            if pos + 8 > n: out['boundary'] = 'record-fixed-header-short';out['failure_offset'] = start+pos;break
            sources = int.from_bytes(b[pos+2:pos+4], 'big');aux = b[pos+1]
            size = 8 + 4*sources + 4*aux
            out['records'].append(dict(offset=start+pos, record_type=b[pos], aux_words=aux, sources=sources, required=size, available=n-pos))
            if size > n-pos: out['boundary'] = 'record-source-or-aux-extent-short';break
            pos += size
        else: out.update(boundary='declared-report-envelopes-fit; record-semantics-unverified', trailing_captured_bytes=n-pos)
    elif proto in (1, 58):
        out.update(kind='ICMPv4 own fixed-prefix' if proto == 1 else 'ICMPv6 own fixed-prefix',
                   family_scope='cross-family IPv4-declared ICMPv6 observation; not canonical IPv6 carrier' if proto == 58 else 'IPv4-declared ICMP')
        if n:
            typ = b[0];minimum = (20 if typ in (13,14) else 12 if typ in (17,18) else 8) if proto == 1 else {1:8,2:8,3:8,4:8,128:8,129:8,130:24,131:24,132:24,133:8,134:16,135:24,136:24,137:40,143:8}.get(typ,4)
            out.update(type=typ, code=b[1] if n >= 2 else None, minimum_fixed=minimum,
                       boundary='fixed-prefix-short' if n < minimum else 'fixed-prefix-fits; body-options-and-quoted-children-not-certified-here')
    elif proto == 97:
        out.update(kind='EtherIP observed header; all profiles unselected', minimum_fixed=2)
        if n >= 2:
            value = int.from_bytes(b[:2], 'big')
            out.update(observed_version=value >> 12, observed_reserved=value & 4095,
                       attempted_inner_ethernet_offset=start+2, attempted_inner_ethernet_available=n-2,
                       selected_child=False)
    elif proto == 137:
        out.update(kind='MPLS label-stack envelopes only', labels=[])
        pos = 0
        while pos + 4 <= n:
            v = int.from_bytes(b[pos:pos+4], 'big')
            out['labels'].append(dict(offset=start+pos, label=v >> 12, tc=(v >> 9)&7, bottom=bool(v & 0x100), ttl=v & 255))
            pos += 4
            if v & 0x100:
                out.update(boundary='bottom-of-stack-captured; payload-profile-unverified', payload_offset=start+pos, payload_available=n-pos);break
        else: out.update(boundary='no-captured-bottom-of-stack', trailing_label_prefix_bytes=n-pos)
    elif proto in (51, 43):
        ah = proto == 51
        out.update(kind='AH repeated envelopes' if ah else 'IPv6 Routing envelope under IPv4; context unconfirmed', headers=[])
        pos = 0
        while pos < n:
            minimum = 12 if ah else 8
            if pos + 2 > n: out.update(boundary='extension-length-prefix-short', failure_offset=start+pos);break
            size = (b[pos+1]+2)*4 if ah else (b[pos+1]+1)*8
            out['headers'].append(dict(offset=start+pos, next_header=b[pos], declared_length=size, available=n-pos))
            if size < minimum: out['boundary'] = 'extension-length-below-fixed';break
            if pos + size > n: out['boundary'] = 'extension-envelope-short';break
            pos += size
            if b[pos-size] != proto: out.update(boundary='current-envelope-fits; next-profile-unverified', next_offset=start+pos);break
        else: out['boundary'] = 'no-next-header-bytes'
    elif proto == 4:
        out.update(kind='IP-in-IP attempted IPv4 fixed envelopes', headers=[])
        pos = start
        for _ in range(8):
            available = end-pos
            if available < 20: out.update(boundary='inner-IP-fixed-header-short', failure_offset=pos, failure_available=available);break
            ihl = (w[pos]&15)*4;total = int.from_bytes(w[pos+2:pos+4], 'big');version = w[pos] >> 4
            out['headers'].append(dict(offset=pos, version=version, header_length=ihl, declared_total_length=total,
                                      fragment_word=int.from_bytes(w[pos+6:pos+8], 'big'), next_protocol=w[pos+9], available=available))
            if version != 4 or ihl < 20: out['boundary'] = 'inner-IP-version-or-IHL-invalid';break
            if pos+ihl > end or total and total < ihl: out['boundary'] = 'inner-IP-header-envelope-short';break
            end = min(end, pos+total) if total else end
            if int.from_bytes(w[pos+6:pos+8], 'big') & 0x3fff: out['boundary'] = 'inner-IP-fragment; child-not-admitted';break
            nxt = w[pos+9];pos += ihl
            if nxt != 4: out.update(boundary='inner-fixed-header-fits; next-profile-unverified', next_offset=pos, next_available=end-pos);break
    elif proto == 47:
        out.update(kind='GRE attempted header envelope only', minimum_fixed=4)
        if n >= 4:
            flags = int.from_bytes(b[:2], 'big')
            out.update(observed_flags=flags, observed_version=flags & 7, declared_protocol=int.from_bytes(b[2:4], 'big'),
                       scope='GRE-specific admission belongs to its separate source audit; this is an outer literal header observation')
    else: out['kind'] = 'declared child not implemented by this diagnostic'
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for arg in ('capture','audit-modules','uncovered','eol-diff','tcp-plan','tcp-classifier','tcp-constructor',
                'parent-plan','parent-constructor','typed-proposal','typed-carrier-binding','typed-constructor','native','out'):
        ap.add_argument('--'+arg, type=Path, required=True)
    a = ap.parse_args();a.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(a.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    targets = {x['frame'] for x in json.loads(a.uncovered.read_text())}
    original = {i:r for i,r in enumerate(capture_records(a.capture),1) if i in targets}
    assert original.keys() == targets
    tc = module('source_tcp_classifier', a.tcp_classifier)
    pure = module('source_tcp_constructor', a.tcp_constructor)
    pp = module('source_parent_constructor', a.parent_constructor)
    typed = module('source_typed_constructor', a.typed_constructor)
    tcp = {x['frame']:x for x in rows(a.tcp_plan) if x['frame'] in targets}
    parent = {x['frame']:x for x in rows(a.parent_plan) if x['frame'] in targets}
    typed_plan = {x['frame']:x for x in rows(a.typed_proposal) if x['frame'] in targets}
    carriers = {x['frame']:x for x in rows(a.typed_carrier_binding) if x['corpus']=='sharon' and x['frame'] in targets}
    differential = {x['frame']:x for x in rows(a.eol_diff) if x['frame'] in targets}
    assert differential.keys() == targets
    sdk = a.native/'preservation/sharon/events.jsonl'
    producer = json.loads(sdk.with_name('events-producer.json').read_text())
    assert producer['exit_code'] == 0 and producer['capture_SHA256'] == sha(a.capture) and producer['export_SHA256'] == sha(sdk)
    contributions = collections.defaultdict(list)
    for event in rows(sdk):
        for n in {r['Number'] for r in event.get('source_bytes',{}).get('PacketRefs') or []} & targets:
            contributions[n].append(dict(id=event['id'], protocol=event['protocol'], status=event['status'], expert=event.get('expert'),
                                        whole_event_canonical_SHA256=digest(event)))
    ledgers = [];counts = collections.Counter();overlaps = collections.Counter();boundaries = collections.Counter()
    for n in sorted(targets):
        r = original[n];r['frame'] = n;w = r['wire'];wirehash = hashlib.sha256(w).hexdigest();p = outer(r);child = bounded_child(w,p)
        links = []
        if n in tcp:
            x = tcp[n];loc = tc.parent(r);f = tc.failure(w,loc,pure.own_failure)
            assert wirehash == x['wire_SHA256'] and same(x['record'],{k:v for k,v in r.items() if k!='wire'})
            assert same(loc,x['literal_location']) and same(f,x['own_failure'])
            assert p['own_offset'] == loc['own_offset'] and p['own_end'] == loc['own_end'] and p['declared_child_protocol'] == 6
            links.append(dict(proof='TCP705 whole row/worker', scope='whole-row-and-worker', original_plan_row_SHA256=digest(x), category=f['category']))
        if n in parent:
            x = parent[n];assert p['declared_child_protocol'] == 97
            assert same(pp.expected(x['expected_new_event']['id'],r),x['expected_new_event'])
            links.append(dict(proof='EtherIP17532 bounded whole native expectation', scope='whole-parent-native-object', original_plan_row_SHA256=digest(x)))
        if n in typed_plan:
            x = typed_plan[n];assert p['declared_child_protocol'] in (1,58)
            protocol = 'icmp' if p['declared_child_protocol']==1 else 'icmpv6'
            location = dict(raw_start=0,raw_end=len(w),own_offset=p['own_offset'],own_end=p['own_end'])
            assert x['protocol'] == protocol and x['source_SHA256'] == wirehash
            assert same(typed.project(protocol,w,location),x['expected_added_values'])
            links.append(dict(proof='Network23 original direct source plan', scope='typed-values-and-Source1-ranges-only', original_plan_row_SHA256=digest(x), values=len(x['expected_added_values'])))
        if n in carriers and p['declared_child_protocol']==58:
            x = carriers[n];loc = x['location']
            assert x['wire_SHA256'] == wirehash and loc['raw_start']==0 and loc['raw_end']==len(w)
            assert loc['offset']==p['own_offset'] and loc['end']==p['own_end'] and x['protocol']=='icmpv6'
            links.append(dict(proof='Network23 root carrier binding', scope='original-own-offset-and-envelope-only', original_plan_row_SHA256=digest(x), profiles=loc['profiles']))
        key = 'TCP705' if n in tcp else 'EtherIP17532' if n in parent else 'ICMP23-source-bound' if p['declared_child_protocol'] in (1,58) else 'further-child-scope-not-whole-certified'
        overlaps[key] += 1;counts[p['declared_child_protocol']] += 1;boundaries[str(p['declared_child_protocol'])+':'+child.get('boundary',child['kind'])] += 1
        d = differential[n]
        ledgers.append(dict(frame=n,wire_SHA256=wirehash,record={k:v for k,v in r.items()if k!='wire'},
                            original_EOLOptions=p,declared_child_facts=child,existing_plan_rebindings=links,
                            coverage_class=key,current_native_observations=contributions[n],
                            historical_EOLOld_row_SHA256=digest(d['old']),historical_EOLNew_row_SHA256=digest(d['new']),
                            historical_layer_change=dict(before=d['old']['layers'],after=d['new']['layers']),
                            limits='Only explicitly named whole-object certificates close those scopes. Common fixed/chunk envelopes and byte fields do not establish full application, connection or whole-row parity.'))
    ledger = a.out/'source-classification.jsonl'
    ledger.write_text(''.join(json.dumps(x,sort_keys=True)+'\n' for x in ledgers),encoding='utf8')
    negatives=[]
    for name,offset,value in [('outer-version',14,0x66),('outer-EtherType',12,0x86),('fragment',21,1),('IHL',14,0x4f)]:
        rec = dict(original[sorted(targets)[0]]);w=bytearray(rec['wire']);w[offset]=value;rec['wire']=bytes(w)
        try: outer(rec)
        except AssertionError: negatives.append(name)
        else: raise AssertionError('source negative accepted '+name)
    result=dict(scope=__doc__,capture_SHA256=sha(a.capture),sources=len(ledgers),all_original_sources_bound=True,
                declared_child_protocol_counts=dict(counts),explicit_scope_classes=dict(overlaps),header_boundary_counts=dict(boundaries),
                ledger_SHA256=sha(ledger),negative_controls=negatives,native_source=producer['source'],native_producer_SHA256=sha(sdk.with_name('events-producer.json')),
                input_SHA256={k:sha(v) for k,v in vars(a).items() if isinstance(v,Path) and v.is_file()},
                auditor_SHA256=sha(__file__),references=[
                    'https://www.rfc-editor.org/rfc/rfc9260.html#section-3.2',
                    'https://www.rfc-editor.org/rfc/rfc3376.html#section-4.2',
                    'https://www.rfc-editor.org/rfc/rfc7761.html#section-4.9',
                    'https://www.rfc-editor.org/rfc/rfc3032.html#section-2.1'],
                remaining='The original EOL whole-row gap is not silently set to zero. 145 rows compose with TCP whole expectations, 14 sources with parent expectations. 37 ICMP sources have byte/field evidence, not a new EOL whole-row proof. Other107 retain explicit bounded common-header facts and unverified child/body scope.')
    out=a.out/'summary.json';out.write_text(json.dumps(result,indent=2,sort_keys=True)+'\n',encoding='utf8')
    print(json.dumps(dict(summary=str(out),SHA256=sha(out),sources=len(ledgers),classes=dict(overlaps))))


if __name__ == '__main__': main()
