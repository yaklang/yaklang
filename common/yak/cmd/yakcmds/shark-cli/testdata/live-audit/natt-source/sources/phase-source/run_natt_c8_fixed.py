"""Execution-only closed C8 producer: preserve all records, including capture-close publications."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time,traceback

root=Path(__file__).parent
ap=argparse.ArgumentParser()
for name in ['bins','out']:ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False)
capture=root/'NATT-session-isolation-source-02/actual01/C8-valid-original-peers.pcap'
reference=root/'NATT-session-isolation-source-02/actual01/summary.json'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def git(*args):return subprocess.check_output(['git',*args],encoding='utf8').strip()
try:
 assert sha(capture)=='80e228e228677e835960540ceae7226d7d6ede544cb3c79ac050ba0b0467bbc9'
 assert sha(reference)=='0553ca7f5c9c89caa7d7865590160d19c25e3810c0d8e8905c4e98585b70baa2'
 assert git('rev-parse','HEAD')==a.source and not git('status','--porcelain','--untracked-files=all')
 proofs=[]
 for kind in ['events','yak']:
  binary=a.bins/(kind+'.exe');build_path=a.bins/(kind+'-receipt.json');build=read(build_path)
  assert build['source']==a.source and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
  paths=[binary,build_path,capture,reference,Path(__file__)]
  before={str(p):sha(p) for p in paths}
  argv=[str(binary),'replay',str(capture),'-','--ignore-embedded-secrets'] if kind=='events' else [str(binary),'shark','-r',str(capture),'--json','--ignore-embedded-secrets']
  export=a.out/(kind+'.jsonl');stderr=a.out/(kind+'.stderr');start=time.monotonic()
  with export.open('xb') as stdout,stderr.open('xb') as err:
   run=subprocess.run(argv,stdout=stdout,stderr=err,env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=90)
  after={p:sha(p) for p in before}
  with export.open('rb') as f:rows=sum(1 for line in f if line.strip())
  proof=dict(kind=kind,arguments=argv,binary_build_source=build['source'],build_receipt_SHA256=sha(build_path),
   binary_SHA256=sha(binary),capture_SHA256=sha(capture),reference_SHA256=sha(reference),rows=rows,
   exit_code=run.returncode,seconds=round(time.monotonic()-start,3),export_SHA256=sha(export),stderr_SHA256=sha(stderr),
   source_before=before,source_after=after,sources_byte_equal=before==after,controller_SHA256=sha(__file__))
  (a.out/(kind+'-producer.json')).write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
  assert before==after and run.returncode==0
  assert git('rev-parse','HEAD')==a.source and not git('status','--porcelain','--untracked-files=all')
  proofs.append(proof)
 summary=dict(scope=__doc__,source=a.source,executions=proofs,physical_capture_packets=8,
  output_not_filtered_repaired_or_normalized=True,no_runtime_output_used_to_create_expectations=True)
 (a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8')
 print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),closed_executions=len(proofs),rows=[p['rows'] for p in proofs])))
except Exception:
 (a.out/'failure.json').write_text(json.dumps(dict(failure=traceback.format_exc(),no_success_certificate=True),indent=2)+'\n',encoding='utf8')
 raise
