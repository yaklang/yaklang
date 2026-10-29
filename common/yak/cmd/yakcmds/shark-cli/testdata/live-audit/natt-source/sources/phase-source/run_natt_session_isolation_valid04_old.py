"""Run valid NAS-bearing isolation inputs, retaining both unmatched RADIUS EOF observations."""
from pathlib import Path
import hashlib,json,os,subprocess,time,sys
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
bins=root/'bins-31';source_inputs=root/'NATT-session-isolation-source-02/actual01'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
head=subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()
reference=source_inputs/'summary.json';assert sha(reference)=='0553ca7f5c9c89caa7d7865590160d19c25e3810c0d8e8905c4e98585b70baa2'
proofs=[]
for name,digest,expected in [
 ('A2-valid-radius-only','d48db1b8608123df52fd957cc88770821c19e5c5efc9a442b43006b30d738f33',['radius']*2),
 ('B8-valid-isolated-RRQ','656da57ea9373a651d72fb26d4dfe47cab3b3f413532bb0002fd4430e38e0bc5',['stun','coap','tftp','radius']*2)]:
 capture=source_inputs/(name+'.pcap');assert sha(capture)==digest
 folder=out/name;folder.mkdir()
 for kind in ['events','yak']:
  binary=bins/(kind+'.exe');build_path=bins/(kind+'-receipt.json');build=read(build_path)
  assert build['source']=='5c88aed7aa9ee8c7438aca30a6bfaa114fff85d9' and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
  argv=[str(binary),'replay',str(capture),'-','--ignore-embedded-secrets'] if kind=='events' else [str(binary),'shark','-r',str(capture),'--json','--ignore-embedded-secrets']
  export=folder/(kind+'.jsonl');err=folder/(kind+'.stderr');started=time.monotonic()
  with export.open('xb') as stdout,err.open('xb') as stderr:
   run=subprocess.run(argv,stdout=stdout,stderr=stderr,env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=90)
  assert run.returncode==0
  rows=[json.loads(r) for r in export.read_bytes().splitlines()];assert len(rows)==len(expected)+2
  labels=[r['protocol'] if kind=='events' or 'protocol' in r else r['published_pdus'][0]['protocol'] for r in rows]
  assert labels[:len(expected)]==(expected if kind=='events' else [n.upper() for n in expected]),labels
  if kind=='events':assert all(r['protocol']=='radius' and r['status']=='incomplete' and r['error']=='RADIUS UDP unmatched observed Identifiers' for r in rows[len(expected):])
  else:assert all(r['record_type']=='protocol_publication' and r['publication_phase']=='capture_close' and len(r['published_pdus'])==1 and r['published_pdus'][0]['protocol']=='radius' and r['published_pdus'][0]['status']=='incomplete' for r in rows[len(expected):])
  proof=dict(case=name,kind=kind,arguments=argv,binary_build_source=build['source'],binary_SHA256=sha(binary),
   build_receipt_SHA256=sha(build_path),capture_SHA256=sha(capture),reference_SHA256=sha(reference),rows=len(rows),
   labels=labels,export_SHA256=sha(export),stderr_SHA256=sha(err),exit_code=run.returncode,seconds=round(time.monotonic()-started,3))
  (folder/(kind+'-producer.json')).write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');proofs.append(proof)
assert subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()==head
summary=dict(scope=__doc__,execution_worktree_HEAD=head,producer_code_source='5c88aed7aa9ee8c7438aca30a6bfaa114fff85d9',
 controller_SHA256=sha(__file__),source_input_reference_SHA256=sha(reference),source_unchanged=True,outputs=proofs,
 no_current_candidate_output_used=True)
(out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),closed_executions=len(proofs))))
