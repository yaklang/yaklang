"""Importable source-only GTPv2 API leaf/range checker for frozen30 inputs.
No native implementation or output is used to construct expected values.
Unknown metadata is excluded from coverage; all contracted wire/derived keys are
checked for exact owner, presence, value and literal-only Wire Field Ranges.
"""
import argparse,base64,collections,copy,hashlib,importlib.util,ipaddress,json,sys
from pathlib import Path
WR='Wire Field Ranges'
EP_OTHER={4,5,6,7,*range(25,32),*range(240,245)}
LITERAL={'Flags','Message Type','Message Length','TEID','Sequence Number','Priority or Spare Octet','IE Type','IE Length','Spare and Instance','Recovery Counter','Cause','Cause Flags','Offending IE Type','Offending IE Length','Offending IE Instance Octet','Bearer ID Octet','F-TEID Flags','IPv4 Address','IPv6 Address','Opaque Value','Captured Value'}
DERIVED={'Version','Piggyback','TEID Present','MP Bit Observed','Ignored Header Spare Bits','Message Priority','Ignored Last Octet Spare','Instance','Ignored IE Spare','Offending IE Instance','EPS Bearer ID','Ignored Value Spare','Interface Type'}
RESERVED=LITERAL|DERIVED|{WR,'Messages','Header','IEs','Body','IE Name','Message Name'}

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,d):
 with Path(p).open('x',encoding='utf8')as f:json.dump(d,f,indent=2);f.write('\n')
def encode(w):return base64.b64encode(w).decode('ascii')

