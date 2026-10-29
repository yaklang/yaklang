"""Independent RSVP native/CLI source preparation and export verifier.

Preparation starts no binaries. Protocol values/ranges come only from frozen
source45 facts and their pure Python source model. Go files are bound only for
public alias/export-schema metadata. Current/future runtime values are never
an expected-value oracle. Full plans/captures/expected trees stay external.
"""
import argparse,base64,collections,copy,datetime,hashlib,importlib.util,ipaddress,json,re,struct
from pathlib import Path
WR='Wire Field Ranges'
SPECS=[
 ('rsvp.version_and_flags','Version and Flags','header','unsigned',8,False),('rsvp.msg','Message Type','header','unsigned',8,False),('rsvp.message_checksum','Checksum','header','unsigned',16,False),('rsvp.sending_ttl','Send TTL','header','unsigned',8,False),('rsvp.reserved','Reserved','header','unsigned',8,False),('rsvp.message_length','Message Length','header','unsigned',16,False),
 ('rsvp.length','Length','object','unsigned',16,False),('rsvp.object','Class','object','unsigned',8,False),('rsvp.ctype','C-Type','object','unsigned',8,False),
 ('rsvp.session.proto','Protocol ID','session','unsigned',8,False),('rsvp.session.flags','Flags','session','unsigned',8,False),('rsvp.session.port','Destination Port','session','unsigned',16,False),('rsvp.hop.logical_interface','Logical Interface Handle','hop','unsigned',32,False),('rsvp.refresh_interval','Refresh Period Milliseconds','time','unsigned',32,False),
 ('rsvp.error_flags','Flags','error','unsigned',8,False),('rsvp.error.error_code','Error Code','error','unsigned',8,False),('rsvp.error_value','Error Value','error','unsigned',16,False),('rsvp.style.flags','Flags','style','unsigned',8,False),('rsvp.style.style','Option Vector','style','unsigned',24,False),('rsvp.filter.unused','Unused','filter','unsigned',16,False),('rsvp.filter.port','Source Port','filter','unsigned',16,False),('rsvp.sender.unused','Unused','sender','unsigned',16,False),('rsvp.sender.port','Source Port','sender','unsigned',16,False),
 ('rsvp.version','Version','header','unsigned',4,True),('rsvp.flags','Flags','header','unsigned',4,True),
 ('rsvp.session.ip','Destination Address','session','address',0,False),('rsvp.hop.neighbor_address','Hop Address','hop','address',0,False),('rsvp.error.error_node','Error Node Address','error','address',0,False),('rsvp.filter.ip','Source Address','filter','address',0,False),('rsvp.sender.ip','Source Address','sender','address',0,False),('rsvp.confirm.receiver_address','Receiver Address','confirm','address',0,False),('rsvp.scope.address','Address','scope','address',0,False),
 ('rsvp.checksum_omitted','Checksum Omitted','root','boolean',0,True),('rsvp.checksum_verified','Checksum Verified','root','boolean',0,True)]
SCOPES={'session':1,'hop':3,'time':5,'error':6,'scope':7,'style':8,'filter':10,'sender':11,'confirm':15}
NAMES={1:'Path',2:'Resv',3:'PathErr',4:'ResvErr',5:'PathTear',6:'ResvTear',7:'ResvConf'}


