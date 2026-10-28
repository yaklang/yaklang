"""Certify complete CLI generations against whole original-source ICMPv4 objects."""
import argparse,collections,copy,hashlib,itertools,json,sys
from pathlib import Path
from icmp4_expected_source import expected,same,typecode

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()

def projection(e):
 fields=collections.defaultdict(list)
 for v in e['field_values']['values']:fields[v['name']].append(v['value'])
 out=dict(id=e['id'],protocol=e['protocol'],status=e['status'],completeness=e['completeness'],response_to=e['response_to'],byte_source=e['field_values']['source'],fields=dict(fields),field_values=e['field_values'])
 if e.get('error')or e.get('expert'):
  out.update(summary=e['summary'],error=e.get('error',''),expert=e.get('expert',''))
 return out

def expected_row(old,before,after):
 assert old['protocol']=='ICMP'and old['protocol_status']=='decoded'
 assert old['source']==before['source']and old['destination']==before['destination']
 assert same(old['pdus'],[projection(before)])
 assert old['info']=='ICMP '+typecode(before['session']['Type'],before['session']['Code'])
 out=copy.deepcopy(old)
 out.update(pdus=[projection(after)],info=after['summary'],protocol_status='incomplete',visibility='partial-message',protocol_scope='observed ICMP header fields; complete message not admitted',protocol_evidence='ICMP generic header observed; body or options incomplete, invalid or limited')
 return out

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for name in('before','after','expected','SDK-bridge','capture','audit-modules','out'):ap.add_argument('--'+name,type=Path,required=True)
 ap.add_argument('--CLI-before',required=True)
 a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(1,str(a.audit_modules))
 from vxlan_endpoint_projection_audit import capture_records
 meta=json.loads((a.expected/'summary.json').read_text());bridge=json.loads((a.SDK_bridge/'summary.json').read_text())
 assert bridge['whole_source_expected_changes']==475 and bridge['old_source_objects_verified']==5009
 assert bridge['expected_source_proof_SHA256']==sha(a.expected/'summary.json')
 assert meta['source_constructor_SHA256']==sha(Path(__file__).with_name('icmp4_expected_source.py'))and meta['literal_parser_SHA256']==sha(Path(__file__).with_name('literal_own_header.py'))
 part=next(v for v in meta['corpora']if v['corpus']=='sharon');ledger=a.expected/'sharon'/'expected.jsonl'
 assert part['capture_SHA256']==sha(a.capture)and part['expected_ledger_SHA256']==sha(ledger)
 chosen={v['frame']:v for v in map(json.loads,ledger.open())if v['expected_new_event']['status']=='incomplete'}
 assert len(chosen)==20
 source={}
 for frame,r in enumerate(capture_records(a.capture),1):
  if frame not in chosen:continue
  r['frame']=frame;row=chosen[frame]
  assert hashlib.sha256(r['wire']).hexdigest()==row['record']['wire_SHA256']
  before=expected(row['id'],r,old=True);after=expected(row['id'],r)
  assert same(before,row['old_event'])and same(after,row['expected_new_event'])
  source[frame]=(before,after)
 assert source.keys()==chosen.keys()
 files=[];observed=set();positives=[];cases=sorted(a.before.glob('*/*/yak-producer.json'));assert len(cases)==37
 for producer in cases:
  other=a.after/producer.relative_to(a.before);om=json.loads(producer.read_text());nm=json.loads(other.read_text());old=producer.with_name('yak.jsonl');new=other.with_name('yak.jsonl')
  assert om['source']==a.CLI_before and nm['source']==bridge['source_after']and om['exit_code']==nm['exit_code']==0
  assert om['capture_SHA256']==nm['capture_SHA256']and om['keylog_used']==nm['keylog_used']and om['arguments'][1:]==nm['arguments'][1:]
  assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)
  count=literal=changed=0
  with old.open('rb')as left,new.open('rb')as right:
   for x,y in itertools.zip_longest(left,right):
    assert x is not None and y is not None;count+=1
    if x==y:literal+=1;continue
    oldrow,newrow=json.loads(x),json.loads(y);number=oldrow['number']
    assert producer.parent.name=='sharon'and number in source and number not in observed
    before,after=source[number];want=expected_row(oldrow,before,after)
    assert same(want,newrow),(number,{k:[want.get(k),newrow.get(k)]for k in want.keys()|newrow.keys()if not same(want.get(k),newrow.get(k))})
    observed.add(number);changed+=1;positives.append((oldrow,newrow,before,after))
  files.append(dict(case=producer.parent.name,rows=count,literal_rows=literal,whole_source_expected_changes=changed,before_SHA256=sha(old),after_SHA256=sha(new),before_producer_SHA256=sha(producer),after_producer_SHA256=sha(other)))
 assert observed==source.keys()
 old,new,before,after=positives[0];negative=[]
 for name,mutate in[
  ('decoded-status',lambda r:r.__setitem__('protocol_status','decoded')),
  ('complete-message-visibility',lambda r:r.__setitem__('visibility','captured')),
  ('invented-completion',lambda r:r['pdus'][0]['byte_source'].__setitem__('CompletedIn',copy.deepcopy(r['pdus'][0]['byte_source']['PacketRefs'][0]))),
  ('wrong-PDU-ID',lambda r:r['pdus'][0].__setitem__('id',r['pdus'][0]['id']+1)),
  ('foreign-domain',lambda r:r['capture_domain'].__setitem__('Interface',0)),
  ('dropped-native-fields',lambda r:r['pdus'][0].__setitem__('fields',{})),
  ('invented-source',lambda r:r.__setitem__('source','203.0.113.254')),
  ('timestamp-coerced-to-text',lambda r:r.__setitem__('number',str(r['number']))),
  ('generic-error-noise',lambda r:r.__setitem__('info',r['info']+' [incomplete/undecoded]'))]:
  forged=copy.deepcopy(new);mutate(forged);assert not same(expected_row(old,before,after),forged);negative.append(name)
 summary=dict(scope=__doc__,source_before=a.CLI_before,SDK_source_before=meta['source_before'],source_after=bridge['source_after'],generations=len(files),total_rows=sum(v['rows']for v in files),literal_rows=sum(v['literal_rows']for v in files),whole_source_expected_changes=sum(v['whole_source_expected_changes']for v in files),source_frames=sorted(source),negative_controls=negative,source_certificate_SHA256=sha(a.expected/'summary.json'),SDK_bridge_SHA256=sha(a.SDK_bridge/'summary.json'),capture_SHA256=sha(a.capture),constructor_SHA256=sha(Path(__file__).with_name('icmp4_expected_source.py')),auditor_SHA256=sha(Path(__file__)),files=files)
 (a.out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
 print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),counts={k:summary[k]for k in('generations','total_rows','literal_rows','whole_source_expected_changes')})),flush=True)

if __name__=='__main__':main()