class Oracle:
 def __init__(self,raw,budget,policy):
  self.raw=raw;self.tokens=budget;self.initial_budget=budget;self.policy=policy;self.issues=[];self.accepted_IE_nodes=0;self.max_IE_depth=-1
 def issue(self,kind,**kw):self.issues.append({'kind':kind,**kw})
 def u(self,at,n):return int.from_bytes(self.raw[at:at+n],'big')
 def literal(self,row,key,at,n,end,fmt='uint'):
  if not(0<=at<=at+n<=min(end,len(self.raw))):return None
  w=self.raw[at:at+n]
  value=encode(w)if fmt=='bytes'else(str(ipaddress.ip_address(w))if fmt=='address'else int.from_bytes(w,'big'))
  row[key]=value;row.setdefault(WR,{})[key]=[at,at+n];return value
 def octets(self,row,key,start,end):self.literal(row,key,start,end-start,end,'bytes')
 def ies(self,start,captured_end,declared_end,depth):
  rows=[];at=start
  if start==captured_end:return None,False
  if depth>8:self.issue('budget',reason='group-depth',at=start,depth=depth);return None,True
  while at<captured_end:
   if self.tokens<=0:self.issue('budget',reason='IE-count',at=at);return rows or None,True
   self.tokens-=1;self.accepted_IE_nodes+=1;self.max_IE_depth=max(self.max_IE_depth,depth);row={};rows.append(row)
   typ=self.literal(row,'IE Type',at,1,captured_end)
   if typ is not None:row['IE Name']=self.policy['IE_names'].get(str(typ),'Unselected IE type %d'%typ).strip()
   n=self.literal(row,'IE Length',at+1,2,captured_end);si=self.literal(row,'Spare and Instance',at+3,1,captured_end)
   if si is not None:row['Instance']=si&15;row['Ignored IE Spare']=si>>4
   if n is None or si is None:
    self.issue('short'if captured_end<declared_end else'format',reason='IE-header',at=at);return rows,True
   finish=at+4+n;body_start=at+4
   if finish>captured_end:
    self.octets(row,'Captured Value',body_start,captured_end)
    self.issue('format'if finish>declared_end else'short',reason='IE-extent',at=at,declared_end=finish,available_end=captured_end);return rows,True
   if typ==93:
    child,hard=self.ies(body_start,finish,finish,depth+1);row['IEs']=child
    if hard:return rows,True
   else:
    body={};row['Body']=body
    if typ==3 and n==1:self.literal(body,'Recovery Counter',body_start,1,finish)
    elif typ==2 and n in(2,6):
     self.literal(body,'Cause',body_start,1,finish);self.literal(body,'Cause Flags',body_start+1,1,finish)
     if n==6:
      self.literal(body,'Offending IE Type',body_start+2,1,finish);self.literal(body,'Offending IE Length',body_start+3,2,finish);x=self.literal(body,'Offending IE Instance Octet',body_start+5,1,finish);body['Offending IE Instance']=x&15
    elif typ==73 and n==1:
     x=self.literal(body,'Bearer ID Octet',body_start,1,finish);body['EPS Bearer ID']=x&15;body['Ignored Value Spare']=x>>4
    elif typ==87 and n>=5 and n==5+4*bool(self.raw[body_start]&128)+16*bool(self.raw[body_start]&64):
     f=self.literal(body,'F-TEID Flags',body_start,1,finish);body['Interface Type']=f&63;self.literal(body,'TEID',body_start+1,4,finish);pos=body_start+5
     if f&128:self.literal(body,'IPv4 Address',pos,4,finish,'address');pos+=4
     if f&64:self.literal(body,'IPv6 Address',pos,16,finish,'address')
    else:self.octets(body,'Opaque Value',body_start,finish)
   at=finish
  return rows or None,False
 def build(self):
  top={'Messages':None};messages=[];at=0;index=0
  if not self.raw:self.issue('short',reason='no-first-field');return top
  while at<len(self.raw):
   row={'Header':{}};messages.append(row);h=row['Header'];flags=self.literal(h,'Flags',at,1,len(self.raw));h['Version']=flags>>5
   if flags>>5!=2:self.issue('version',at=at);break
   p=bool(flags&16);t=bool(flags&8);mp=bool(flags&4);h.update({'Piggyback':p,'TEID Present':t,'MP Bit Observed':mp,'Ignored Header Spare Bits':flags&3})
   typ=self.literal(h,'Message Type',at+1,1,len(self.raw));n=self.literal(h,'Message Length',at+2,2,len(self.raw))
   if typ is not None:row['Message Name']=self.policy['message_names'].get(str(typ),'Unselected message type %d'%typ).strip()
   if n is None:self.issue('short',reason='common-header',at=at);break
   declared=at+4+n;limit=min(declared,len(self.raw));header=12 if t else 8
   if t:self.literal(h,'TEID',at+4,4,limit)
   seq=at+(8 if t else 4);self.literal(h,'Sequence Number',seq,3,limit);last=self.literal(h,'Priority or Spare Octet',seq+3,1,limit)
   active_mp=t and mp and typ in self.policy['EPC_types']
   if last is not None:
    h['Ignored Last Octet Spare']=last&15 if active_mp else last
    if active_mp:h['Message Priority']=last>>4
   if declared<at+header:self.issue('format',reason='message-under-header',at=at);break
   if limit<at+header:self.issue('short',reason='conditional-header',at=at);break
   second_bad=index==1 and p
   if second_bad:self.issue('format',reason='second-P1',at=at)
   if declared>len(self.raw):self.issue('short',reason='message-extent',at=at)
   ie_rows,hard=self.ies(at+header,limit,declared,0);row['IEs']=ie_rows
   if hard or declared>len(self.raw)or second_bad:break
   at=declared;index+=1
   if not p:
    if at<len(self.raw):self.issue('format',reason='unindicated-trailer',at=at)
    break
   if at==len(self.raw):self.issue('format',reason='P1-missing-message',at=at);break
   if index>=2:break
  top['Messages']=messages or None;return top

class VerifyError(AssertionError):pass

def metadata_guard(value,path):
 if isinstance(value,dict):
  for k,v in value.items():
   if k in RESERVED:raise VerifyError('contracted field hidden or misplaced under extra metadata at '+str(path+[k]))
   metadata_guard(v,path+[k])
 elif isinstance(value,list):
  for i,v in enumerate(value):metadata_guard(v,path+[i])