def dg(w):return hashlib.sha256(w).hexdigest()
def sha(p):return dg(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):Path(p).write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def module(path,name):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def strict(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return set(a)==set(b)and all(strict(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(strict(x,y)for x,y in zip(a,b))
 return a==b

def records(path):
 w=Path(path).read_bytes();variants={bytes.fromhex('d4c3b2a1'):('<',1000),bytes.fromhex('a1b2c3d4'):('>',1000),bytes.fromhex('4d3cb2a1'):('<',1),bytes.fromhex('a1b23c4d'):('>',1)}
 endian,unit=variants[w[:4]];major,minor,zone,sig,snap,link=struct.unpack_from(endian+'HHiIII',w,4);assert(major,minor,link)==(2,4,1)
 pos=24;out=[]
 while pos<len(w):
  sec,frac,cap,orig=struct.unpack_from(endian+'IIII',w,pos);start=pos+16;wire=w[start:start+cap];assert len(wire)==cap<=snap
  out.append({'number':len(out)+1,'wire':wire,'caplen':cap,'origlen':orig,'time_ns':sec*10**9+frac*unit,'domain':{'Section':0,'Interface':0,'Encapsulation':''}});pos=start+cap
 assert pos==len(w);return out

def anchor(frame,pdu):
 w=frame['wire'];typ=int.from_bytes(w[12:14],'big');ip=14
 if typ==2048:
  width=(w[ip]&15)*4;assert w[ip]>>4==4 and width>=20 and w[ip+9]==46 and not int.from_bytes(w[ip+6:ip+8],'big')&0x3fff
  end=ip+int.from_bytes(w[ip+2:ip+4],'big');src=str(ipaddress.ip_address(w[ip+12:ip+16]));dst=str(ipaddress.ip_address(w[ip+16:ip+20]));layer='IPv4'
 else:
  assert typ==34525 and w[ip]>>4==6 and w[ip+6]==46
  width=40;end=ip+40+int.from_bytes(w[ip+4:ip+6],'big');src=str(ipaddress.ip_address(w[ip+8:ip+24]));dst=str(ipaddress.ip_address(w[ip+24:ip+40]));layer='IPv6'
 start=ip+width;stop=min(end,len(w));assert w[start:stop]==pdu
 short=frame['caplen']<frame['origlen'];assert end<=len(w)or short
 return{k:v for k,v in frame.items()if k!='wire'}|{'physical_SHA256':dg(w),'PDU_SHA256':dg(pdu),'PDU_length':len(pdu),'PDU_physical_range':[start,stop],'IP_declared_end':end,'parent_complete':end<=len(w),'real_snaplen_short':short,'source':src,'destination':dst,'IP_layer':layer,'physical_headers':['Ethernet',layer],'capture_RSVP_layer':end<=len(w)}

def scoped(tree,scope):
 if scope=='root':return[tree]
 if scope=='header':return[tree['Header']]if tree.get('Header')else[]
 out=[]
 for row in tree.get('Objects')or[]:
  if scope=='object':out.append(row.get('Header')or{});continue
  if row.get('Header',{}).get('Class')!=SCOPES[scope]:continue
  body=row.get('Body')or{}
  if scope=='scope':out.extend(body.get('Addresses')or[])
  else:out.append(body)
 return out

def aliases(tree,raw):
 out=[]
 for name,key,scope,typ,bits,derived in SPECS:
  # Frozen public policy: checksum state aliases require the complete fixed
  # common8B header. Missing Checksum/Length is unknown, never default false.
  if name in('rsvp.checksum_omitted','rsvp.checksum_verified')and len(raw)<8:continue
  for row in scoped(tree,scope):
   if key not in row or row[key]is None:continue
   value=copy.deepcopy(row[key]);item={'name':name,'value':value,'origin':'derived'if derived else'pdu'}
   if typ=='address':
    a=ipaddress.ip_address(value);item['value']=str(a.ipv4_mapped)if isinstance(a,ipaddress.IPv6Address)and a.ipv4_mapped else str(a)
   if not derived:
    span=row.get(WR,{}).get(key)
    if not span or span[1]<=span[0]:continue
    item['pdu_range']={'offset':span[0],'length':span[1]-span[0]}
   out.append(item)
 return sorted(out,key=lambda row:row['name'])

def status_from_source(case,a,raw):
 cats=case['required_categories'];selected_v1=bool(raw)and raw[0]>>4==1
 independent_parent_format=a['parent_complete']and not a['real_snaplen_short']and selected_v1 and(len(raw)<8 or int.from_bytes(raw[6:8],'big')>len(raw))
 if cats['is_format']or cats['is_integrity']or independent_parent_format:return'malformed',independent_parent_format
 if cats['is_budget']:return'limited',False
 if cats['is_context']:return'context-required',False
 if case['shortage']:return('incomplete'if a['real_snaplen_short']else'malformed'),False
 return'decoded',False

def prepare(phase,out):
 assert not out.exists();out.mkdir()
 apipath=phase/'rsvp_direct_api_checker46_v02.py';modelpath=phase/'rsvp_boundary_source45.py';apiplan=phase/'RSVP-direct-API-source-plan46-02'
 assert sha(apipath)=='bcb932e9499d55ae2c5538c7e425fa4755d62faea993c2171a38e32fb11b6e1d'
 api=module(apipath,'rsvp_api_frozen');old,_=api.load_plan(apiplan);model=module(modelpath,'rsvp_model_frozen')
 caps={'official':phase/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap','authored':phase/'RSVP-boundary-source45-01/authored-controls.pcap'}
 inputs=[Path(__file__),apipath,modelpath,apiplan/'manifest.json',apiplan/'source-cases.json',apiplan/'probe-input.jsonl']+list(caps.values())
 rows=[];default_deltas=[]
 for kind,cap in caps.items():
  for frame in records(cap):
   key='RSVP-official-frame'+str(frame['number'])if kind=='official'else next(key for key in old if key.startswith('RSVP-authored-')and key.endswith('-frame'+str(frame['number']).zfill(2)))
   base=old[key];rawpath=Path(base['raw_path']);raw=rawpath.read_bytes();inputs.append(rawpath);a=anchor(frame,raw)
   if base['budget']==4096:case=copy.deepcopy(base)
   else:
    src=model.decode_source(raw,4096);case=api.expected_row(key,rawpath,4096,src,base['kind'],a['parent_complete']);default_deltas.append({'sourcecase':key,'direct_budget':base['budget'],'native_budget':4096,'reason':'same frozen original raw; existing pure source model independently evaluated before native runtime; no packet/value correction'})
   status,parent_format=status_from_source(case,a,raw)
   row={'sourcecase':key,'capture_key':kind,'capture':str(cap),'capture_SHA256':sha(cap),'raw_path':str(rawpath),'raw_SHA256':dg(raw),'default_budget':4096,'anchor':a,'expected_status':status,'independent_full_parent_format':parent_format,'source_case':case,'aliases':aliases(case['fields'],raw)}
   rows.append(row)
 assert len(rows)==33 and len(default_deltas)==3
 # Typed/schema-only source binding. These repository files are not runtime
 # immutable inputs: root will fix partial presentation after this plan freezes.
 repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
 metadata_files=['common/pcapx/pcaputil/protocol_rsvp_fields.go','common/pcapx/pcaputil/protocol_rsvp.go','common/pcapx/pcaputil/protocol_evidence.go','common/pcapx/pcaputil/protocol_field_registry.go','common/pcapx/pcaputil/capture_protocol_decoder.go','common/yak/cmd/yakcmds/shark-cli/decode.go','common/yak/cmd/yakcmds/shark-cli/command.go','common/yak/cmd/yakcmds/shark-cli/presentation.go','common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events/main.go']
 put(out/'schema-source-binding.json',[{'path':str(repo/p),'SHA256':sha(repo/p),'scope':'metadata/routing only; not protocol value oracle and not frozen future production code'}for p in metadata_files])
 put(out/'source-rows.json',rows);put(out/'default-budget-source-deltas.json',default_deltas)
 put(out/'alias-contract.json',[{'name':name,'API_key':key,'scope':scope,'type':typ,'bits':bits,'origin':'derived'if derived else'pdu'}for name,key,scope,typ,bits,derived in SPECS])
 contract={'physical_inputs':33,'official':9,'authored':24,'native_default_budget':4096,'direct_extra_budget0_not_included':True,
 'event':'one native rsvp callback per unfragmented original physical record, no EOF/state publication; positive generated PDU ids bind by source refs, never preassigned',
 'fields':'Session retains all source literal/derived prefixes; GetFields same on decoded outcome, nil+fields_error on refusal; full opaque Intserv can be decoded but field layouts incomplete',
 'source':'captured original PDU bytes/SHA, exact physical packet/domain, IP endpoints only; no advertised SESSION selector substituted into endpoint fields',
 'association':'flow/tcp_flow/transaction/response/direction zero; no captured UDP/TCP ports/connection/stream identity',
 'CompletedIn':'do not manufacture presence; if SourceBytes provides it, require matching original physical ref and matching event alias; missing remains missing',
 'private_aliases':'34 registered descriptors, every occurrence preserved in source order, unsigned bit widths/no exact_value<=32; derived has no octet range; selected literal range bound to exact original PDU',
 'short_header_checksum_aliases':'both omitted/verified absent unless whole common8B captured; no false defaults',
 'snapshot':'coverage limited/Incomplete for false Field Layouts Complete independent of decoded/status/completeness; official16 Intserv bodies remain opaque and never counted complete',
 'CLI':'physical row metadata and source time exact; SDK->CLI snapshot bridge only after independent source verification; captionRSVP and explicitstatus for bound partial/diagnostic events; logical Ethernet>IP>RSVP once; captured Layers retain actual header chain',
 'physical_layer_schema':'packetSummary.Layers intentionally omits Payload/DecodeFailure; this is a header-chain proof, not a claim the full decoder diagnostic layer list was exported',
 'native_parent_promotion':'complete bounded IP + selected v1 + captured fixed common short/declared RSVP length exceeds complete IP payload proves wrong-length Format; true CI short remains incomplete; unknownVersion not promoted; existing independent Format/Integrity cannot be hidden by Context/Budget',
 'primary_native_parent':[{'url':'https://www.rfc-editor.org/rfc/rfc2205.txt','section':'3.3','LF_lines':[2707,2709]},{'section':'Appendix B','LF_lines':[5346,5352]}],
 'not_claimed':['all native metadata/whole event objects','Go reflect.Type','mutation/privacy ownership tests','Windows pixels/highlights','new fragmented or live captures','real network loss recovery']}
 put(out/'schema-and-scope.json',contract)
 put(out/'runtime-command-plan.json',{'commands':[{'context':kind,'events_args':['replay',str(cap),'-','--ignore-embedded-secrets'],'CLI_args':['shark','-r',str(cap),'--json','--ignore-embedded-secrets']}for kind,cap in caps.items()],'field_descriptors_args':['shark','--list-fields','--json'],'execution':'parent-supplied closed build receipts/binary hashes and fresh output dirs required; preparation never starts a process'})
 frozen={str(p):sha(p)for p in inputs};frozen.update({str(p):sha(p)for p in out.iterdir()if p.is_file()})
 put(out/'manifest.json',{'frozen_inputs':frozen,'API_checker':str(apipath),'API_plan':str(apiplan),'source_model':str(modelpath),'controller':str(Path(__file__)),'controller_SHA256':sha(__file__),'source_cases':33,'alias_descriptors':34,'no_future_runtime_or_candidate_values_read':True,'no_Go_binary_native_CLI_TShark_started':True})
 print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'source_rows_SHA256':sha(out/'source-rows.json'),'cases':len(rows),'literal_leaves':sum(sum(leaf['literal']for leaf in row['source_case']['leaves'])for row in rows),'derived_bits':sum(sum(not leaf['literal']for leaf in row['source_case']['leaves'])for row in rows),'private_alias_occurrences':sum(len(row['aliases'])for row in rows)}))

def load(plan):
 m=read(plan/'manifest.json')
 for path,digest in m['frozen_inputs'].items():assert sha(path)==digest,'frozen source changed '+path
 api=module(m['API_checker'],'rsvp_native_API');return m,read(plan/'source-rows.json'),api

def check_snapshot(snapshot,e,row):
 a=row['anchor'];assert snapshot['schema_version']==1 and snapshot['pdu']==e['id']and strict(snapshot['source'],e['source_bytes']);assert not snapshot.get('truncated',False)
 private=[v for v in snapshot['values']if v['name'].startswith('rsvp.')]
 assert strict(private,row['aliases']),'private RSVP values/order/origins/ranges differ from source'
 by=collections.defaultdict(list)
 for v in snapshot['values']:by[v['name']].append(v)
 eventfacts={'pdu.id':e['id'],'pdu.length':a['PDU_length'],'flow.id':0,'source':a['source'],'destination':a['destination'],'status':row['expected_status'],'protocol':'rsvp'}
 complete=e['source_bytes'].get('CompletedIn')
 if complete is not None:
  assert strict(complete,{'Number':a['number'],'Domain':a['domain']});eventfacts['pdu.completed_in']=a['number']
 else:assert not by['pdu.completed_in']
 for name,value in eventfacts.items():
  assert len(by[name])==1;item=by[name][0];assert strict(item['value'],value)and item['origin']=='event'and'pdu_range'not in item
  if name in('pdu.id','pdu.length','flow.id','pdu.completed_in'):assert item.get('exact_value')==str(value)
 for name in('tcp.flow.id','pdu.response_to'):assert not by[name]
 assert [v['value']for v in by['ip.addr']]==[a['source'],a['destination']]
 incomplete=e['session'].get('Field Layouts Complete')is not True
 assert snapshot.get('incomplete',False)is incomplete
 assert snapshot.get('completeness')==('limited'if incomplete else e['completeness'])
 return len(private)

def time_ns(text):
 m=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)',text);assert m
 dt=datetime.datetime.strptime(m[1],'%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc);value=int(dt.timestamp())*10**9+int((m[2]or'').ljust(9,'0'))
 if m[3]!='Z':value-=(int(m[3][1:3])*3600+int(m[3][4:])*60)*10**9*(1 if m[3][0]=='+'else-1)
 return value

def verify_rows(sources,events,cli,api):
 assert len(events)==len(cli)==len(sources),'exact physical callback/row counts; no EOF association publication'
 byframe={};seen=set();stats=collections.Counter()
 for e in events:
  assert type(e['id'])is int and e['id']>0 and e['id']not in seen;seen.add(e['id']);refs=e['source_bytes']['PacketRefs'];assert len(refs)==1;number=refs[0]['Number'];assert number not in byframe;byframe[number]=e
 for source,c in zip(sources,cli):
  a=source['anchor'];raw=Path(source['raw_path']).read_bytes();assert dg(raw)==source['raw_SHA256'];e=byframe.pop(a['number'])
  assert e['protocol']=='rsvp'and e['transport']=='network'and e['profile']=='rsvp-bounded-control'and e['entry']=='RSVPControlFields'
  assert e['status']==source['expected_status'];assert e['completeness']==('message'if e['status']=='decoded'else e['status'])
  assert e['admission']==('observed-IP-next-header46-and-complete-base-wire'if e['status']=='decoded'else'observed-IP-next-header46')
  assert e['source']==a['source']and e['destination']==a['destination']and strict(e['domain'],a['domain'])
  assert all(e.get(k,0)==0 for k in('flow','tcp_flow_id','transaction','response_to','direction'))
  assert e['carrier_layers']in(None,[])and base64.b64decode(e['raw'],validate=True)==raw
  s=e['source_bytes'];assert s['Kind']=='captured'and s['ParentPDU']==0 and s['ParentPDUs']in(None,[])and s['SHA256']==dg(raw)and strict(s['PacketRefs'],[{'Number':a['number'],'Domain':a['domain']}])
  stats.update({'session_'+k:v for k,v in api.verify_fields(source['source_case'],raw,e['session']).items()})
  if e['status']=='decoded':
   assert not e.get('error')and not e.get('fields_error');stats.update({'fields_'+k:v for k,v in api.verify_fields(source['source_case'],raw,e['fields']).items()})
   name=NAMES[raw[1]];assert e['summary']=='RSVP '+name+' · advertised selectors; reservation/authentication unverified'
  else:assert e.get('error')and e['fields']is None and e.get('fields_error');stats['nondecoded_prefix_observations']+=1
  stats['private_aliases']+=check_snapshot(e['field_values'],e,source)
  assert c.get('record_type')!='protocol_publication'and c['number']==a['number']and c['captured_length']==a['caplen']and c['length']==a['origlen']and time_ns(c['time'])==a['time_ns']
  assert strict(c['capture_domain'],a['domain'])and c['source']==a['source']and c['destination']==a['destination']
  assert c.get('transport','')in('','network')and c.get('source_port',0)==0 and c.get('destination_port',0)==0
  for k in('tcp_flow_id','stream_id','conversation_id'):assert c.get(k,0)==0
  assert not c.get('connection_protocol')and not c.get('connection_stack')and not c.get('connection_evidence')
  for k,value in [('physical_source',a['source']),('physical_destination',a['destination']),('message_source',a['source']),('message_destination',a['destination'])]:
   if c.get(k):assert c[k]==value
  assert c.get('message_transport','')in('','network')
  physical=a['physical_headers']+(['RSVP']if a['capture_RSVP_layer']else[])
  assert c['layers']==physical,'physical header chain changed or invented'
  assert c['protocol']=='RSVP'and c['protocol_status']==e['status']and c['protocol_stack']==a['physical_headers']+['RSVP'],'progressive RSVP caption/status/stack missing'
  assert c['protocol_stack'].count('RSVP')==1
  if e['status']=='decoded':assert c['info']==e['summary']
  else:assert c.get('info'),'explicit diagnostic must remain visible'
  if a['real_snaplen_short']:assert'truncated capture'in c['info']
  assert len(c.get('pdus')or[])==1;pdu=c['pdus'][0]
  assert pdu['id']==e['id']and pdu['protocol']=='rsvp'and pdu['status']==e['status']and pdu['completeness']==e['completeness']and pdu['response_to']==0
  assert strict(pdu['byte_source'],s)and strict(pdu['field_values'],e['field_values']);fm=collections.defaultdict(list)
  for v in pdu['field_values']['values']:fm[v['name']].append(v['value'])
  assert strict(pdu['fields'],dict(fm));stats['SDK_CLI_source_snapshot_bridges']+=1;stats['physical_sources_and_progressive_stacks']+=1
  stats['decoded_wire_messages'if e['status']=='decoded'else'other_wire_outcomes']+=1
 assert not byframe
 return dict(stats)

def verify_descriptors(path):
 tree=read(path);assert tree['schema_version']==1;rows={r['name']:r for r in tree['fields']if r['name'].startswith('rsvp.')};assert set(rows)=={r[0]for r in SPECS}
 for name,key,scope,typ,bits,derived in SPECS:
  row=rows[name];assert row['protocol']=='rsvp'and row['type']==typ and row.get('bits',0)==bits and row['origin']==('derived'if derived else'pdu')
 return len(rows)

def verify_exports(plan,context,events_path,cli_path,out,descriptors=None):
 assert not out.exists();out.mkdir();manifest,rows,api=load(plan);sources=[r for r in rows if r['capture_key']==context];assert sources
 before={str(p):sha(p)for p in [Path(events_path),Path(cli_path),Path(__file__),plan/'manifest.json']}
 events=[json.loads(line)for line in Path(events_path).read_bytes().splitlines()if line.strip()];cli=[json.loads(line)for line in Path(cli_path).read_bytes().splitlines()if line.strip()]
 counts=verify_rows(sources,events,cli,api)
 if descriptors:counts['registered_RSVP_descriptors']=verify_descriptors(descriptors);before[str(descriptors)]=sha(descriptors)
 assert before=={p:sha(p)for p in before};load(plan)
 put(out/'summary.json',{'plan_manifest_SHA256':sha(plan/'manifest.json'),'controller_SHA256':sha(__file__),'context':context,'inputs':before,'counts':counts,'no_runtime_values_used_as_source_expected':True,'all_frozen_inputs_unchanged':True,'whole_unknown_metadata_or_Windows_not_claimed':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':counts}))

def mock_rows(sources):
 events=[];cli=[]
 for ident,row in enumerate(sources,1):
  a=row['anchor'];raw=Path(row['raw_path']).read_bytes();session=copy.deepcopy(row['source_case']['fields']);status=row['expected_status'];session['Wire Grammar Complete']=status=='decoded'
  e={'id':ident,'flow':0,'protocol':'rsvp','transport':'network','profile':'rsvp-bounded-control','entry':'RSVPControlFields','admission':'observed-IP-next-header46-and-complete-base-wire'if status=='decoded'else'observed-IP-next-header46','status':status,'completeness':'message'if status=='decoded'else status,'source':a['source'],'destination':a['destination'],'domain':a['domain'],'direction':0,'carrier_layers':None,'transaction':0,'response_to':0,'session':session,'fields':copy.deepcopy(session)if status=='decoded'else None,'raw':base64.b64encode(raw).decode(),'source_bytes':{'Kind':'captured','SHA256':dg(raw),'ParentPDU':0,'ParentPDUs':None,'PacketRefs':[{'Number':a['number'],'Domain':a['domain']}]},'summary':'RSVP '+NAMES[raw[1]]+' · advertised selectors; reservation/authentication unverified'if status=='decoded'else'rsvp'}
  if status!='decoded':e['error']='synthetic refusal witness';e['fields_error']='synthetic fields refusal'
  values=[]
  for name,value in [('pdu.id',ident),('flow.id',0),('pdu.length',len(raw)),('protocol','rsvp'),('source',a['source']),('destination',a['destination']),('status',status)]:
   v={'name':name,'value':value,'origin':'event'}
   if name in('pdu.id','flow.id','pdu.length'):v['exact_value']=str(value)
   values.append(v)
  values += [{'name':'ip.addr','value':x,'origin':'event'}for x in(a['source'],a['destination'])]+copy.deepcopy(row['aliases'])
  incomplete=session.get('Field Layouts Complete')is not True;e['field_values']={'schema_version':1,'pdu':ident,'source':copy.deepcopy(e['source_bytes']),'values':values,'completeness':'limited'if incomplete else e['completeness']}
  if incomplete:e['field_values']['incomplete']=True
  fm=collections.defaultdict(list)
  for v in values:fm[v['name']].append(v['value'])
  c={'number':a['number'],'time':datetime.datetime.fromtimestamp(a['time_ns']/1e9,datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S')+'.'+str(a['time_ns']%10**9).zfill(9)+'Z','captured_length':a['caplen'],'length':a['origlen'],'capture_domain':a['domain'],'source':a['source'],'destination':a['destination'],'layers':a['physical_headers']+(['RSVP']if a['capture_RSVP_layer']else[]),'protocol':'RSVP','protocol_status':status,'protocol_stack':a['physical_headers']+['RSVP'],'info':e['summary']if status=='decoded'else'RSVP diagnostic '+status,'pdus':[{'id':ident,'protocol':'rsvp','status':status,'completeness':e['completeness'],'response_to':0,'byte_source':copy.deepcopy(e['source_bytes']),'field_values':copy.deepcopy(e['field_values']),'fields':dict(fm)}]}
  if a['real_snaplen_short']:c['info']+=' [truncated capture]'
  events.append(e);cli.append(c)
 return events,cli

def selftest(plan,out):
 assert not out.exists();out.mkdir();manifest,rows,api=load(plan);totals=collections.Counter();contexts={}
 for context in('official','authored'):
  selected=[r for r in rows if r['capture_key']==context];events,cli=mock_rows(selected);counts=verify_rows(selected,events,cli,api);totals.update(counts);contexts[context]=counts
 negatives=[]
 authored=[r for r in rows if r['capture_key']=='authored'];events,cli=mock_rows(authored);short=next(i for i,r in enumerate(authored)if r['sourcecase']=='RSVP-authored-scene01-frame01')
 def rejects(label,mutator):
  es=copy.deepcopy(events);cs=copy.deepcopy(cli);mutator(es,cs)
  try:verify_rows(authored,es,cs,api)
  except(AssertionError,KeyError,TypeError,ValueError):negatives.append(label)
  else:raise AssertionError('selftest accepted '+label)
 rejects('short header invented checksum omitted false',lambda es,cs:es[short]['field_values']['values'].append({'name':'rsvp.checksum_omitted','value':False,'origin':'derived'}))
 rejects('short header invented checksum verified false',lambda es,cs:es[short]['field_values']['values'].append({'name':'rsvp.checksum_verified','value':False,'origin':'derived'}))
 rejects('advertised selector promoted to physical source',lambda es,cs:es[-1].update({'source':'198.51.100.2:40000'}))
 rejects('flow identity inherited across peers',lambda es,cs:es[-1].update({'flow':7}))
 rejects('partial RSVP caption falls back to IPv4',lambda es,cs:cs[2].update({'protocol':'IPv4','protocol_stack':['Ethernet','IPv4']}))
 rejects('fake RSVP capture layer for true snaplen-short',lambda es,cs:cs[2]['layers'].append('RSVP'))
 off=[r for r in rows if r['capture_key']=='official'];oe,oc=mock_rows(off);bad=copy.deepcopy(oe);bad[0]['field_values'].pop('incomplete');bad[0]['field_values']['completeness']='message'
 try:verify_rows(off,bad,oc,api)
 except(AssertionError,KeyError,TypeError,ValueError):negatives.append('Intserv opaque falsely complete snapshot')
 else:raise AssertionError('opaque completeness falseclaim accepted')
 put(out/'summary.json',{'plan_manifest_SHA256':sha(plan/'manifest.json'),'controller_SHA256':sha(__file__),'counts':dict(totals),'contexts':contexts,'negative_mutations_rejected':negatives,'candidate_runtime_or_replay_not_performed':True,'metadata_not_counted_as_wire_proof':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':dict(totals),'negative_mutations':len(negatives)}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--prepare',action='store_true');p.add_argument('--selftest',action='store_true');p.add_argument('--phase',type=Path);p.add_argument('--plan',type=Path);p.add_argument('--context',choices=['official','authored']);p.add_argument('--events-export',type=Path);p.add_argument('--CLI-export',type=Path);p.add_argument('--descriptors',type=Path);p.add_argument('--out',type=Path,required=True);a=p.parse_args()
 if a.prepare:assert a.phase;prepare(a.phase,a.out)
 elif a.selftest:assert a.plan;selftest(a.plan,a.out)
 else:assert a.plan and a.context and a.events_export and a.CLI_export;verify_exports(a.plan,a.context,a.events_export,a.CLI_export,a.out,a.descriptors)
if __name__=='__main__':main()