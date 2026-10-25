"""Compare original LDP discovery PDUs, typed values and contributor domains.

Usage: ldp_discovery_audit.py ORIGINAL_LDP_JSON FULL_NATIVE_JSONL CAPTURE OUTPUT [FULL_CLI_JSONL]
Oracle: full original capture, TShark -2 -n -Y ldp -T json -x --no-duplicate-keys.
No Decode-As, label normalization or capture transformation. TCP remains a gap.
All four arguments must stay outside Git; aggregate summary can be copied later.
"""
import base64,collections,hashlib,ipaddress,json,struct,sys
from pathlib import Path

oracle,events,capture,out=map(Path,sys.argv[1:5])
cli=Path(sys.argv[5]) if len(sys.argv)>5 else None
for p in (oracle,events,capture,out,*([cli] if cli else [])):
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):
        raise SystemExit('Raw capture and audit evidence must stay outside Git')
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def walk(v):
    if isinstance(v,dict):
        yield v
        for x in v.values():yield from walk(x)
    elif isinstance(v,list):
        for x in v:yield from walk(x)
def values(v,name):
    for row in walk(v):
        if name in row:
            x=row[name]
            yield from x if isinstance(x,list) else [x]
def canon(x):
    if isinstance(x,bool):return int(x)
    if isinstance(x,(int,float)):return int(x)
    if x in ('True','False'):return int(x=='True')
    if x.startswith('0x') or x.isdecimal():return int(x,16 if x.startswith('0x') else 10)
    try:return str(ipaddress.ip_address(x))
    except ValueError:return x
def reject(w):
    # Independent bounded RFC5036 Hello envelope checks for oracle fuzz views.
    if len(w)<26 or len(w)>4100:return 'outside selected discovery byte bounds'
    if struct.unpack_from('!H',w)[0]!=1:return 'version outside v1'
    if struct.unpack_from('!H',w,2)[0]+4!=len(w):return 'incomplete or inconsistent PDU length'
    at=10
    while at<len(w):
        if at+8>len(w):return 'incomplete message header'
        typ,n=struct.unpack_from('!HH',w,at);at+=4
        if typ!=0x100:return 'outside selected Hello message type'
        end=at+n
        if n<12 or end>len(w):return 'Hello length outside PDU'
        at+=4;first=True
        while at<end:
            if at+4>end:return 'incomplete TLV header'
            flags,n=struct.unpack_from('!HH',w,at);typ=flags&0x3fff;at+=4
            if at+n>end:return 'TLV length crosses message'
            if first and (flags!=0x400 or n!=4):return 'missing leading Common Hello Parameters'
            if not first and typ==0x400:return 'duplicate Common Hello Parameters'
            if typ in (0x400,0x401,0x402,0x403) and n!=(16 if typ==0x403 else 4):return 'known TLV has invalid length'
            first=False;at+=n
    return None
native=collections.defaultdict(list);status=collections.Counter();unreferenced=[]
for line in events.open(encoding='utf-8-sig'):
    e=json.loads(line)
    if e['protocol']!='ldp':continue
    status[e['status']]+=1
    refs=e['source_bytes'].get('PacketRefs') or []
    if not refs:unreferenced.append(e['id'])
    for ref in refs:native[ref['Number']].append(e)
known=['ldp.hdr.version','ldp.hdr.pdu_len','ldp.hdr.ldpid.lsr','ldp.hdr.ldpid.lsid','ldp.msg.type','ldp.msg.len','ldp.msg.id',
       'ldp.msg.tlv.type','ldp.msg.tlv.len','ldp.msg.tlv.hello.hold','ldp.msg.tlv.hello.targeted','ldp.msg.tlv.hello.requested',
       'ldp.msg.tlv.hello.gtsm','ldp.msg.tlv.hello.res','ldp.msg.tlv.ipv4.taddr','ldp.msg.tlv.ipv6.taddr','ldp.msg.tlv.hello.cnf_seqno']
checks=collections.Counter();fail=[];refused=[];outside=[];matched=[];seen=set();counts=collections.Counter()
for packet in json.loads(oracle.read_text(encoding='utf-8-sig')):
    l=packet['_source']['layers'];frame=int(l['frame']['frame.number'])
    if 'udp' not in l:
        outside.append({'frame':frame,'stack':l['frame']['frame.protocols'],'reason':'TCP/session native receiver pending'})
        continue
    counts['udp_oracle_views']+=1
    if 'ldp_raw' not in l:
        refused.append({'frame':frame,'reason':'oracle has no complete raw PDU view'})
        continue
    raw=bytes.fromhex(l['ldp_raw'][0]);reason=reject(raw)
    es=[e for e in native[frame] if base64.b64decode(e['raw'])==raw]
    if reason:
        refused.append({'frame':frame,'reason':reason})
        if es:fail.append({'frame':frame,'reason':'native accepts rejected discovery envelope'})
        continue
    if len(es)!=1:
        fail.append({'frame':frame,'reason':'complete oracle discovery must have exactly one byte-identical native PDU','matches':len(es)})
        continue
    e=es[0];seen.add(e['id']);checks['wire_bytes']+=1
    iface=int(l['frame'].get('frame.interface_id',0))
    refs=e['source_bytes']['PacketRefs']
    if not any(r['Number']==frame and r['Domain']['Interface']==iface for r in refs):fail.append({'frame':frame,'reason':'capture interface/contributor mismatch'})
    if e['flow']!=0:fail.append({'frame':frame,'reason':'discovery invented a TCP/session identity'})
    checks['domain_and_no_session_inference']+=1
    observed=collections.defaultdict(list)
    for v in e['field_values']['values']:observed[v['name']].append(canon(v['value']))
    for name in known:
        want=list(map(canon,values(l,name)));got=observed[name]
        if want or got:
            checks[name]+=1
            if want!=got:fail.append({'frame':frame,'field':name,'wireshark':want,'native':got})
    matched.append(frame)
extra=sorted({e['id'] for es in native.values() for e in es}-seen)
if cli:
    rows=[]
    for line in cli.open(encoding='utf-8-sig'):
        row=json.loads(line)
        if row['protocol']!='LDP':continue
        rows.append(row['number'])
        checks['cli_progressive_path']+=1
        if row.get('protocol_stack',[])[-2:]!=['UDP','LDP']:
            fail.append({'frame':row['number'],'reason':'discovery progressive path omits UDP/LDP'})
    if sorted(rows)!=sorted(matched):fail.append({'reason':'CLI LDP rows differ from byte-verified native/oracle frames'})
result={'scope':'Original full-capture UDP discovery wire bytes and ordered selected fields; no TCP session/label-distribution coverage claim',
        'capture_sha256':sha(capture),'oracle_sha256':sha(oracle),'native_export_sha256':sha(events),'audit_sha256':sha(Path(__file__)),
        'native_status':dict(status),'counts':dict(counts),'matched_discovery_pdus':len(matched),'checks':dict(checks),
        'checks_total':sum(checks.values()),'refused_or_incomplete_views':refused,'outside_native_scope':outside,
        'extra_native_pdu_ids':extra,'unreferenced_native_pdu_ids':unreferenced,'failures':fail}
out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:v for k,v in result.items() if k in ('native_status','counts','matched_discovery_pdus','checks_total','failures','extra_native_pdu_ids')}))
assert not fail and not extra and not unreferenced,'see aggregate and preserved original evidence'
