"""Exact LDP TCP PDUs and ordered typed fields, preserving original frame/domain.

Usage: ldp_session_audit.py TSHARK_LDP_RAW_JSON FULL_NATIVE_JSONL OUT_JSON
All raw inputs/output must stay outside Git. Oracle reads the complete original
capture with -2 -n -Y ldp -T json -x --no-duplicate-keys. Supplemental interface
exports are separate inputs/results, never substitutes for the original oracle.
"""
import base64,collections,hashlib,ipaddress,json,struct,sys
from pathlib import Path

oracle,events,out=map(Path,sys.argv[1:4])
for p in (oracle,events,out):
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):raise SystemExit('Raw evidence must stay outside Git')
def sha(b):return hashlib.sha256(b).hexdigest()
def rows(v):return v if isinstance(v,list) else [v]
def raw_rows(v):return [v] if v and isinstance(v[0],str) else v or []
def values(v,key):
    if isinstance(v,dict):
        for k,x in v.items():
            if k==key:yield from rows(x)
            elif not k.endswith('_raw'):yield from values(x,key)
    elif isinstance(v,list):
        for x in v:yield from values(x,key)
def canon(v):
    if isinstance(v,bool):return int(v)
    if isinstance(v,(int,float)):return int(v)
    if isinstance(v,str):
        if v.startswith('0x') or v.isdecimal():return int(v,16 if v.startswith('0x') else 10)
        try:return str(ipaddress.ip_network(v,strict=False)) if '/' in v else str(ipaddress.ip_address(v))
        except ValueError:return v
    return v

def rejection(w):
    # Independent receive-envelope checks. In particular, a plausible PDU
    # header cannot validate a message length that extends beyond those bytes.
    at=10
    while at<len(w):
        if at+8>len(w):return 'incomplete message header'
        header,n=struct.unpack_from('!HH',w,at);typ=header&0x7fff;at+=4;end=at+n
        if n<4 or end>len(w):return 'message length outside complete PDU'
        if typ not in (1,0x200,0x201,0x300,0x301,0x400,0x401,0x402,0x403,0x404):return 'unselected TCP message type'
        if header&0x8000:return 'outside selected base message U-bit encoding'
        at+=4;tlvs=[]
        while at<end:
            if at+4>end:return 'incomplete TLV header'
            flags,n=struct.unpack_from('!HH',w,at);tt=flags&0x3fff;at+=4
            if at+n>end:return 'TLV crosses message boundary'
            data=w[at:at+n];at+=n;tlvs.append(tt)
            fixed={0x103:1,0x200:4,0x201:4,0x202:4,0x300:10,0x301:4,0x500:14,0x600:4}
            if tt in fixed and n!=fixed[tt]:return 'known TLV fixed length violation'
            if tt==0x500:
                ver,keep=struct.unpack_from('!HH',data)
                if ver!=1 or not keep or not data[4]&0x40 and data[5]:return 'invalid Common Session Parameters'
            if tt==0x101:
                if n<2:return 'incomplete address-family field'
                afi=int.from_bytes(data[:2],'big')
                if afi not in (1,2):return 'unselected address family'
                if (n-2)%(4 if afi==1 else 16):return 'address-list width violation'
            if tt==0x104 and n%4:return 'path-vector width violation'
            if tt==0x100:
                if not data:return 'empty FEC'
                pos=0;elements=0
                while pos<len(data):
                    ft=data[pos];pos+=1;elements+=1
                    if ft==1:
                        if typ not in (0x402,0x403) or elements!=1 or pos!=len(data):return 'wildcard FEC scope violation'
                    elif ft==2:
                        if pos+3>len(data):return 'incomplete prefix FEC header'
                        afi,bits=struct.unpack_from('!HB',data,pos);pos+=3
                        if afi not in (1,2):return 'unselected prefix address family'
                        if bits>(32 if afi==1 else 128):return 'invalid prefix bit length'
                        pos+=(bits+7)//8
                        if pos>len(data):return 'prefix crosses FEC TLV'
                    else:return 'unselected FEC element'
                if typ!=0x400 and elements!=1:return 'multiple FEC elements outside Mapping'
        required={1:[0x300],0x200:[0x500],0x300:[0x101],0x301:[0x101],0x400:[0x100,0x200],0x401:[0x100],0x402:[0x100],0x403:[0x100],0x404:[0x100,0x600]}.get(typ,[])
        for i,tt in enumerate(required):
            if i>=len(tlvs) or (tlvs[i]!=tt and not (tt==0x200 and tlvs[i] in (0x201,0x202))):return 'missing or misordered mandatory TLV'
    return None
es=[];index=collections.defaultdict(list)
for line in events.open(encoding='utf-8-sig'):
    e=json.loads(line)
    if e['protocol']!='ldp' or e['transport']!='tcp':continue
    es.append(e)
    for ref in e['source_bytes'].get('PacketRefs') or []:index[(ref['Number'],sha(base64.b64decode(e.get('raw') or '')))].append(e)
