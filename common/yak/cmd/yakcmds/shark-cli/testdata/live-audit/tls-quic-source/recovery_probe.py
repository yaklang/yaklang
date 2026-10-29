"""Independent owned-capture recovery witnesses. Never imports native decoders.

All exports and packet transformations remain external to the repository. The
TLS source plan is frozen before candidate execution and distinguishes an exact
record AEAD counter from TCP delivery/HTTP state recovery. No key bytes saved.
"""
import argparse,base64,collections,hashlib,hmac,json,os,shutil,struct,subprocess,sys,time
from pathlib import Path

PY=Path(sys.executable)
def sha(data):return hashlib.sha256(data).hexdigest()
def file_sha(path):return sha(Path(path).read_bytes())
def write(path,value):
    with Path(path).open('x',encoding='utf8',newline='\n') as f:json.dump(value,f,indent=2);f.write('\n')
def guard():assert shutil.disk_usage('C:/').free>20*1024**3,'20GiB disk guard'
def hkdf(secret,label,n,hashfn):
    label=b'tls13 '+label;info=n.to_bytes(2,'big')+bytes([len(label)])+label+b'\0';out=b'';last=b''
    for i in range(1,256):
        last=hmac.new(secret,last+info+bytes([i]),hashfn).digest();out+=last
        if len(out)>=n:return out[:n]
    raise AssertionError('HKDF size')
def prf(secret,seed,n,hashfn):
    a=seed;out=b''
    while len(out)<n:a=hmac.new(secret,a,hashfn).digest();out+=hmac.new(secret,a+seed,hashfn).digest()
    return out[:n]
def keys(path):
    result={}
    for line in Path(path).read_text('ascii').splitlines():
        p=line.split()
        if not p or p[0].startswith('#'):continue
        assert len(p)==3
        k=(p[0],bytes.fromhex(p[1]));value=bytes.fromhex(p[2])
        assert k not in result or result[k]==value;result[k]=value
    return result
def tls_clear_records(records,hellos,secret):
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM,ChaCha20Poly1305
    result=[]
    for (cid,direction),entries in sorted(records.items()):
        ch,sh=hellos[cid,1],hellos[cid,2];cipher=sh['cipher'];rnd=ch['random'];name='CLIENT' if direction==0 else 'SERVER'
        tls13=cipher in (0x1301,0x1302,0x1303);hashfn=hashlib.sha384 if cipher in (0x1302,0xc030,0xc02c) else hashlib.sha256
        keylen=32 if cipher in (0x1302,0x1303,0xc030,0xc02c,0xcca8,0xcca9) else 16
        chacha=cipher in (0x1303,0xcca8,0xcca9);aead=ChaCha20Poly1305 if chacha else AESGCM
        encrypted=False;epoch='handshake';seq=0;current=None
        if not tls13:
            ivlen=12 if chacha else 4;block=prf(secret['CLIENT_RANDOM',rnd],b'key expansion'+sh['random']+rnd,2*(keylen+ivlen),hashfn)
            key=block[direction*keylen:(direction+1)*keylen];iv=block[2*keylen+direction*ivlen:2*keylen+(direction+1)*ivlen]
        for e in sorted(entries,key=lambda e:e['offset']):
            w=e['wire'];detail={k:v for k,v in e.items()if k!='wire'};detail.update(connection=cid,direction=direction,record_SHA256=sha(w),record_length=len(w),wire_type=w[0])
            if w[0]==20:
                encrypted=True;seq=0;detail['plaintext_type']=20;detail['authenticated']=False;result.append(detail);continue
            if tls13 and w[0]==23:
                if current is None:current=secret[name+'_HANDSHAKE_TRAFFIC_SECRET',rnd]
                def opened(s,n):
                    k=hkdf(s,b'key',keylen,hashfn);v=hkdf(s,b'iv',12,hashfn);nonce=(int.from_bytes(v,'big')^n).to_bytes(12,'big')
                    return aead(k).decrypt(nonce,w[5:],w[:5])
                try:plain=opened(current,seq)
                except Exception:
                    assert epoch=='handshake';current=secret[name+'_TRAFFIC_SECRET_0',rnd];epoch='application';seq=0;plain=opened(current,seq)
                detail.update(authenticated=True,authenticated_sequence=seq,epoch=epoch);seq+=1
                plain=plain.rstrip(b'\0');assert plain;typ=plain[-1];plain=plain[:-1]
                if typ==22 and plain[:1]==b'\x18':current=hkdf(current,b'traffic upd',hashfn().digest_size,hashfn);seq=0
            elif not tls13 and encrypted:
                typ=w[0];ct=w[5:];nonce=(int.from_bytes(iv,'big')^seq).to_bytes(12,'big') if chacha else iv+ct[:8]
                if not chacha:ct=ct[8:]
                aad=seq.to_bytes(8,'big')+w[:3]+(len(ct)-16).to_bytes(2,'big');plain=aead(key).decrypt(nonce,ct,aad)
                detail.update(authenticated=True,authenticated_sequence=seq,epoch='application');seq+=1
            else:
                typ=w[0];plain=w[5:];detail['authenticated']=False
            detail.update(plaintext_type=typ,plaintext_length=len(plain),plaintext_SHA256=sha(plain));result.append(detail)
    return result

