"""Offline source48 versus pinned Wireshark PDML; no expectation adjustment.

Exact primitives, aggregate highlights, absent rendering, numeric differences,
opaque semantics and reference bounds are distinct classes of observations.
"""
from pathlib import Path
import argparse,base64,collections,copy,hashlib,ipaddress,json,xml.etree.ElementTree as ET
from decimal import Decimal,InvalidOperation
P=Path(__file__).parent.parent

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with p.open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,allow_nan=False);f.write('\n')
def refs(element,ancestors=()):
 for child in element:
  if child.tag=='field':
   yield dict(child.attrib),ancestors
   yield from refs(child,ancestors+(dict(child.attrib),))
  else:yield from refs(child,ancestors)

def numeric_matches(leaf,ref):
 show=ref.get('show');value=leaf['value']
 try:
  if leaf['kind']=='ieee32':
   d=Decimal(show)
   category=value['classification']
   if category=='nan':return d.is_nan()
   if category in('positive-infinity','negative-infinity'):return d.is_infinite()and d.is_signed()==(category=='negative-infinity')
   if d.is_nan()or d.is_infinite():return False
   if category in('positive-zero','negative-zero'):return d.is_zero()and d.is_signed()==(category=='negative-zero')
   return d==Decimal(value['exact_decimal'])
  if leaf['kind']=='bool':return str(show).lower()==str(value).lower()
  if leaf['kind']=='address':return ipaddress.ip_address(show)==ipaddress.ip_address(value)
  if leaf['kind']=='bytes':return True # Numeric semantics not claimed for bytes.
  return int(show,0)==value
 except(ValueError,TypeError,InvalidOperation):return False

def one(item,all_refs,raw):
 leaf=item['leaf'];m=item['mapping'];a,b=leaf.get('range',leaf.get('source_range'));base=item['physical_PDU_start'];alias=m['alias'];mode=m['reference_span_policy']
 out={'sourcecase':item['sourcecase'],'scope':item['scope'],'source_path':item['source_path'],'source_logical_span':[a,b],'alias':alias,'source_kind':leaf['kind'],'source_value':leaf['value'],'literal':leaf['literal'],'span_policy':mode}
 if alias is None:out['classification']='source-unmapped-not-reference-pass';return out
 target=m['expected_aggregate_logical_span']if mode.startswith('aggregate')else[a,b]
 pos=base+(target[0]if target else a)
 candidates=[f for f,_ in all_refs if f.get('name')==alias and f.get('pos')==str(pos)]
 if not candidates:out['classification']='reference-unrendered-selected-alias';return out
 expected_size=(target[1]-target[0])if target else None
 sized=[f for f in candidates if expected_size is not None and f.get('size')==str(expected_size)]
 selected=sized if sized else candidates
 if len(selected)!=1:
  out['classification']='reference-ambiguous-owner';out['reference_candidates']=selected;return out
 ref=selected[0];out['reference']=ref
 refspan=[int(ref.get('pos','0'))-base,int(ref.get('pos','0'))-base+int(ref.get('size','0'))];out['reference_logical_span']=refspan
 out['rendered_value_matches']=numeric_matches(leaf,ref)
 inside=0<=refspan[0]<=refspan[1]<=len(raw)
 out['reference_span_inside_captured_PDU']=inside
 out['raw_reference_value_matches_its_own_span']=inside and ref.get('value','').lower()==raw[refspan[0]:refspan[1]].hex()
 span_match=target is not None and refspan==target
 if not out['rendered_value_matches']:out['classification']='reference-rendered-value-difference';return out
 primitive_match=refspan==[a,b]
 if primitive_match and(not leaf['literal']or out['raw_reference_value_matches_its_own_span']):
  # A nominally aggregate alias can render a complete1B parameter number on a
  # partial header. Its actual source primitive span, not its alias, controls
  # the exactness class. No declared owner length is inferred from this.
  out['classification']='exact-literal-value-and-primitive-span'if leaf['literal']else'exact-derived-value-and-source-span'
 elif not span_match:out['classification']='reference-value-matches-highlight-span-differs';return out
 elif leaf['literal']and not out['raw_reference_value_matches_its_own_span']:out['classification']='reference-value-matches-without-exact-wire-proof';return out
 elif mode.startswith('aggregate'):
  out['classification']='reference-aggregate-owner-match-not-primitive'if m['aggregate_owner_fully_proved_and_captured']else'reference-aggregate-owner-unproved-or-crosses-parent'
 else:out['classification']='exact-literal-value-and-primitive-span'if leaf['literal']else'exact-derived-value-and-source-span'
 if m.get('source_units'):
  text=ref.get('showname','');unit=m['source_units'];out['source_units']=unit
  if'bits/s'in text or'bits/sec'in text:out['unit_rendering']='explicit-bits-per-second-source-is-bytes'
  elif unit=='bytes/s'and any(s in text for s in('bytes/s','bytes/sec','bytes per second','Bps')):out['unit_rendering']='explicit-unit-compatible'
  elif unit=='microseconds'and any(s in text for s in('microseconds','microsecond','us','μs')):out['unit_rendering']='explicit-unit-compatible'
  elif unit=='bytes'and'byte'in text.lower():out['unit_rendering']='explicit-unit-compatible'
  else:out['unit_rendering']='unit-not-rendered-not-a-unit-agreement'
 return out

