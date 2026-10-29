"""Fresh TLS source-only AEAD/peer/tuple-generation and isolated WS checks.

No native output, no native label oracle, no old physical-frame count. Ciphertext
record boundaries bind literal TCP bytes; every protected record authenticates
independently. Only hashes/public scalars are serialized, never key/plaintext.
Certificate trust, PSK binder/signature verification and 0-RTT remain unclaimed.
"""
import argparse,collections,copy,csv,hashlib,importlib.util,json,sys,traceback,xml.etree.ElementTree as ET
from pathlib import Path
from cryptography.hazmat.primitives.ciphers.aead import AESGCM,ChaCha20Poly1305

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def load(name,path):
    s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def values(node,name):return [f for f in node.iter('field')if f.get('name')==name]
def num(f):return int(f.get('show'),0)

def decrypt(entries,hellos,secrets,recovery):
    rows=[];clear={}
    for(cid,direction),group in sorted(entries.items()):
        ch,sh=hellos[cid,1],hellos[cid,2];cipher=sh['cipher'];random=ch['random'];side='SERVER'if direction else'CLIENT'
        tls13=cipher in(0x1301,0x1302,0x1303);hf=hashlib.sha384 if cipher in(0x1302,0xc030,0xc02c)else hashlib.sha256
        keylen=32 if cipher in(0x1302,0x1303,0xc030,0xc02c,0xcca8,0xcca9)else 16;chacha=cipher in(0x1303,0xcca8,0xcca9);aead=ChaCha20Poly1305 if chacha else AESGCM
        enabled=False;sequence=0;epoch='handshake';current=None
        if not tls13:
            ivlen=12 if chacha else 4;block=recovery.prf(secrets['CLIENT_RANDOM',random],b'key expansion'+sh['random']+random,2*(keylen+ivlen),hf)
            key=block[direction*keylen:(direction+1)*keylen];iv=block[2*keylen+direction*ivlen:2*keylen+(direction+1)*ivlen]
        for e in sorted(group,key=lambda e:e['offset']):
            w=e['wire'];r=dict(connection=cid,direction=direction,index=e['index'],TCP_stream_offset=e['offset'],source_frames=e['refs'],record_SHA256=sha(w),record_bytes=len(w),wire_type=w[0],legacy_version=int.from_bytes(w[1:3],'big'),record_length=len(w)-5)
            if w[0]==20:
                assert w[5:]==b'\x01';enabled=True;sequence=0;plain=w[5:];typ=20;authenticated=False
            elif tls13 and w[0]==23:
                if current is None:current=secrets[side+'_HANDSHAKE_TRAFFIC_SECRET',random]
                def opened(secret,pn):
                    k=recovery.hkdf(secret,b'key',keylen,hf);iv=recovery.hkdf(secret,b'iv',12,hf)
                    nonce=(int.from_bytes(iv,'big')^pn).to_bytes(12,'big');return aead(k).decrypt(nonce,w[5:],w[:5])
                try:plain=opened(current,sequence)
                except Exception:
                    assert epoch=='handshake';current=secrets[side+'_TRAFFIC_SECRET_0',random];epoch='application';sequence=0;plain=opened(current,sequence)
                r.update(epoch=epoch,authenticated_sequence=sequence);sequence+=1;plain=plain.rstrip(b'\0');assert plain;typ=plain[-1];plain=plain[:-1];authenticated=True
                if typ==22 and plain[:1]==b'\x18':current=recovery.hkdf(current,b'traffic upd',hf().digest_size,hf);sequence=0
            elif not tls13 and enabled:
                ct=w[5:];nonce=(int.from_bytes(iv,'big')^sequence).to_bytes(12,'big')if chacha else iv+ct[:8]
                if not chacha:ct=ct[8:]
                aad=sequence.to_bytes(8,'big')+w[:3]+(len(ct)-16).to_bytes(2,'big');plain=aead(key).decrypt(nonce,ct,aad);typ=w[0];authenticated=True
                r.update(epoch='application',authenticated_sequence=sequence);sequence+=1
            else:plain=w[5:];typ=w[0];authenticated=False
            r.update(authenticated=authenticated,plaintext_type=typ,plaintext_bytes=len(plain),plaintext_SHA256=sha(plain));rows.append(r);clear[cid,direction,e['index']]=plain
    return rows,clear

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('capture-root','audit','recovery','generator','reference','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();sys.path.insert(0,str(a.audit))
    deps=[Path(__file__),a.audit/'tls_resumption_audit.py',a.audit/'capture_clock_wire_audit.py',a.audit/'owned_tls_clock_audit.py',a.audit/'owned_quic_clock_audit.py',a.recovery,a.generator]
    cap=a.capture_root/'live.pcap';key=a.capture_root/'session.keys';manifest=a.capture_root/'manifest.json';peer=read(manifest)
    bound={str(p):file_sha(p)for p in deps+[cap,key,manifest,a.reference/'producer.json',a.reference/'reference.pdml',a.reference/'labels.tsv']}
    assert peer['capture_SHA256']==file_sha(cap)and peer['generator_SHA256']==file_sha(a.generator)and not peer['failures']and not peer['explicit_capture_omission']and not peer['network_fault_injection']and not peer['early_data_claim']
    try:
        audit=load('fresh_source_tls_helpers',a.audit/'tls_resumption_audit.py');recovery=load('fresh_source_crypto_helpers',a.recovery)
        physical,conns,owners,tls,hellos,total=audit.wire_connections(cap,peer);entries=collections.defaultdict(list)
        for(keyid,digest),group in tls.items():
            for e in group:assert sha(e['wire'])==digest;entries[keyid].append(e)
        source,clear=decrypt(entries,hellos,recovery.keys(key),recovery)
        old=recovery.tls_clear_records(entries,hellos,recovery.keys(key));assert len(old)==len(source)
        for r,v in zip(source,old):
            assert r['record_SHA256']==v['record_SHA256']and r['authenticated']==v['authenticated']
            if'plaintext_SHA256'in v:assert r['plaintext_SHA256']==v['plaintext_SHA256']and r['plaintext_bytes']==v['plaintext_length']
        resumed={};handshakes=collections.Counter();http=[];alerts=collections.Counter();application=collections.defaultdict(bytearray);application_segments=collections.defaultdict(list)
        for cid,c in conns.items():
            receipts=[r for r in peer['peer_receipts']if(r['profile'],r['generation'])==(c['profile'],c['generation'])];assert len(receipts)==2
            isresumed=c['generation']in(1,2);resumed[cid]=isresumed
            for r in receipts:assert r['session_reused']is isresumed and r['ALPN']=='http/1.1'
            ch,sh=hellos[cid,1],hellos[cid,2];assert sh['cipher']==(0xc02b if c['profile']=='tls12'else 0x1302)
            if c['profile']=='tls13':assert sh['extensions'][43]==b'\x03\x04'and(41 in sh['extensions'])is isresumed
            elif isresumed:assert ch['session_id']==sh['session_id']and ch['session_id']and ch['extensions'][35]
            c.update(peer_session_resumed=isresumed,client_random_SHA256=sha(ch['random']),server_random_SHA256=sha(sh['random']),cipher=sh['cipher'])
        for r in source:
            cid,d=r['connection'],r['direction'];plain=clear[cid,d,r['index']]
            if r['plaintext_type']==22:
                at=0
                while at<len(plain):
                    assert at+4<=len(plain);kind=plain[at];end=at+4+int.from_bytes(plain[at+1:at+4],'big');assert end<=len(plain);handshakes[cid,d,kind]+=1;at=end
            if r['authenticated']and r['plaintext_type']==21:assert plain==b'\x01\0';alerts[cid,d]+=1
            if r['plaintext_type']==23:
                assert r['authenticated'];start=len(application[cid,d]);application[cid,d]+=plain;application_segments[cid,d].append((start,start+len(plain),r))
        assert all(alerts[cid,d]==1 for cid in conns for d in(0,1))
        operations={(o['profile'],o['generation'],o['index']):o for o in peer['operations']}
        for(cid,d),body in application.items():
            body=bytes(body);at=0;c=conns[cid];index=0
            while at<len(body):
                stop=body.index(b'\r\n\r\n',at)+4;head=body[at:stop];fields=head[:-4].split(b'\r\n');length=int(next((x.split(b':',1)[1]for x in fields if x.lower().startswith(b'content-length:')),b'0'));end=stop+length;assert end<=len(body)
                raw=body[at:end];op=operations[c['profile'],c['generation'],index];assert sha(raw)==op['response_SHA256'if d else'request_SHA256']
                if d:assert fields[0]==b'HTTP/1.1 200 OK'and sha(body[stop:end])==op['body_SHA256']and length==op['body_bytes']
                else:assert fields[0].split()==[b'GET',f"/{c['profile']}/{c['generation']}/{index}".encode(),b'HTTP/1.1']and length==0
                parents=[r for lo,hi,r in application_segments[cid,d]if lo<end and at<hi]
                http.append(dict(connection=cid,direction=d,index=index,application_stream_range=[at,end],message_SHA256=sha(raw),body_SHA256=sha(body[stop:end]),body_bytes=length,authenticated_record_parents=[dict(record_SHA256=r['record_SHA256'],index=r['index'],source_frames=r['source_frames'])for r in parents]));index+=1;at=end
            assert index==len([o for o in peer['operations']if(o['profile'],o['generation'])==(c['profile'],c['generation'])])
        ws=ET.parse(a.reference/'reference.pdml').getroot();assert len(ws)==len(physical);ws_records=collections.Counter();ws_http_req=collections.Counter();ws_http_body=collections.Counter();ws_hs=collections.Counter();ws_resumed={};streamchecks=0
        for frame,p in enumerate(ws,1):
            assert num(values(p,'frame.number')[0])==frame
            if frame not in owners:continue
            cid,d=owners[frame];assert num(values(p,'tcp.stream')[0])==cid-1;streamchecks+=1
            for rec in values(p,'tls.record'):
                types=values(rec,'tls.record.opaque_type')or values(rec,'tls.record.content_type');vers=values(rec,'tls.record.version');lengths=values(rec,'tls.record.length');assert len(types)==len(vers)==len(lengths)==1
                ws_records[cid,d,num(types[0]),num(vers[0]),num(lengths[0])]+=1
            kinds=[num(f)for f in values(p,'tls.handshake.type')]
            for kind in kinds:ws_hs[cid,d,kind]+=1
            if 2 in kinds:ws_resumed[cid]=bool(values(p,'tls.resumed'))if conns[cid]['profile']=='tls12'else bool(values(p,'tls.handshake.extensions.psk.identity.selected'))
            methods=values(p,'http.request.method');uris=values(p,'http.request.uri');assert len(methods)==len(uris)
            for m,u in zip(methods,uris):assert m.get('show')=='GET';ws_http_req[u.get('show')]+=1
            for data in values(p,'http.file_data'):ws_http_body[sha(bytes.fromhex(data.get('value')))]+=1
        expected_records=collections.Counter((r['connection'],r['direction'],r['wire_type'],r['legacy_version'],r['record_length'])for r in source)
        assert ws_records==expected_records and ws_hs==handshakes and ws_resumed==resumed
        assert ws_http_req==collections.Counter(f"/{o['profile']}/{o['generation']}/{o['index']}"for o in peer['operations'])and ws_http_body==collections.Counter(o['body_SHA256']for o in peer['operations'])
        with(a.out/'record-source.jsonl').open('x',encoding='utf8')as f:
            for r in source:f.write(json.dumps(r,separators=(',',':'))+'\n')
        write(a.out/'HTTP-source.json',http);write(a.out/'connection-source.json',conns)
        after={p:file_sha(p)for p in bound};assert bound==after
        summary=dict(scope=__doc__,capture_SHA256=file_sha(cap),keylog_SHA256=file_sha(key),peer_manifest_SHA256=file_sha(manifest),physical_frames=len(physical),TCP_frames=len(owners),marker_UDP_frames=len(physical)-len(owners),SYN_generations=len(conns),real_peer_resumed_connections=sum(resumed.values()),TLS_records=len(source),TLS_wire_bytes=total,AEAD_authenticated_records=sum(r['authenticated']for r in source),AEAD_failures=0,authenticated_close_notify_records=sum(alerts.values()),source_HTTP_requests=sum(r['direction']==0 for r in http),source_HTTP_responses=sum(r['direction']==1 for r in http),all_HTTP_messages_match_actual_peer_hashes=True,source_TCP_record_refs_exact=True,WS_TCP_frame_generation_checks=streamchecks,WS_record_header_occurrences_equal_source=True,WS_record_byte_equality_claim=False,WS_handshake_messages_equal_source=True,WS_actual_peer_resumption_equal=True,WS_HTTP_fields_and_body_hashes_equal_peer=True,record_source_SHA256=file_sha(a.out/'record-source.jsonl'),HTTP_source_SHA256=file_sha(a.out/'HTTP-source.json'),connection_source_SHA256=file_sha(a.out/'connection-source.json'),input_hashes=bound,inputs_unchanged=bound==after,no_native_or_Yak_output_read=True,certificate_signature_trust_PSK_binder_or_0RTT_claim=False)
        write(a.out/'summary.json',summary);print(json.dumps({k:v for k,v in summary.items()if type(v)in(int,bool)}),flush=True);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'))),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