def compare(expected,actual,path=None,counts=None):
 path=[]if path is None else path;counts=collections.Counter()if counts is None else counts
 def fail(msg):raise VerifyError(str(path)+': '+msg)
 if isinstance(expected,dict):
  if not isinstance(actual,dict):fail('expected object')
  required=set(expected)-{WR};missing=required-set(actual)
  if missing:fail('missing contracted fields '+str(sorted(missing)))
  for k in set(actual)-set(expected):
   if k==WR:
    if actual[k]not in(None,{}):fail('unexpected nonempty Wire Field Ranges')
   elif k in RESERVED:fail('unexpected contracted field '+k)
   else:metadata_guard(actual[k],path+[k]);counts['unverified_metadata_keys']+=1
  er=expected.get(WR,{})
  if er:
   ar=actual.get(WR)
   if not isinstance(ar,dict)or set(ar)!=set(er):fail('Wire Field Ranges keys differ')
   for k,span in er.items():
    x=ar[k]
    if not isinstance(x,list)or len(x)!=2 or any(type(v)is not int or v<0 for v in x)or x!=span:fail('range mismatch for '+k)
    counts['literal_wire_leaves']+=1
    if span[0]==span[1]:counts['empty_literal_ranges_not_typed_selectable']+=1
  elif WR in actual and actual[WR]not in(None,{}):fail('derived-only owner has nonempty ranges')
  for k,v in expected.items():
   if k!=WR:compare(v,actual[k],path+[k],counts)
 elif isinstance(expected,list):
  if not isinstance(actual,list)or len(actual)!=len(expected):fail('collection length/shape differs')
  for i,(e,a)in enumerate(zip(expected,actual)):compare(e,a,path+[i],counts)
 else:
  if type(actual)is not type(expected)or actual!=expected:fail('value/type differs')
  if path and path[-1]in DERIVED:counts['derived_values']+=1
  elif path and path[-1]in('IE Name','Message Name'):counts['verified_display_names']+=1
 return counts

def expected_case(raw,budget,policy):
 if type(budget)is not int or budget<0:raise VerifyError('budget must be nonnegative integer')
 oracle=Oracle(raw,budget,policy);expected=oracle.build();return expected,oracle

def verify_case(sourcecase,raw,budget,fields,error=None,categories=None,*,policy,check_errors=True):
 expected,oracle=expected_case(raw,budget,policy);counts=compare(expected,fields)
 if check_errors:
  if not isinstance(error,str):raise VerifyError(sourcecase+': error must be string')
  if not isinstance(categories,dict):raise VerifyError(sourcecase+': categories required')
  for key in('is_context','is_budget','is_format'):
   if type(categories.get(key))is not bool:raise VerifyError(sourcecase+': missing/invalid '+key)
  kinds={x['kind']for x in oracle.issues}
  if categories['is_budget']!=('budget'in kinds):raise VerifyError(sourcecase+': budget identity differs from source resource boundary')
  if 'format'in kinds and not categories['is_format']:raise VerifyError(sourcecase+': proven structural format violation not identified')
  if not(kinds&{'short','version','format'})and categories['is_format']:raise VerifyError(sourcecase+': invented structural format violation')
  if kinds and not error:raise VerifyError(sourcecase+': refused/incomplete source requires error')
  if any(categories.values())and not error:raise VerifyError(sourcecase+': error identity without error text')
  if not kinds and error and not categories['is_context']:raise VerifyError(sourcecase+': structurally complete error lacks explicit context limitation')
  if 'budget'not in kinds and categories['is_budget']:raise VerifyError(sourcecase+': invented resource limitation')
  counts['error_contracts']+=1
 counts['accepted_IE_nodes']=oracle.accepted_IE_nodes;return dict(counts)

def policy_from_files(IE_file,message_file):
 ie=read(IE_file)['names'];msg=read(message_file)['names'];epc={int(k)for k,v in msg.items()if int(k)not in EP_OTHER and int(k)not in(0,178)and not v.startswith('Reserved')};return {'IE_names':ie,'message_names':msg,'EPC_types':epc}

