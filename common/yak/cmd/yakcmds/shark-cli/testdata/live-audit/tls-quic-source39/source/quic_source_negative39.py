"""Source-only AEAD checker controls over every fresh cipher/direction/epoch.

Mutations are ephemeral ciphertext/key hypotheses, not transformed captures or
new traffic. No secrets/wire/plaintext are serialized; no native output is read.
"""
import argparse,hashlib,importlib.util,json,sys
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def load(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('source','capture-root','audit','recovery','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();sys.path.insert(0,str(a.audit))
    q=load('negative_quic_source',Path(__file__).parent/'quic_source39.py');r=load('negative_kdf',a.recovery);w=load('negative_physical',a.audit/'capture_clock_wire_audit.py')
    assert file_sha(a.source/'summary.json')=='6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859'
    source=read(a.source/'summary.json');rows=[json.loads(line)for line in(a.source/'packet-source.jsonl').read_text('utf8').splitlines()];assert file_sha(a.source/'packet-source.jsonl')==source['packet_source_SHA256']
    conns={c['connection']:c for c in read(a.source/'connection-source.json')};keys=r.keys(a.capture_root/'session.keys');physical=w.physical(a.capture_root/'live.pcap');chosen={}
    for row in rows:
        if row['authenticated']and not row['long_header']:chosen.setdefault((row['connection'],row['direction'],row['epoch']),row)
    assert len(chosen)==18;results=[]
    for(cid,d,epoch),row in chosen.items():
        c=conns[cid];cipher=next(k for k,v in q.SUITES.items()if v==c['suite']);random=next(rnd for label,rnd in keys if label=='CLIENT_TRAFFIC_SECRET_0'and sha(rnd)==c['client_random_SHA256']);side='SERVER'if d else'CLIENT';secret=keys[side+'_TRAFFIC_SECRET_0',random]
        raw=physical[row['frame']-1]['wire'][row['packet_range'][0]:row['packet_range'][1]];pnoff=1+len(bytes.fromhex(row['destination_CID']))
        opened=q.open_packet(raw,pnoff,False,secret,cipher,epoch,row['PN']-1,r.hkdf);assert opened is not None and opened['PN']==row['PN']and sha(opened['plaintext'])==row['plaintext_SHA256']
        other=next(v for k,v in conns.items()if k!=cid);other_random=next(rnd for label,rnd in keys if label=='CLIENT_TRAFFIC_SECRET_0'and sha(rnd)==other['client_random_SHA256'])
        mutations=[('last ciphertext/tag bit',raw[:-1]+bytes([raw[-1]^1]),secret,epoch),('protected phase/AAD bit',bytes([raw[0]^4])+raw[1:],secret,epoch),('DCID/AAD bit',raw[:1]+bytes([raw[1]^1])+raw[2:],secret,epoch),('opposite directional secret',raw,keys[('CLIENT'if d else'SERVER')+'_TRAFFIC_SECRET_0',random],epoch),('other tuple generation secret',raw,keys[side+'_TRAFFIC_SECRET_0',other_random],epoch),('adjacent future key epoch',raw,secret,epoch+1),('short HP sample',raw[:pnoff+19],secret,epoch)]
        if epoch:mutations.append(('prior key epoch',raw,secret,epoch-1))
        rejected=[]
        for name,b,s,e in mutations:
            assert q.open_packet(b,pnoff,False,s,cipher,e,row['PN']-1,r.hkdf)is None;rejected.append(name)
        results.append(dict(connection=cid,direction=d,epoch=epoch,source_frame=row['frame'],source_packet_SHA256=row['packet_SHA256'],positive_source_AEAD=True,rejected_mutations=rejected))
    result=dict(scope=__doc__,source_summary_SHA256=file_sha(a.source/'summary.json'),positive_cipher_direction_epoch_witnesses=18,rejected_mutations=sum(len(r['rejected_mutations'])for r in results),results=results,no_native_or_Yak_output_read=True,source_decoder_SHA256=file_sha(Path(__file__).parent/'quic_source39.py'),checker_SHA256=file_sha(__file__))
    with a.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=file_sha(a.out),positive=18,rejected=result['rejected_mutations'])),flush=True)
if __name__=='__main__':main()
