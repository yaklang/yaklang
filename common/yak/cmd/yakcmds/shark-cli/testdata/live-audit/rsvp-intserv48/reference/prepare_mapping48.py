"""Freeze source-first Wireshark aliases and highlight-owner policies48.

No capture dissection or candidate SDK/Go output is read in preparation. All
primitive values/spans are from frozen independent source45/source48 facts.
An aggregate reference highlight is never an exact primitive-span match.
"""
from pathlib import Path
import argparse,base64,copy,hashlib,importlib.util,json,re
ROOT=Path(__file__).parent;P=ROOT.parent;N=P/'Intserv-native-CLI-source48-01'

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with p.open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,allow_nan=False);f.write('\n')
def load(p,name):
 spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
S=load(N/'intserv_native_schema48.py','reference_only_source_schema48')

def get(tree,path):
 parts=S.parts(path)
 if parts and parts[0]=='Root':parts=parts[1:]
 for x in parts:tree=tree[x]
 return tree

def base_alias(path,tree):
 key=path.rsplit('.',1)[1]
 if path.startswith('Header.'):
  return{'Message Type':'rsvp.msg','Checksum':'rsvp.message_checksum','Send TTL':'rsvp.sending_ttl','Message Length':'rsvp.message_length','Version':'rsvp.version','Flags':'rsvp.flags'}.get(key)
 m=re.match(r'Objects\[(\d+)\]\.',path)
 if not m:return None
 cls=tree['Objects'][int(m[1])]['Header']['Class']
 if'.Header.'in path:return{'Length':'rsvp.length','Class':'rsvp.object','C-Type':'rsvp.ctype'}.get(key)
 tables={1:{'Destination Address':'rsvp.session.ip','Protocol ID':'rsvp.session.proto','Flags':'rsvp.session.flags','Destination Port':'rsvp.session.port'},3:{'Hop Address':'rsvp.hop.neighbor_address_ipv4','Logical Interface Handle':'rsvp.hop.logical_interface'},5:{'Refresh Period Milliseconds':'rsvp.refresh_interval'},8:{'Flags':'rsvp.style.flags','Option Vector':'rsvp.style.style'},10:{'Source Address':'rsvp.filter.ip','Source Port':'rsvp.filter.port'},11:{'Source Address':'rsvp.sender.ip','Source Port':'rsvp.sender.port'},15:{'Receiver Address':'rsvp.confirm.receiver_address_ipv4'}}
 return tables.get(cls,{}).get(key)

def intserv_alias(leaf,cls,source,raw,object_end):
 path=leaf['path'];key=path.rsplit('.',1)[1];prefix={9:'flowspec',12:'tspec',13:'adspec'}[cls];alias=None;mode='exact-primitive-span';aggregate=None;owner_known=True
 if path=='Header.Version':alias='rsvp.'+prefix+'.message_format_version'
 elif path=='Header.Message Length Words':alias='rsvp.data_length'
 elif'.Parameters['not in path and key=='Service Number':alias='rsvp.'+prefix+'.service_header'
 elif'.Parameters['not in path and key=='Data Length Words':alias='rsvp.data_length'
 elif key=='Break Bit Observed'and cls==13:alias='rsvp.adspec.break_bit'
 elif'.Parameters['in path and key=='Parameter Number':
  alias='rsvp.adspec.type'if cls==13 else'rsvp.parameter'
  if cls!=13:
   mode='aggregate-parameter-owner-span';a=leaf['range'][0];service_path=path.split('.Parameters[')[0];parameter_path=path.rsplit('.Header.',1)[0];p=get(source['fields'],parameter_path);svc=get(source['fields'],service_path)
   aggregate=p.get('declared_range');upper=min(object_end,len(raw),svc.get('declared_range',[0,object_end])[1]);owner_known=aggregate is not None and aggregate[1]<=upper
 elif'.Parameters['in path and key=='Flags'and cls!=13:alias='rsvp.parameter_flags'
 elif'.Parameters['in path and key=='Data Length Words':alias='rsvp.adspec.len'if cls==13 else'rsvp.parameter_length'
 elif'.Body.'in path:
  if cls in(9,12):alias={'Token Bucket Rate':'rsvp.'+prefix+'.token_bucket_rate','Token Bucket Size':'rsvp.'+prefix+'.token_bucket_size','Peak Data Rate':'rsvp.'+prefix+'.peak_data_rate','Minimum Policed Unit':'rsvp.minimum_policed_unit','Maximum Packet Size':'rsvp.maximum_packet_size'}.get(key)
  elif key in('Composed IS Hop Count','Composed Available Path Bandwidth','Composed Minimum Path Latency','Composed Path MTU'):
   alias='rsvp.adspec.float'if key=='Composed Available Path Bandwidth'else'rsvp.adspec.uint';mode='aggregate-header-plus-value-span';a,b=leaf['range'];aggregate=[a-4,b]
 units={'Token Bucket Rate':'bytes/s','Token Bucket Size':'bytes','Peak Data Rate':'bytes/s','Minimum Policed Unit':'bytes','Maximum Packet Size':'bytes','Composed Available Path Bandwidth':'bytes/s','Composed Minimum Path Latency':'microseconds','Composed Path MTU':'bytes'}.get(key)
 return{'alias':alias,'reference_span_policy':mode if alias else'unselected-alias','expected_aggregate_logical_span':aggregate,'aggregate_owner_fully_proved_and_captured':owner_known,'source_units':units}

