"""Compare original L2TPv3 wire envelopes and selected fields with TShark.

Usage: l2tp3_wire_audit.py ORACLE_JSON NATIVE_JSONL OUTPUT_JSON
Oracle: tshark -2 -n -r CAPTURE -Y l2tp -T json -x --no-duplicate-keys.
Data's l2tp_raw contains only its header: derive the full native PDU from the
original enclosing IP/UDP lengths, never from a guessed pseudowire layout.
Captures and raw exports must stay outside Git. Refused and uncovered views
are preserved explicitly; the report must not promote them to verified PDUs.
"""
import base64, collections, hashlib, ipaddress, json, sys
from pathlib import Path

oracle, native, output = map(Path, sys.argv[1:4])
for p in [oracle, native, output]:
    if any((a/'.git').exists() for a in [p.resolve(), *p.resolve().parents]):
        raise ValueError('raw audit evidence must remain outside Git')

def sha(b): return hashlib.sha256(b).hexdigest()
def descriptors(x): return ([x] if isinstance(x[0], str) else x) if x else []
def number(v): return int(v, 16 if v.startswith('0x') else 10) if isinstance(v,str) else int(v)
def fields(x, key):
    if isinstance(x, dict):
        if key in x:
            v=x[key]
            yield from v if isinstance(v, list) else [v]
        for k,v in x.items():
            if not k.endswith('_raw'): yield from fields(v,key)
    elif isinstance(x,list):
        for v in x: yield from fields(v,key)

def carrier(ls, offset):
    frame=bytes.fromhex(ls['frame_raw'][0])
    candidates=[]
    for r in descriptors(ls.get('ip_raw')):
        w=bytes.fromhex(r[0]);at=int(r[1])
        if len(w)<20 or w[0]>>4!=4 or frame[at:at+len(w)]!=w: continue
        ihl=(w[0]&15)*4;end=at+int.from_bytes(w[2:4],'big')
        if ihl<20 or ihl>len(w): continue
        proto=w[9];payload=at+ihl;src=str(ipaddress.ip_address(w[12:16]));dst=str(ipaddress.ip_address(w[16:20]))
        if proto==115 and payload==offset:
            invalid=int.from_bytes(w[2:4],'big')<ihl
            candidates.append((at,frame[offset:] if invalid else frame[offset:min(end,len(frame))],False,src,dst,end>len(frame),'invalid IPv4 total length' if invalid else None))
        if proto==17 and payload+8==offset and payload+8<=len(frame):
            u=frame[payload:offset];uend=payload+int.from_bytes(u[4:6],'big')
            if uend<=end and uend>=offset:
                candidates.append((at,frame[offset:min(uend,len(frame))],True,src+':'+str(int.from_bytes(u[:2],'big')),dst+':'+str(int.from_bytes(u[2:4],'big')),uend>len(frame),None))
    # Selected IPv6 direct carrier. Extension chains remain a visible gap.
    for r in descriptors(ls.get('ipv6_raw')):
        w=bytes.fromhex(r[0]);at=int(r[1])
        if len(w)<40 or w[0]>>4!=6 or frame[at:at+len(w)]!=w: continue
        end=at+40+int.from_bytes(w[4:6],'big')
        if w[6]==115 and at+40==offset:
            candidates.append((at,frame[offset:min(end,len(frame))],False,str(ipaddress.ip_address(w[8:24])),str(ipaddress.ip_address(w[24:40])),end>len(frame),None))
    return max(candidates,key=lambda x:x[0]) if candidates else None

events=[];index=collections.defaultdict(list)
for line in native.open(encoding='utf-8-sig'):
    e=json.loads(line)
    if e['protocol']!='l2tp' or e['profile']!='l2tp3-bounded-wire-envelope': continue
    events.append(e);wire=base64.b64decode(e.get('raw') or '')
    for ref in e['source_bytes'].get('PacketRefs') or []:
        index[(ref['Number'],sha(wire))].append(e)

