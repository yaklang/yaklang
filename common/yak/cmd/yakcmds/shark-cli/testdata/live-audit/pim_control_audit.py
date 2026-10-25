"""Original PIM wire/field audit; captures and raw exports remain outside Git.

Usage: pim_control_audit.py ORACLE_JSON EVENTS_JSONL OUTPUT_JSON
Oracle: TShark -2 -n -Y pim -T json -x --no-duplicate-keys.
Refused wire views are retained separately, never successful field checks.
Encoded address wrapper/leaf duplicates are recorded and compared as one
logical field only when the nested tree explicitly repeats the same field.
"""
import base64,collections,hashlib,ipaddress,json,sys
from pathlib import Path

oracle,events,out=map(Path,sys.argv[1:4])
for p in [oracle,events,out]:
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):raise ValueError('Raw evidence must remain outside Git')
def sha(w):return hashlib.sha256(w).hexdigest()
def walk(x):
    if isinstance(x,dict):
        yield x
        for k,v in x.items():
            if not k.endswith('_raw'):yield from walk(v)
    elif isinstance(x,list):
        for v in x:yield from walk(v)
def number(v):return int(v,16 if v.startswith('0x') else 10) if isinstance(v,str) else int(v)
def canon(v):
    if isinstance(v,(bool,int)):return int(v)
    try:return str(ipaddress.ip_address(v))
    except ValueError:pass
    try:return number(v)
    except ValueError:return v
def descriptors(x):
    if not x:return []
    return [x] if isinstance(x[0],str) else x
def checksum(w,pseudo=b''):
    b=pseudo+w
    if len(b)%2:b+=b'\0'
    s=sum(int.from_bytes(b[i:i+2],'big') for i in range(0,len(b),2))
    while s>>16:s=(s&65535)+(s>>16)
    return (~s)&65535
def enclosing(layers,offset,wire):
    candidates=[]
    for kind in ['ip','ipv6']:
        for raw in descriptors(layers.get(kind+'_raw')):
            b=bytes.fromhex(raw[0]);at=int(raw[1])
            if at+len(b)!=offset:continue
            if kind=='ip' and len(b)>=20 and b[0]>>4==4 and b[9]==103:
                candidates.append((at,ipaddress.ip_address(b[12:16]),ipaddress.ip_address(b[16:20])))
            if kind=='ipv6' and len(b)>=40 and b[0]>>4==6 and b[6]==103:
                candidates.append((at,ipaddress.ip_address(b[8:24]),ipaddress.ip_address(b[24:40])))
    if not candidates and offset==0:
        # TShark's reassembled data source has logical offset zero. Accept
        # an enclosing IPv6 fragment header only after all oracle fragment
        # ranges independently reconstruct exactly this logical PIM value.
        raws=descriptors(layers.get('ipv6_raw'));trees=layers.get('ipv6') or []
        trees=trees if isinstance(trees,list) else [trees]
        for raw,tree in zip(raws,trees):
            b=bytes.fromhex(raw[0]);frag=tree.get('ipv6.fraghdr') or {};parts=(tree.get('ipv6.fragments') or {}).get('ipv6.fragment_raw')
            if len(b)<40 or b[6]!=44 or number(frag.get('ipv6.fraghdr.nxt','0'))!=103 or not parts:continue
            ranges=sorted((int(p[1]),bytes.fromhex(p[0])) for p in descriptors(parts))
            assembled=b'';valid=True;seen_parts={}
            for start,value in ranges:
                key=(start,len(value))
                if seen_parts.get(key)==value:continue # exact duplicate, never a partial/conflicting overlap
                if start!=len(assembled):valid=False;break
                seen_parts[key]=value
                assembled+=value
            if valid and assembled==wire:candidates.append((int(raw[1]),ipaddress.ip_address(b[8:24]),ipaddress.ip_address(b[24:40])))
    return max(candidates,key=lambda x:x[0]) if candidates else None

es=[];index=collections.defaultdict(list)
for line in events.open(encoding='utf-8-sig'):
    e=json.loads(line)
    if e['protocol']!='pim':continue
    es.append(e);w=base64.b64decode(e.get('raw') or '')
    for ref in e['source_bytes'].get('PacketRefs') or []:index[(ref['Number'],sha(w))].append(e)
