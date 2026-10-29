"""Execute the new independently source-audited real Windows captures."""
from pathlib import Path
import argparse,collections,hashlib,json,os,subprocess,time
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir()
bins=root/'bins-38';proofs=[]
for name,folder in [('TLS','fresh-TLS-resumption39-03'),('QUIC','fresh-QUIC-epochs39-01')]:
    source=root/folder;capture=source/'live.pcap';keys=source/'session.keys';manifest=source/'manifest.json'
    assert capture.exists()and keys.exists()and manifest.exists()
    target=a.out/name;target.mkdir()
    frozen={k:dict(path=str(p),SHA256=sha(p),bytes=p.stat().st_size)for k,p in dict(capture=capture,keylog=keys,peer_manifest=manifest).items()}
    for kind in('events','yak'):
        receipt=bins/(kind+'-receipt.json');build=read(receipt);binary=bins/(kind+'.exe');assert build['exit_code']==0 and build['source']=='d6772965c669e288705f7e0d0e25c62972226832' and sha(binary)==build['binary_SHA256']
        command=([str(binary),'replay',str(capture),str(keys),'--ignore-embedded-secrets']if kind=='events'else[str(binary),'shark','-r',str(capture),'--json','--ignore-embedded-secrets','--tls-keylog',str(keys)])
        output=target/(kind+'.jsonl');start=time.monotonic()
        with output.open('xb')as stream,(target/(kind+'.stderr')).open('xb')as err:
            p=subprocess.run(command,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=180)
        assert all(sha(v['path'])==v['SHA256']for v in frozen.values())and sha(binary)==build['binary_SHA256']
        counts=collections.Counter();rows=0
        for line in output.open(encoding='utf-8-sig'):
            row=json.loads(line);rows+=1;counts[row.get('protocol',row.get('Protocol','<missing>'))]+=1
        proof=dict(source=build['source'],arguments=command,controller_SHA256=sha(__file__),build_receipt_SHA256=sha(receipt),binary_SHA256=sha(binary),inputs=frozen,exit_code=p.returncode,seconds=round(time.monotonic()-start,3),export_SHA256=sha(output),rows=rows,raw_protocol_counts=dict(counts),whole_object_correctness_not_yet_asserted=True)
        write(target/(kind+'-producer.json'),proof);proofs.append(proof);assert p.returncode==0
        print(json.dumps(dict(capture=name,mode=kind,rows=rows,raw_protocol_counts=dict(counts),closed=True)),flush=True)
write(a.out/'summary.json',dict(scope=__doc__,source=proofs[0]['source'],producers=proofs,source_interpretation_requires_separate_independent_auditor=True,artifacts_external=True))
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'))))
