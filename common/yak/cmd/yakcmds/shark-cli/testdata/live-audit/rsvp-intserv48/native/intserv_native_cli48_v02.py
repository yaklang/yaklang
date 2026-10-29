"""External source-only38-frame RSVP Intserv native/CLI plan and verifier.

Only offline prepare/selftest/verify. No Go/build/native/CLI/TShark process is
started. Source49 direct-budget inputs remain immutable; default4096 is a
separate explicit source derivation. Full data/captures/expected stay external.
"""
from pathlib import Path
import argparse,base64,collections,copy,hashlib,importlib.util,json,struct
ROOT=Path(__file__).parent;P=ROOT.parent

def load(p,name):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
S=load(ROOT/'intserv_native_schema48.py','independent_default_Intserv_schema48');L=S.legacy
WR=S.WR

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with Path(p).open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,allow_nan=False);f.write('\n')
def checksum(w):return S.base.checksum(w)

def frame(raw,short):
 # Authored carrier, never misrepresented as a captured public packet.
 original_pdu_length=int.from_bytes(raw[6:8],'big')if short else len(raw)
 assert original_pdu_length>=len(raw)
 ip=struct.pack('!BBHHHBBH4s4s',0x45,0,20+original_pdu_length,1,0,64,46,0,bytes([192,0,2,1]),bytes([192,0,2,2]))
 ip=ip[:10]+struct.pack('!H',checksum(ip))+ip[12:]
 wire=bytes.fromhex('0200000000020200000000010800')+ip+raw
 return wire,34+original_pdu_length