def prepare_tls(a):
    guard();sys.path.insert(0,str(a.audit));from tls_resumption_audit import wire_connections
    from capture_clock_wire_audit import ip,physical
    peer=json.loads((a.root/'capture-01/manifest.json').read_text('utf8'));capture=a.root/'capture-01/live.pcap';keylog=a.root/'capture-01/session.keys'
    source,conns,owners,tls,hellos,_=wire_connections(capture,peer);entries=collections.defaultdict(list)
    for (key,digest),vs in tls.items():
        for e in vs:assert sha(e['wire'])==digest;entries[key].append(e)
    plain=tls_clear_records(entries,hellos,keys(keylog));by_wire={(e['connection'],e['direction'],e['record_SHA256']):e for e in plain}
    cases=[]
    for profile in ['tls12','tls13']:
        folder=a.root/'controls-01'/(profile+'-missing-one-server-record');doc=json.loads((folder/'manifest.json').read_text('utf8'));records=physical(folder/'observation.pcap');cid=doc['target_original_connection'];dropped=doc['omitted_records'];assert len(dropped)==1
        missing=dropped[0]['source_frame'];r=source[missing-1];assert owners[missing]==(cid,1) and file_sha(capture)==doc['source_capture_SHA256']
        _,head,tcp=ip(r);off=(tcp[12]>>4)*4;w=tcp[off:];seq=int.from_bytes(tcp[4:8],'big');assert len(w)==5+int.from_bytes(w[3:5],'big') and w[0]==23
        assert sha(r['wire'])==dropped[0]['source_wire_SHA256'];assert sha(w)in[k[1]for k in tls if k[0]==(cid,1)]
        ledger=doc['ledger'];assert len(records)==len(ledger)==len(source)-1
        for n,(rec,item)in enumerate(zip(records,ledger),1):
            # physical() independently requires captured_length==original_length
            # for every original and retained PCAP record before returning it.
            original=source[item['source_frame']-1];assert rec['wire']==original['wire'] and rec['time_ns']==original['time_ns'] and len(rec['wire'])==len(original['wire']) and item['number']==n
        later=[]
        for e in sorted(entries[cid,1],key=lambda e:e['offset']):
            if min(e['refs'])<=missing:continue
            item=by_wire[cid,1,sha(e['wire'])]
            assert item.get('authenticated') is True
            later.append(item)
        assert later
        retained={x['source_frame'] for x in ledger};tcp_after=[]
        for n in sorted(retained & set(conns[cid]['frames'])):
            if owners[n]!=(cid,1):continue
            _,_,seg=ip(source[n-1]);start=(seg[12]>>4)*4;data=seg[start:]
            if not data:continue
            q=int.from_bytes(seg[4:8],'big');tcp_after.append((n,q,q+len(data)))
        assert all(not(q<seq+len(w) and end>seq)for _,q,end in tcp_after),'no retransmission repairs omitted range'
        cases.append(dict(case=folder.name,capture=str(folder/'observation.pcap'),capture_SHA256=file_sha(folder/'observation.pcap'),keylog=str(keylog),keylog_SHA256=file_sha(keylog),source_manifest_SHA256=file_sha(folder/'manifest.json'),target_original_SYN_generation=cid,profile=profile,direction=1,
            omitted_source_frame=missing,omitted_TCP_sequence_start=seq,omitted_TCP_sequence_end=seq+len(w),omitted_record=by_wire[cid,1,sha(w)],retained_later_records_with_independent_AEAD_authentication=later,
            expected_native_TCP_gap_is_not_misidentification=True,expected_no_same_direction_HTTP_over_gap=True,record_AEAD_counter_known_from_complete_original_capture_only=True,all_retained_physical_bytes_time_and_lengths_exact=True,missing_range_not_repaired=True))
    a.out.mkdir(exist_ok=False);write(a.out/'plan-before.json',dict(scope=__doc__,script_SHA256=file_sha(__file__),source_capture=str(capture),source_capture_SHA256=file_sha(capture),literal_original_frames=len(source),literal_original_TLS_records=len(plain),independently_AEAD_authenticated_records=sum(e.get('authenticated')is True for e in plain),cases=cases,network_fault_injection=False,plaintext_authentication_does_not_prove_application_state_recovery=True))
    print(json.dumps(dict(plan=str(a.out/'plan-before.json'),plan_SHA256=file_sha(a.out/'plan-before.json'),TLS_records=len(plain),later_authenticated_by_case={c['case']:len(c['retained_later_records_with_independent_AEAD_authentication'])for c in cases})))

