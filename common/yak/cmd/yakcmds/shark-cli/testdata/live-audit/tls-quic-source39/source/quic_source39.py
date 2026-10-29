"""Independent fresh QUIC v1 AEAD/CID/PN/STREAM/QPACK source witnesses.

Initial protection derives from the literal ODCID. Handshake/application keys
come from the owned peer keylog and observed ClientHello random. PN reconstruction
uses only already-authenticated source packets, never Wireshark/native values.
Keys and complete plaintext remain in memory. No native output is read.
"""
import argparse,collections,hashlib,hmac,importlib.util,json,sys,traceback,xml.etree.ElementTree as ET
from pathlib import Path

SALT=bytes.fromhex('38762cf7f55934b34d179ae6a4c80cadccbb7f0a')
SUITES={0x1301:'AES_128_GCM_SHA256',0x1302:'AES_256_GCM_SHA384',0x1303:'CHACHA20_POLY1305_SHA256'}
def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def load(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def vi(w,at):
    assert at<len(w);n=1<<(w[at]>>6);assert at+n<=len(w);return int.from_bytes(w[at:at+n],'big')&((1<<(8*n-2))-1),at+n
def packet_header(w):
    assert len(w)>0 and w[0]&0x40
    if not w[0]&0x80:return dict(long=False,end=len(w))
    assert len(w)>=7 and int.from_bytes(w[1:5],'big')==1;at=5;n=w[at];at+=1;dcid=w[at:at+n];at+=n;assert len(dcid)==n and n<=20
    n=w[at];at+=1;scid=w[at:at+n];at+=n;assert len(scid)==n and n<=20
    typ=(w[0]>>4)&3;assert typ in(0,2),'only observed Initial/Handshake supported here'
    if typ==0:n,at=vi(w,at);at+=n;assert at<=len(w)
    n,at=vi(w,at);assert at+n<=len(w)
    return dict(long=True,end=at+n,pn_offset=at,dcid=dcid,scid=scid,space='initial'if typ==0 else'handshake')

def frames(w):
    out=[];at=0
    def integer():
        nonlocal at
        n,at=vi(w,at);return n
    def take(n):
        nonlocal at
        assert 0<=n<=len(w)-at;b=w[at:at+n];at+=n;return b
    while at<len(w):
        start=at;t=integer();r={'type':t}
        if t==0:
            n=1
            while at<len(w)and w[at]==0:at+=1;n+=1
            r.update(name='PADDING',length=n)
        elif t==1:r['name']='PING'
        elif t in(2,3):
            largest,delay,count,first=integer(),integer(),integer(),integer();assert count<4096
            r.update(name='ACK_ECN'if t==3 else'ACK',largest=largest,delay=delay,ranges=[[0,first]]+[[integer(),integer()]for _ in range(count)])
            if t==3:r['ECN']=[integer(),integer(),integer()]
        elif t==4:r.update(name='RESET_STREAM',stream=integer(),error=integer(),final_size=integer())
        elif t==5:r.update(name='STOP_SENDING',stream=integer(),error=integer())
        elif t==6:
            off,n=integer(),integer();r.update(name='CRYPTO',offset=off,length=n,data=take(n))
        elif t==7:
            n=integer();r.update(name='NEW_TOKEN',data=take(n),length=n)
        elif 8<=t<=15:
            sid=integer();off=integer()if t&4 else 0;n=integer()if t&2 else len(w)-at;r.update(name='STREAM',stream=sid,offset=off,length=n,FIN=bool(t&1),data=take(n))
        elif t in(0x10,0x12,0x13,0x14,0x16,0x17):r.update(name={0x10:'MAX_DATA',0x12:'MAX_STREAMS_BIDI',0x13:'MAX_STREAMS_UNI',0x14:'DATA_BLOCKED',0x16:'STREAMS_BLOCKED_BIDI',0x17:'STREAMS_BLOCKED_UNI'}[t],limit=integer())
        elif t in(0x11,0x15):r.update(name='MAX_STREAM_DATA'if t==0x11 else'STREAM_DATA_BLOCKED',stream=integer(),limit=integer())
        elif t==0x18:
            sequence,retire=integer(),integer();n=take(1)[0];assert 1<=n<=20;r.update(name='NEW_CONNECTION_ID',sequence=sequence,retire_prior_to=retire,CID=take(n),reset_token=take(16))
        elif t==0x19:r.update(name='RETIRE_CONNECTION_ID',sequence=integer())
        elif t in(0x1a,0x1b):r.update(name='PATH_CHALLENGE'if t==0x1a else'PATH_RESPONSE',data=take(8))
        elif t in(0x1c,0x1d):
            r.update(name='CONNECTION_CLOSE'if t==0x1c else'CONNECTION_CLOSE_APP',error=integer())
            if t==0x1c:r['trigger_type']=integer()
            n=integer();r['reason']=take(n).decode('utf8')
        elif t==0x1e:r['name']='HANDSHAKE_DONE'
        elif t in(0x30,0x31):
            n=integer()if t&1 else len(w)-at;r.update(name='DATAGRAM',length=n,data=take(n))
        else:raise AssertionError(('uncovered authenticated frame type',t))
        r['plaintext_range']=[start,at];out.append(r)
    return out

def open_packet(raw,pnoff,long,secret,cipher,epoch,largest,hkdf):
    from cryptography.hazmat.primitives.ciphers import Cipher,algorithms,modes
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM,ChaCha20Poly1305
    if pnoff+20>len(raw):return None
    hf=hashlib.sha384 if cipher==0x1302 else hashlib.sha256;kl=16 if cipher==0x1301 else 32;chacha=cipher==0x1303
    hp=hkdf(secret,b'quic hp',kl,hf);current=secret
    for _ in range(epoch):current=hkdf(current,b'quic ku',hf().digest_size,hf)
    key=hkdf(current,b'quic key',kl,hf);iv=hkdf(current,b'quic iv',12,hf);sample=raw[pnoff+4:pnoff+20]
    mask=Cipher(algorithms.ChaCha20(hp,sample),None).encryptor().update(bytes(5))if chacha else Cipher(algorithms.AES(hp),modes.ECB()).encryptor().update(sample)[:5]
    w=bytearray(raw);w[0]^=mask[0]&(15 if long else 31);width=(w[0]&3)+1
    for i in range(width):w[pnoff+i]^=mask[i+1]
    truncated=int.from_bytes(w[pnoff:pnoff+width],'big');win=1<<(8*width);expected=largest+1;pn=(expected&~(win-1))|truncated
    if pn+win//2<=expected and pn+win<1<<62:pn+=win
    elif pn>expected+win//2 and pn>=win:pn-=win
    candidates=[pn]+[p for p in(pn-win,pn+win)if 0<=p<1<<62];aead=ChaCha20Poly1305(key)if chacha else AESGCM(key)
    for pn in candidates:
        nonce=(int.from_bytes(iv,'big')^pn).to_bytes(12,'big')
        try:plain=aead.decrypt(nonce,bytes(w[pnoff+width:]),bytes(w[:pnoff+width]))
        except Exception:continue
        assert not(w[0]&(0x0c if long else 0x18)),'authenticated reserved bits are nonzero'
        if not long:assert((w[0]>>2)&1)==epoch%2
        return dict(PN=pn,PN_bytes=width,epoch=epoch,key_phase=None if long else(w[0]>>2)&1,plaintext=plain)
    return None

def cells_add(cells,start,data,source):
    for offset,byte in enumerate(data,start):
        cell=cells.setdefault(offset,[byte,set()]);assert cell[0]==byte,'authenticated overlapping bytes conflict';cell[1].add(source)
def prefix(cells):
    at=0
    while at in cells:at+=1
    return bytes(cells[i][0]for i in range(at))
def frame_public(fr):
    return {k:dict(bytes=len(v),SHA256=sha(v))if isinstance(v,bytes)else v for k,v in fr.items()}

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('capture-root','audit','recovery','generator','libs','reference','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();sys.path[:0]=[str(a.libs),str(a.audit)]
    import pylsqpack
    wire=load('fresh_quic_wire',a.audit/'capture_clock_wire_audit.py');tls=load('fresh_quic_hello',a.audit/'tls_resumption_audit.py');crypto=load('fresh_quic_kdf',a.recovery)
    cap=a.capture_root/'live.pcap';keys=a.capture_root/'session.keys';manifest=a.capture_root/'manifest.json';peer=read(manifest);assert file_sha(a.generator)==peer['generator_SHA256']
    dependencies=[Path(__file__),a.generator,a.recovery,a.audit/'capture_clock_wire_audit.py',a.audit/'tls_resumption_audit.py',a.libs/'pylsqpack/_binding.pyd',a.reference/'producer.json',a.reference/'reference.pdml',cap,keys,manifest]
    bound={str(p):file_sha(p)for p in dependencies};secrets=crypto.keys(keys);physical=wire.physical(cap);contexts=[];routes=collections.defaultdict(set);packets=[];private=[];markers=[];tails=[];crypto_cells=collections.defaultdict(dict);handshake_cursor=collections.defaultdict(int);handshakes=[];stream_cells=collections.defaultdict(dict);final_sizes={}
    def route(c,cid,d):
        assert 1<=len(cid)<=20;routes[cid,d].add(c['id']);c['CIDs'].add((cid.hex(),d))
    def update_handshakes(c,d,space):
        key=(c['id'],d,space);data=prefix(crypto_cells[key]);at=handshake_cursor[key]
        while at+4<=len(data):
            n=int.from_bytes(data[at+1:at+4],'big');end=at+4+n
            if end>len(data):break
            typ=data[at];body=data[at+4:end]
            if typ==1:
                assert d==0 and space=='initial';h=tls.hello(body,True);c['random']=h['random'];assert('CLIENT_TRAFFIC_SECRET_0',h['random'])in secrets;c['ClientHello_SHA256']=sha(data[at:end])
            elif typ==2:
                assert d==1 and space=='initial';h=tls.hello(body,False);assert h['cipher']in SUITES;c['cipher']=h['cipher'];c['ServerHello_SHA256']=sha(data[at:end])
            handshakes.append(dict(connection=c['id'],direction=d,space=space,type=typ,crypto_range=[at,end],message_SHA256=sha(data[at:end]),source_frames=sorted(set().union(*(crypto_cells[key][i][1]for i in range(at,end))))));at=end
        handshake_cursor[key]=at
    try:
        for frame,r in enumerate(physical,1):
            _,iph,udp=wire.ip(r);assert iph[9]==17 and len(udp)>=8;sp,dp=int.from_bytes(udp[:2],'big'),int.from_bytes(udp[2:4],'big');length=int.from_bytes(udp[4:6],'big');assert length==len(udp)
            if peer['server_port']not in(sp,dp):assert peer['readiness']['marker_port']in(sp,dp);markers.append(frame);continue
            direction=int(sp==peer['server_port']);data=udp[8:];base=len(r['wire'])-len(data);at=0
            while at<len(data):
                try:h=packet_header(data[at:])
                except(AssertionError,IndexError):tails.append(dict(frame=frame,range=[base+at,len(r['wire'])],bytes=len(data)-at,SHA256=sha(data[at:]),role='unprotected-unparsed-datagram-tail'));break
                raw=data[at:at+h['end']];space=h.get('space','application');candidate=[]
                if h['long']:
                    candidate=[(contexts[i-1],h['dcid'])for i in routes.get((h['dcid'],direction),())]
                    if not candidate and space=='initial'and direction==0:
                        c=dict(id=len(contexts)+1,ODCID=h['dcid'],CIDs=set(),largest=collections.defaultdict(lambda:-1),cipher=None,random=None,first_frame=frame,client_ports=set(),authenticated_epochs=collections.defaultdict(set));contexts.append(c);route(c,h['dcid'],0);route(c,h['scid'],1);candidate=[(c,h['dcid'])]
                else:
                    for(cid,d),ids in routes.items():
                        if d==direction and raw[1:1+len(cid)]==cid:candidate.extend((contexts[i-1],cid)for i in ids)
                successes=[]
                for c,cid in candidate:
                    if space=='initial':
                        initial=hmac.new(SALT,c['ODCID'],hashlib.sha256).digest();secret=crypto.hkdf(initial,b'server in'if direction else b'client in',32,hashlib.sha256);cipher=0x1301;epochs=[0]
                    elif c['random']is None or c['cipher']is None:continue
                    else:
                        secret=secrets[('SERVER'if direction else'CLIENT')+('_HANDSHAKE_TRAFFIC_SECRET'if space=='handshake'else'_TRAFFIC_SECRET_0'),c['random']];cipher=c['cipher'];epochs=[0]if space=='handshake'else range(5)
                    pnoff=h['pn_offset']if h['long']else 1+len(cid)
                    for epoch in epochs:
                        opened=open_packet(raw,pnoff,h['long'],secret,cipher,epoch,c['largest'][direction,space],crypto.hkdf)
                        if opened is not None:successes.append((c,cid,opened));break
                row=dict(frame=frame,packet_range=[base+at,base+at+len(raw)],packet_SHA256=sha(raw),packet_bytes=len(raw),source_port=sp,destination_port=dp,direction=direction,long_header=h['long'],space=space,authenticated=False)
                assert len(successes)<=1,'ambiguous authenticated connection binding'
                if successes:
                    c,cid,opened=successes[0];prior=c['largest'][direction,space];c['largest'][direction,space]=max(prior,opened['PN']);c['client_ports'].add(dp if direction else sp)
                    if h['long']:route(c,h['scid'],1-direction)
                    fs=frames(opened['plaintext']);c['authenticated_epochs'][direction].add(opened['epoch'])
                    row.update(connection=c['id'],destination_CID=cid.hex(),authenticated=True,PN=opened['PN'],PN_bytes=opened['PN_bytes'],epoch=opened['epoch'],key_phase=opened['key_phase'],out_of_order=opened['PN']<prior,plaintext_SHA256=sha(opened['plaintext']),plaintext_bytes=len(opened['plaintext']),frames=[frame_public(f)for f in fs])
                    for f in fs:
                        if f['name']=='CRYPTO':cells_add(crypto_cells[c['id'],direction,space],f['offset'],f['data'],frame)
                        elif f['name']=='NEW_CONNECTION_ID':assert f['retire_prior_to']<=f['sequence'];route(c,f['CID'],1-direction)
                        elif f['name']=='STREAM':
                            key=(c['id'],direction,f['stream']);cells_add(stream_cells[key],f['offset'],f['data'],frame)
                            if f['FIN']:
                                end=f['offset']+len(f['data']);assert key not in final_sizes or final_sizes[key]==end;final_sizes[key]=end
                    update_handshakes(c,direction,space);private.append((c['id'],direction,fs,frame))
                else:row.update(candidate_connections=sorted(set(c['id']for c,cid in candidate)),failure='no source AEAD match; no PN or plaintext inferred')
                packets.append(row);at+=len(raw)
        assert len(contexts)==len(peer['profiles']);assert sum(not r['authenticated']for r in packets)==peer['network']['corrupted']
        assert {c['cipher']for c in contexts}==set(SUITES)
        for c in contexts:
            assert c['random']is not None and c['cipher']is not None
            for d in(0,1):assert c['authenticated_epochs'][d]=={0,1,2}
        # Parse source SETTINGS first, then replay authenticated STREAM delivery
        # in original packet order so QPACK inserts/blocked blocks remain ordered.
        settings={};stream_types={}
        for key,cells in stream_cells.items():
            cid,d,sid=key;data=prefix(cells)
            if sid&2:
                typ,start=vi(data,0);stream_types[key]=(typ,start)
                if typ==0:
                    at=start
                    while at<len(data):
                        t,at=vi(data,at);n,at=vi(data,at);end=at+n;assert end<=len(data)
                        if t==4:
                            values={};p=at
                            while p<end:k,p=vi(data,p);v,p=vi(data,p);assert k not in values;values[k]=v
                            settings[cid,d]=values
                        at=end
        assert all((c['id'],d)in settings for c in contexts for d in(0,1))
        decoders={(c['id'],d):pylsqpack.Decoder(settings[c['id'],1-d].get(1,0),settings[c['id'],1-d].get(7,0))for c in contexts for d in(0,1)}
        live_cells=collections.defaultdict(dict);cursor=collections.defaultdict(int);headers={};blocked={};bodies=collections.defaultdict(bytearray);h3frames=[]
        def got_header(cid,d,sid,decoded):
            assert(cid,d,sid)not in headers;headers[cid,d,sid]=[(k.decode('ascii'),v.decode('ascii'))for k,v in decoded]
        for cid,d,fs,frame in private:
            for f in fs:
                if f['name']!='STREAM':continue
                key=cid,d,f['stream'];cells_add(live_cells[key],f['offset'],f['data'],frame);data=prefix(live_cells[key]);sid=f['stream'];at=cursor[key]
                if sid&2:
                    if key not in stream_types:continue
                    typ,start=stream_types[key]
                    if len(data)<start:continue
                    at=max(at,start)
                    if typ==2 and len(data)>at:
                        resumed=decoders[cid,d].feed_encoder(data[at:])
                        for stream in resumed:_,head=decoders[cid,d].resume_header(stream);got_header(cid,d,stream,head);blocked.pop((cid,d,stream))
                    cursor[key]=len(data);continue
                while at<len(data):
                    try:t,p=vi(data,at);n,p=vi(data,p)
                    except AssertionError:break
                    end=p+n
                    if end>len(data):break
                    content=data[p:end];h3frames.append(dict(connection=cid,direction=d,stream=sid,type=t,length=n,stream_range=[at,end],payload_SHA256=sha(content),completed_source_frame=frame))
                    if t==0:bodies[key]+=content
                    elif t==1:
                        try:_,head=decoders[cid,d].feed_header(sid,content);got_header(cid,d,sid,head)
                        except pylsqpack.StreamBlocked:blocked[key]=sha(content)
                    else:raise AssertionError(('unexpected request-stream H3 frame',t))
                    at=end
                cursor[key]=at
        assert not blocked
        operations={(o['suite'],o['stream']):o for o in peer['operations']};HTTP=[]
        for key,head in headers.items():
            cid,d,sid=key;c=contexts[cid-1];op=operations[SUITES[c['cipher']],sid];fields=dict(head);assert len(fields)==len(head)
            data=prefix(stream_cells[key]);assert len(data)==len(stream_cells[key])==final_sizes[key]==cursor[key],'no application delivery through a capture hole'
            if not d:assert fields=={':method':'POST',':scheme':'https',':authority':'audit.invalid',':path':op['path'],'content-type':'application/octet-stream'}
            else:assert fields=={':status':'200','content-type':'application/octet-stream'}
            body=bytes(bodies[key]);assert len(body)==op['response_bytes'if d else'request_bytes']and sha(body)==op['response_sha256'if d else'request_sha256']
            HTTP.append(dict(connection=cid,direction=d,stream=sid,suite=SUITES[c['cipher']],headers=head,body_bytes=len(body),body_SHA256=sha(body),complete_contiguous_source=True,final_size=final_sizes[key],source_frames=sorted(set().union(*(v[1]for v in stream_cells[key].values())))))
        assert len(HTTP)==2*len(peer['operations'])
        # Wireshark is comparison only. Its packet boundaries/IDs/PNs never
        # entered the source decoder above.
        ws=ET.parse(a.reference/'reference.pdml').getroot();assert len(ws)==len(physical);reference=[];differences=[]
        by_frame=collections.defaultdict(list)
        for r in packets:by_frame[r['frame']].append(r)
        for frame,p in enumerate(ws,1):
            for q in p.findall('proto'):
                if q.get('name')!='quic':continue
                start,n=int(q.get('pos')),int(q.get('size'));matches=[r for r in by_frame[frame]if r['packet_range']==[start,start+n]]
                if not matches:continue
                r=matches[0];pn=[f for f in q.iter('field')if f.get('name')=='quic.packet_number'];failed=any(f.get('name')=='quic.decryption_failed'for f in q.iter('field'))
                if r['authenticated']:
                    good=len(pn)==1 and int(pn[0].get('show'),0)==r['PN']and not failed
                    item=dict(frame=frame,packet_SHA256=r['packet_SHA256'],source_connection=r['connection'],source_PN=r['PN'],source_epoch=r['epoch'],WS_PN=int(pn[0].get('show'),0)if len(pn)==1 else None,WS_decryption_failed=failed,equal=good)
                    reference.append(item)
                    if not good:differences.append(item)
        source_conns=[dict(connection=c['id'],suite=SUITES[c['cipher']],first_frame=c['first_frame'],ClientHello_SHA256=c['ClientHello_SHA256'],ServerHello_SHA256=c['ServerHello_SHA256'],client_random_SHA256=sha(c['random']),client_ports=sorted(c['client_ports']),authenticated_CID_routes=[dict(CID=cid,direction=d)for cid,d in sorted(c['CIDs'])],epochs={str(d):sorted(es)for d,es in c['authenticated_epochs'].items()})for c in contexts]
        with(a.out/'packet-source.jsonl').open('x',encoding='utf8')as f:
            for r in packets:f.write(json.dumps(r,separators=(',',':'))+'\n')
        write(a.out/'HTTP3-source.json',HTTP);write(a.out/'connection-source.json',source_conns);write(a.out/'handshake-source.json',handshakes);write(a.out/'HTTP3-frame-source.json',h3frames);write(a.out/'reference-differences.json',differences)
        after={p:file_sha(p)for p in bound};assert bound==after
        summary=dict(scope=__doc__,physical_frames=len(physical),marker_frames=len(markers),observed_QUIC_packets=len(packets),source_AEAD_authenticated_packets=sum(r['authenticated']for r in packets),source_unopened_packets=sum(not r['authenticated']for r in packets),source_connections=len(contexts),source_ciphers=len(set(c['cipher']for c in contexts)),key_epochs_per_direction={str(c['id']):{str(d):sorted(es)for d,es in c['authenticated_epochs'].items()}for c in contexts},source_authenticated_out_of_order_packets=sum(r.get('out_of_order',False)for r in packets),unprotected_unparsed_tails=tails,HTTP3_requests=sum(r['direction']==0 for r in HTTP),HTTP3_responses=sum(r['direction']==1 for r in HTTP),all_HTTP3_headers_QPACK_decoded=True,all_HTTP3_body_hashes_equal_actual_peers=True,no_hole_or_cross_connection_byte_stitch=True,WS_authenticated_source_packet_checks=len(reference),WS_packet_differences=len(differences),peer_declared_network_fault_counts=peer['network'],missing_datagrams_not_individually_identified_from_unseen_wire=True,packet_source_SHA256=file_sha(a.out/'packet-source.jsonl'),HTTP3_source_SHA256=file_sha(a.out/'HTTP3-source.json'),connection_source_SHA256=file_sha(a.out/'connection-source.json'),reference_differences_SHA256=file_sha(a.out/'reference-differences.json'),inputs=bound,inputs_unchanged=bound==after,no_native_or_Yak_output_read=True,no_peer_certificate_or_replay_protection_claim=True,pylsqpack=pylsqpack.__version__)
        write(a.out/'summary.json',summary);print(json.dumps({k:v for k,v in summary.items()if type(v)in(int,bool)}),flush=True);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'))),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
