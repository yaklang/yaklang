"""Annotate raw PIM coverage differences with independent original IP bounds.

Usage: pim_fuzz_review.py ORACLE_JSON PIM_AUDIT_JSON OUTPUT_JSON
No count is removed or normalized. A structural IP barrier explains why bytes
dissected by the oracle cannot yet be admitted as a complete network message.
Messages with no proved barrier stay unresolved for the next profile review.
"""
import collections,hashlib,json,sys
from pathlib import Path

oracle,audit,out=map(Path,sys.argv[1:4])
for p in [oracle,audit,out]:
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):raise ValueError('Raw evidence must stay outside Git')
def sha(w):return hashlib.sha256(w).hexdigest()
def descriptors(v):return [] if not v else [v] if isinstance(v[0],str) else v
rows={int(p['_source']['layers']['frame']['frame.number']):p['_source']['layers'] for p in json.loads(oracle.read_text())}
original=json.loads(audit.read_text());review=[]
for m in original['missing']:
    ls=rows[m['frame']];cap=int(ls['frame']['frame.cap_len']);barriers=[]
    original_frame=bytes.fromhex(ls['frame_raw'][0])
    if len(original_frame)!=cap:raise ValueError('Original frame byte length differs from capture metadata')
    for family in ['ip','ipv6']:
        for d in descriptors(ls.get(family+'_raw')):
            b=bytes.fromhex(d[0]);at=int(d[1])
            if len(d)>5 and int(d[5])!=0:continue # logical/reassembled data source is not a physical frame offset
            if at<0 or original_frame[at:at+len(b)]!=b:raise ValueError('Header bytes do not belong to the original physical frame')
            if at>=m['offset']:continue
            reasons=[]
            if family=='ip':
                if len(b)<20:reasons.append('truncated IPv4 base header')
                else:
                    header=(b[0]&15)*4;length=int.from_bytes(b[2:4],'big')
                    if b[0]>>4!=4:reasons.append('IPv4 version mismatch')
                    if header<20 or header>len(b):reasons.append('IPv4 IHL outside captured header')
                    if length<header:reasons.append('IPv4 Total Length smaller than header')
                    if at+length>cap:reasons.append('IPv4 declared datagram exceeds captured frame')
                    if int.from_bytes(b[6:8],'big')&0x3fff:reasons.append('IPv4 fragment requires proved complete reassembly')
            else:
                if len(b)<40:reasons.append('truncated IPv6 base header')
                else:
                    length=int.from_bytes(b[4:6],'big')
                    if b[0]>>4!=6:reasons.append('IPv6 version mismatch')
                    if length==0:reasons.append('IPv6 zero payload length requires separately validated jumbo/empty semantics')
                    if at+40+length>cap:reasons.append('IPv6 declared datagram exceeds captured frame')
            if reasons:barriers.append({'family':family,'header_offset':at,'header_sha256':sha(b),'reasons':reasons})
    review.append({**m,'oracle_stack':ls['frame']['frame.protocols'],'independent_IP_barriers':barriers,
        'disposition':'bounded network admission barrier proved; original missing view retained' if barriers else 'unresolved carrier/extension profile; original missing view retained'})
result={'scope':'Independent structural IP-bound review of original unmatched PIM views; no decoder labels, counts or raw differences rewritten',
 'oracle_sha256':sha(oracle.read_bytes()),'original_audit_sha256':sha(audit.read_bytes()),'review_script_sha256':sha(Path(__file__).read_bytes()),
 'original_missing_views':len(review),'with_proved_IP_barrier':sum(bool(r['independent_IP_barriers']) for r in review),
 'still_unresolved':sum(not r['independent_IP_barriers'] for r in review),'review':review}
out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:v for k,v in result.items() if k!='review'}))