def execute(a):
    guard();plan=json.loads(a.plan.read_text('utf8'));a.out.mkdir(exist_ok=False);receipt=json.loads(a.receipt.read_text('utf8'));assert receipt['exit_code']==0 and file_sha(a.binary)==receipt['binary_SHA256']
    receipts=[]
    for c in plan['cases']:
        guard();cap=Path(c['capture']);assert file_sha(cap)==c['capture_SHA256'];key=Path(c['keylog']);assert file_sha(key)==c['keylog_SHA256'];folder=a.out/c['case'];folder.mkdir()
        args=[str(a.binary),'replay',str(cap),str(key),'--ignore-embedded-secrets'];env=os.environ.copy();env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB');start=time.time()
        with (folder/'events.jsonl').open('xb')as stdout,(folder/'stderr').open('xb')as stderr:r=subprocess.run(args,stdout=stdout,stderr=stderr,env=env,timeout=180)
        proof=dict(arguments=args,environment={'GOMAXPROCS':'1','GOMEMLIMIT':'1GiB'},source=receipt['source'],binary_SHA256=file_sha(a.binary),build_receipt_SHA256=file_sha(a.receipt),plan_SHA256=file_sha(a.plan),capture_SHA256=file_sha(cap),keylog_SHA256=file_sha(key),exit_code=r.returncode,seconds=round(time.time()-start,3),export_SHA256=file_sha(folder/'events.jsonl'),stderr_SHA256=file_sha(folder/'stderr'))
        write(folder/'producer.json',proof);assert r.returncode==0;receipts.append(proof)
    write(a.out/'summary.json',dict(scope='Actual compiled EXE execution only; source interpretation requires the separate audit',producers=receipts))
    print(json.dumps(dict(cases=len(receipts),summary=str(a.out/'summary.json'))))

if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);sub=ap.add_subparsers(dest='command',required=True)
    p=sub.add_parser('prepare-tls')
    for n in ['root','audit','out']:p.add_argument('--'+n,type=Path,required=True)
    p=sub.add_parser('execute')
    for n in ['plan','binary','receipt','out']:p.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();prepare_tls(a)if a.command=='prepare-tls'else execute(a)
