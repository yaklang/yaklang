"""Freeze598 source-only direct OSPFv2 codec inputs and wire-leaf comparison facts.

556 real source rows,24 prior authored controls and17 new authored wires/18
budget scenarios are fixed. No candidate/Go/native output is read. Opaque source
bytes remain external; facts store length/hash, not passwords or complete blobs.
"""
import argparse,collections,hashlib,ipaddress,json,shutil
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def h(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def sum16(w):
 if len(w)%2:w+=b'\0'
 n=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,len(w),2))
 while n>>16:n=(n&65535)+(n>>16)
 return(~n)&65535

def rename(path):
 out=[]
 for i,key in enumerate(path):
  if key=='Type'and path[:2]==['LLS','TLVs']:key='TLV Type'
  elif key=='TOS Count':key='TOS/MT-ID Count'
  elif key=='TOS Metrics':key='TOS/MT-ID Metrics'
  elif key=='TOS'and'TOS Metrics'in path:key='TOS/MT-ID Selector'
  elif key=='Flags and TOS':key='Flags and TOS/MT-ID'
  out.append(key)
 return out

def value(raw,encoding):
 if encoding in('ipv4','id','mask'):return str(ipaddress.IPv4Address(raw))
 if encoding=='opaque':return{'bytes':len(raw),'SHA256':h(raw)}
 assert encoding=='uint';return int.from_bytes(raw,'big')