def pcaps(path,pdus):
 with path.open('xb')as f:
  f.write(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
  for i,(raw,short)in enumerate(pdus,1):
   wire,orig=frame(raw,short);f.write(struct.pack('<IIII',1761000000+i,i*100,len(wire),orig));f.write(wire)

def prepare(out):
 assert not out.exists();out.mkdir();(out/'raw').mkdir()
 src=P/'Intserv-source48-01/frozen-plan01';assert sha(src/'manifest.json')=='1b57dd45ed6ea226d62569a1f9c6f06981bc85bef2210d115e30d68fc3e7ad66'
 source49_manifest=read(src/'manifest.json')
 for oldpath,digest in source49_manifest['inputs'].items():assert sha(oldpath)==digest,'source49 original input changed '+oldpath
 assert sha(src/'source-facts.json')==source49_manifest['outputs']['source-facts.json']
 source49=read(src/'source-facts.json');assert len(source49)==49
 for oldrow in source49:assert sha(oldrow['input_path'])==oldrow['raw_SHA256'],'frozen source49 PDU changed '+oldrow['sourcecase']
 assert sha(P/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap')=='aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361'
 originals=read(P/'RSVP-official-source45-01/source-facts.json');assert len(originals)==9
 before_paths=[Path(__file__),ROOT/'intserv_native_schema48.py',P/'rsvp_native_cli_checker46.py',P/'rsvp_boundary_source45.py',P/'Intserv-source48-01/intserv_neutral_source48.py',P/'rsvp_intserv_source46.py',src/'manifest.json',src/'source-facts.json',src/'probe-input.jsonl',P/'RSVP-official-source45-01/source-facts.json']
 before={str(p):sha(p)for p in before_paths}
 authored={}
 for row in source49:
  if row['origin']=='authored':authored.setdefault(row['metadata']['authored_scene'],row)
 assert len(authored)==29
 authored_rows=list(authored.items());capture=out/'authored-default4096.pcap'
 pcaps(capture,[(Path(r['input_path']).read_bytes(),r['metadata']['capture_short'])for _,r in authored_rows])
 caps={'official':P/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap','authored':capture};rows=[];deltas=[];default_neutral=[]
 for kind,cap in caps.items():
  before[str(cap)]=sha(cap)
  for f in L.records(cap):
   if kind=='official':
    old=next(x for x in originals if x['frame']==f['number']);raw=Path(old['PDU_file']).read_bytes();assert hashlib.sha256(raw).hexdigest()==old['PDU_SHA256'];case='Intserv-official-frame'+str(f['number']);oldtargets=[r for r in source49 if r['origin']=='real'and r['metadata']['frame']==f['number']]
   else:
    scene,old=authored_rows[f['number']-1];raw=Path(old['input_path']).read_bytes();case=scene;oldtargets=[r for r in source49 if r['origin']=='authored'and r['metadata']['authored_scene']==scene]
   path=out/'raw'/(kind+'-'+str(f['number'])+'.pdu');path.write_bytes(raw);a=L.anchor(f,raw);expected=S.compose(raw,oldtargets)
   cats={key:any(x['kind']==value for x in expected['issues'])for key,value in[('is_format','format'),('is_integrity','integrity'),('is_context','context'),('is_budget','budget')]}
   c={'fields':expected['fields'],'raw_SHA256':hashlib.sha256(raw).hexdigest(),'required_categories':cats,'shortage':any(x['kind']=='need_more'for x in expected['issues'])}
   status,parent_format=L.status_from_source(c,a,raw)
   row={'sourcecase':case,'capture_key':kind,'capture':str(cap),'capture_SHA256':sha(cap),'raw_path':str(path),'raw_SHA256':sha(path),'default_global_budget':4096,'anchor':a,'expected_status':status,'independent_complete_parent_wrong_length':parent_format,'source_case':c,'composed_source':expected,'aliases':S.aliases(expected['fields'],raw)};rows.append(row)
   for target in expected['nested']:
    default_neutral.append({'sourcecase':case,'object_index':target['object_index'],'body_start':target['body_start'],'object_end':target['object_end'],'source':target['source'],'default_global_budget':4096,'neutral_remaining_budget_is_not_global_API_remaining':True})
    oldmatches=[r for r in oldtargets if r['target_object_body_start']==target['body_start']]
    assert oldmatches
    for previous in oldmatches:
     assert previous['raw_SHA256']==hashlib.sha256(raw).hexdigest()
     assert previous['target_object_declared_end']==target['object_end']and previous['object_class']==target['source']['fields']['Object Class']
     same=S.neutral.equal(previous['source']['fields'],target['source']['fields'])
     if previous['probe_global_budget']==4096:assert same,'default source field tree unexpectedly changed'
     deltas.append({'old_sourcecase':previous['sourcecase'],'default_sourcecase':case,'old_probe_global_budget':previous['probe_global_budget'],'default_global_budget':4096,'raw_bytes_and_SHA_identical':True,'wire_field_tree_equal':same,'reason':'Default wholePDU budget4096; shared budget case has one physical record instead of five direct evaluations. No new packet/value oracle.'})
 assert len(rows)==38 and len(default_neutral)==45
 contract=[{'name':n,'API_key':k,'scope':s,'type':t,'bits':b,'origin':'derived'if d else'pdu'}for n,k,s,t,b,d in S.SPECS]
 put(out/'source-rows.json',rows);put(out/'default4096-neutral-facts.json',default_neutral);put(out/'direct49-to-default38-deltas.json',deltas);put(out/'alias-contract65.json',contract)
 put(out/'runtime-command-plan.json',{'commands':[{'context':kind,'events_args':['replay',str(cap),'-','--ignore-embedded-secrets'],'CLI_args':['shark','-r',str(cap),'--json','--ignore-embedded-secrets']}for kind,cap in caps.items()],'field_descriptors_args':['shark','--list-fields','--json'],'native_budget':4096,'execution':'Only parent executes closed exact-source builds in fresh output directories; this controller starts no process.'})
 put(out/'schema-and-scope.json',{'physical_frames':38,'official':9,'authored':29,'Intserv_object_subtrees':45,'direct49_budget_matrix_not_physical_inputs':True,'default_budget':4096,'source64bit_event_aliases_exact_value':True,'private_aliases':65,'unknown_service_or_parameter':'wire envelope complete can decode, but snapshot limited and Field Layouts Complete false','semantic_value_issues':'preserve wire decoded state and fields; no verified reservation/installed path inferred','true_snaplen':'captured length < original length; original_IP_extent from independently authored carrier; PDU common Length remains original44 for nine short controls','complete_parent':'inner word/parameter contradictions independently format, never reclassified as snaplen shortage','no_object_or_parameter_connection_state':'flow/TCP/transaction/response/direction zero; advertised endpoints never replace physical carrier','source_layer':'captured PDU bytes/SHA+actual single frame ref; logical RSVP appended once after actual physical IP chain','metadata_limits':'checks source fields/ranges, aliases, SDK-to-CLI snapshot and physical rows; no claim of whole unknown event metadata, Windows pixels, procedure completeness or installed Intserv state','source_only_no_candidate_runtime_read':True})
 metadata=P/'rsvp_intserv_API48.py';repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
 put(out/'schema-source-binding.json',{'source_to_API_naming_reference':{'path':str(metadata),'SHA256':sha(metadata),'used_for':'names/types/shape only, no candidate runtime values'},'Go_field_registry':{'path':str(repo/'common/pcapx/pcaputil/protocol_rsvp_fields.go'),'SHA256':sha(repo/'common/pcapx/pcaputil/protocol_rsvp_fields.go'),'used_for':'descriptor schema only; not an immutable future production source requirement'}})
 assert before=={p:sha(p)for p in before}
 frozen=before|{str(p):sha(p)for p in out.rglob('*')if p.is_file()}
 put(out/'manifest.json',{'frozen_inputs':frozen,'source49_manifest_SHA256':sha(src/'manifest.json'),'controller_SHA256':sha(__file__),'source_rows_SHA256':sha(out/'source-rows.json'),'schema_source_SHA256':sha(ROOT/'intserv_native_schema48.py'),'source_rows':38,'Intserv_objects':45,'registered_aliases':65,'no_candidate_runtime_read_or_Go_replay_started':True})
 print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'source_rows_SHA256':sha(out/'source-rows.json'),'physical_frames':len(rows),'Intserv_objects':len(default_neutral),'private_alias_occurrences':sum(len(r['aliases'])for r in rows),'statuses':dict(collections.Counter(r['expected_status']for r in rows))}))

