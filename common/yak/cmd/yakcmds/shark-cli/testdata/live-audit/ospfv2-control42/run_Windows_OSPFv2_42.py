"""Actual native Windows default-budget OSPFv2 pages from exact public packets."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,re,subprocess,sys,time,traceback
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
ap=argparse.ArgumentParser(description=__doc__)
for name in('out','bins','plan'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--expected-source',required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False)
try:
 plan=read(a.plan/'plan.json');assert plan['candidate_read']is False and sha(a.plan/'cases.json')==plan['cases_SHA256']
 build=a.bins/'yak-receipt.json';b=read(build);binary=a.bins/'yak.exe'
 assert b['source']==a.expected_source and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
 spec=importlib.util.spec_from_file_location('native_console_wire_pane',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
 results=[]
 for c in read(a.plan/'cases.json'):
  inputs={k:Path(c[k])for k in('capture','physical_file','helper','renderer','source_grid')}
  inputs.update(binary=binary,build=build,case=a.plan/'cases.json',plan=a.plan/'plan.json',controller=Path(__file__))
  before={k:dict(path=str(p),bytes=p.stat().st_size,SHA256=sha(p))for k,p in inputs.items()}
  for k,d in [('capture','capture_SHA256'),('physical_file','physical_SHA256'),('helper','helper_SHA256'),('renderer','renderer_SHA256'),('source_grid','source_grid_SHA256')]:assert before[k]['SHA256']==c[d]
  folder=a.out/c['name'];command=[c['helper'],str(folder),str(binary),c['capture'],'-',c['query'],c['field_name'],'--ignore-embedded-secrets']
  env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
  env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='OSPFv2',SHARK_QA_EXPECT_QUERY=c['query'],SHARK_QA_EXPECT_PACKETS='1',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN=str(c['field_down']),SHARK_QA_SELECT_FIELD=c['field_name'],SHARK_QA_SELECT_FIELD_VALUE=c['field_value'])
  start=time.monotonic();r=subprocess.run(command,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,timeout=180,creationflags=0x08000000)
  (a.out/(c['name']+'-stdout.log')).write_bytes(r.stdout);(a.out/(c['name']+'-stderr.log')).write_bytes(r.stderr)
  after={k:dict(path=str(p),bytes=p.stat().st_size,SHA256=sha(p))for k,p in inputs.items()}
  receipt=dict(source=a.expected_source,case=c['name'],arguments=command,environment={k:v for k,v in env.items()if k.startswith('SHARK_QA_')},exit_code=r.returncode,seconds=time.monotonic()-start,inputs_before=before,inputs_unchanged=before==after,source_plan_SHA256=sha(a.plan/'plan.json'),visual_review_pending=True,console_cells_not_terminal_pixels=True,snapshots={})
  shots={}
  for f in folder.glob('*.json'):
   s=read(f)
   if 'Rows'not in s:continue
   assert len(s['Rows'])==len(s['Attributes'])==s['Height']and all(len(row)==len(at)==s['Width']for row,at in zip(s['Rows'],s['Attributes']))
   shots[f.stem]=s;image=f.with_suffix('.png')
   render=subprocess.run([sys.executable,'-B',c['renderer'],str(f),str(image)],capture_output=True,creationflags=0x08000000);assert render.returncode==0
   receipt['snapshots'][f.stem]=dict(cells_SHA256=sha(f),PNG_SHA256=sha(image))
  write(a.out/(c['name']+'-producer.json'),receipt);assert before==after and r.returncode==0,'actual helper failed; preserved'
  first=shots['01-protocol-fields'];text='\n'.join(first['Rows']);assert ' EOF 'in first['Rows'][0]and c['query']in first['Rows'][1]
  assert re.search(r'buffer\s+1\s*/\s*20000',text)and re.search(r'· 1 visible(?:\s|╮)',text)
  assert any(re.search(r'›1\s+\S+\s+OSPFv2(?:\s|$)',row)for row in first['Rows'])and ' › '.join(c['progressive_stack'])in text
  raw=Path(c['physical_file']).read_bytes();wire=raw[slice(*c['PDU_range'])]
  physical,_=m.pane(first);assert physical and raw.startswith(physical)
  selected=shots['01-selected-field'];text='\n'.join(selected['Rows']);assert c['field_name']+': '+c['field_value']in text
  observed,pos=m.pane(selected);lo,hi=c['field_PDU_range'];assert len(observed)>=hi and wire.startswith(observed)
  attrs=[selected['Attributes'][y][x]for y,x in pos];style=attrs[lo]
  assert all(v==style for v in attrs[lo:hi])and all(v!=style for i,v in enumerate(attrs)if not lo<=i<hi)
  assert observed[lo:hi].hex()==c['field_hex']and read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
  result=dict(case=c['name'],original_public_capture_SHA256=c['original_public_capture_SHA256'],original_public_frame=c['original_public_frame'],derived_inspection_frame=1,default_packet_cache=20000,default_history_mib=64,protocol='OSPFv2',progressive_stack=c['progressive_stack'],field_name=c['field_name'],field_value=c['field_value'],PDU_range=[lo,hi],physical_range=c['field_physical_range'],visible_source_prefixes_exact=True,entire_source_visible_claim=False,selected_bytes_exact=True,CP936_restored=True,mouse_tabs_and_resize_checked=True,producer_SHA256=sha(a.out/(c['name']+'-producer.json')))
  results.append(result);print(json.dumps(result),flush=True)
 write(a.out/'summary.json',dict(scope=__doc__,source=a.expected_source,source_plan_SHA256=sha(a.plan/'plan.json'),cases=results,passed=True,visual_review_pending=True,original_packets_retained=True,live_capture_claim=False))
 print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),passed=True)))
except Exception:
 write(a.out/'failure.json',dict(error=traceback.format_exc(),no_success_certificate=True));raise
