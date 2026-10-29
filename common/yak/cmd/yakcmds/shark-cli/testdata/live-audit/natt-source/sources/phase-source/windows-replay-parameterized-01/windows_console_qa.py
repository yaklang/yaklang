"""Parameter-only driver for the existing native Windows console QA harness.
This tests the CLI executable without typing into or automating desktop apps.
"""
import argparse,hashlib,json,os,subprocess,sys,time
from pathlib import Path
ap=argparse.ArgumentParser()
for name in('bins','manifest','helper','renderer','out'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--source',required=True);ap.add_argument('--cases',nargs='+',required=True)
a=ap.parse_args();assert os.name=='nt';a.out.mkdir(exist_ok=False)
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for chunk in iter(lambda:f.read(1<<20),b''):h.update(chunk)
 return h.hexdigest()
binary=a.bins/'yak.exe';receipt_path=a.bins/'yak-receipt.json';receipt=json.loads(receipt_path.read_text());binary_hash=sha(binary)
assert receipt['source']==a.source and receipt['exit_code']==0 and receipt['binary_SHA256']==binary_hash
cases=json.loads(a.manifest.read_text());proofs=[]
for name in a.cases:
 case=cases[name];assert sha(case['capture'])==case['capture_SHA256'];dest=a.out/name
 query='fields pdu.id == '+str(case['pdu']);argv=[str(a.helper),str(dest),str(binary),case['capture'],case['keys'],query,case['field'],'--ignore-embedded-secrets']
 env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL=case['protocol'],SHARK_QA_EXPECT_QUERY=query,SHARK_QA_EXPECT_PACKETS=str(case['packets']),SHARK_QA_EXPECT_ADAPTIVE='1')
 started=time.monotonic();run=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,creationflags=0x08000000,timeout=120)
 (a.out/(name+'.stdout')).write_bytes(run.stdout);(a.out/(name+'.stderr')).write_bytes(run.stderr)
 proof=dict(source=receipt['source'],binary_SHA256=binary_hash,build_receipt_SHA256=sha(receipt_path),helper_SHA256=sha(a.helper),renderer_SHA256=sha(a.renderer),capture_SHA256=case['capture_SHA256'],manifest_SHA256=sha(a.manifest),driver_SHA256=sha(Path(__file__)),arguments=argv,expected=case,exit_code=run.returncode,seconds=round(time.monotonic()-started,3),snapshots={},visual_review_pending=True)
 for cells in dest.glob('*.json'):
  state=json.loads(cells.read_text(encoding='utf8'))
  if 'Rows'in state:
   assert len(state['Rows'])==len(state['Attributes'])==state['Height']and all(len(row)==len(attr)==state['Width']for row,attr in zip(state['Rows'],state['Attributes']))
   subprocess.run([sys.executable,str(a.renderer),str(cells),str(cells.with_suffix('.png'))],check=True,creationflags=0x08000000)
   proof['snapshots'][cells.stem]=dict(cells_SHA256=sha(cells),PNG_SHA256=sha(cells.with_suffix('.png')))
 if (dest/'restored.json').exists():
  proof['restored']=json.loads((dest/'restored.json').read_text());assert proof['restored']==dict(error='<nil>',input_cp=936,output_cp=936)
 path=a.out/(name+'-proof.json');path.write_text(json.dumps(proof,indent=2,sort_keys=True)+'\n',encoding='utf8');assert run.returncode==0,(name,(run.stdout+run.stderr).decode('utf8',errors='replace'))
 assert len(proof['snapshots'])==15;proofs.append(dict(case=name,path=str(path),SHA256=sha(path)))
 print(json.dumps(dict(case=name,exit_code=run.returncode,snapshots=len(proof['snapshots']),restored=proof['restored'],seconds=proof['seconds'])),flush=True)
summary=dict(source=a.source,actual_cases=len(proofs),actual_console_buffers=15*len(proofs),driver_SHA256=sha(Path(__file__)),case_proofs=proofs,visual_review_pending=True)
(a.out/'runtime-summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(a.out/'runtime-summary.json'),SHA256=sha(a.out/'runtime-summary.json'))),flush=True)
