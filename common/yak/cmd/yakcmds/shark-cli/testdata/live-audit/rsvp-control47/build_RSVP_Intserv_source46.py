"""Build independent original16 RSVP Intserv source facts and old-PDML audit.

No new capture, authored protocol matrix, Go or TShark execution. No candidate
output/value decoder is read. Reference aggregate highlight spans remain
reported as aggregate spans rather than normalized into literal leaf matches.
"""
import argparse
import collections
from decimal import Decimal
import hashlib
import importlib.util
import json
from pathlib import Path
import xml.etree.ElementTree as ET


def dg(w):return hashlib.sha256(w).hexdigest()
def sha(p):return dg(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,v):Path(p).write_text(json.dumps(v,ensure_ascii=False,indent=2,allow_nan=False)+'\n',encoding='utf-8')
def mod(path):
 s=importlib.util.spec_from_file_location('intserv_source_only',path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def find(proto,alias,pos,size=None):
 rows=[x for x in proto.iter('field')if x.get('name')==alias and x.get('pos')==str(pos)and(size is None or x.get('size')==str(size))]
 assert len(rows)==1,(alias,pos,size,len(rows));return rows[0]

def reference_audit(row,proto,pdu):
 facts=row['source'];cls=row['object_class'];prefix={9:'flowspec',12:'tspec',13:'adspec'}[cls];base=row['PDU_physical_start'];selected=[];missing=[]
 # Resolve aliases from source owner paths, never source values from PDML.
 for leaf in facts['leaves']:
  path=leaf['path'];span=leaf.get('range',leaf.get('source_range'));a,b=span;alias=None;kind='exact-literal-span'
  if path=='Header.Version':alias='rsvp.'+prefix+'.message_format_version';kind='exact-derived-bit-span'
  elif path=='Header.Message Length Words':alias='rsvp.data_length'
  elif '.Parameters['not in path and path.endswith('.Header.Service Number'):alias='rsvp.'+prefix+'.service_header'
  elif '.Parameters['not in path and path.endswith('.Header.Data Length Words'):alias='rsvp.data_length'
  elif path.endswith('.Header.Break Bit Observed')and cls==13:alias='rsvp.adspec.break_bit';kind='exact-derived-bit-span'
  elif '.Parameters['in path and path.endswith('.Header.Parameter Number'):
   alias='rsvp.adspec.type'if cls==13 else'rsvp.parameter';kind='exact-literal-span'if cls==13 else'aggregate-parameter-owner-span'
  elif '.Parameters['in path and path.endswith('.Header.Flags')and cls!=13:alias='rsvp.parameter_flags'
  elif '.Parameters['in path and path.endswith('.Header.Data Length Words'):alias='rsvp.adspec.len'if cls==13 else'rsvp.parameter_length'
  elif '.Body.'in path:
   field=path.rsplit('.',1)[1]
   if cls in(9,12):alias={'Token Bucket Rate':'rsvp.'+prefix+'.token_bucket_rate','Token Bucket Size':'rsvp.'+prefix+'.token_bucket_size','Peak Data Rate':'rsvp.'+prefix+'.peak_data_rate','Minimum Policed Unit':'rsvp.minimum_policed_unit','Maximum Packet Size':'rsvp.maximum_packet_size'}.get(field)
   else:
    alias='rsvp.adspec.float'if field=='Composed Available Path Bandwidth'else'rsvp.adspec.uint';kind='aggregate-header-plus-value-span'
  if alias is None:
   missing.append({'source_path':path,'source_range':span,'reason':'no selected existing primitive alias; not a reference pass'});continue
  if kind=='aggregate-header-plus-value-span':ref=find(proto,alias,base+a-4,8);refspan=[a-4,b]
  elif kind=='aggregate-parameter-owner-span':
   # Parameter ID is rendered on a container that spans its header+data.
   parameter_start=a;words=int.from_bytes(pdu[a+2:a+4],'big');refspan=[a,a+4+4*words];ref=find(proto,alias,base+a,refspan[1]-refspan[0])
  else:ref=find(proto,alias,base+a,b-a);refspan=span
  show=ref.get('show')
  if leaf['kind']=='ieee32':assert leaf['value']['classification']in('finite','positive-zero')and Decimal(show)==Decimal(leaf['value']['exact_decimal'])
  elif leaf['kind']=='bool':assert(show=='True')is leaf['value']
  else:assert int(show,0)==leaf['value']
  if leaf['literal']:
   assert ref.get('value','').lower()==pdu[refspan[0]:refspan[1]].hex()
  selected.append({'source_path':path,'source_range':span,'reference_alias':alias,'reference_range':refspan,'reference_physical_range':[base+refspan[0],base+refspan[1]],'match_class':kind,'source_value':leaf['value']})
 return {'frame':row['frame'],'object_index':row['object_index'],'selected_matches':selected,'unrendered_not_passes':missing}

def run(phase,out):
 assert not out.exists();out.mkdir()
 original=phase/'RSVP-official-source45-01';primary=phase/'RSVP-Intserv-primary46-01';modelpath=phase/'rsvp_intserv_source46.py';model=mod(modelpath)
 facts=read(original/'source-facts.json');pdml=ET.parse(original/'reference.pdml').getroot().findall('packet')
 assert sha(original/'source-facts.json')=='99436adc7179b876259cb2b179e70e0fe1c157b3feb343a74e9774864b3ee1b2'
 assert sha(original/'reference.pdml')=='f6c1226dd1b832651a30470e2ab8d8575b3eea44cd8f969d891c5879b2831d20'
 inputs=[Path(__file__),modelpath,original/'source-facts.json',original/'reference.pdml',primary/'manifest.json']+[primary/f'rfc{n}.txt'for n in(2210,2215,2212)]+[Path(r['PDU_file'])for r in facts]
 before={str(p):sha(p)for p in inputs}
 rows=[];references=[];unique=collections.defaultdict(list);counts=collections.Counter();issuecounts=collections.Counter();classes=collections.Counter()
 for frame in facts:
  pdu=Path(frame['PDU_file']).read_bytes();assert dg(pdu)==frame['PDU_SHA256'];proto=next(p for p in pdml[frame['frame']-1].findall('proto')if p.get('name')=='rsvp')
  for index,obj in enumerate(frame['source']['objects']):
   cls=obj['class']
   if cls not in(9,12,13):continue
   assert obj['C-Type']==2;start,end=obj['value_span'];assert dg(pdu[start:end])==obj['value_SHA256']
   source=model.decode_complete(pdu,start,end,cls)
   row={'frame':frame['frame'],'domain':frame['domain'],'PDU_path':frame['PDU_file'],'PDU_SHA256':frame['PDU_SHA256'],'PDU_physical_start':frame['PDU_physical_range'][0],'object_index':index,'object_class':cls,'object_C_Type':2,'object_header_range':obj['header_span'],'body_range':[start,end],'physical_body_range':[frame['PDU_physical_range'][0]+start,frame['PDU_physical_range'][0]+end],'body_SHA256':obj['value_SHA256'],'source':source}
   rows.append(row);references.append(reference_audit(row,proto,pdu));unique[(cls,obj['value_SHA256'])].append({'frame':row['frame'],'object_index':index,'body_range':[start,end]});classes[str(cls)]+=1
   counts['objects']+=1;counts['literal_wire_leaves']+=sum(x['literal']for x in source['leaves']);counts['derived_bit_values']+=sum(not x['literal']for x in source['leaves']);counts['service_headers']+=len(source['fields']['Services'])
   counts['parameter_headers']+=sum(len(s['Parameters'])for s in source['fields']['Services'])
   counts['IEEE32_literals']+=sum(x['kind']=='ieee32'for x in source['leaves'])
   for issue in source['semantic_value_issues']:issuecounts[issue['code']]+=1
 assert len(rows)==16 and len(unique)==3
 put(out/'source-facts.json',rows);put(out/'reference-comparison.json',references)
 put(out/'unique-body-inventory.json',[{'object_class':key[0],'body_SHA256':key[1],'occurrences':len(owners),'owners':owners}for key,owners in unique.items()])
 boundaries={'scope':'next-stage requirements only; no authored wire matrix or product test run claimed',
 'length_units':'each message/service/parameter has its own4B header; length16 counts32bit data words excluding that same header; advance4+4*length even zero',
 'namespace':'SENDER_TSPEC class12 service1/param127 differs from FLOWSPEC class9 service5/param127 and ADSPEC class13 service1 composed params4/6/8/10; ADSPEC service5 length0 is legal and requires no token bucket',
 'enclosure':'message end must be independently bounded by object/capture; service by message; parameter by service; never borrow sibling/header/trailing bytes to complete a primitive',
 'partial':'retain every complete primitive and owned partial residual; distinguish true snaplen shortage from complete declared-parent contradiction; no default float/integer/address/header node when bytes missing',
 'unknown':'preserve complete service/parameter header, own declared opaque value and skip only via proved extent; no semantic labels under unknown version/service/class context',
 'budgets':'recommend shared bounded tokens for object/SCOPE address/service/parameter nodes; scalars/header words do not create unlimited zero-length loops; no performance benchmark here',
 'IEEE32':'retain exact unsigned32 bits; finite values can be JSON-safe exact decimal; positive infinity only for peak p; explicit special-value classification string/enum instead of numeric JSON Infinity/NaN; no unguarded nonfinite arithmetic',
 'nonnegative_domains':'r/b and pathBW finite nonnegative representation, excluding negativezero, all NaNs/infinities; p permits positive infinity only. RFC2215 separately says r/b positive and m/M positive,m<=M; record semantic issues without losing raw fields',
 'special_integer':'minimum path latency 0xffffffff=indeterminate (microseconds); pathBW0=unknown; MTU bytes includesIP/upper headers, no link header',
 'flags':'serviceB and parameterI are observed advertisements, not verified transit/capability/authentication; all reserved bits retained; do not silently infer installed service',
 'existing_semantic_issues':'all9 observed tokenbucket records m0;2 receiver FlowSpec records M0. Fields/lengths remain observable; future malformed/context/expert policy not selected by this source-only audit',
 'uncovered':['Guaranteed service2 andRSpec130','nonfinite/negativezero wire examples','unknown service/parameter examples','partial/overrun/zero-budget adversarial examples','actual parameter INVALID/BREAK bit1','Intserv Go/native/CLI support'],
 'primary_refs':{'message_service_parameter_headers':{'RFC':2210,'sections':['A1.1','A1.2','A1.3','A1.4'],'LF_lines':[[1631,1655],[1687,1722],[1743,1780],[1799,1808]]},'TSPEC':{'RFC':2210,'section':'3.1','LF_lines':[523,639]},'ControlledLoadFLOWSPEC':{'RFC':2210,'section':'3.2.1','LF_lines':[646,760]},'ADSPEC':{'RFC':2210,'sections':['3.3','3.3.2','3.3.4'],'LF_lines':[[850,937],[939,1005],[1205,1228]]},'TokenBucketDomain':{'RFC':2215,'section':'3.6','LF_lines':[683,803]}}}
 put(out/'boundary-and-representation-plan.json',boundaries)
 counts['distinct_body_SHA256']=len(unique)
 for r in references:
  for m in r['selected_matches']:counts['reference_'+m['match_class']]+=1
  counts['reference_unrendered_not_passes']+=len(r['unrendered_not_passes'])
 assert before=={p:sha(p)for p in before}
 put(out/'manifest.json',{'inputs':before,'outputs':{str(p):sha(p)for p in out.iterdir()if p.is_file()},'source_model':str(modelpath),'source_model_SHA256':sha(modelpath),'builder':str(Path(__file__)),'builder_SHA256':sha(__file__),'no_Go_or_new_native_CLI_or_TShark_execution':True,'no_candidate_source_or_values_read':True,'original_source45_unchanged':True})
 put(out/'summary.json',{'manifest_SHA256':sha(out/'manifest.json'),'source_facts_SHA256':sha(out/'source-facts.json'),'reference_comparison_SHA256':sha(out/'reference-comparison.json'),'counts':dict(counts),'object_classes':dict(classes),'semantic_value_issues':dict(issuecounts),'semantic_validity_not_inferred_from_Wireshark_rendering':True,'reference_span_differences_preserved':True,'candidate_support_not_claimed':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':dict(counts),'semantic_issues':dict(issuecounts)}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--phase',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();run(a.phase,a.out)
if __name__=='__main__':main()