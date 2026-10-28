"""Freeze source-built before/after entire ICMPv4 objects for every old native ICMP observation."""
import argparse,base64,collections,hashlib,json,sys
from pathlib import Path
ap=argparse.ArgumentParser()
for n in('native','capture-weber','capture-sharon','audit-modules','bins','out'):ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(1,str(a.audit_modules))
from vxlan_endpoint_projection_audit import capture_records
from icmp4_expected_source import source,fields,expected,same

def sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb')as f:
  for block in iter(lambda:f.read(1<<20),b''):h.update(block)
 return h.hexdigest()
receipt=json.loads((a.bins/'events-receipt.json').read_text());assert receipt['source']==a.source and receipt['binary_SHA256']==sha(a.bins/'events.exe')and receipt['exit_code']==0
summary=[]
for name,cap in[('weber',a.capture_weber),('sharon',a.capture_sharon)]:
 folder=a.out/name;folder.mkdir();sdk=a.native/'preservation'/name/'events.jsonl';producer=sdk.with_name('events-producer.json');meta=json.loads(producer.read_text());assert meta['source']==a.source and meta['export_SHA256']==sha(sdk)and meta['capture_SHA256']==sha(cap)and meta['binary_SHA256']==receipt['binary_SHA256']and meta['exit_code']==0
 groups=collections.defaultdict(list);total=0
 for line in sdk.open(encoding='utf8'):
  e=json.loads(line)
  if e['protocol']!='icmp':continue
  refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1;groups[refs[0]['Number']].append(e);total+=1
 rows=[];failures=[]
 for frame,r in enumerate(capture_records(cap),1):
  if frame not in groups:continue
  r['frame']=frame
  for old in groups[frame]:
   try:
    loc=source(r);before=expected(old['id'],r,old=True);assert same(before,old),{'whole_old_event_mismatch':[k for k in before.keys()|old.keys()if not same(before.get(k),old.get(k))]};after=expected(old['id'],r)
    source_facts={k:v for k,v in r.items()if k!='wire'};source_facts['wire_hex']=r['wire'].hex();source_facts['wire_SHA256']=hashlib.sha256(r['wire']).hexdigest();fields_map,required,status=fields(r,loc)
    rows.append(dict(frame=frame,id=old['id'],record=source_facts,source_profile=loc,old_event=before,expected_new_event=after,intentional_change=not same(before,after),own_required=required,own_available=loc['end']-loc['offset'],whole_old_object_verified=True))
   except Exception as exc:failures.append(dict(frame=frame,id=old['id'],error=repr(exc),old_event=old,record={k:(v.hex()if isinstance(v,bytes)else v)for k,v in r.items()}))
 for file,data in[('expected.jsonl',rows),('uncovered.jsonl',failures)]:
  (folder/file).write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in data),encoding='utf8')
 item=dict(corpus=name,selected_all_ICMP_events=total,whole_before_source_verified=len(rows),uncovered=len(failures),changed=sum(x['intentional_change']for x in rows),unchanged=sum(not x['intentional_change']for x in rows),new_statuses=dict(collections.Counter(x['expected_new_event']['status']for x in rows)),new_incomplete=[dict(frame=x['frame'],id=x['id'],type=x['expected_new_event']['session']['Type'],code=x['expected_new_event']['session']['Code'],available=x['own_available'],required=x['own_required'],scope=x['source_profile']['scope'])for x in rows if x['expected_new_event']['status']=='incomplete'],source_scopes=dict(collections.Counter(x['source_profile']['scope']for x in rows)),raw_slices=dict(collections.Counter('inner-IP literal extraction'if x['source_profile']['raw_start']else'whole original record'for x in rows)),expected_ledger_SHA256=sha(folder/'expected.jsonl'),uncovered_SHA256=sha(folder/'uncovered.jsonl'),capture_SHA256=sha(cap),SDK_SHA256=sha(sdk),producer_SHA256=sha(producer));summary.append(item);print(json.dumps({k:v for k,v in item.items()if k!='new_incomplete'}),flush=True)
result=dict(scope='Whole-object source expectations before next ICMPv4 change; all old protocol=icmp observations are dynamically enumerated. New expected objects are plans, not a new-binary pass.',source_before=a.source,source_constructor_SHA256=sha(Path(__file__).with_name('icmp4_expected_source.py')),auditor_SHA256=sha(Path(__file__)),literal_parser_SHA256=sha(Path(__file__).with_name('literal_own_header.py')),corpora=summary)
target=a.out/'summary.json';target.write_text(json.dumps(result,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(target),SHA256=sha(target))),flush=True)
assert not any(x['uncovered']for x in summary)