def load_plan(plan):
 m=read(plan/'manifest.json')
 for p,h in m['frozen_inputs'].items():assert sha(p)==h,'frozen source changed '+p
 assert sha(__file__)==m['controller_SHA256']and sha(ROOT/'intserv_native_schema48.py')==m['schema_source_SHA256']
 return m,read(plan/'source-rows.json')

def walk(tree,path=''):
 if isinstance(tree,dict):
  yield path,tree
  for k,v in tree.items():
   if k!=WR:yield from walk(v,path+'.'+k if path else k)
 elif isinstance(tree,list):
  for i,v in enumerate(tree):yield from walk(v,path+'['+str(i)+']')

def get(tree,path):
 tokens=S.parts(path)
 if tokens and tokens[0]=='Root':tokens=tokens[1:]
 for t in tokens:tree=tree[t]
 return tree

def check_fields(row,raw,actual):
 expected=row['source_case']['fields'];wanted=S.inventory(expected);observed=S.inventory(actual);assert set(wanted)==set(observed),('literal range inventory differs',set(wanted)-set(observed),set(observed)-set(wanted));stats=collections.Counter()
 for path,(value,span)in wanted.items():
  got,rng=observed[path];assert L.strict(span,rng)and 0<=rng[0]<=rng[1]<=len(raw),('source range',path)
  if isinstance(value,dict):
   assert isinstance(got,dict)
   for k in('IEEE754 Bits','Value','Classification','Units'):assert L.strict(got[k],value[k]),('float',path,k)
  else:assert L.strict(value,got),('literal value',path,value,got)
  stats['API_literal_range_occurrences']+=1
 # Every source-derived scalar has no fabricated independent octet range.
 known={spec[1]for spec in S.SPECS}|S.FLOATS|{'Version and Unused','Message Length Words','Data Length Words','Opaque Service Data','Opaque Parameter Data','Unparsed Parameter Data','Unparsed Service Data','Unparsed Intserv Data','Opaque Intserv Data','Trailing Intserv Bytes','Unparsed Body','Opaque Body'}
 for path,node in walk(actual):
  try:want=get(expected,path)if path else expected
  except(KeyError,IndexError,TypeError):want={}
  for name,value in node.items():
   if name in known:assert isinstance(want,dict)and name in want,('invented/mis-owned primitive',path,name)
   if name in('Authentication Verified','Reservation State Inferred','Procedure Semantics Verified'):assert value is False
 for path,node in walk(expected):
  for key,value in node.items():
   if key==WR or isinstance(value,(dict,list))or key in node.get(WR,{}):continue
   got=get(actual,path+'.'+key if path else key)
   if key in known:assert L.strict(value,got)and key not in get(actual,path or'Root').get(WR,{}),('derived value/range',path,key);stats['derived_scalar_observations']+=1
  for key in('Objects','Services','Parameters','Addresses'):
   if key in node:
    actual_node=get(actual,path)if path else actual;assert len(node[key]or[])==len(actual_node.get(key)or[]),('collection',path,key)
 for key in('Wire Grammar Complete','Field Layouts Complete','Checksum Omitted','Checksum Verified'):assert actual.get(key)is expected.get(key),('completeness/checksum',key)
 wanted_issues=set()
 for item in row['composed_source']['semantic_issues']:
  issue=item['issue'];part=issue['path'].rsplit('.',1);owner='Objects['+str(item['object_index'])+'].Body.'+S.mapped(part[0]);wanted_issues.add((owner,part[1],S.semantic_code(issue['code'],part[1])))
 got_issues={(path,x['Field'],x['Code'])for path,node in walk(actual)for x in node.get('Observed Value Domain Issues',[])}
 assert got_issues==wanted_issues,('semantic value issues differ',wanted_issues,got_issues)
 stats['source_semantic_issue_occurrences']=len(wanted_issues)
 return stats