used=set();missing=[];differences=[];refusals=[];outside=[];checks=collections.Counter();proof=[];duplicates=[]
views=0
for packet in json.loads(oracle.read_text(encoding='utf-8-sig')):
    ls=packet['_source']['layers'];frame=int(ls['frame']['frame.number']);iface=int(ls['frame'].get('frame.interface_id',0))
    raw=descriptors(ls.get('pim_raw'));trees=ls.get('pim') or [];trees=trees if isinstance(trees,list) else [trees]
    if len(raw)!=len(trees):outside.append({'frame':frame,'reason':'oracle raw/tree cardinality mismatch','raw':len(raw),'trees':len(trees)});continue
    for desc,tree in zip(raw,trees):
        views+=1;w=bytes.fromhex(desc[0]);offset=int(desc[1]);ver=number(tree.get('pim.version','0'));typ=number(tree.get('pim.type','255'))
        if ver!=2:outside.append({'frame':frame,'offset':offset,'reason':'unselected PIM version','version':ver});continue
        candidates=[e for e in index[(frame,sha(w))] if e['id'] not in used]
        if len(candidates)!=1:
            missing.append({'frame':frame,'offset':offset,'type':typ,'wire_bytes':len(w),'sha256':sha(w),'matching_native':len(candidates)});continue
        e=candidates[0];used.add(e['id']);checks['exact_wire_occurrence']+=1
        refs=e['source_bytes'].get('PacketRefs') or []
        if not any(r['Number']==frame and r['Domain']['Interface']==iface for r in refs):differences.append({'frame':frame,'reason':'contributor/interface mismatch'})
        checks['capture_domain']+=1
        if e['flow']!=0 or e['transport']!='network':differences.append({'frame':frame,'reason':'control advertisement became a transport connection'})
        checks['no_invented_transport_flow']+=1
        if e['status']!='decoded':
            refusals.append({'frame':frame,'offset':offset,'type':typ,'native_status':e['status'],'reason':e.get('error'),'sha256':sha(w)})
            continue
        ctx=enclosing(ls,offset,w)
        if ctx is None:differences.append({'frame':frame,'reason':'original enclosing IP header unavailable'})
        else:
            _,src,dst=ctx
            if (str(src),str(dst))!=(e['source'],e['destination']):differences.append({'frame':frame,'reason':'outer control endpoint mismatch','oracle':[str(src),str(dst)],'native':[e['source'],e['destination']]})
            checks['enclosing_IP_endpoints']+=1
            n=8 if typ==1 else len(w)
            pseudo=src.packed+dst.packed+n.to_bytes(4,'big')+b'\0\0\0\x67' if src.version==6 else b''
            valid=checksum(w[:n],pseudo)==0 or typ==1 and checksum(w,pseudo)==0
            if not valid or e['session'].get('Checksum Verified') is not True:differences.append({'frame':frame,'reason':'independent PIM checksum mismatch'})
            checks['independent_checksum']+=1
        actual=collections.defaultdict(list)
        for v in e.get('field_values',{}).get('values',[]):
            if v['name'].startswith('pim.') and not v['name'].startswith('pim.observed.'):
                actual[v['name']].append(canon(v['value']))
        def field_values(x,key):
            if isinstance(x,dict):
                nested=x.get(key+'_tree')
                repeated=nested is not None and any(key in c for c in walk(nested))
                if key in x:
                    val=x[key];vals=val if isinstance(val,list) else [val]
                    if repeated:duplicates.append({'frame':frame,'field':key,'wrapper_values':vals})
                    else:yield from vals
                for k,v in x.items():
                    if not k.endswith('_raw'):yield from field_values(v,key)
            elif isinstance(x,list):
                for v in x:yield from field_values(v,key)
        for key,got in sorted(actual.items()):
            want=list(map(canon,field_values(tree,key)))
            checks[key]+=max(len(got),len(want))
            if want!=got:differences.append({'frame':frame,'pdu':e['id'],'field':key,'wireshark':want,'native':got})
        proof.append({'frame':frame,'pdu':e['id'],'type':typ,'sha256':sha(w),'source':e['source'],'destination':e['destination'],'null_register':e['session'].get('Null Register')})
result={'scope':'Original PIMv2 complete selected messages; independent checksum, physical control endpoints, ordered typed fields and exact occurrence/contributor matching',
 'oracle_sha256':sha(oracle.read_bytes()),'native_export_sha256':sha(events.read_bytes()),'audit_sha256':sha(Path(__file__).read_bytes()),
 'native_status':dict(collections.Counter(e['status'] for e in es)),'oracle_views':views,'matched_wire_views':len(used),'decoded_verified_messages':len(proof),
 'checks':dict(checks),'checks_total':sum(checks.values()),'missing':missing,'field_or_identity_differences':differences,
 'refusals':refusals,'outside_selected_scope':outside,'oracle_encoded_address_wrapper_duplicates':duplicates,
 'unmatched_native':[{'pdu':e['id'],'status':e['status'],'reason':e.get('error'),'frames':[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []]} for e in es if e['id'] not in used],'proof':proof}
out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:(len(v) if isinstance(v,list) else v) for k,v in result.items() if k in ['native_status','oracle_views','matched_wire_views','decoded_verified_messages','checks_total','missing','field_or_identity_differences','refusals','outside_selected_scope','unmatched_native']}))
assert proof,'empty audit cannot establish protocol support'
assert not missing and not differences,'inspect preserved original differences'
