"""Execute the Windows binary and capture actual console cells and attributes."""
from pathlib import Path
import argparse,base64,hashlib,json,os,subprocess,time
P=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser()
for k in('bins','native','audit','plan','out'):ap.add_argument('--'+k,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
read=lambda p:json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==a.source
 assert not[x for x in subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,text=True).splitlines()if x[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
guard();audit=read(a.audit);assert audit['passed']and audit['source']==a.source and audit['dataset']=='source50'
native=a.native/'events.jsonl';assert audit['inputs'][str(native)]==sha(native)
events=[json.loads(x)for x in native.read_text('utf8').splitlines()];assert len(events)==71
plan=read(a.plan);assert plan['expected_not_from_candidate']and plan['physical_frames']==115
binary=a.bins/'yak.exe';build=a.bins/'yak-receipt.json'
assert read(build)['source']==a.source and read(build)['exit_code']==0 and read(build)['binary_SHA256']==sha(binary)
helper=P/'Windows-console-ack-build45-01/native-console.exe'
renderer=P/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
assert sha(helper)=='12e0aa1b8b3cfb4696e1d680334dae490e269de37773e01aafa35bc4a6d8b68c'
assert sha(renderer)=='427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83'
a.out.mkdir(exist_ok=False);proofs=[]
for case in plan['cases']:
 guard();matches=[e for e in events if e.get('raw')and e['source_bytes']['PacketRefs']==[{'Number':case['frame'],'Domain':{'Section':0,'Interface':0,'Encapsulation':''}}]]
 assert len(matches)==1;event=matches[0];raw=base64.b64decode(event['raw'])
 assert len(raw)==case['PDU_bytes']and hashlib.sha256(raw).hexdigest()==case['raw_SHA256']
 capture=Path(case['capture']);assert sha(capture)==case['capture_SHA256']
 query='fields pdu.id == '+str(event['id']);folder=a.out/case['name']
 args=[str(helper),str(folder),str(binary),str(capture),'-',query,case['word'],'--ignore-embedded-secrets']
 env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
 env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='SSH',SHARK_QA_EXPECT_QUERY=query,SHARK_QA_EXPECT_PACKETS='115',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN=str(case['field_down']),SHARK_QA_SELECT_FIELD=case['select_field'],SHARK_QA_SELECT_FIELD_VALUE=case['select_value'])
 inputs={str(p):sha(p)for p in(a.plan,a.audit,native,binary,build,helper,renderer,capture,Path(case['raw_path']),Path(__file__))}
 start=time.monotonic()
 with(a.out/(case['name']+'.stdout')).open('xb')as out,(a.out/(case['name']+'.stderr')).open('xb')as err:
  r=subprocess.run(args,stdout=out,stderr=err,env=env,timeout=180,creationflags=0x08000000)
 shots={}
 if folder.exists():
  for f in folder.glob('*.json'):
   data=read(f)
   if 'Rows'not in data:continue
   assert len(data['Rows'])==len(data['Attributes'])==data['Height']and all(len(x)==len(y)==data['Width']for x,y in zip(data['Rows'],data['Attributes']))
   image=f.with_suffix('.png');render=subprocess.run([os.sys.executable,'-B',str(renderer),str(f),str(image)],capture_output=True,creationflags=0x08000000);assert render.returncode==0
   shots[f.stem]=dict(cells_SHA256=sha(f),PNG_SHA256=sha(image))
 guard();proof=dict(source=a.source,frame=case['frame'],source_PDU_id=event['id'],arguments=args,environment={k:v for k,v in env.items()if k.startswith('SHARK_QA_')},inputs=inputs,inputs_unchanged=inputs=={p:sha(p)for p in inputs},exit_code=r.returncode,seconds=round(time.monotonic()-start,3),snapshots=shots,console_cells_not_Terminal_pixels=True,root_visual_and_range_review_pending=True)
 put(a.out/(case['name']+'-producer.json'),proof);proofs.append(proof)
 assert r.returncode==0 and proof['inputs_unchanged'],'preserve failed execution and select a fresh plan/execution name'
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
put(a.out/'producer-summary.json',dict(source=a.source,closed_cases=len(proofs),runs=proofs,root_review_pending=True))
print(json.dumps(dict(source=a.source,closed_cases=len(proofs),root_review_pending=True)))