def verify_rows(sources,events,cli):
 assert len(events)==len(cli)==len(sources),'one callback/physical row each; no invented session/EOF publication'
 byframe={};ids=set();stats=collections.Counter()
 for e in events:
  assert type(e['id'])is int and e['id']>0 and e['id']not in ids;ids.add(e['id']);refs=e['source_bytes']['PacketRefs'];assert len(refs)==1 and refs[0]['Number']not in byframe;byframe[refs[0]['Number']]=e
 for row,c in zip(sources,cli):
  a=row['anchor'];raw=Path(row['raw_path']).read_bytes();assert sha(row['raw_path'])==row['raw_SHA256'];e=byframe.pop(a['number']);status=row['expected_status']
  assert(e['protocol'],e['transport'],e['profile'],e['entry'])==('rsvp','network','rsvp-bounded-control','RSVPControlFields')
  assert e['status']==status and e['completeness']==('message'if status=='decoded'else status)
  assert e['admission']==('observed-IP-next-header46-and-complete-base-wire'if status=='decoded'else'observed-IP-next-header46')
  assert e['source']==a['source']and e['destination']==a['destination']and L.strict(e['domain'],a['domain'])
  assert all(e.get(k,0)==0 for k in('flow','tcp_flow_id','transaction','response_to','direction'));assert e['carrier_layers']in(None,[])
  assert base64.b64decode(e['raw'],validate=True)==raw
  source=e['source_bytes'];assert source['Kind']=='captured'and source['ParentPDU']==0 and source['ParentPDUs']in(None,[])and source['SHA256']==row['raw_SHA256']and L.strict(source['PacketRefs'],[{'Number':a['number'],'Domain':a['domain']}])
  stats.update({'session_'+k:v for k,v in check_fields(row,raw,e['session']).items()})
  if status=='decoded':
   assert not e.get('error')and not e.get('fields_error');stats.update({'fields_'+k:v for k,v in check_fields(row,raw,e['fields']).items()});assert e['summary']=='RSVP '+L.NAMES[raw[1]]+' · advertised selectors; reservation/authentication unverified'
  else:assert e.get('error')and e['fields']is None and e.get('fields_error')
  stats['private_alias_occurrences']+=L.check_snapshot(e['field_values'],e,row)
  assert c.get('record_type')!='protocol_publication'and c['number']==a['number']and c['captured_length']==a['caplen']and c['length']==a['origlen']and L.time_ns(c['time'])==a['time_ns']
  assert L.strict(c['capture_domain'],a['domain'])and c['source']==a['source']and c['destination']==a['destination']
  assert c.get('transport','')in('','network')and c.get('source_port',0)==0 and c.get('destination_port',0)==0
  assert all(c.get(k,0)==0 for k in('tcp_flow_id','stream_id','conversation_id'))
  assert not any(c.get(k)for k in('connection_protocol','connection_stack','connection_evidence'))
  for k,v in[('physical_source',a['source']),('physical_destination',a['destination']),('message_source',a['source']),('message_destination',a['destination'])]:
   if c.get(k):assert c[k]==v
  assert c.get('message_transport','')in('','network')
  assert c['layers']==a['physical_headers']+(['RSVP']if a['capture_RSVP_layer']else[])
  assert c['protocol']=='RSVP'and c['protocol_status']==status and c['protocol_stack']==a['physical_headers']+['RSVP']and c['protocol_stack'].count('RSVP')==1
  if status=='decoded':assert c['info']==e['summary']
  else:assert c.get('info')
  if a['real_snaplen_short']:assert'truncated capture'in c['info']
  assert len(c.get('pdus')or[])==1;pdu=c['pdus'][0]
  assert pdu['id']==e['id']and pdu['protocol']=='rsvp'and pdu['status']==status and pdu['completeness']==e['completeness']and pdu['response_to']==0
  assert L.strict(pdu['byte_source'],source)and L.strict(pdu['field_values'],e['field_values'])
  fm=collections.defaultdict(list)
  for v in pdu['field_values']['values']:fm[v['name']].append(v['value'])
  assert L.strict(pdu['fields'],dict(fm));stats['SDK_CLI_private_snapshot_bridges']+=1;stats['physical_sources_progressive_stacks']+=1;stats['status_'+status]+=1
 assert not byframe
 return stats

