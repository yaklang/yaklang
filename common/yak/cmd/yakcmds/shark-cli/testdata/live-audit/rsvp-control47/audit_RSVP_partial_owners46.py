"""Read-only source-owner audit for two preserved RSVP direct differences.

Recomputes source coordinates from frozen authored octets and primary text.
Does not repair facts, plans, candidate output or repository source.
"""
import argparse
import hashlib
import ipaddress
import json
from pathlib import Path


def dg(raw):return hashlib.sha256(raw).hexdigest()
def sha(path):return dg(Path(path).read_bytes())
def read(path):return json.loads(Path(path).read_text(encoding='utf-8-sig'))
def put(path,obj):Path(path).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def u(raw,a,b):return int.from_bytes(raw[a:b],'big')

def run(phase,out):
 assert not out.exists();out.mkdir()
 plan=phase/'RSVP-direct-API-source-plan46-01'
 cases=read(plan/'source-cases.json')
 facts=read(phase/'RSVP-boundary-source45-01/source-facts.json')
 actual=phase/'RSVP-direct-actual46-01/direct.jsonl'
 assert sha(actual)=='d6800df8a4386423f55a26dc3836d9f338cc7e67ead0552fd02c71bd52cc5291'
 rows=[json.loads(line)for line in actual.read_bytes().splitlines()if line.strip()]
 primary=phase/'RSVP-primary-download45-01/rfc2205.txt'
 inputs=[Path(__file__),plan/'manifest.json',plan/'source-cases.json',phase/'RSVP-boundary-source45-01/source-facts.json',actual,primary]
 results=[]
 for scene in (8,4):
  old=next(row for row in facts if row['scene']==scene)
  expected=next(row for row in cases if row['sourcecase']==old['sourcecase'])
  got=next(row for row in rows if row['sourcecase']==old['sourcecase'])
  rawpath=Path(expected['raw_path']);physicalpath=Path(old['physical_path'])
  inputs += [rawpath,physicalpath]
  raw=rawpath.read_bytes();wire=physicalpath.read_bytes()
  assert dg(raw)==expected['raw_SHA256']==old['PDU_SHA256']
  start,end=old['carrier']['PDU_physical_range']
  assert raw==wire[start:end]and len(wire)==old['carrier']['captured_length']==old['carrier']['original_length']
  assert wire[23]==46 and wire[14]>>4==4
  ipend=14+u(wire,16,18)
  assert ipend==end and ipend<=len(wire)
  declared=u(raw,6,8)
  if scene==8:
   assert declared==16==len(raw)and u(raw,8,10)==12 and raw[10:12]==bytes([1,1])
   a,b=12,16
   value=str(ipaddress.IPv4Address(raw[a:b]))
   assert value=='203.0.113.9'and start+a==46 and start+b==50
   assert got['fields']['Unparsed Body']==expected['fields']['Objects'][0]['Body']['Destination Address'].encode().decode() if False else True
   import base64
   assert base64.b64decode(got['fields']['Unparsed Body'])==raw[a:b]
   assert 'Body'not in got['fields']['Objects'][0]
   results.append({'sourcecase':expected['sourcecase'],'RSVP_declared_end':declared,'object_declared_range':[8,20],
                   'object_header_range':[8,12],'observed_primitive':'Objects[0].Body.Destination Address',
                   'value':value,'range':[a,b],'physical_range':[start+a,start+b],'raw_value_SHA256':dg(raw[a:b]),
                   'physical_IP_declared_end':ipend,'physical_captured_end':len(wire),'Ethernet_padding_bytes':len(wire)-ipend,
                   'source_format_proof':'object end20 exceeds complete RSVP parent16; no required-capture retry or sibling scanning',
                   'current_output':'same bytes owned as Root.Unparsed Body; complete selected Destination Address field omitted',
                   'required_conservative_repair':'retain complete Body.Destination Address with [12,16), retain Format, no Protocol ID/Flags/Port defaults, no semantic state',
                   'primary':{'section':'RFC2205 A.1','LF_lines':[4272,4282]}})
  else:
   assert declared==20 and len(raw)==28
   assert got['is_format']is True and expected['required_categories']['is_format']is False
   lines=primary.read_text(encoding='utf-8').split('\n')
   assert 'each RSVP message must occupy exactly'in lines[2706]
   assert 'RSVP Length field does not match message'in lines[5350]
   results.append({'sourcecase':expected['sourcecase'],'RSVP_declared_end':declared,'captured_IP_payload_bytes':len(raw),
                   'declared_RSVP_checksum_scope':[0,20],'unindicated_tail_range':[20,28],'tail_SHA256':dg(raw[20:28]),
                   'wire_values_and_ranges_remain_frozen':True,
                   'source_policy_erratum':'initial checker inferred is_format=false from no recorded issue; primary explicitly supports wrong-length formatting error',
                   'old_actual_and_plan_preserved':True,
                   'not_a_product_classification_bug':True,
                   'primary':[{'section':'RFC2205 3.3','LF_lines':[2707,2709]}, {'section':'RFC2205 Appendix B','LF_lines':[5346,5352]}]})
 before={str(path):sha(path)for path in inputs}
 put(out/'source-owner-facts.json',results)
 assert before=={path:sha(path)for path in before}
 put(out/'summary.json',{'inputs':before,'source_owner_facts_SHA256':sha(out/'source-owner-facts.json'),'cases':2,'repository_or_Go_read':False,'candidate_values_not_used_as_source_expectation':True,'frozen_source_plans_unchanged':True,'scope':'read-only analysis, no candidate execution or expected-value modification'})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json')}))


def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--phase',type=Path,required=True);parser.add_argument('--out',type=Path,required=True)
 args=parser.parse_args();run(args.phase,args.out)
if __name__=='__main__':main()