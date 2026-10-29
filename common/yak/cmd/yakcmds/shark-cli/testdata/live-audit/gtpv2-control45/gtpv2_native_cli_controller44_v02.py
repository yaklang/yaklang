"""Frozen-source GTPv2 native/CLI controller. Preparation never starts binaries.
Runtime requires explicit clean build receipts supplied by the parent agent.
Protocol values/ranges/names derive from frozen raw wire+source checker, never
from either runtime export. SDK->CLI bridging proves concordance only after the
SDK fields and aliases independently pass those source checks.
"""
import argparse,base64,collections,copy,datetime,hashlib,importlib.util,ipaddress,json,os,re,struct,subprocess,time
from pathlib import Path
WR='Wire Field Ranges'
SPECS=[
 ('gtpv2.flags','Flags','header','unsigned',8,False),('gtpv2.message_type','Message Type','header','unsigned',8,False),('gtpv2.message_length','Message Length','header','unsigned',16,False),('gtpv2.teid','TEID','header','unsigned',32,False),('gtpv2.seq','Sequence Number','header','unsigned',24,False),('gtpv2.priority_or_spare','Priority or Spare Octet','header','unsigned',8,False),
 ('gtpv2.ie_type','IE Type','ie','unsigned',8,False),('gtpv2.ie_len','IE Length','ie','unsigned',16,False),('gtpv2.ie_spare_and_instance','Spare and Instance','ie','unsigned',8,False),('gtpv2.recovery','Recovery Counter','body','unsigned',8,False),('gtpv2.cause','Cause','body','unsigned',8,False),('gtpv2.cause.flags','Cause Flags','body','unsigned',8,False),('gtpv2.offending_ie.type','Offending IE Type','body','unsigned',8,False),('gtpv2.offending_ie.length','Offending IE Length','body','unsigned',16,False),('gtpv2.offending_ie.instance_octet','Offending IE Instance Octet','body','unsigned',8,False),('gtpv2.ebi_octet','Bearer ID Octet','body','unsigned',8,False),('gtpv2.f_teid.flags','F-TEID Flags','body','unsigned',8,False),('gtpv2.f_teid.teid','TEID','body','unsigned',32,False),
 ('gtpv2.version','Version','header','unsigned',3,True),('gtpv2.msg_priority','Message Priority','header','unsigned',4,True),('gtpv2.ie_instance','Instance','ie','unsigned',4,True),('gtpv2.ie_spare','Ignored IE Spare','ie','unsigned',4,True),('gtpv2.ebi','EPS Bearer ID','body','unsigned',4,True),('gtpv2.offending_ie.instance','Offending IE Instance','body','unsigned',4,True),('gtpv2.f_teid.interface_type','Interface Type','body','unsigned',6,True),('gtpv2.p','Piggyback','header','boolean',0,True),('gtpv2.t','TEID Present','header','boolean',0,True),('gtpv2.mp_bit','MP Bit Observed','header','boolean',0,True),('gtpv2.f_teid.ipv4','IPv4 Address','body','address',0,False),('gtpv2.f_teid.ipv6','IPv6 Address','body','address',0,False)]

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def module(path):
 s=importlib.util.spec_from_file_location('frozen_gtpv2_source',path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def records(path):
 blob=Path(path).read_bytes();magic=blob[:4];formats={bytes.fromhex('d4c3b2a1'):('<',1000),bytes.fromhex('4d3cb2a1'):('<',1),bytes.fromhex('a1b2c3d4'):('>',1000),bytes.fromhex('a1b23c4d'):('>',1)};assert magic in formats;order,unit=formats[magic];_,_,_,_,_,link=struct.unpack(order+'HHIIII',blob[4:24]);assert link==1;at=24;rows=[]
 while at<len(blob):
  sec,frac,cap,orig=struct.unpack(order+'IIII',blob[at:at+16]);start=at+16;w=blob[start:start+cap];assert len(w)==cap;rows.append({'number':len(rows)+1,'raw_file_offset':start,'wire':w,'caplen':cap,'origlen':orig,'time_ns':sec*10**9+frac*unit,'link':link,'domain':{'Section':0,'Interface':0,'Encapsulation':''}});at=start+cap
 assert at==len(blob);return rows

def anchor(row,pdu):
 w=row['wire'];assert w[12:14]==b'\x08\0';ip=14;ihl=(w[ip]&15)*4;assert w[ip]>>4==4 and w[ip+9]==17 and not int.from_bytes(w[ip+6:ip+8],'big')&0x3fff;end=ip+int.from_bytes(w[ip+2:ip+4],'big');u=ip+ihl;length=int.from_bytes(w[u+4:u+6],'big');a=u+8;b=min(end,u+length,len(w));assert w[a:b]==pdu
 source=str(ipaddress.IPv4Address(w[ip+12:ip+16]));destination=str(ipaddress.IPv4Address(w[ip+16:ip+20]));sp=int.from_bytes(w[u:u+2],'big');dp=int.from_bytes(w[u+2:u+4],'big');return {k:v for k,v in row.items()if k!='wire'}|{'original_frame_SHA256':dg(w),'PDU_physical_range':[a,b],'PDU_SHA256':dg(pdu),'PDU_length':len(pdu),'IPv4_declared_end':end,'UDP_declared_end':u+length,'UDP_source_port':sp,'UDP_destination_port':dp,'IP_source':source,'IP_destination':destination,'source_endpoint':source+':'+str(sp),'destination_endpoint':destination+':'+str(dp),'carrier_complete':end<=len(w)and u+length<=end and row['caplen']>=row['origlen'],'physical_required_prefix':['Ethernet','IPv4','UDP']}

def scoped(tree,scope):
 messages=tree['Messages']or[]
 if scope=='header':return [m['Header']for m in messages]
 result=[]
 def visit(parent):
  for ie in parent.get('IEs')or[]:
   if scope=='ie':result.append(ie)
   elif 'Body'in ie:result.append(ie['Body'])
   visit(ie)
 for m in messages:visit(m)
 return result

def protocol_aliases(tree):
 result=[]
 for name,key,scope,typ,bits,derived in SPECS:
  for row in scoped(tree,scope):
   if key not in row:continue
   value=copy.deepcopy(row[key]);item={'name':name,'value':value,'origin':'derived'if derived else'pdu'}
   if typ=='address':
    address=ipaddress.ip_address(value);item['value']=str(address.ipv4_mapped)if isinstance(address,ipaddress.IPv6Address)and address.ipv4_mapped is not None else str(address)
   if not derived:
    span=row.get(WR,{}).get(key)
    if not span or span[1]<=span[0]:continue
    item['pdu_range']={'offset':span[0],'length':span[1]-span[0]}
   result.append(item)
 return sorted(result,key=lambda r:r['name'])

def strict(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return set(a)==set(b)and all(strict(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(strict(x,y)for x,y in zip(a,b))
 return a==b

def source_snapshot(snapshot,event,source,expected_aliases):
 assert snapshot['schema_version']==1 and snapshot['pdu']==event['id'];assert strict(snapshot['source'],event['source_bytes']);assert not snapshot.get('truncated',False);values=snapshot['values'];actual=[x for x in values if x['name'].startswith('gtpv2.')];assert strict(actual,expected_aliases),'private protocol alias values/order/origin/ranges differ';by=collections.defaultdict(list)
 for x in values:by[x['name']].append(x)
 expected={'pdu.id':event['id'],'pdu.length':source['PDU_length'],'flow.id':0,'source':source['source_endpoint'],'destination':source['destination_endpoint'],'status':event['status']}
 if event['protocol']:expected['protocol']=event['protocol']
 if 'CompletedIn'in event['source_bytes']:expected['pdu.completed_in']=source['number']
 for key,value in expected.items():
  assert len(by[key])==1 and type(by[key][0]['value'])is type(value)and by[key][0]['value']==value,(key,value)
  item=by[key][0];assert item['origin']=='event'and'pdu_range'not in item
  if key in('pdu.id','pdu.length','flow.id','pdu.completed_in'):assert item.get('exact_value')==str(value)
 for key in('tcp.flow.id','pdu.response_to'):assert not by[key]
 assert [x['value']for x in by['ip.addr']]==[source['IP_source'],source['IP_destination']]
 if event['status']=='decoded':assert snapshot.get('completeness')=='message'and not snapshot.get('incomplete',False)
 else:assert snapshot.get('incomplete')is True and snapshot.get('completeness')=='limited'
 return len(actual)

def prepare(phase,out):
 assert not out.exists();out.mkdir();checker=phase/'gtpv2_api_source_checker43_v02.py';assert sha(checker)=='0a526831d4bd9e2e41faba4b1f519c24cae6f69bbf8361fa19620084de82ef10';mod=module(checker);inputs,facts,policy,oldmanifest=mod.load_plan(phase/'GTPv2-API-source-plan43-01');public=phase/'GTP-official-download43-01/project-upload.pcap';authored=phase/'GTPv2-boundary-source43-01/plan03/authored-controls.pcap';frozen={str(p):sha(p)for p in[Path(__file__),checker,public,authored,phase/'GTPv2-API-source-plan43-01/manifest.json',phase/'GTPv2-API-source-plan43-01/probe-input.jsonl',phase/'GTPv2-API-source-plan43-01/source-cases.json']};source_rows=[]
 for kind,cap in[('official',public),('authored',authored)]:
  for frame in records(cap):
   row=next(f for f in facts.values()if(f['kind']=='official capture')==(kind=='official')and f['frame']==frame['number']);raw=Path(row['raw_path']).read_bytes();assert dg(raw)==row['rawSHA'];tree,oracle=mod.expected_case(raw,4096,policy);a=anchor(frame,raw);source_rows.append({'sourcecase':row['sourcecase'],'capture_key':kind,'capture':str(cap),'capture_SHA256':sha(cap),'PDU_file':row['raw_path'],'direct_budget':row['budget'],'native_budget':4096,'anchor':a,'auto_selected_GTPv2':a['carrier_complete']and not oracle.issues,'explicit_selected_GTPv2':a['carrier_complete'],'source_issue_kinds':sorted({x['kind']for x in oracle.issues}),'source_message_names':[m.get('Message Name')for m in tree['Messages']or[]if'Message Name'in m],'source_expected_literal_leaves':sum(len(r.get(WR,{}))for r in walk_maps(tree)),'source_expected_alias_count':len(protocol_aliases(tree)),'empty_PDU_may_have_no_callback':not raw})
 assert len(source_rows)==30;put(out/'source-input-expectations.json',source_rows)
 contexts=[{'name':'official-auto','capture_key':'official','capture':str(public),'explicit':False},{'name':'authored-auto','capture_key':'authored','capture':str(authored),'explicit':False},{'name':'authored-explicit','capture_key':'authored','capture':str(authored),'explicit':True}];put(out/'contexts.json',contexts)
 schema={'field_schema_version':1,'private_aliases':[{'name':n,'API_key':k,'owner_scope':sc,'type':t,'bits':b,'origin':'derived'if d else'pdu'}for n,k,sc,t,b,d in SPECS],'source_values':'frozen raw and unchanged source checker only; alias registration provides names/types/routing only','event_export':['id','flow','protocol','status','profile','completeness','summary','expert','source_bytes','fields','session','field_values','raw','entry','admission','source','destination','direction','transport','domain','carrier_layers','transaction','response_to'],'CLI_export':'physical rows contain pdus[].field_values and fields=FieldMap; no CLI logical tree is exported','scope':'SDK session and fields checked source per-leaf; CLI physical metadata, names/progressive stack, typed snapshots independently checked then exact SDK/CLI bridge; display render/HEX colors remain separate Windows proof','nullable_empty_payload':'callback zero or one only; if absent this is no field coverage; if explicit callback exists, nondecoded/no default primitives required','true_capture_short':'authored frame14 is incomplete physical UDP and must not acquire GTP identity even explicit; do not substitute direct-codec result for ingress','not_identification':'explicit non-v2/partial is a diagnostic selection, never status decoded or confirmed protocol identity','resource_difference':'native default IE budget4096, depth8; authored case17 has2complete Recovery IE and differs from direct1 by design; preserve old direct facts'};put(out/'schema-and-scope.json',schema)
 repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang');schema_files=['common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events/main.go','common/yak/cmd/yakcmds/shark-cli/command.go','common/yak/cmd/yakcmds/shark-cli/decode.go','common/pcapx/pcaputil/protocol_field_registry.go','common/pcapx/pcaputil/protocol_evidence.go','common/pcapx/pcaputil/protocol_session.go','common/pcapx/pcaputil/protocol_gtpv2_fields.go'];put(out/'schema-source-binding.json',[{'path':str(repo/p),'SHA256':sha(repo/p),'read_scope':'public export/schema/alias metadata, not protocol value oracle'}for p in schema_files]);frozen.update({str(p):sha(p)for p in out.iterdir()if p.is_file()});put(out/'manifest.json',{'frozen_inputs':frozen,'checker':str(checker),'checker_plan':str(phase/'GTPv2-API-source-plan43-01'),'native_default_budget':4096,'depth_limit':8,'contexts':3,'source_cases':30,'no_new_capture_scenarios':True,'source_expectations_read_from_raw_not_candidate':True,'no_Go_build_replay_or_binary_process_started':True});print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'source_expectations_SHA256':sha(out/'source-input-expectations.json'),'auto_selected':sum(r['auto_selected_GTPv2']for r in source_rows),'explicit_selected':sum(r['explicit_selected_GTPv2']for r in source_rows)}))

def walk_maps(x):
 if isinstance(x,dict):
  yield x
  for k,v in x.items():
   if k!=WR:yield from walk_maps(v)
 elif isinstance(x,list):
  for v in x:yield from walk_maps(v)

def load(plan):
 manifest=read(plan/'manifest.json')
 for p,d in manifest['frozen_inputs'].items():assert sha(p)==d,'frozen source changed '+p
 mod=module(manifest['checker']);_,_,policy,_=mod.load_plan(Path(manifest['checker_plan']));return manifest,read(plan/'source-input-expectations.json'),read(plan/'contexts.json'),mod,policy

def jsonlines(p):return [json.loads(x)for x in Path(p).read_bytes().splitlines()if x.strip()]

def verify_exports(plan,context,events_file,cli_file):
 manifest,sources,_,mod,policy=load(plan);sources=[r for r in sources if r['capture_key']==context['capture_key']];events=jsonlines(events_file);cli=jsonlines(cli_file);physical=[r for r in cli if r.get('record_type')!='protocol_publication'];assert len(physical)==len(sources);assert not any(r.get('record_type')=='protocol_publication'for r in cli),'no GTP association/EOF publication expected';byframe=collections.defaultdict(list);ids=set();stats=collections.Counter()
 for e in events:
  assert type(e['id'])is int and e['id']>0 and e['id']not in ids;ids.add(e['id']);refs=e['source_bytes']['PacketRefs'];assert len(refs)==1;byframe[refs[0]['Number']].append(e)
 for source,row in zip(sources,physical):
  a=source['anchor'];frame=a['number'];raw=Path(source['PDU_file']).read_bytes();assert row['number']==frame and row['captured_length']==a['caplen']and row['length']==a['origlen'];assert strict(row['capture_domain'],a['domain']);assert row['source']==a['IP_source']and row['destination']==a['IP_destination'];assert rfc3339_ns(row['time'])==a['time_ns'];layers=row['layers'];assert layers[:3]==a['physical_required_prefix'];assert not row.get('stream_id',0)and not row.get('conversation_id',0)and not row.get('tcp_flow_id',0);assert not row.get('connection_protocol')and not row.get('connection_stack');ev=byframe.pop(frame,[])
  if not raw and not ev:
   assert not row.get('pdus');stats['empty_PDU_no_callback_no_field_coverage']+=1;continue
  assert len(ev)==1,'exactly one native callback for nonempty UDP original';e=ev[0];ref={'Number':frame,'Domain':a['domain']};s=e['source_bytes'];assert strict(e['domain'],a['domain'])and strict(s['PacketRefs'],[ref]);assert s['Kind']=='captured'and s['ParentPDU']==0 and s['ParentPDUs']in(None,[]);assert e['transport']=='udp'and e['carrier_layers']in(None,[]);assert e['source']==a['source_endpoint']and e['destination']==a['destination_endpoint'];assert all(e.get(k,0)==0 for k in('flow','tcp_flow_id','transaction','response_to','direction'));wire=base64.b64decode(e['raw'],validate=True)if e['raw']is not None else b'';assert wire==raw;assert s['SHA256']==(dg(raw)if raw else'');selected=source['explicit_selected_GTPv2']if context['explicit']else source['auto_selected_GTPv2'];pdus=row.get('pdus')or[];assert len(pdus)==1;cp=pdus[0];assert cp['id']==e['id'];assert cp['protocol']==e['protocol']and cp['status']==e['status']and cp['completeness']==e['completeness'];assert cp['response_to']==0;assert strict(cp['byte_source'],s);assert strict(cp['field_values'],e['field_values']);fm=collections.defaultdict(list)
  for v in cp['field_values']['values']:fm[v['name']].append(v['value'])
  assert strict(cp['fields'],dict(fm));stats['native_CLI_snapshot_bridges']+=1
  if selected:
   assert e['protocol']=='gtpv2'and e['profile']=='gtpv2-bounded-UDP-observation'and e['entry']=='GTPv2ControlFields';assert e['admission']==('explicit-decode-as'if context['explicit']else'complete-selected-wire-grammar-and-service-hint');tree,oracle=mod.expected_case(raw,4096,policy);session_counts=mod.verify_case(source['sourcecase'],raw,4096,e['session'],policy=policy,check_errors=False);stats.update({'session_'+k:v for k,v in session_counts.items()});aliases=protocol_aliases(tree);stats['private_aliases']+=source_snapshot(e['field_values'],e,a,aliases)
   for key in('Authentication Verified','Procedure Semantics Verified','Bearer State Inferred'):assert e['session'][key]is False
   if not oracle.issues:
    assert e['status']=='decoded'and e['completeness']=='message';assert not e.get('error')and not e.get('fields_error');field_counts=mod.verify_case(source['sourcecase'],raw,4096,e['fields'],policy=policy,check_errors=False);stats.update({'fields_'+k:v for k,v in field_counts.items()});assert strict(s.get('CompletedIn'),ref);summary='GTPv2-C '+' + '.join(source['source_message_names'])+' · wire observation; bearer/session unverified';assert e['summary']==summary and row['info']==summary;stats['decoded_source_datagrams']+=1
   else:
    assert e['status']!='decoded'and e['fields']is None and e.get('fields_error');assert s.get('CompletedIn')is None;stats['explicit_diagnostic_not_identification']+=1
   if e['status']=='decoded':
    assert row['protocol']=='GTPv2-C'and row.get('protocol_status')=='decoded';assert row.get('protocol_stack')==a['physical_required_prefix']+['GTPv2-C'];stats['progressive_GTP_stacks']+=1
   elif row['protocol']=='GTPv2-C':
    assert row.get('protocol_status')==e['status']and row['protocol_status']!='decoded';stack=row.get('protocol_stack')or[];assert stack[:3]==a['physical_required_prefix']and stack[-1]=='GTPv2-C'and stack.count('GTPv2-C')==1;stats['explicit_diagnostic_GTP_caption_not_wire_confirmation']+=1
   else:assert row['protocol']=='UDP';stats['explicit_diagnostic_retained_transport_caption']+=1
  else:
   assert e['protocol']!='gtpv2'and row['protocol']!='GTPv2-C';assert not any(x['name'].startswith('gtpv2.')for x in e['field_values']['values']);stats['GTP_identity_refusals']+=1
   if not a['carrier_complete']:assert e['status']!='decoded';stats['carrier_refusals']+=1
  stats['physical_sources_verified']+=1
 assert not byframe,'callbacks with no matching source frame';return dict(stats)

def rfc3339_ns(s):
 m=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)',s);assert m;value=datetime.datetime.strptime(m[1],'%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc);ns=int(value.timestamp())*10**9+int((m[2]or'').ljust(9,'0'))
 if m[3]!='Z':ns-=int(m[3][1:3])*3600*10**9*(1 if m[3][0]=='+'else-1)+int(m[3][4:])*60*10**9*(1 if m[3][0]=='+'else-1)
 return ns

def verify_descriptors(path):
 d=read(path);assert d['schema_version']==1;rows={r['name']:r for r in d['fields']if r['name'].startswith('gtpv2.')};assert set(rows)=={r[0]for r in SPECS}
 for name,key,scope,typ,bits,derived in SPECS:
  a=rows[name];assert a['protocol']=='gtpv2'and a['type']==typ and a.get('bits',0)==bits and a['origin']==('derived'if derived else'pdu')
 return len(rows)

def receipt(binary,path,source):
 r=read(path);assert r['source']==source and r['exit_code']==0 and sha(binary)==r['binary_SHA256'];return {'path':str(path),'SHA256':sha(path),'binary':str(binary),'binary_SHA256':sha(binary),'source':source}

def execute(plan,events,yak,events_receipt,yak_receipt,source,out):
 assert not out.exists();out.mkdir();manifest,sources,contexts,mod,policy=load(plan);builds={'events':receipt(events,events_receipt,source),'yak':receipt(yak,yak_receipt,source)};frozen=manifest['frozen_inputs']|{str(q):sha(q)for q in(events,yak,events_receipt,yak_receipt)};runs=[];env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='1GiB');flags=0x08000000 if os.name=='nt'else 0
 for context in contexts:
  folder=out/context['name'];folder.mkdir()
  for kind,binary in [('events',events),('yak',yak)]:
   argv=[str(binary),'replay',context['capture'],'-','--ignore-embedded-secrets']if kind=='events'else[str(binary),'shark','-r',context['capture'],'--json','--ignore-embedded-secrets']
   if context['explicit']:argv+=['udp:2123:gtpv2']if kind=='events'else['--decode-as','udp:2123:gtpv2']
   target=folder/(kind+'.jsonl');stderr=folder/(kind+'.stderr');start=time.monotonic()
   with target.open('xb')as stdout,stderr.open('xb')as err:r=subprocess.run(argv,stdout=stdout,stderr=err,env=env,creationflags=flags,timeout=90)
   rec={'kind':kind,'context':context['name'],'command':argv,'exit_code':r.returncode,'seconds':round(time.monotonic()-start,3),'binary':builds[kind],'capture_SHA256':sha(context['capture']),'export':str(target),'export_SHA256':sha(target),'stderr_SHA256':sha(stderr),'controller_SHA256':sha(__file__)};put(folder/(kind+'-producer.json'),rec);runs.append(rec);assert r.returncode==0
  result=verify_exports(plan,context,folder/'events.jsonl',folder/'yak.jsonl');put(folder/'verification.json',{'counts':result,'source_plan_manifest_SHA256':sha(plan/'manifest.json'),'controller_SHA256':sha(__file__)} )
 argv=[str(yak),'shark','--list-fields','--json'];target=out/'field-descriptors.json';stderr=out/'field-descriptors.stderr'
 with target.open('xb')as stdout,stderr.open('xb')as err:r=subprocess.run(argv,stdout=stdout,stderr=err,env=env,creationflags=flags,timeout=30)
 put(out/'field-descriptors-producer.json',{'command':argv,'returncode':r.returncode,'binary':builds['yak'],'export_SHA256':sha(target),'stderr_SHA256':sha(stderr)});assert r.returncode==0;descriptors=verify_descriptors(target);assert frozen=={p:sha(p)for p in frozen};put(out/'summary.json',{'source_plan_manifest_SHA256':sha(plan/'manifest.json'),'controller_SHA256':sha(__file__),'producer_source':source,'closed_runtime_executions':len(runs)+1,'runs':runs,'registered_GTP_descriptors':descriptors,'all_frozen_inputs_unchanged':True,'contexts':[{'name':c['name'],'verification_SHA256':sha(out/c['name']/'verification.json')}for c in contexts],'no_expected_values_from_runtime':True,'full_SDK_or_CLI_whole_objects_not_claimed':'unknown metadata is not source-verified; explicit source tree/private aliases and physical/progressive fields are verified; no Windows pixels or mutation-proof claim'});print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json')}))

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--prepare',action='store_true');ap.add_argument('--phase',type=Path);ap.add_argument('--plan',type=Path);ap.add_argument('--events',type=Path);ap.add_argument('--yak',type=Path);ap.add_argument('--events-receipt',type=Path);ap.add_argument('--yak-receipt',type=Path);ap.add_argument('--source');ap.add_argument('--out',required=True,type=Path);a=ap.parse_args()
 if a.prepare:assert a.phase;prepare(a.phase,a.out)
 else:assert all([a.plan,a.events,a.yak,a.events_receipt,a.yak_receipt,a.source]);execute(a.plan,a.events,a.yak,a.events_receipt,a.yak_receipt,a.source,a.out)
if __name__=='__main__':main()