used=set();checks=collections.Counter();missing=[];differences=[];refusals=[];outside=[];proof=[];generated=[];blocked_carrier=[]
for packet in json.loads(oracle.read_text(encoding='utf-8-sig')):
    ls=packet['_source']['layers'];frame=int(ls['frame']['frame.number']);iface=int(ls['frame'].get('frame.interface_id',0))
    raw=descriptors(ls.get('l2tp_raw'));trees=ls.get('l2tp') or [];trees=trees if isinstance(trees,list) else [trees]
    if len(raw)!=len(trees):
        outside.append({'frame':frame,'reason':'oracle raw/tree cardinality mismatch','raw':len(raw),'trees':len(trees)});continue
    for desc,tree in zip(raw,trees):
        offset=int(desc[1]);ctx=carrier(ls,offset)
        if ctx is None:
            outside.append({'frame':frame,'offset':offset,'reason':'complete original direct IP115/UDP carrier not established'});continue
        at,w,udp,src,dst,cut,invalid=ctx
        parent_bad=[]
        for u in descriptors(ls.get('udp_raw')):
            uw=bytes.fromhex(u[0]);ua=int(u[1])
            if ua>=at or len(uw)<8:continue
            for ip in descriptors(ls.get('ip_raw')):
                iw=bytes.fromhex(ip[0]);ia=int(ip[1])
                if len(iw)>=20 and ia+(iw[0]&15)*4==ua and iw[9]==17 and ua+int.from_bytes(uw[4:6],'big')>ia+int.from_bytes(iw[2:4],'big'):
                    parent_bad.append({'udp_offset':ua,'declared_udp_length':int.from_bytes(uw[4:6],'big'),'enclosing_IP_end':ia+int.from_bytes(iw[2:4],'big')})
        if parent_bad:
            blocked_carrier.append({'frame':frame,'offset':offset,'reason':'inner L2TP view lies under an invalid outer UDP length; no valid tunnel delivery claim','proof':parent_bad});continue
        if udp and (len(w)<2 or w[1]&15!=3):
            outside.append({'frame':frame,'offset':offset,'reason':'UDP L2TP version is not v3'});continue
        matches=[e for e in index[(frame,sha(w))] if e['id'] not in used]
        if len(matches)!=1:
            missing.append({'frame':frame,'offset':offset,'sha256':sha(w),'bytes':len(w),'matches':len(matches),'udp':udp,'cut':cut});continue
        e=matches[0];used.add(e['id']);checks['exact_whole_carrier_PDU']+=1
        refs=e['source_bytes'].get('PacketRefs') or []
        if not any(r['Number']==frame and r['Domain']['Interface']==iface for r in refs):
            differences.append({'frame':frame,'reason':'physical contributor/interface mismatch'})
        checks['contributor_interface']+=1
        if e['source']!=src or e['destination']!=dst:
            differences.append({'frame':frame,'reason':'actual carrier endpoint mismatch','native':[e['source'],e['destination']],'oracle':[src,dst]})
        checks['actual_carrier_endpoints']+=1
        if e['flow']!=0: differences.append({'frame':frame,'reason':'midstream control/data invented connection identity'})
        checks['no_unobserved_session_identity']+=1
        if e['status']!='decoded':
            refusals.append({'frame':frame,'sha256':sha(w),'status':e['status'],'reason':e.get('error'),'cut':cut,'independent_carrier_error':invalid});continue
        if invalid or cut: differences.append({'frame':frame,'reason':'invalid/incomplete carrier became a decoded message','invalid':invalid,'cut':cut})
        actual=collections.defaultdict(list)
        for v in e.get('field_values',{}).get('values',[]):
            if v['name'].startswith('l2tp.'):actual[v['name']].append(v['value'])
        for key,got in sorted(actual.items()):
            want=list(fields(tree,key))
            if got and isinstance(got[0],(int,bool)):want=list(map(number,want))
            if key in ['l2tp.cookie','l2tp.avp.assigned_cookie']:want=[v.replace(':','').lower() for v in want]
            checks[key]+=max(len(got),len(want))
            if want!=got:differences.append({'frame':frame,'field':key,'wireshark':want,'native':got})
        m=e.get('session') or {}
        if not m.get('Control'):
            checks['opaque_payload_never_guessed']+=1
            if m.get('Payload Decoded') or m.get('Ethernet Source') or m.get('Cookie Matches Proposal'):
                differences.append({'frame':frame,'reason':'unobserved cookie/pseudowire was guessed'})
            if e.get('completeness')!='limited':differences.append({'frame':frame,'reason':'unknown body fields treated as exhaustive'})
            # Wireshark's generated type0 means unknown; it isn't an AVP or
            # evidence that a specific pseudowire protocol was negotiated.
            for v in fields(tree,'l2tp.avp.pseudowire_type'):
                if number(v)==0:generated.append({'frame':frame,'field':'l2tp.avp.pseudowire_type','value':0,'meaning':'generated unknown; no wire AVP'})
        proof.append({'frame':frame,'pdu':e['id'],'sha256':sha(w),'bytes':len(w),'control':bool(m.get('Control')),'message':m.get('Packet Name'),'source':src,'destination':dst})

result={'scope':'Original direct IP115 and v3 UDP wire envelopes, not inferred midstream pseudowire parameters or endpoint delivery',
 'oracle_sha256':sha(oracle.read_bytes()),'native_sha256':sha(native.read_bytes()),'audit_sha256':sha(Path(__file__).read_bytes()),
 'native_status':dict(collections.Counter(e['status'] for e in events)),'matched_wire_views':len(used),'verified_envelopes':len(proof),
 'checks':dict(checks),'checks_total':sum(checks.values()),'missing':missing,'field_or_identity_differences':differences,
 'refusals':refusals,'outside_selected_scope':outside,'blocked_by_invalid_carrier':blocked_carrier,'oracle_generated_unknown_pseudowire':generated,
 'unmatched_native':[{'pdu':e['id'],'status':e['status'],'reason':e.get('error'),'frames':[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []]} for e in events if e['id'] not in used], 'proof':proof}
output.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:(len(v) if isinstance(v,list) else v) for k,v in result.items() if k in ['native_status','matched_wire_views','verified_envelopes','checks_total','missing','field_or_identity_differences','refusals','outside_selected_scope','unmatched_native']}))
assert proof,'empty evidence cannot establish support'
assert not missing and not differences,'preserve and investigate original differences'