def load_plan(path):
 root=Path(path);manifest=read(root/'manifest.json')
 for p,h in manifest['frozen_inputs'].items():
  if sha(p)!=h:raise VerifyError('frozen input changed: '+p)
 inputs=[json.loads(x)for x in(root/'probe-input.jsonl').read_text('utf8').splitlines()];facts={x['sourcecase']:x for x in read(root/'source-cases.json')};policy=policy_from_files(manifest['IE_names_file'],manifest['message_names_file']);return inputs,facts,policy,manifest

def prepare(phase,out):
 assert not out.exists();out.mkdir();(out/'real-PDU').mkdir();p=phase/'GTPv2-boundary-source43-01/plan03';official=phase/'GTP-official-source43-02/official-source-facts.json';old_inputs=[q for q in p.rglob('*')if q.is_file()]+[official,phase/'GTP-official-source43-02/summary.json'];before={str(q):sha(q)for q in old_inputs};assert sha(official)=='6b1fc5ec07ebf0467064754501dbba70fd74656fec8fde17129a373d34a958b8';assert sha(p/'source-grid.json')=='a576ac2e5d6b5128475c217b574e27453558b2742a063fd82c0db4e6875ba0e2';cap=phase/'GTP-official-download43-01/project-upload.pcap';blob=cap.read_bytes();assert sha(cap)=='b62192ea1eba8e3bce2bff600222ce1ac41caf01867f0336a026f161da355030';at=24;frames=[]
 while at<len(blob):
  assert at+16<=len(blob);n=int.from_bytes(blob[at+8:at+12],'little');frames.append(blob[at+16:at+16+n]);at+=16+n
 assert at==len(blob)and len(frames)==6;rows=[];inputs=[]
 for f,wire in zip(read(official),frames):
  assert dg(wire)==f['original_frame_SHA256'];a,b=f['payload_physical_span'];raw=wire[a:b];assert dg(raw)==f['payload_SHA256'];q=out/'real-PDU'/('official-frame%d.bin'%f['frame']);q.write_bytes(raw);sid='official/frame%d'%f['frame'];rows.append({'sourcecase':sid,'kind':'official capture','frame':f['frame'],'domain':f['domain'],'original_capture_SHA256':sha(cap),'raw_path':str(q),'rawSHA':dg(raw),'budget':4096,'depth_limit':8,'source_fact_file':str(official),'source_fact_SHA256':sha(official)})
 for c in read(p/'source-grid.json'):
  for w in c['wires']:
   q=Path(w['raw_file']);assert sha(q)==w['raw_SHA256'];rows.append({'sourcecase':'authored/'+c['sourcecase']+'/wire%d'%w['wire_index'],'kind':'authored public-spec control','original_sourcecase':c['sourcecase'],'wire_index':w['wire_index'],'frame':w['authored_capture_frame'],'domain':w['authored_domain'],'raw_path':str(q),'rawSHA':w['raw_SHA256'],'budget':w['resource_parameters']['IE_budget'],'depth_limit':8,'source_fact_file':str(p/'source-grid.json'),'source_fact_SHA256':sha(p/'source-grid.json')})
 for r in rows:
  q=Path(r['raw_path']);inputs.append({'sourcecase':r['sourcecase'],'input_path':str(q),'byte_offset':0,'byte_length':q.stat().st_size,'rawSHA':r['rawSHA'],'budget':r['budget'],'depth_limit':8})
 assert len(rows)==len(inputs)==30 and len({x['sourcecase']for x in rows})==30;put(out/'source-cases.json',rows)
 with(out/'probe-input.jsonl').open('x',encoding='utf8')as f:
  for r in inputs:f.write(json.dumps(r,separators=(',',':'))+'\n')
 names=phase/'GTPv2-IE-names43-01/IE-type-names.json';msgs=phase/'GTPv2-boundary-source43-01/message-type-names.json';protected=before|{str(q):sha(q)for q in[Path(__file__),cap,names,msgs,out/'source-cases.json',out/'probe-input.jsonl']}|{r['raw_path']:r['rawSHA']for r in rows};assert before=={q:sha(q)for q in before};put(out/'manifest.json',{'frozen_inputs':protected,'IE_names_file':str(names),'message_names_file':str(msgs),'source_cases':30,'real_capture_cases':6,'authored_cases':24,'no_old_fact_changes':True,'contract':'parent pure API names; literal ranges only, shared IE budget, group depth0..8; null/absent collections explicit; names metadata TrimSpace; no candidate/output read','expected_error_categories':'budget identity exact; proven full-parent format requires format; incomplete/version require nonempty error but format/context split is not forced; complete opaque semantic limitation may be context','wire_coverage_excludes_bit_derived_names_or_unverified_metadata':True});print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'input_SHA256':sha(out/'probe-input.jsonl'),'cases':30}))