def descriptors(path):
 data=read(path);assert data['schema_version']==1;actual={d['name']:d for d in data['fields']if d['name'].startswith('rsvp.')};assert set(actual)=={s[0]for s in S.SPECS}
 for n,k,s,t,b,d in S.SPECS:
  a=actual[n];assert(a['protocol'],a['type'],a.get('bits',0),a['origin'])==('rsvp',t,b,'derived'if d else'pdu')
 return len(actual)

def mock(sources):
 copied=copy.deepcopy(sources)
 for row in copied:
  tree=row['source_case']['fields']
  for item in row['composed_source']['semantic_issues']:
   issue=item['issue'];owner,field=issue['path'].rsplit('.',1);node=get(tree,'Objects['+str(item['object_index'])+'].Body.'+S.mapped(owner));node.setdefault('Observed Value Domain Issues',[]).append({'Field':field,'Code':S.semantic_code(issue['code'],field)})
 return L.mock_rows(copied)

def selftest(plan,out):
 assert not out.exists();out.mkdir();m,rows=load_plan(plan);totals=collections.Counter();contexts={}
 for context in('official','authored'):
  subset=[r for r in rows if r['capture_key']==context];es,cs=mock(subset);count=verify_rows(subset,es,cs);totals.update(count);contexts[context]=dict(count)
 authored=[r for r in rows if r['capture_key']=='authored'];events,cli=mock(authored);neg=[]
 def reject(label,change):
  es=copy.deepcopy(events);cs=copy.deepcopy(cli);change(es,cs)
  try:verify_rows(authored,es,cs)
  except(AssertionError,KeyError,IndexError,TypeError,ValueError):neg.append(label)
  else:raise AssertionError('negative accepted '+label)
 partial=next(i for i,r in enumerate(authored)if r['sourcecase']=='float-prefix-three-bytes');nan=next(i for i,r in enumerate(authored)if r['sourcecase']=='b-nan-retain-payload-bits');unknown=next(i for i,r in enumerate(authored)if r['sourcecase']=='unknown-parameter-before-known')
 reject('invent short float zero',lambda es,cs:es[partial]['session']['Objects'][0]['Body']['Services'][0]['Parameters'][0]['Body'].__setitem__('Token Bucket Rate',0))
 reject('lose NaN payload bits',lambda es,cs:es[nan]['session']['Objects'][0]['Body']['Services'][0]['Parameters'][0]['Body']['Token Bucket Size'].__setitem__('IEEE754 Bits',0x7fc00000))
 reject('claim unknown parameter layout complete',lambda es,cs:es[unknown]['session'].__setitem__('Field Layouts Complete',True))
 reject('sticky flow identity',lambda es,cs:es[-1].__setitem__('flow',3))
 reject('advertised selector as captured endpoint',lambda es,cs:cs[-1].__setitem__('source','198.51.100.7:5000'))
 reject('partial caption drops to IPv4',lambda es,cs:cs[partial].__setitem__('protocol','IPv4'))
 reject('fake physical RSVP for snaplen',lambda es,cs:cs[partial]['layers'].append('RSVP'))
 reject('SDK CLI value bridge mismatch',lambda es,cs:cs[-1]['pdus'][0]['field_values'].__setitem__('pdu',9999))
 put(out/'summary.json',{'plan_SHA256':sha(plan/'manifest.json'),'counts':dict(totals),'contexts':contexts,'negative_controls_rejected':neg,'real_candidate_runtime_not_executed':True,'old49_sources_unchanged':True})
 print(json.dumps({'selftest':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':dict(totals),'negative_controls':len(neg)}))

def verify(plan,context,events,cli,fields,out):
 assert not out.exists();out.mkdir();m,rows=load_plan(plan);subset=[r for r in rows if r['capture_key']==context];assert subset
 inputs={str(p):sha(p)for p in[events,cli,fields,Path(__file__),plan/'manifest.json']}
 es=[json.loads(l)for l in events.read_bytes().splitlines()if l.strip()];cs=[json.loads(l)for l in cli.read_bytes().splitlines()if l.strip()];counts=verify_rows(subset,es,cs);counts['registered_RSVP_descriptors']=descriptors(fields)
 assert inputs=={p:sha(p)for p in inputs};load_plan(plan)
 put(out/'summary.json',{'context':context,'plan_SHA256':sha(plan/'manifest.json'),'inputs':inputs,'counts':dict(counts),'future_source_binding_owned_by_parent_closed_producer':True,'no_runtime_values_used_to_create_expected':True,'whole_unselected_metadata_or_Windows_not_claimed':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':dict(counts)}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=['prepare','selftest','verify']);p.add_argument('--plan',type=Path);p.add_argument('--context',choices=['official','authored']);p.add_argument('--events',type=Path);p.add_argument('--CLI',type=Path);p.add_argument('--descriptors',type=Path);p.add_argument('--out',type=Path,required=True);a=p.parse_args()
 if a.mode=='prepare':prepare(a.out)
 elif a.mode=='selftest':assert a.plan;selftest(a.plan,a.out)
 else:assert a.plan and a.context and a.events and a.CLI and a.descriptors;verify(a.plan,a.context,a.events,a.CLI,a.descriptors,a.out)
if __name__=='__main__':main()