def compare(plan,actual,out):
 assert not out.exists();out.mkdir();m=read(plan/'manifest.json')
 for p,h in m['inputs'].items():assert sha(p)==h
 for name,h in m['files'].items():assert sha(plan/name)==h
 producer=read(actual/'producer.json');assert producer['plan_SHA256']==sha(plan/'manifest.json')and producer['all_processes_closed']
 for p,h in producer['inputs'].items():assert sha(p)==h
 for run in producer['runs']:assert run['exit_code']==0 and sha(run['output'])==run['output_SHA256']and sha(run['stderr'])==run['stderr_SHA256']
 items=read(plan/'source-to-reference-mapping.json');owners=read(plan/'source-bounded-owners.json');source_rows=read(P/'Intserv-native-CLI-source48-01/plan02/source-rows.json');sources={(r['capture_key'],r['anchor']['number']):r for r in source_rows}
 documents={context:ET.parse(actual/context/'reference.pdml').getroot().findall('packet')for context in('official','authored')};assert len(documents['official'])==9 and len(documents['authored'])==29
 result=[];counts=collections.Counter();contexts=collections.defaultdict(collections.Counter);extra=[];framefacts=[]
 packet_refs={}
 for key,row in sources.items():
  context,frame=key;packet=documents[context][frame-1];all_refs=list(refs(packet));packet_refs[key]=all_refs;fields={f.get('name'):f for f,_ in all_refs if f.get('name')in('frame.number','frame.cap_len','frame.len','frame.time_epoch')};a=row['anchor']
  for alias,value in [('frame.number',frame),('frame.cap_len',a['caplen']),('frame.len',a['origlen'])]:assert int(fields[alias]['show'])==value
  assert Decimal(fields['frame.time_epoch']['show'])*10**9==a['time_ns']
  experts=[f.get('show')for f,_ in all_refs if f.get('name')=='_ws.expert.message'];protocols=[p.get('name')for p in packet.findall('proto')]
  framefacts.append({'context':context,'frame':frame,'sourcecase':row['sourcecase'],'source_status':row['expected_status'],'source_snaplen_short':a['real_snaplen_short'],'reference_protocol_chain':protocols,'reference_expert_messages':experts,'metadata_exact':True})
 for item in items:
  key=item['context'],item['frame'];row=sources[key];raw=Path(row['raw_path']).read_bytes();r=one(item,packet_refs[key],raw);result.append(r);counts[r['classification']]+=1;contexts[item['context']][r['classification']]+=1
 # Inspect extra semantic interpretations using frozen source-owned extents and
 # original PDML ancestry. This is observation, not a new expected-value oracle.
 semantic_aliases={'rsvp.tspec.token_bucket_rate','rsvp.tspec.token_bucket_size','rsvp.tspec.peak_data_rate','rsvp.flowspec.token_bucket_rate','rsvp.flowspec.token_bucket_size','rsvp.flowspec.peak_data_rate','rsvp.minimum_policed_unit','rsvp.maximum_packet_size','rsvp.adspec.uint','rsvp.adspec.float'}
 for key,row in sources.items():
  raw=Path(row['raw_path']).read_bytes();base=row['anchor']['PDU_physical_range'][0];bounded=[o for o in owners if(o['context'],o['frame'])==key];selected={(i['mapping']['alias'],i['physical_PDU_start']+(i['mapping']['expected_aggregate_logical_span']or i['leaf'].get('range',i['leaf'].get('source_range')))[0])for i in items if(i['context'],i['frame'])==key and i['mapping']['alias']}
  for field,parents in packet_refs[key]:
   alias=field.get('name');pos=int(field.get('pos','0'));size=int(field.get('size','0'));a,b=pos-base,pos-base+size
   if alias not in semantic_aliases:continue
   wrappers=[x for x in parents if x.get('name')in('rsvp.tspec','rsvp.flowspec','rsvp.adspec')]
   target=None
   for o in bounded:
    start,end=o['object_logical_range']
    if start<=a<end or any(int(w.get('pos','-1'))==base+start for w in wrappers):target=o;break
   reason=None
   if not(0<=a<=b<=len(raw)):reason='reference-semantic-field-range-exceeds-captured-PDU'
   elif target:
    start,end=target['object_logical_range'];s=target['source'];version=s['fields'].get('Header',{}).get('Version')
    if a<start or b>end:reason='reference-semantic-field-crosses-proved-object'
    elif version is not None and version!=0:reason='reference-v0-layout-under-future-version'
    else:
     for service in s['fields'].get('Services',[]):
      interval=service.get('declared_range')
      if not interval:continue
      sa,sb=interval
      if sa+4<=a<sb and not service.get('Layout Selected',False):reason='reference-guaranteed-layout-beyond-selected-profile'if service['Header'].get('Service Number')==2 else'reference-meaning-under-unselected-service'
      for parameter in service.get('Parameters',[]):
       span=parameter.get('declared_range')
       if span and span[0]+4<=a<span[1]and parameter.get('Layout Selected')is False:reason='reference-meaning-under-unselected-parameter'
   if reason:extra.append({'context':key[0],'frame':key[1],'sourcecase':row['sourcecase'],'reason':reason,'alias':alias,'reference_logical_span':[a,b],'reference':field,'source_object':target['object_index']if target else None})
   elif(alias,pos)not in selected:extra.append({'context':key[0],'frame':key[1],'sourcecase':row['sourcecase'],'reason':'reference-semantic-field-without-selected-source-primitive','alias':alias,'reference_logical_span':[a,b],'reference':field})
 # Negative controls check span/raw-proof distinctions without changing sources.
 negatives=[];ex=next((i for i in items if i['leaf']['literal']and i['mapping']['reference_span_policy']=='exact-primitive-span'and one(i,packet_refs[(i['context'],i['frame'])],Path(sources[(i['context'],i['frame'])]['raw_path']).read_bytes())['classification']=='exact-literal-value-and-primitive-span'))
 key=ex['context'],ex['frame'];raw=Path(sources[key]['raw_path']).read_bytes();good=one(ex,packet_refs[key],raw);f=good['reference']
 bad=copy.deepcopy(f);bad['size']=str(int(bad['size'])+1);assert one(ex,[(bad,())],raw)['classification']!='exact-literal-value-and-primitive-span';negatives.append('aggregate span cannot pass primitive')
 bad=copy.deepcopy(f);bad['value']='00'*int(bad['size']);assert one(ex,[(bad,())],raw)['classification']!='exact-literal-value-and-primitive-span';negatives.append('wire proof cannot be inferred from correct rendered value')
 bad=copy.deepcopy(f);bad['show']='987654321';assert one(ex,[(bad,())],raw)['classification']=='reference-rendered-value-difference';negatives.append('wrong rendered value')
 put(out/'per-source-leaf-comparison.json',result);put(out/'reference-extra-interpretations.json',extra);put(out/'physical-reference-observations.json',framefacts)
 put(out/'selftest-summary.json',{'negative_controls_rejected':negatives,'source_plan_unchanged':True,'not_candidate_product_test':True})
 stats={'source_items':len(items),'physical_frames':len(sources),'Intserv_objects':len(owners),'classifications':dict(counts),'contexts':{k:dict(v)for k,v in contexts.items()},'extra_reference_interpretations':dict(collections.Counter(x['reason']for x in extra)),'source_semantic_issues':sum(len(r['composed_source']['semantic_issues'])for r in source_rows),'source_semantic_issue_packets':sum(bool(r['composed_source']['semantic_issues'])for r in source_rows),'units':dict(collections.Counter(x['unit_rendering']for x in result if'unit_rendering'in x))}
 put(out/'summary.json',{'counts':stats,'source_plan_SHA256':sha(plan/'manifest.json'),'reference_producer_SHA256':sha(actual/'producer.json'),'checker_SHA256':sha(__file__),'comparison_files_SHA256':{p.name:sha(p)for p in out.iterdir()if p.is_file()},'capture_keys_full_PDML_values_external_only':True,'reference_agreement_not_product_validation':True,'no_source_values_ranges_or_alias_mapping_adjusted_after_actual':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':stats},ensure_ascii=False))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--plan',type=Path,required=True);p.add_argument('--actual',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();compare(a.plan,a.actual,a.out)