def selftest(plan,out):
 assert not out.exists();out.mkdir();inputs,facts,policy,manifest=load_plan(plan);expected={};stats=collections.Counter()
 for row in inputs:
  raw=Path(row['input_path']).read_bytes();assert dg(raw)==row['rawSHA'];fields,oracle=expected_case(raw,row['budget'],policy);expected[row['sourcecase']]=fields;kinds={x['kind']for x in oracle.issues};categories={'is_context':bool(kinds&{'short','version'}),'is_budget':'budget'in kinds,'is_format':'format'in kinds};stats.update(verify_case(row['sourcecase'],raw,row['budget'],copy.deepcopy(fields),'source-only synthetic refusal'if kinds else'',categories,policy=policy))
 def pick(part):return next(r for r in inputs if part in r['sourcecase'])
 def run_negative(name,part,mutator):
  row=pick(part);value=copy.deepcopy(expected[row['sourcecase']]);mutator(value,row);raw=Path(row['input_path']).read_bytes()
  try:verify_case(row['sourcecase'],raw,row['budget'],value,policy=policy,check_errors=False)
  except VerifyError:return {'name':name,'rejected':True}
  raise AssertionError('negative mutation accepted: '+name)
 negatives=[]
 negatives.append(run_negative('literal-flags-value','echo-T0-recovery',lambda x,r:x['Messages'][0]['Header'].__setitem__('Flags',0)))
 negatives.append(run_negative('literal-range-offset','echo-T0-recovery',lambda x,r:x['Messages'][0]['Header'][WR].__setitem__('Flags',[1,2])))
 negatives.append(run_negative('derived-bool-is-not-integer','echo-T0-recovery',lambda x,r:x['Messages'][0]['Header'].__setitem__('Piggyback',0)))
 negatives.append(run_negative('derived-field-fake-range','echo-T0-recovery',lambda x,r:x['Messages'][0]['Header'][WR].__setitem__('Version',[0,1])))
 negatives.append(run_negative('T0-phantom-TEID','echo-T0-recovery',lambda x,r:x['Messages'][0]['Header'].__setitem__('TEID',0)))
 negatives.append(run_negative('partial-sequence-default','T1-sequence-two-byte-prefix',lambda x,r:x['Messages'][0]['Header'].__setitem__('Sequence Number',0)))
 negatives.append(run_negative('no-first-field-default','zero-byte-header',lambda x,r:x.__setitem__('Messages',[{'Header':{'Flags':0,WR:{'Flags':[0,1]}}}])))
 negatives.append(run_negative('inactive-priority-default','receive-nonzero-spares',lambda x,r:x['Messages'][0]['Header'].__setitem__('Message Priority',10)))
 negatives.append(run_negative('counter-missing-body-default','capture-short-Recovery',lambda x,r:x['Messages'][0]['IEs'][0].__setitem__('Body',{'Recovery Counter':0,WR:{'Recovery Counter':[12,13]}})))
 def add_second(x,r):
  full,_=expected_case(Path(r['input_path']).read_bytes(),4096,policy);x['Messages'][0]['IEs'].append(full['Messages'][0]['IEs'][1])
 negatives.append(run_negative('budget-second-IE-delivery','IE-count-budget1',add_second))
 def add_deep(x,r):
  node=x['Messages'][0]['IEs'][0]
  for _ in range(8):node=node['IEs'][0]
  node['IEs']=[{'IE Type':93,WR:{'IE Type':[48,49]},'IE Name':'Bearer Context'}]
 negatives.append(run_negative('depth9-child-created','group-depth-budget8',add_deep))
 negatives.append(run_negative('P0-tail-resynchronization','P0-unindicated-trailer',lambda x,r:x['Messages'][0]['IEs'].append({'IE Type':250,WR:{'IE Type':[18,19]},'IE Name':'Unselected IE type 250'})))
 negatives.append(run_negative('third-message-delivered','second-P1-third-message',lambda x,r:x['Messages'].append(copy.deepcopy(expected[pick('echo-T0-recovery')['sourcecase']]['Messages'][0]))))
 negatives.append(run_negative('known-name-mislabeled','echo-T0-recovery',lambda x,r:x['Messages'][0]['IEs'][0].__setitem__('IE Name','Other')))
 def wrong_owner(x,r):x['Messages'][0]['IEs'][0]['Spare and Instance']^=1
 negatives.append(run_negative('literal-instance-octet-mutated','echo-T0-recovery',wrong_owner))
 def peer_inherit(x,r):x['Messages'][0]['IEs'][0]['Body']['Opaque Value']=encode(b'A')
 rowB=next(r for r in inputs if 'same-TEID-distinct-peers/wire2'in r['sourcecase']);value=copy.deepcopy(expected[rowB['sourcecase']]);peer_inherit(value,rowB)
 try:verify_case(rowB['sourcecase'],Path(rowB['input_path']).read_bytes(),rowB['budget'],value,policy=policy,check_errors=False)
 except VerifyError:negatives.append({'name':'same-TEID-cross-peer-body-inheritance','rejected':True})
 else:raise AssertionError('cross-peer inheritance accepted')
 put(out/'negative-controls.json',negatives);put(out/'summary.json',{'plan_manifest_SHA256':sha(plan/'manifest.json'),'checker_source_SHA256':sha(__file__),'source_only_synthetic_selftest':True,'candidate_or_native_output_read':False,'cases':len(inputs),'counts':dict(stats),'negative_controls':len(negatives),'all_negative_controls_rejected':True,'negative_controls_SHA256':sha(out/'negative-controls.json'),'no_source_case_or_capture_changes':True});print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'cases':30,'counts':dict(stats),'negative_controls':len(negatives)}))

