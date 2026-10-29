"""Run the unchanged official6 with the closed pre-GTP native/CLI binaries."""
from pathlib import Path
import collections,hashlib,json,os,subprocess,time
root=Path(__file__).parent;out=root/'GTPv2-official-before-cd7af45-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
cap=root/'GTP-official-download43-01/project-upload.pcap';assert sha(cap)=='b62192ea1eba8e3bce2bff600222ce1ac41caf01867f0336a026f161da355030'
results=[];rows={}
for kind in('events','yak'):
 binary=root/f'bins-42/{kind}.exe';receipt=binary.with_name(kind+'-receipt.json');build=json.loads(receipt.read_text('utf8'));assert build['source']=='cd7af6a67e998ed1a6466a11f2b37c66fad742a8'and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
 args=[str(binary),'replay',str(cap),'-','--ignore-embedded-secrets']if kind=='events'else[str(binary),'shark','-r',str(cap),'--json','--ignore-embedded-secrets']
 start=time.monotonic();output=out/(kind+'.jsonl');err=out/(kind+'.stderr');frozen={str(p):sha(p)for p in(binary,receipt,cap,Path(__file__))}
 with output.open('xb')as f,err.open('xb')as e:r=subprocess.run(args,stdout=f,stderr=e,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
 assert r.returncode==0 and frozen=={p:sha(p)for p in frozen}
 rows[kind]=[json.loads(l)for l in output.read_text('utf8').splitlines()];assert len(rows[kind])==6
 results.append(dict(kind=kind,arguments=args,source=build['source'],exit_code=r.returncode,seconds=time.monotonic()-start,inputs=frozen,export_SHA256=sha(output),stderr_SHA256=sha(err),protocol_counts=dict(collections.Counter(x.get('protocol','')for x in rows[kind]))))
proof=dict(scope=__doc__,original_capture_SHA256=sha(cap),source=build['source'],SDK_physical_events=6,CLI_physical_rows=6,runs=results,controller_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(proof))
