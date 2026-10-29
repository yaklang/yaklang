"""Inspect the source-frozen unknown-method reply in actual Windows console cells."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,subprocess,time,traceback
root=Path(__file__).parent
ap=argparse.ArgumentParser(description=__doc__)
for key in('bins','out'):ap.add_argument('--'+key,type=Path,required=True)
ap.add_argument('--source',required=True);ap.add_argument('--down',type=int,default=65);a=ap.parse_args()
assert not a.out.exists();a.out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
plan=root/'SSH-SDK-CLI-source49-01/plan01/windows-unknown-reply-source-plan.json'
assert sha(plan)=='925d66e3fe6d7b01795c9bfe0f08c8e424c9648a209420667a8a0900328e9090'
c=read(plan);capture=Path(c['capture']);raw=Path(c['whole_PDU']['path']).read_bytes()
assert sha(capture)==c['capture_SHA256']and hashlib.sha256(raw).hexdigest()==c['whole_PDU']['SHA256']and len(raw)==32
lo,hi=c['opaque_field']['independent_source_span'];field=raw[lo:hi]
assert len(field)==18 and hashlib.sha256(field).hexdigest()==c['opaque_field']['SHA256']
binary=a.bins/'yak.exe';receipt=a.bins/'yak-receipt.json';b=read(receipt)
assert b['source']==a.source and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
helper=root/'Windows-console-ack-build45-01/native-console.exe'
renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
assert sha(helper)=='12e0aa1b8b3cfb4696e1d680334dae490e269de37773e01aafa35bc4a6d8b68c'
assert sha(renderer)=='427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83'
inputs={str(p):sha(p)for p in(plan,capture,Path(c['whole_PDU']['path']),binary,receipt,helper,renderer,Path(__file__))}
folder=a.out/'unknown-method-reply'
args=[str(helper),str(folder),str(binary),str(capture),'-',c['filter'],'Opaque Message Data','--ignore-embedded-secrets']
env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='SSH',SHARK_QA_EXPECT_QUERY=c['filter'],SHARK_QA_EXPECT_PACKETS='148',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN=str(a.down),SHARK_QA_BYTE_FIELD='Opaque Message Data',SHARK_QA_BYTE_HEX=field.hex())
# No SHARK_QA_BYTE_RANGE: the existing API has no range for this extracted
# value. Source equality cannot manufacture a captured-frame offset.
started=time.monotonic()
try:
 p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,timeout=180,creationflags=0x08000000)
 (a.out/'stdout').write_bytes(p.stdout);(a.out/'stderr').write_bytes(p.stderr)
 proof=dict(source=a.source,arguments=args,source_plan_SHA256=sha(plan),environment={k:v for k,v in env.items()if k.startswith('SHARK_QA_')},inputs=inputs,exit_code=p.returncode,seconds=time.monotonic()-started,inputs_unchanged=inputs=={p:sha(p)for p in inputs},console_cells_not_terminal_pixels=True,visual_review_pending=True,snapshots={})
 shots={}
 for path in folder.glob('*.json'):
  value=read(path)
  if 'Rows'not in value:continue
  assert len(value['Rows'])==len(value['Attributes'])==value['Height']and all(len(x)==len(y)==value['Width']for x,y in zip(value['Rows'],value['Attributes']))
  image=path.with_suffix('.png');r=subprocess.run([os.sys.executable,'-B',str(renderer),str(path),str(image)],capture_output=True,creationflags=0x08000000);assert r.returncode==0
  shots[path.stem]=value;proof['snapshots'][path.stem]=dict(cells_SHA256=sha(path),PNG_SHA256=sha(image))
 put(a.out/'producer.json',proof)
 assert p.returncode==0 and proof['inputs_unchanged'],'actual helper failed; preserved'
 selected=shots['01-selected-byte-field'];text='\n'.join(selected['Rows'])
 assert '3 Extracted field value'in text and'frame offsets unavailable'in text
 for forbidden in c['forbidden_labels']:assert forbidden not in text,forbidden
 assert c['expected_literal_method']in text and c['expected_packet_name']in text
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 put(a.out/'summary.json',dict(source=a.source,source_plan_SHA256=sha(plan),passed=True,source_frame=69,source_complete_PDU_bytes=32,actual_complete_PDU_check_pending=True,actual_extracted_value_bytes=18,actual_extracted_value_equals_source=True,no_invented_frame_offsets=True,CP936_restored=True,default_cache_packets=20000,default_history_mib=64,mouse_tabs_resize_checked=True,root_visual_review_pending=True,producer_SHA256=sha(a.out/'producer.json')))
 print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),passed=True)))
except Exception:
 put(a.out/'failure.json',dict(error=traceback.format_exc(),no_success_certificate=True));raise
