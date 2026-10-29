"""Freeze source-exact loss of an entire intermediate QUIC key epoch.

Source is a real owned aioquic capture; omissions are observation controls, not
new live network faults. Every retained QUIC packet is independently AEAD
checked before any candidate output is read. Secrets/plaintext are not saved.
"""
import argparse,collections,hashlib,hmac,json,shutil,struct,sys,xml.etree.ElementTree as ET
from pathlib import Path
from recovery_probe import keys,hkdf,file_sha,sha,write,guard

def values(node,name):return [x for x in node.iter('field')if x.get('name')==name]
def only(node,name):
    rows=values(node,name);assert len(rows)==1,(name,len(rows));return rows[0]
def num(node,name):return int(only(node,name).get('show'),0)
def vint(w,at):
    n=1<<(w[at]>>6);assert at+n<=len(w);return int.from_bytes(w[at:at+n],'big')&((1<<(n*8-2))-1),at+n
def bounds(w):
    assert w and w[0]&0x40
    if not w[0]&0x80:return None
    assert len(w)>=7 and int.from_bytes(w[1:5],'big')==1
    at=5;dl=w[at];at+=1+dl;assert at<len(w);sl=w[at];at+=1+sl;assert at<=len(w)
    if (w[0]>>4)&3==0:n,at=vint(w,at);at+=n;assert at<=len(w)
    n,at=vint(w,at);assert at+n==len(w);return at
def unprotect(raw,pn,cid,secret,cipher,epoch):
    from cryptography.hazmat.primitives.ciphers import Cipher,algorithms,modes
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM,ChaCha20Poly1305
    hf=hashlib.sha384 if cipher==0x1302 else hashlib.sha256;kl=16 if cipher==0x1301 else 32;chacha=cipher==0x1303
    # RFC9001 key-update HP key stays unchanged across epochs.
    hp=hkdf(secret,b'quic hp',kl,hf);current=secret
    for _ in range(epoch):current=hkdf(current,b'quic ku',hf().digest_size,hf)
    key=hkdf(current,b'quic key',kl,hf);iv=hkdf(current,b'quic iv',12,hf);at=1+len(cid);assert raw[1:at]==cid and at+20<=len(raw)
    sample=raw[at+4:at+20]
    if chacha:mask=Cipher(algorithms.ChaCha20(hp,sample),mode=None).encryptor().update(bytes(5))
    else:mask=Cipher(algorithms.AES(hp),modes.ECB()).encryptor().update(sample)[:5]
    w=bytearray(raw);w[0]^=mask[0]&31;n=(w[0]&3)+1
    for i in range(n):w[at+i]^=mask[i+1]
    assert int.from_bytes(w[at:at+n],'big')==pn%2**(n*8)
    nonce=(int.from_bytes(iv,'big')^pn).to_bytes(12,'big');aead=ChaCha20Poly1305(key)if chacha else AESGCM(key)
    plain=aead.decrypt(nonce,bytes(w[at+n:]),bytes(w[:at+n]))
    return plain,((w[0]>>2)&1),n