used=set();checks=collections.Counter();missing=[];fail=[];prefix=[];refused=[];proof=[];views=0
partitions=collections.defaultdict(set);reverse_partitions=collections.defaultdict(set)
for p in json.loads(oracle.read_text(encoding='utf-8-sig')):
    layers=p['_source']['layers']
    if 'tcp' not in layers:continue
    frame=int(layers['frame']['frame.number']);iface=int(layers['frame'].get('frame.interface_id',0))
    objects=rows(layers.get('ldp',{}));raws=raw_rows(layers.get('ldp_raw'))
    for i,obj in enumerate(objects):
        views+=1
        if i>=len(raws):prefix.append({'frame':frame,'reason':'oracle omitted complete raw PDU'});continue
        w=bytes.fromhex(raws[i][0])
        if len(w)<18 or w[:2]!=b'\0\1' or int.from_bytes(w[2:4],'big')+4!=len(w):
            prefix.append({'frame':frame,'bytes':len(w),'reason':'no complete selected v1 PDU envelope'});continue
        reason=rejection(w)
        if reason:
            prefix.append({'frame':frame,'bytes':len(w),'reason':reason,'sha256':sha(w)});continue
        candidates=[e for e in index[(frame,sha(w))] if e['id'] not in used]
        if len(candidates)!=1:
            missing.append({'frame':frame,'sha256':sha(w),'bytes':len(w),'matching_unused_pdus':len(candidates)});continue
        e=candidates[0];used.add(e['id']);checks['exact_wire_occurrence']+=1
        if not any(ref['Number']==frame and ref['Domain']['Interface']==iface for ref in e['source_bytes'].get('PacketRefs') or []):fail.append({'frame':frame,'reason':'contributor capture interface mismatch'})
        checks['capture_domain']+=1
        if not e['flow']:fail.append({'frame':frame,'reason':'TCP PDU has no native flow generation'})
        checks['native_flow_generation']+=1
        stream=layers['tcp'].get('tcp.stream')
        if stream is None:fail.append({'frame':frame,'reason':'oracle omitted TCP stream identity'})
        else:
            partitions[(iface,str(stream))].add(e['flow'])
            reverse_partitions[(iface,e['flow'])].add(str(stream))
        if e['status'] not in ('decoded','deferred'):
            refused.append({'frame':frame,'pdu':e['id'],'status':e['status'],'reason':e.get('error')})
        actual=collections.defaultdict(list)
        for v in e.get('field_values',{}).get('values',[]):
            if v['name'].startswith(('ldp.hdr.','ldp.msg.')):actual[v['name']].append(canon(v['value']))
        if not actual:
            continue # explicit semantic refusal, not a successful field comparison
        for key,observed in sorted(actual.items()):
            want=list(map(canon,values(obj,key)))
            checks[key]+=max(len(want),len(observed))
            if want!=observed:fail.append({'frame':frame,'pdu':e['id'],'field':key,'wireshark':want,'native':observed})
        proof.append({'frame':frame,'pdu':e['id'],'flow':e['flow'],'sha256':sha(w),'contributor_frames':[ref['Number'] for ref in e['source_bytes'].get('PacketRefs') or []]})
splits=[{'interface':k[0],'oracle_stream':k[1],'native_flows':sorted(v)} for k,v in partitions.items() if len(v)!=1]
merges=[{'interface':k[0],'native_flow':k[1],'oracle_streams':sorted(v)} for k,v in reverse_partitions.items() if len(v)!=1]
result={'scope':'Original complete LDP TCP PDUs, ordered registered fields and contributor/domain/flow evidence; semantic refusals and cross-interface oracle omissions stay explicit',
 'oracle_sha256':sha(oracle.read_bytes()),'native_export_sha256':sha(events.read_bytes()),'audit_sha256':sha(Path(__file__).read_bytes()),
 'native_status':dict(collections.Counter(e['status'] for e in es)),'native_pdus':len(es),'native_flows':len({e['flow'] for e in es}),
 'oracle_views':views,'matched_wire_pdus':len(used),'checks':dict(checks),'checks_total':sum(checks.values()),'missing':missing,'field_or_identity_differences':fail,
 'oracle_prefix_views':prefix,'semantic_refusals':refused,'unmatched_native_pdus':[{'pdu':e['id'],'flow':e['flow'],'status':e['status'],'reason':e.get('summary'),'frames':[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []]} for e in es if e['id'] not in used],
 'session_partitions':{'oracle_stream_domain_groups':len(partitions),'native_flow_domain_groups':len(reverse_partitions),'splits':splits,'merges':merges},
 'proof':proof}
out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:(len(v) if isinstance(v,list) else v) for k,v in result.items() if k in ('native_status','native_pdus','native_flows','oracle_views','matched_wire_pdus','checks_total','missing','field_or_identity_differences','oracle_prefix_views')}))
assert views and used,'empty audit cannot establish positive protocol coverage'
assert not missing and not fail and not splits and not merges,'inspect preserved original differences'
