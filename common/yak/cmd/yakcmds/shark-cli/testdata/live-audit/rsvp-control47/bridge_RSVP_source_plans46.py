"""Bridge two frozen external RSVP source plans without candidate output."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

def dg(raw):return hashlib.sha256(raw).hexdigest()
def sha(path):return dg(Path(path).read_bytes())
def read(path):return json.loads(Path(path).read_text(encoding='utf-8-sig'))
def literal_bytes(obj):return json.dumps(obj,ensure_ascii=False,indent=2).encode('utf-8')
def put(path,obj):Path(path).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

def run(old,new,out):
 assert not out.exists();out.mkdir()
 oldrows=read(old/'source-cases.json');newrows=read(new/'source-cases.json')
 oldinput=(old/'probe-input.jsonl').read_bytes();newinput=(new/'probe-input.jsonl').read_bytes()
 assert oldinput==newinput
 ol=oldinput.splitlines();nl=newinput.splitlines()
 assert len(oldrows)==len(newrows)==len(ol)==len(nl)==34
 pairs=[];changed=[]
 for a,b,ia,ib in zip(oldrows,newrows,ol,nl):
  assert a['sourcecase']==b['sourcecase']==json.loads(ia)['sourcecase']==json.loads(ib)['sourcecase']
  assert ia==ib
  af=literal_bytes(a['fields']);bf=literal_bytes(b['fields']);al=literal_bytes(a['leaves']);bl=literal_bytes(b['leaves'])
  assert af==bf and al==bl
  if a!=b:
   assert a['sourcecase']=='RSVP-authored-scene04-frame04'
   assert a['required_categories']=={'is_format':False,'is_integrity':False,'is_budget':False,'is_context':False}
   assert b['required_categories']=={'is_format':True,'is_integrity':False,'is_budget':False,'is_context':False}
   check=copy.deepcopy(b);check['required_categories']=a['required_categories'];check['source_issue_facts']=a['source_issue_facts']
   assert check==a
   assert len(b['source_issue_facts'])==len(a['source_issue_facts'])+1
   changed.append(a['sourcecase'])
  pairs.append({'sourcecase':a['sourcecase'],'old_new_fields_literal_bytes_equal':True,'old_fields_SHA256':dg(af),'new_fields_SHA256':dg(bf),'old_new_leaf_arrays_literal_bytes_equal':True,'old_leaf_array_SHA256':dg(al),'new_leaf_array_SHA256':dg(bl),'old_new_input_record_bytes_equal':True,'old_input_record_SHA256':dg(ia),'new_input_record_SHA256':dg(ib),'literal_leaves':sum(x['literal']for x in a['leaves']),'derived_values':sum(not x['literal']for x in a['leaves'])})
 assert changed==['RSVP-authored-scene04-frame04']
 inputs={str(path):sha(path)for path in [Path(__file__),old/'manifest.json',new/'manifest.json',old/'source-cases.json',new/'source-cases.json',old/'probe-input.jsonl',new/'probe-input.jsonl']}
 put(out/'bridge.json',{'inputs':inputs,'pairs':pairs,'old_new_full_probe_input_bytes_equal':True,'old_probe_input_SHA256':dg(oldinput),'new_probe_input_SHA256':dg(newinput),'only_error_policy_delta':changed,'no_runtime_or_candidate_output_read':True})
 assert inputs=={path:sha(path)for path in inputs}
 put(out/'summary.json',{'inputs':inputs,'bridge_SHA256':sha(out/'bridge.json'),'cases':34,'literal_leaves_unchanged':sum(x['literal_leaves']for x in pairs),'derived_values_unchanged':sum(x['derived_values']for x in pairs),'all_34_field_trees_leaf_arrays_and_input_records_literal_equal':True,'only_error_policy_delta':changed})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'bridge_SHA256':sha(out/'bridge.json')}))

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--old',type=Path,required=True);parser.add_argument('--new',type=Path,required=True);parser.add_argument('--out',type=Path,required=True)
 args=parser.parse_args();run(args.old,args.new,args.out)
if __name__=='__main__':main()