def run(a):
    guard();sys.path.insert(0,str(a.audit));from vxlan_endpoint_projection_audit import capture_records
    records=list(capture_records(a.capture));manifest=json.loads(a.manifest.read_text('utf8'));assert len(records)==2036
    oracle=ET.parse(a.reference).getroot();assert len(oracle)==len(records);secret=keys(a.keylog);rows=[];randoms={};ciphers={}
    for frame,(r,p)in enumerate(zip(records,oracle),1):
        assert num(p,'frame.number')==frame and num(p,'frame.cap_len')==r['captured_length'] and num(p,'frame.len')==r['original_length']
        connection=num(p,'quic.connection.number');sp=num(p,'udp.srcport');dp=num(p,'udp.dstport');direction=int(sp==manifest['server_port']);assert(sp==manifest['server_port'])!=(dp==manifest['server_port'])
        if values(p,'tls.handshake.random'):
            for f in values(p,'tls.handshake.random'):
                # Only the CH random is a keylog lookup key. Avoid assuming an
                # arbitrary SH field is the per-connection identity.
                rnd=bytes.fromhex(f.get('show').replace(':',''))
                if ('CLIENT_TRAFFIC_SECRET_0',rnd)in secret:
                    assert connection not in randoms or randoms[connection]==rnd;randoms[connection]=rnd
        for q in p.findall('proto'):
            if q.get('name')!='quic':continue
            start,size=int(q.get('pos')),int(q.get('size'));raw=r['wire'][start:start+size];assert len(raw)==size;long=bool(raw[0]&128)
            if not values(q,'quic.header_form'):
                # Literal unprotected datagram tails are separate observations,
                # never packets/authenticated data merely because WS calls the
                # containing protocol subtree QUIC.
                assert any('(Random) padding data appended to the datagram'in f.get('showname','')for f in q.iter('field'))
                rows.append(dict(source_frame=frame,packet_offset=start,packet_end=start+size,packet_SHA256=sha(raw),oracle_connection=connection,direction=direction,source_port=sp,destination_port=dp,long_header=None,packet_role='unprotected-datagram-padding',packet_number=None,key_phase=None));continue
            assert num(q,'quic.header_form')==int(long);pn=[int(f.get('show'),0)for f in values(q,'quic.packet_number')]
            if long:bounds(raw)
            phase=None if long else only(q,'quic.key_phase').get('show')=='True'
            dcid=bytes.fromhex(only(q,'quic.dcid').get('show').replace(':',''))
            rows.append(dict(source_frame=frame,packet_offset=start,packet_end=start+size,packet_SHA256=sha(raw),destination_cid=dcid.hex(),oracle_connection=connection,direction=direction,source_port=sp,destination_port=dp,long_header=long,packet_number=pn[0]if pn else None,key_phase=int(phase)if phase is not None else None))
    assert len(randoms)==3
    for cid in randoms:
        matching=[p for p in oracle if num(p,'quic.connection.number')==cid and values(p,'tls.handshake.ciphersuite')]
        suites={int(f.get('show'),0)for p in matching for f in values(p,'tls.handshake.ciphersuite')};assert len(suites)==1;ciphers[cid]=next(iter(suites))
    groups=collections.defaultdict(list)
    for row in rows:
        if row['long_header']is False and row['packet_number']is not None:groups[row['oracle_connection'],row['direction']].append(row)
    for (cid,direction),items in groups.items():
        epoch=-1;previous=None
        for row in sorted(items,key=lambda x:x['packet_number']):
            if row['key_phase']!=previous:epoch+=1;previous=row['key_phase']
            row['epoch']=epoch;raw=records[row['source_frame']-1]['wire'][row['packet_offset']:row['packet_end']];label=('SERVER'if direction else'CLIENT')+'_TRAFFIC_SECRET_0'
            plain,phase,pnlen=unprotect(raw,row['packet_number'],bytes.fromhex(row['destination_cid']),secret[label,randoms[cid]],ciphers[cid],epoch)
            assert phase==row['key_phase'];row.update(independently_AEAD_authenticated=True,plaintext_SHA256=sha(plain),plaintext_bytes=len(plain),packet_number_bytes=pnlen)
        assert epoch==2,(cid,direction,epoch)
    a.out.mkdir(exist_ok=False);target=min(randoms);omitted={r['source_frame']for r in rows if r['oracle_connection']==target and r['direction']==0 and r.get('epoch')==1}
    assert omitted
    for frame in omitted:assert all(r['oracle_connection']==target and r['direction']==0 and r.get('epoch')==1 for r in rows if r['source_frame']==frame),'never discard an unrelated coalesced packet'
    keep=[i for i in range(1,len(records)+1)if i not in omitted];dest=a.out/'observation.pcap';links={r['link']for r in records};assert len(links)==1
    with dest.open('xb')as out:
        out.write(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,next(iter(links))))
        for frame in keep:
            r=records[frame-1];s,ns=divmod(r['time_ns'],10**9);w=r['wire'];out.write(struct.pack('<IIII',s,ns,len(w),r['original_length'])+w)
    reread=list(capture_records(dest));assert len(reread)==len(keep)
    for original,r in zip(keep,reread):
        source=records[original-1];assert r['wire']==source['wire']and r['time_ns']==source['time_ns']and r['original_length']==source['original_length']
    write(a.out/'packet-source-plan.jsonl.meta.json',dict(rows=len(rows),source_capture_SHA256=file_sha(a.capture),fresh_reference_SHA256=file_sha(a.reference)))
    with(a.out/'packet-source-plan.jsonl').open('x',encoding='utf8')as out:
        for row in rows:out.write(json.dumps(row,separators=(',',':'))+'\n')
    plan=dict(scope=__doc__,script_SHA256=file_sha(__file__),source_capture=str(a.capture),source_capture_SHA256=file_sha(a.capture),source_manifest_SHA256=file_sha(a.manifest),source_reference_SHA256=file_sha(a.reference),source_packet_plan_SHA256=file_sha(a.out/'packet-source-plan.jsonl'),original_physical_frames=len(records),original_QUIC_packets=len(rows),independently_authenticated_short_packets=sum(r.get('independently_AEAD_authenticated')is True for r in rows),target_oracle_connection=target,target_direction=0,omitted_original_frames=sorted(omitted),ledger=[dict(number=n,source_frame=i,source_wire_SHA256=sha(records[i-1]['wire']),time_ns=records[i-1]['time_ns'],section=records[i-1]['section'],interface=records[i-1]['interface'])for n,i in enumerate(keep,1)],
        conservative_authentication_failure_is_not_misidentification=True,no_authenticated_application_or_stale_plaintext_from_unopened_epoch2=True,retained_reverse_direction_and_other_connections_require_source_exact_authentication=True,
        cases=[dict(case='original-real-network-loss',capture=str(a.capture),capture_SHA256=file_sha(a.capture),keylog=str(a.keylog),keylog_SHA256=file_sha(a.keylog)),dict(case='all-client-middle-epoch-omitted',capture=str(dest),capture_SHA256=file_sha(dest),keylog=str(a.keylog),keylog_SHA256=file_sha(a.keylog))])
    write(a.out/'plan-before.json',plan);print(json.dumps(dict(plan_SHA256=file_sha(a.out/'plan-before.json'),packets=len(rows),AEAD_short=sum(r.get('independently_AEAD_authenticated')is True for r in rows),omitted_frames=len(omitted),target=target,retained_epoch2_short=sum(r['oracle_connection']==target and r['direction']==0 and r.get('epoch')==2 for r in rows))))

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ['capture','manifest','keylog','reference','audit','out']:p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
