"""Freeze whole prefix-retention outputs from old literal sources, before replay.
The preceding extent/Scope expected objects are the baseline. Complete/unchanged
objects are held literal; failing selected readers retain only complete source
words and explicit nil/list shape. No candidate native24 output is read.
"""
import argparse,copy,hashlib,json,sys,collections
from pathlib import Path

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def literal(v):return json.dumps(v,ensure_ascii=False,sort_keys=True,separators=(',',':'))
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('scope-plan','bindings','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(Path(__file__).parent));from pim_prefix_tree_expected02 import whole_event,projection,tree;from pim_literal_prefix02 import decode;from pim_literal_expected import checksum
 bindings=a.bindings.read_text(encoding='utf8');cases={};target_total=0
 for case in('weber','sharon'):
  plans=[];changes=[];added_values=0;removed_values=0
  for l in(a.scope_plan/(case+'-all-PIM-whole-plan.jsonl')).open(encoding='utf8'):
   x=json.loads(l);old=x['whole_next_extent_scope_expected'];new,facts=whole_event(x['source'],old,bindings);item=dict(frame=x['frame'],id=x['id'],source=x['source'],baseline_extent_scope_expected=old,whole_prefix_expected=new,literal_prefix02_facts=facts);plans.append(item)
   if literal(old)!=literal(new):
    changes.append(item);target_total+=1
    before=collections.Counter(literal(v)for v in old['field_values']['values']);after=collections.Counter(literal(v)for v in new['field_values']['values']);added_values+=sum((after-before).values());removed_values+=sum((before-after).values())
  for kind,rows in[('all',plans),('changed',changes)]:
   fp=a.out/(case+'-'+kind+'-prefix-whole-plan.jsonl');fp.write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in rows),encoding='utf8')
  cases[case]=dict(events=len(plans),source_whole_expected_changes=len(changes),whole_literal=len(plans)-len(changes),typed_values_added_or_range_changed=added_values,typed_values_removed_or_range_changed=removed_values,all_plan_SHA256=sha(a.out/(case+'-all-prefix-whole-plan.jsonl')),changed_plan_SHA256=sha(a.out/(case+'-changed-prefix-whole-plan.jsonl')))
 # Source-only semantic controls exercise actual truncated word/option borders.
 controls=[]
 def one(name,raw,required,absent):
  w=bytearray.fromhex(raw);w[2:4]=bytes(2);w[2:4]=checksum(w,'192.0.2.1','224.0.0.13',len(w)).to_bytes(2,'big');f=decode(w,'192.0.2.1','224.0.0.13',len(w));available={x['alias']:x['value']for x in f['fields']if x['alias']}
  for k,v in required.items():assert type(available.get(k))is type(v)and available[k]==v,(name,k,available)
  for k in absent:assert k not in available,(name,k,available)
  controls.append(dict(name=name,wire_hex=w.hex(),required=required,absent=absent,facts=f))
 one('AF complete encoding missing','2200000001',{'pim.addr_address_family':1},['pim.addr_encoding_type','pim.group'])
 one('flags complete mask missing','22000000010081',{'pim.group_addr.flags.b':True,'pim.group_addr.flags.z':True},['pim.mask_len','pim.group'])
 one('unsupported encoding retains AF encoding and no address','2200000001010000ffffffff',{'pim.addr_address_family':1,'pim.addr_encoding_type':1},['pim.mask_len','pim.group'])
 one('Hello TLV crosses capture but complete propagation word exists','20000000000200048009',{'pim.t':True,'pim.propagation_delay':9},['pim.override_interval'])
 one('Hello TLV complete declared short body retains propagation word','20000000000200028009',{'pim.t':True,'pim.propagation_delay':9},['pim.override_interval'])
 one('Hello partial unknown value is not a whole value','20000000123400050102',{'pim.optiontype':0x1234,'pim.optionlength':5},[])
 one('short State Refresh option keeps its two full octets','20000000001500020102',{'pim.state_refresh_version':1,'pim.state_refresh_interval':2},['pim.state_refresh_reserved'])
 one('zero-byte attempted address never creates a record','22000000',{},['pim.addr_address_family','pim.addr_encoding_type'])
 unknown=controls[-3]['facts'];assert not any(x['path'][-1]=='Unknown Option Value'for x in unknown['fields'])
 (a.out/'source-controls.json').write_text(json.dumps(controls,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
 result=dict(scope=__doc__,source_constructor_SHA256=sha(Path(__file__).with_name('pim_literal_prefix02.py')),whole_constructor_SHA256=sha(Path(__file__).with_name('pim_prefix_tree_expected02.py')),driver_SHA256=sha(Path(__file__)),prior_extent_source_plan_SHA256=sha(a.scope_plan/'summary.json'),unchanged_binding_API_SHA256=sha(a.bindings),cases=cases,whole_source_expected_targets=target_total,source_controls=len(controls),controls_SHA256=sha(a.out/'source-controls.json'),note='The old source v01 and every pre-fix failure remain preserved. v02 additionally retains flag-derived values when Mask is missing and known Hello TLV body prefixes bounded by both declared option and physical availability. No opaque unknown-value prefix is promoted to a complete value; no new protocol identity/state is inferred.')
 f=a.out/'summary.json';f.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(f),SHA256=sha(f),cases=cases,targets=target_total),ensure_ascii=False))
if __name__=='__main__':main()