def verify_actual(plan,actual,out):
 assert not out.exists();out.mkdir();inputs,facts,policy,manifest=load_plan(plan);rows=[json.loads(s)for s in actual.read_text('utf8').splitlines()if s.strip()];assert len(rows)==len(inputs);stats=collections.Counter();failures=[]
 for source,row in zip(inputs,rows):
  try:
   for k in('sourcecase','rawSHA','budget'):assert row[k]==source[k],k
   raw=Path(source['input_path']).read_bytes();assert dg(raw)==source['rawSHA'];stats.update(verify_case(source['sourcecase'],raw,source['budget'],row['fields'],row['error'],{k:row[k]for k in('is_context','is_budget','is_format')},policy=policy))
  except(Exception)as e:failures.append({'sourcecase':source['sourcecase'],'reason':str(e)})
 put(out/'failures.json',failures);put(out/'summary.json',{'checker_source_SHA256':sha(__file__),'plan_manifest_SHA256':sha(plan/'manifest.json'),'actual':str(actual),'actual_SHA256':sha(actual),'rows':len(rows),'counts':dict(stats),'failures':len(failures),'failures_SHA256':sha(out/'failures.json'),'pass':not failures});print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'failures':len(failures)}));assert not failures

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--prepare',action='store_true');ap.add_argument('--phase',type=Path);ap.add_argument('--plan',type=Path);ap.add_argument('--selftest',action='store_true');ap.add_argument('--actual',type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args()
 if a.prepare:assert a.phase;prepare(a.phase,a.out)
 elif a.selftest:assert a.plan;selftest(a.plan,a.out)
 else:assert a.plan and a.actual;verify_actual(a.plan,a.actual,a.out)
if __name__=='__main__':main()