def prepare(out):
 assert not out.exists();out.mkdir();plan=N/'plan02';m=read(plan/'manifest.json');assert sha(plan/'manifest.json')=='fc3a502c4eb4d7e8b54679901510b72afe9fa5af8ed6ad53c666d16b4e2e831b'
 for p,h in m['frozen_inputs'].items():assert sha(p)==h
 inputs={str(p):sha(p)for p in[Path(__file__),plan/'manifest.json',plan/'source-rows.json',N/'intserv_native_schema48.py',ROOT/'metadata-producer.json',ROOT/'version.txt',ROOT/'field-dictionary.txt',ROOT/'current-preferences.txt']}
 dictionary={line.split('\t')[2]for line in(ROOT/'field-dictionary.txt').read_text(encoding='utf-8').splitlines()if line.startswith('F\t')}
 rows=read(plan/'source-rows.json');items=[];captures={};boundaries=[]
 for row in rows:
  raw=Path(row['raw_path']).read_bytes();assert hashlib.sha256(raw).hexdigest()==row['raw_SHA256'];tree=row['source_case']['fields'];inv=S.inventory(tree);source=S.base.decode_source(raw,4096);base_leaves=[]
  for leaf in source['leaves']:
   try:get(tree,leaf['path'])
   except(KeyError,IndexError,TypeError):continue
   if leaf['literal']and leaf['path']not in inv:continue
   if '.Body.Opaque Body' in leaf['path']:continue
   base_leaves.append(leaf)
  mapped_paths=set()
  for leaf in base_leaves:
   alias=base_alias(leaf['path'],tree);mapped_paths.add(leaf['path']);items.append({'context':row['capture_key'],'frame':row['anchor']['number'],'sourcecase':row['sourcecase'],'scope':'base','object_index':None,'object_class':None,'source_path':leaf['path'],'leaf':leaf,'mapping':{'alias':alias,'reference_span_policy':'exact-primitive-span'if alias else'unselected-alias','expected_aggregate_logical_span':None,'aggregate_owner_fully_proved_and_captured':True,'source_units':None},'physical_PDU_start':row['anchor']['PDU_physical_range'][0]})
  for target in row['composed_source']['nested']:
   oi=target['object_index'];cls=tree['Objects'][oi]['Header']['Class'];s=target['source'];object_start=target['body_start']-4;object_end=target['object_end']
   boundaries.append({'context':row['capture_key'],'frame':row['anchor']['number'],'sourcecase':row['sourcecase'],'object_index':oi,'class':cls,'object_logical_range':[object_start,object_end],'body_start':target['body_start'],'source':s,'physical_PDU_start':row['anchor']['PDU_physical_range'][0]})
   for leaf in s['leaves']:
    path='Objects['+str(oi)+'].Body.'+S.mapped(leaf['path']);mapped_paths.add(path);mapping=intserv_alias(leaf,cls,s,raw,object_end)
    items.append({'context':row['capture_key'],'frame':row['anchor']['number'],'sourcecase':row['sourcecase'],'scope':'Intserv','object_index':oi,'object_class':cls,'source_path':path,'leaf':leaf,'mapping':mapping,'physical_PDU_start':row['anchor']['PDU_physical_range'][0]})
  for path,(value,span)in inv.items():
   if path in mapped_paths or path.endswith('.IEEE754 Bits')or path.endswith('.Value'):continue
   if isinstance(value,dict):continue
   # E.g. the later TIME sibling of a hard-invalid Intserv object remains owned
   # root residual bytes; it is not independently decoded as a TIME observation.
   if isinstance(value,str):
    try:wire=base64.b64decode(value,validate=True)
    except(ValueError,TypeError):continue
    if wire!=raw[span[0]:span[1]]:continue
    leaf={'path':path,'literal':True,'kind':'bytes','value':value,'range':span,'raw_SHA256':hashlib.sha256(wire).hexdigest()}
    items.append({'context':row['capture_key'],'frame':row['anchor']['number'],'sourcecase':row['sourcecase'],'scope':'owned-residual','object_index':None,'object_class':None,'source_path':path,'leaf':leaf,'mapping':{'alias':None,'reference_span_policy':'unselected-alias','expected_aggregate_logical_span':None,'aggregate_owner_fully_proved_and_captured':True,'source_units':None},'physical_PDU_start':row['anchor']['PDU_physical_range'][0]})
  captures[row['capture_key']]={'capture':row['capture'],'capture_SHA256':row['capture_SHA256']}
 for item in items:
  a=item['mapping']['alias'];assert a is None or a in dictionary,a
 put(out/'source-to-reference-mapping.json',items);put(out/'source-bounded-owners.json',boundaries);put(out/'captures.json',captures)
 put(out/'comparison-policy.json',{'primitive_value_and_exact_span':'count only reference physical span equal to independently complete primitive span','aggregate_owner':'numeric value and raw owner bytes can match while highlight encloses header/data; report as aggregate, never exact primitive','unknown_aggregate_extent':'partial parameter header cannot prove entire declared owner, so no aggregate-owner pass','unrendered':'no selected alias or reference field absent; not counted as correct reference interpretation','nonfinite':'compare original IEEE32 bits exactly; classify textual inf/nan via Decimal; NaN payload/sign need original bytes and are not inferred from rendered NaN','units':'check explicit text only; absent unit is omitted, not a units agreement. Source units remain canonical bytes/s,bytes,microseconds.','value_only':'rendered correct value at a different span remains a span difference, not exact primitive pass','extra_reference_interpretations':'report reads/semantic fields outside source-selected bounded version/service/parameter; known Guaranteed support is an additional reference capability, not automatically a Wireshark bug','capture_metadata':'packet number,caplen/origlen and raw per-packet protocol chain are kept separately; no normalized protocol count correctness claim','no_candidate_Go_SDK_or_new_runtime_read':True})
 assert inputs=={p:sha(p)for p in inputs}
 put(out/'manifest.json',{'inputs':inputs,'files':{p.name:sha(p)for p in out.iterdir()if p.is_file()},'source_plan02_SHA256':sha(plan/'manifest.json'),'mapping_frozen_before_capture_dissection':True,'items':len(items),'physical_frames':38,'no_candidate_runtime_or_Go_read':True})
 print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'items':len(items),'selected_alias_items':sum(x['mapping']['alias']is not None for x in items),'Intserv_objects':len(boundaries)}))
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);prepare(p.parse_args().out)