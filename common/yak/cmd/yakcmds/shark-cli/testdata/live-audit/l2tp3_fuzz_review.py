"""Independently classify preserved L2TPv3 fuzz refusals, not count normalization.

Usage: l2tp3_fuzz_review.py ORACLE_JSON WIRE_AUDIT_JSON EXTRA_REFUSALS_JSON OUTPUT
The primary audit retains its nonzero exit and missing views. This supplement
records exactly why those views cannot establish a valid native v3 PDU.
"""
import collections,hashlib,json,sys
from pathlib import Path
original,audit,extras,out=map(Path,sys.argv[1:5])
for p in [original,audit,extras,out]:
    if any((a/'.git').exists() for a in [p.resolve(),*p.resolve().parents]):raise ValueError('Raw audit evidence stays outside Git')
def sha(b):return hashlib.sha256(b).hexdigest()
def desc(x):return ([x] if isinstance(x[0],str) else x) if x else []
ps={int(p['_source']['layers']['frame']['frame.number']):p['_source']['layers'] for p in json.loads(original.read_text(encoding='utf-8-sig'))}
r=json.loads(audit.read_text());reviewed=[]
for row in r['missing']:
    n=row['frame'];ls=ps[n];ipraw=desc(ls.get('ip_raw'));reason=None;proof={}
    for ir in ipraw:
        w=bytes.fromhex(ir[0]);at=int(ir[1])
        if not w or at+len(w)>row['offset']:continue
        assert bytes.fromhex(ls['frame_raw'][0])[at:at+len(w)]==w,(n,'logical IP view cannot prove a physical header')
        version=w[0]>>4;proof={'version':version,'header_bytes':len(w)}
        if version!=4:
            # EtherType is independently visible in the original frame. An
            # IPv6 header under the IPv4 EtherType isn't a valid IPv6 carrier.
            frame=bytes.fromhex(ls['frame_raw'][0])
            assert at==14 and frame[12:14]==b'\x08\x00'
            reason='IP version does not match the observed IPv4 EtherType';proof['ether_type']=0x800;break
        if len(w)<20:continue
        proof['wire_total_length']=int.from_bytes(w[2:4],'big')
        flags=int.from_bytes(w[6:8],'big')
        if flags&0x3fff:
            reason='fragmented IP has no complete native datagram; first-fragment dissection is not delivery';proof['fragment_offset_bytes']=(flags&0x1fff)*8;proof['more_fragments']=bool(flags&0x2000);break
        atopt=20
        while atopt<len(w):
            kind=w[atopt]
            if kind==0:break
            if kind==1:atopt+=1;continue
            length=w[atopt+1] if atopt+1<len(w) else 0
            if length<2 or atopt+length>len(w):
                reason='invalid IPv4 option length/boundary';proof.update({'option_type':kind,'option_length':length,'option_offset':atopt,'remaining_header_bytes':len(w)-atopt});break
            atopt+=length
        if reason:break
    assert reason,(n,'unreviewed missing wire view')
    reviewed.append({**row,'classification':reason,'independent_proof':proof})

extra_packets={int(p['_source']['layers']['frame']['frame.number']):p['_source']['layers'] for p in json.loads(extras.read_text(encoding='utf-8-sig'))}
extra_review=[]
for row in r['unmatched_native']:
    assert row['status'] in ['malformed','incomplete'],row
    assert len(row['frames'])==1,row
    n=row['frames'][0];ls=extra_packets[n]
    assert not ls.get('l2tp'),(n,'oracle unexpectedly decoded L2TP')
    proof=None
    for ir in desc(ls.get('ip_raw')):
        w=bytes.fromhex(ir[0]);at=int(ir[1]);frame=bytes.fromhex(ls['frame_raw'][0])
        if len(w)<20 or w[0]>>4!=4 or w[9]!=115:continue
        assert frame[at:at+len(w)]==w,(n,'extra refusal IP header is not in the original frame')
        ihl=(w[0]&15)*4;total=int.from_bytes(w[2:4],'big');cap=len(frame)-at
        if total<ihl or total>cap:
            proof={'version':4,'protocol':115,'header_bytes':ihl,'wire_total_length':total,'captured_IP_bytes':cap};break
    assert proof,(n,'extra native diagnostic needs an independent invalid/incomplete IP115 proof')
    extra_review.append({**row,'classification':'explicit IP115 carrier refusal, not a decoded L2TP protocol claim','independent_proof':proof})

result={'scope':'exact retained fuzz-boundary review; primary missing/extra counts remain visible',
 'oracle_sha256':sha(original.read_bytes()),'wire_audit_sha256':sha(audit.read_bytes()),'extra_oracle_sha256':sha(extras.read_bytes()),'review_sha256':sha(Path(__file__).read_bytes()),
 'primary_missing':len(r['missing']),'primary_unmatched_native':len(r['unmatched_native']),
 'missing_classifications':dict(collections.Counter(x['classification'] for x in reviewed)),
 'reviewed_missing':reviewed,'reviewed_extra_refusals':extra_review,
 'invalid_parent_UDP_views':len(r['blocked_by_invalid_carrier']),
 'unresolved':[]}
out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:v for k,v in result.items() if k in ['primary_missing','primary_unmatched_native','missing_classifications','invalid_parent_UDP_views','unresolved']}))
