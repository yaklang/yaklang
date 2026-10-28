"""Whole CLI rows for byte-proved ICMPv6 diagnostic coverage, without field waivers."""
from pathlib import Path
import argparse, collections, copy, hashlib, json, sys
from icmpv6_expected import expected_failure,same

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()

def projection(e):
 values=e['field_values'];fields=collections.defaultdict(list)
 for v in values['values']:fields[v['name']].append(v['value'])
 return dict(id=e['id'],protocol=e['protocol'],status=e['status'],completeness=e['completeness'],response_to=e['response_to'],byte_source=values['source'],fields=dict(fields),field_values=values,summary=e['summary'],error=e['error'],expert=e['expert'])

def expected_row(old,event):
 assert old['protocol']=='ICMPv6'and old['protocol_status']==event['status']
 assert old['source']==event['source']and old['destination']==event['destination']
 assert same(old['pdus'],[projection(event)])
 assert old['info'].startswith(event['summary'])and old['info'].count(' [incomplete/undecoded]')==1
 out=copy.deepcopy(old);out['info']=out['info'].replace(' [incomplete/undecoded]','',1)
 return out

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for name in['capture','SDK-proof','diff','audit-modules','out']:ap.add_argument('--'+name,type=Path,required=True)
 a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(1,str(a.audit_modules.resolve()))
 from vxlan_endpoint_projection_audit import capture_records
 proof=json.loads((a.SDK_proof/'summary.json').read_text());ledger=a.SDK_proof/'source63-expected.jsonl'
 assert proof['capture_sharon_SHA256']==sha(a.capture)and proof['expected_source_ledger_SHA256']==sha(ledger)
 assert proof['expected_constructor_SHA256']==sha(Path(__file__).with_name('icmpv6_expected.py'))and proof['literal_parser_SHA256']==sha(Path(__file__).with_name('literal_own_header.py'))
 source={v['frame']:v for v in map(json.loads,ledger.open(encoding='utf8'))};assert len(source)==63
 expected={}
 for number,record in enumerate(capture_records(a.capture),1):
  if number not in source:continue
  s=source[number];record['frame']=number
  assert hashlib.sha256(record['wire']).hexdigest()==s['wire_SHA256']
  e=expected_failure(s['old_event']['id'],record);assert same(e,s['expected_new_event'])
  if 133<=e['session']['Type']<=137:expected[number]=e
 assert len(expected)==60
 full=json.loads((a.diff/'summary.json').read_text())
 assert len(full)==37 and {v['case']for v in full if v['changed']}=={'sharon'}and sum(v['changed']for v in full)==60
 observed=set();positives=[];path=a.diff/'sharon-changed.jsonl'
 for line in path.open(encoding='utf8'):
  v=json.loads(line);number=v['frame'];assert number in expected and number not in observed
  old,new=v['old'],v['new'];e=expected[number]
  assert number==old['number']==new['number']
  want=expected_row(old,e);assert same(want,new),(number,'whole expected row mismatch')
  observed.add(number);positives.append((old,new,e))
 assert observed==expected.keys()
 controls=[];old,new,e=positives[0]
 for name in['decoded-status','source','native-PDU','dropped-source','protocol-stack','type']:
  forged=copy.deepcopy(new)
  if name=='decoded-status':forged['protocol_status']='decoded'
  elif name=='source':forged['source']='203.0.113.254'
  elif name=='native-PDU':forged['pdus']=[{'id':True}]
  elif name=='dropped-source':forged.pop('capture_domain',None)
  elif name=='protocol-stack':forged['protocol_stack']=forged['protocol_stack'][:-1]
  else:forged['number']=str(forged['number'])
  assert not same(expected_row(old,e),forged);controls.append(name)
 summary=dict(scope=__doc__,SDK_source=proof['source_after'],CLI_before='49b0a99102357d22ee6e6c935a2a3a8358503f7c',CLI_after='4c023fd47c2b940b509d08220a4576f525af33e6',complete_generations=37,whole_rows=sum(v['records']for v in full),whole_source_expected_changes=60,all_other_rows_whole_literal=True,all_native_field_projections_whole_literal=True,capture_SHA256=sha(a.capture),SDK_proof_SHA256=sha(a.SDK_proof/'summary.json'),SDK_source_ledger_SHA256=sha(ledger),CLI_diff_SHA256=sha(a.diff/'summary.json'),complete_changed_ledger_SHA256=sha(path),negative_controls=controls,auditor_SHA256=sha(Path(__file__)))
 (a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');print(json.dumps(summary),flush=True)

if __name__=='__main__':main()
