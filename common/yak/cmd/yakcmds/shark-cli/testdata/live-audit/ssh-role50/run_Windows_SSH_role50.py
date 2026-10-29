"""Capture source-bound actual Windows console cells for role50 diagnostics."""
from pathlib import Path
import argparse,base64,hashlib,json,os,subprocess,time
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser()
for n in('bins','native','audit','out'):ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==a.source
 assert not[p for p in subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,text=True).splitlines()if p[3:].endswith(('.go','.yaml','.yml','.zst','.mod','.sum'))]
guard();audit=read(a.audit);assert audit['passed']
plan=root/'Windows-SSH-role-source-plan50-01/plan.json';p=read(plan)
assert sha(plan)=='37b2eea6c83bc639ecc4f222aad82f061458e4c95b6aae1b8921a3fa95816cf7'
assert p['physical_frames']==115
binary=a.bins/'yak.exe';receipt=a.bins/'yak-receipt.json';build=read(receipt)
assert build['source']==a.source and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
native=a.native/'events.jsonl';events=[json.loads(s)for s in native.read_text('utf8').splitlines()]
assert audit['inputs'][str(native)]==sha(native),'audit must bind the exact closed native export'
assert len(events)==71
helper=root/'Windows-console-ack-build45-01/native-console.exe';renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
assert sha(helper)=='12e0aa1b8b3cfb4696e1d680334dae490e269de37773e01aafa35bc4a6d8b68c'
assert sha(renderer)=='427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83'
assert not a.out.exists();a.out.mkdir();proofs=[]
for c in p['cases']:
 guard();selected=[e for e in events if e.get('raw')and e['source_bytes']['PacketRefs']==[{'Number':c['frame'],'Domain':{'Section':0,'Interface':0,'Encapsulation':''}}]]
 assert len(selected)==1;e=selected[0];raw=base64.b64decode(e['raw'])
 assert type(e['id'])is int and e['id']>0 and len(raw)==c['retained_bytes']and hashlib.sha256(raw).hexdigest()==c['retained_SHA256']
 capture=Path(c['capture']);assert sha(capture)==c['capture_SHA256']
 query='fields pdu.id == '+str(e['id']);folder=a.out/c['name']
 args=[str(helper),str(folder),str(binary),str(capture),'-',query,'NEWKEYS'if c['preview_only']else'Expected Sender Role','--ignore-embedded-secrets']
 env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
 env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='SSH',SHARK_QA_EXPECT_QUERY=query,
  SHARK_QA_EXPECT_PACKETS='115',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN='0')
 if c['select_field']:env.update(SHARK_QA_SELECT_FIELD=c['select_field'],SHARK_QA_SELECT_FIELD_VALUE=c['select_value'])
 inputs={str(f):sha(f)for f in(plan,a.audit,native,binary,receipt,helper,renderer,capture,Path(c['raw_path']),Path(__file__))}
 started=time.monotonic();r=subprocess.run(args,capture_output=True,env=env,timeout=180,creationflags=0x08000000)
 (a.out/(c['name']+'.stdout')).write_bytes(r.stdout);(a.out/(c['name']+'.stderr')).write_bytes(r.stderr)
 shots={}
 for f in folder.glob('*.json'):
  data=read(f)
  if 'Rows'not in data:continue
  assert len(data['Rows'])==len(data['Attributes'])==data['Height']and all(len(x)==len(y)==data['Width']for x,y in zip(data['Rows'],data['Attributes']))
  image=f.with_suffix('.png');q=subprocess.run([os.sys.executable,'-B',str(renderer),str(f),str(image)],capture_output=True,creationflags=0x08000000);assert q.returncode==0
  shots[f.stem]=dict(cells_SHA256=sha(f),PNG_SHA256=sha(image))
 guard();proof=dict(source=a.source,source_frame=c['frame'],source_PDU_id=e['id'],arguments=args,
  environment={k:v for k,v in env.items()if k.startswith('SHARK_QA_')},inputs=inputs,inputs_unchanged=inputs=={f:sha(f)for f in inputs},
  exit_code=r.returncode,seconds=round(time.monotonic()-started,3),snapshots=shots,
  console_cells_not_Terminal_pixels=True,root_source_byte_and_visual_review_pending=True)
 put(a.out/(c['name']+'-producer.json'),proof);proofs.append(proof)
 assert r.returncode==0 and proof['inputs_unchanged'],'helper failed; preserve output and use a new execution name'
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
put(a.out/'producer-summary.json',dict(source=a.source,closed_cases=len(proofs),runs=proofs,root_review_pending=True))
print(json.dumps(dict(source=a.source,closed_cases=len(proofs),actual_images_ready=True,root_review_pending=True)))
