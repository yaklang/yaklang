"""Run source-frozen Windows OSPF page, preserving all failures and input hashes."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,subprocess,sys,time,traceback
ROOT=Path(__file__).parent
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir()
 try:
  planroot=ROOT/'Windows-OSPFv3-boundary-source-plan38-01';plan=read(planroot/'plan.json');case=read(planroot/'case.json')
  assert sha(planroot/'case.json')==plan['source_case_SHA256']
  for row in plan['files'].values():assert sha(row['path'])==row['SHA256']
  build=ROOT/'bins-38/yak-receipt.json';producer=read(build);binary=build.with_name('yak.exe')
  assert producer['source']==case['source'] and producer['exit_code']==0 and sha(binary)==producer['binary_SHA256']
  inputs={name:Path(case[name])for name in['capture','helper','helper_receipt','renderer','physical_file','field_plan','source_result_closure']}
  inputs.update(binary=binary,build=build,source_case=planroot/'case.json',source_plan=planroot/'plan.json')
  inputs.update({name:Path(row['path'])for name,row in plan['files'].items()})
  before={name:dict(path=str(p),bytes=p.stat().st_size,SHA256=sha(p))for name,p in inputs.items()}
  for name,key in[('capture','capture_SHA256'),('helper','helper_SHA256'),('helper_receipt','helper_receipt_SHA256'),('renderer','renderer_SHA256'),('physical_file','physical_SHA256'),('field_plan','field_plan_SHA256'),('source_result_closure','source_result_closure_SHA256')]:assert before[name]['SHA256']==case[key]
  write(a.out/'inputs-before.json',before)
  folder=a.out/case['name'];command=[case['helper'],str(folder),str(binary),case['capture'],'-',case['query'],case['field'],'--ignore-embedded-secrets']
  env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
  env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='OSPFv3',SHARK_QA_EXPECT_QUERY=case['query'],SHARK_QA_EXPECT_PACKETS='1',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN=str(case['field_down']),SHARK_QA_SELECT_FIELD=case['field_name'],SHARK_QA_SELECT_FIELD_VALUE=case['field_value'])
  start=time.monotonic();run=subprocess.run(command,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,creationflags=0x08000000,timeout=180)
  (a.out/'stdout.log').write_bytes(run.stdout);(a.out/'stderr.log').write_bytes(run.stderr)
  after={name:dict(path=str(p),bytes=p.stat().st_size,SHA256=sha(p))for name,p in inputs.items()};write(a.out/'inputs-after.json',after)
  proof=dict(source=case['source'],arguments=command,environment={k:v for k,v in env.items()if k.startswith('SHARK_QA_')},exit_code=run.returncode,seconds=time.monotonic()-start,source_bytes_equal=before==after,default_history_mib=64,default_packet_cache=20000,budget_increase=False,visual_review_pending=True,source_plan_SHA256=sha(planroot/'plan.json'),snapshots={})
  for cells in sorted(folder.glob('*.json')):
   if 'Rows'not in read(cells):continue
   target=cells.with_suffix('.png');render=subprocess.run([sys.executable,'-B',case['renderer'],str(cells),str(target)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000);assert render.returncode==0
   proof['snapshots'][cells.stem]=dict(cells_SHA256=sha(cells),PNG_SHA256=sha(target))
  write(a.out/'proof.json',proof);assert before==after;assert run.returncode==0,'actual helper failed, all outputs preserved'
  path=ROOT/'verify_Windows_OSPFv3_boundary38.py';spec=importlib.util.spec_from_file_location('ospfv3_console_source',path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  proof['source_verification']=m.verify(folder,case);proof['passed']=True;write(a.out/'proof.json',proof)
  print(json.dumps(dict(proof=str(a.out/'proof.json'),SHA256=sha(a.out/'proof.json'),passed=True)))
 except Exception:
  write(a.out/'failure.json',dict(error=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