def identities(*names):return{n:n in names for n in('context','budget','integrity','format')}
def metadata(w,fields,leaves):
 checks=[]
 def add(path,v=None,contains=None):
  x={'path':path}
  if contains is None:x['value']=v
  else:x['contains']=contains
  checks.append(x)
 add(['Authentication Verified'],False)
 header=fields.get('Header',{});n=header.get('Packet Length');au=header.get('Authentication Type');full=n is not None and 24<=n<=len(w)
 if full and au in(0,1):add(['Checksum Verified'],sum16(w[:16]+w[24:n])==0);add(['Checksum Scope'],contains='declared OSPFv2 packet excluding eight authentication bytes')
 else:
  add(['Checksum Verified'],False)
  if full and au==2:add(['Checksum Scope'],contains='omitted: cryptographic authentication')
  if full and au is not None and au>2:add(['Checksum Scope'],contains='unselected authentication type')
 required=24
 if full:
  required=n
  if au==2 and 'Authentication Data Length'in fields.get('Authentication',{}):required=n+fields['Authentication']['Authentication Data Length']
  lls=fields.get('LLS')
  if lls:
   start=lls.get('Observed Extent',[required])[0];words=lls.get('Length Words')
   required=start+4 if words is None or words==0 else start+words*4
   complete=words is not None and words>=1 and start+words*4<=len(w)
   verified=bool(complete and au in(0,1)and sum16(w[start:start+words*4])==0)
   add(['LLS','Checksum Verified'],verified);add(['LLS','Authentication Verified'],False)
   if au==2:add(['LLS','Checksum Scope'],contains='omitted: cryptographic LLS authentication')
   elif not complete:add(['LLS','Checksum Scope'],contains='complete LLS block unavailable')
   else:add(['LLS','Checksum Scope'],contains='LLS block only; independent of OSPF packet checksum')
 for i,row in enumerate(fields.get('LSAs',[])):
  start=row['Wire Field Ranges']['LS Age'][0];length=row['Length'];data=w[start:start+length];assert len(data)==length;c0=c1=0
  for b in data[2:]:c0=(c0+b)%255;c1=(c1+c0)%255
  valid=int.from_bytes(data[16:18],'big')!=0 and c0==c1==0
  add(['LSAs',i,'Checksum Verified'],valid)
 for prefix,rows in[(['LSA Headers'],fields.get('LSA Headers',[])),(['Database Description','LSA Headers'],fields.get('Database Description',{}).get('LSA Headers',[]))]:
  for i,row in enumerate(rows):add(prefix+[i,'Checksum Verified'],False)
 add(['Required Captured Bytes'],required)
 return checks

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--phase',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();p=a.phase;v2=p/'OSPFv2-independent-source39-01';review=p/'OSPFv2-review-regressions-source40-01'
 docs=[(v2/'source-grid03/summary.json','2aaf89759d9b4a2e44ff1b075778694d2b2c9eea8db3a3f12b19bae0ffdbbc65'),(v2/'authored-controls01/summary.json','57616b03d6a11b976d06413c6934870d64ff115b79bbaad2b046ec6785fd300e'),(v2/'authored-partial-prefix-proof02.json','e9eaf03b633bcd2058121c9b6ac00b7e53ed944add5cc1fc7e18ce49197a608a'),(review/'summary.json','9023c38af9629fbe2f39071f2c0e8d075c2d6b6be9143bd596e8d9ae68ab9571')];bound={str(Path(__file__)):sha(__file__)}
 for file,expected in docs:assert sha(file)==expected;bound[str(file)]=sha(file)
 full=[]
 for number in(35,37):
  file=v2/('source-grid03/input-%02d-source-grid.json'%number);bound[str(file)]=sha(file)
  for row in read(file):full.append({'sourcecase':'real/input-%02d/frame-%d'%(number,row['frame']),'group':'real556','file':row['original_frame'],'file_SHA256':row['original_frame_SHA256'],'offset':row['PDU_range'][0],'length':row['PDU_range'][1]-row['PDU_range'][0],'rawSHA':row['PDU_SHA256'],'budget':4096,'source':row['source'],'identities':identities(),'error_required':False})
 authored=v2/'authored-controls01/source-facts.json';bound[str(authored)]=sha(authored);partial={r['frame']:r for r in read(v2/'authored-partial-prefix-proof02.json')['controls']}
 for row in read(authored):
  frame=row['frame'];source=row.get('source')
  if source is None:
   # Partial direct controls retain the complete fields already witnessed by
   # the independent prefix proof; do not create values from missing bytes.
   leaves=partial[frame]['complete_source_scalar_prefix_leaves'];fields={}
   for leaf in leaves:
    node=fields
    for pos,key in enumerate(leaf['path'][:-1]):
     nxt=leaf['path'][pos+1]
     if isinstance(node,list):
      while len(node)<=key:node.append({})
      node=node[key]
     else:node=node.setdefault(key,[]if isinstance(nxt,int)else{})
    node[leaf['path'][-1]]=leaf['value'];node.setdefault('Wire Field Ranges',{})[leaf['path'][-1]]=leaf['PDU_span']
   if frame==23:fields['LLS']['Observed Extent']=[fields['Header']['Packet Length'],fields['Header']['Packet Length']+fields['LLS']['Length Words']*4]
   source={'fields':fields,'source_leaves':leaves}
  ident=identities('integrity')if frame in(3,20,21)else identities('format')if frame==7 else identities()
  full.append({'sourcecase':'authored24/frame-%02d/%s'%(frame,row['name']),'group':'authored24','file':row['original_frame'],'file_SHA256':row['original_frame_SHA256'],'offset':row['PDU_range'][0],'length':row['PDU_range'][1]-row['PDU_range'][0],'rawSHA':row['PDU_SHA256'],'budget':4096,'source':source,'identities':None if frame in(9,23)else ident,'error_required':frame in(3,7,9,20,21,23)})
 sourcep=review/'source-witnesses.json';scenariop=review/'direct-codec-scenarios.json';bound.update({str(x):sha(x)for x in(sourcep,scenariop)});byframe={r['frame']:r for r in read(sourcep)}
 for scenario in read(scenariop):
  row=byframe[scenario['frame']];kind=row['kind'];budget=scenario['budget'];ident=identities('format')
  if kind=='unknown-auth-no-trailer-position':ident=identities('context')
  elif kind=='budget-plus-independent-LLS-integrity':ident=identities('budget','integrity')
  elif kind=='independent-digest-after-hard-body-error':ident=identities('integrity')
  elif budget==1:ident=identities('budget')
  full.append({'sourcecase':'review17/frame-%02d/%s/budget-%d'%(row['frame'],scenario['name'],budget),'group':'review17-18scenarios','file':row['PDU_file'],'file_SHA256':row['PDU_SHA256'],'offset':0,'length':row['PDU_captured_bytes'],'rawSHA':row['PDU_SHA256'],'budget':budget,'source':row['source'],'identities':ident,'error_required':True,'review_kind':kind})
 assert len(full)==598 and len({r['sourcecase']for r in full})==598;inputs=[];facts=[];counts=collections.Counter();checks=0
 for row in full:
  file=Path(row['file']);bound.setdefault(str(file),sha(file));assert bound[str(file)]==row['file_SHA256'];whole=file.read_bytes();w=whole[row['offset']:row['offset']+row['length']];assert len(w)==row['length']and h(w)==row['rawSHA'];src=row['source'];selected=[]
  for leaf in src['source_leaves']:
   path=leaf['path']
   if row.get('review_kind')=='budget-plus-independent-LLS-integrity'and row['budget']==1:
    if path[:2]==['LLS','TLVs']or(path[:2]==['Hello','Neighbors']and path[2]>=1):continue
   if row.get('review_kind')=='complete-LLS-missing-authenticator'and row['budget']==1 and path[:2]==['LLS','TLVs']:continue
   span=leaf['PDU_span'];assert 0<=span[0]<=span[1]<=len(w);raw=w[span[0]:span[1]];observed=value(raw,leaf['encoding']);assert observed==leaf['value'];assert h(raw)==leaf['wire_SHA256'];selected.append({'path':rename(path),'PDU_span':span,'encoding':leaf['encoding'],'value':observed,'wire_SHA256':h(raw)})
  target={'sourcecase':row['sourcecase'],'input_path':str(file),'input_file_SHA256':row['file_SHA256'],'byte_offset':row['offset'],'byte_length':row['length'],'rawSHA':row['rawSHA'],'budget':row['budget']};inputs.append(target)
  meta=metadata(w,src['fields'],selected);fact={'sourcecase':row['sourcecase'],'group':row['group'],'rawSHA':row['rawSHA'],'budget':row['budget'],'wire_leaves':selected,'metadata_checks':meta,'error_required':row['error_required'],'identities':row['identities'],'unknown_auth_no_LLS':row.get('review_kind')=='unknown-auth-no-trailer-position','no_CA_absence_after_budget':row.get('review_kind')=='complete-LLS-missing-authenticator'and row['budget']==1}
  if row.get('review_kind')=='budget-plus-independent-LLS-integrity':
   n=int.from_bytes(w[2:4],'big');fact['owned_residuals']=[{'path':['Unparsed Body'],'PDU_span':[48,n],'bytes':n-48,'SHA256':h(w[48:n])}]
  if fact['unknown_auth_no_LLS']:
   n=int.from_bytes(w[2:4],'big');fact['owned_residuals']=[{'path':['Trailing Bytes'],'PDU_span':[n,len(w)],'bytes':len(w)-n,'SHA256':h(w[n:])}]
  facts.append(fact);counts[row['group']]+=1;counts['expected_wire_leaves']+=len(selected);counts['metadata_checks_separate_not_wire_coverage']+=len(meta)
 for file,data in[(a.out/'probe-input.jsonl',inputs),(a.out/'source-case-facts.jsonl',facts)]:
  with file.open('x',encoding='utf8')as f:
   for row in data:f.write(json.dumps(row,separators=(',',':'))+'\n')
 rules={'scope':__doc__,'probe_input_keys':['sourcecase','input_path','input_file_SHA256','byte_offset','byte_length','rawSHA','budget'],'probe_output_keys':['sourcecase','rawSHA','budget','fields','error','is_context','is_budget','is_integrity','is_format'],'range_contract':'every Wire Field Ranges item is an absolute logical PDU [start,end) span; only its own complete direct scalar/blob; exact expected leaf set, no aliases, dedup, path normalization or invented missing values','field_renames':{'LLS.TLVs[].Type':'TLV Type','TOS Count':'TOS/MT-ID Count','TOS Metrics':'TOS/MT-ID Metrics','TOS':'TOS/MT-ID Selector','Flags and TOS':'Flags and TOS/MT-ID'},'opaque_values':'Go []byte JSON base64; independently decode and compare length/hash and original selected source bytes','metadata_coverage':'metadata is separately checked and never counted as wire leaves','error_policy':'four exact identity bools per pure parent contract; direct short digest/LLS assert only nonempty error because no parent-IP context supplied','budget_policy':'only frozen two budget1 coverage cases; omit unvisited neighbor/TLV leaves; budgeted no-CA cannot assert missing CA; independent bad LLS checksum still joins integrity','no_candidate_output_or_values_read':True,'whole_future_native_events_or_typed_aliases_expected':False};write(a.out/'comparison-rules.json',rules)
 assert bound=={path:sha(path)for path in bound};report={'scope':__doc__,'constructor_SHA256':sha(__file__),'scenarios':len(inputs),'distinct_physical_source_inputs':597,'counts':dict(counts),'probe_input':str(a.out/'probe-input.jsonl'),'probe_input_SHA256':sha(a.out/'probe-input.jsonl'),'source_facts':str(a.out/'source-case-facts.jsonl'),'source_facts_SHA256':sha(a.out/'source-case-facts.jsonl'),'comparison_rules_SHA256':sha(a.out/'comparison-rules.json'),'inputs':bound,'inputs_unchanged':True,'no_candidate_output_read':True,'no_Go_or_repo_write':True,'remaining_source_families_not_covered':['unknown LSA continuation is not newly authored here; root targeted Go controls are separate'],'opaque_or_checksum_math_not_authentication_verified':True};write(a.out/'manifest.json',report);print(json.dumps({'manifest':str(a.out/'manifest.json'),'SHA256':sha(a.out/'manifest.json'),'scenarios':len(inputs),'counts':dict(counts),'probe_input_SHA256':report['probe_input_SHA256'],'source_facts_SHA256':report['source_facts_SHA256']}))
if __name__=='__main__':main()
