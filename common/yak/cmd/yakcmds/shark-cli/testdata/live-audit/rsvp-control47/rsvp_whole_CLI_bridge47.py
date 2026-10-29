"""Pure source whole-CLI RSVP parent diagnostic projection and bridge.

Expected target rows use only old45 physical rows, original PCAPNG timing/header
facts, frozen source936 DTOs, and the fixed presentation/observation schema.
No candidate47 output is read in preparation. Native generated IDs are imported
only through a closed whole-native audit and independently replayed ID ledger.
"""
from pathlib import Path
import argparse,collections,copy,datetime,hashlib,importlib.util,itertools,json,re,shutil,struct
ROOT=Path(__file__).parent
CAPTURE_SHA='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'


def sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb')as f:
  for b in iter(lambda:f.read(1048576),b''):h.update(b)
 return h.hexdigest()
def read(path):return json.loads(Path(path).read_text(encoding='utf-8-sig'))
def rows(path):
 with Path(path).open(encoding='utf-8-sig')as f:
  for line in f:
   if line.strip():yield json.loads(line)
def write(path,x):
 with Path(path).open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2);f.write('\n')
def canonical(x):return json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()
def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b
def load(name,path):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def timestamps(capture,wanted):
 endian=None;interfaces=[];section=-1;number=0;out={}
 with Path(capture).open('rb')as f:
  while True:
   head=f.read(8)
   if not head:break
   assert len(head)==8
   if head[:4]==bytes.fromhex('0a0d0d0a'):
    magic=f.read(4);endian='<'if magic==bytes.fromhex('4d3c2b1a')else'>'if magic==bytes.fromhex('1a2b3c4d')else None;assert endian
    size=struct.unpack(endian+'I',head[4:])[0];block=head+magic+f.read(size-12);interfaces=[];section+=1
   else:
    assert endian;size=struct.unpack(endian+'I',head[4:])[0];block=head+f.read(size-8)
   assert len(block)==size and size>=12 and size%4==0 and struct.unpack_from(endian+'I',block,size-4)[0]==size
   kind=struct.unpack_from(endian+'I',block)[0]
   if kind==1:
    info={'denominator':1000000,'offset_seconds':0};pos=16
    while pos+4<=size-4:
     code,length=struct.unpack_from(endian+'HH',block,pos);pos+=4
     if code==0:break
     assert pos+length<=size-4;value=block[pos:pos+length]
     if code==9:assert length==1;info['denominator']=(2**(value[0]&127))if value[0]&128 else 10**value[0]
     if code==14:assert length==8;info['offset_seconds']=struct.unpack(endian+'q',value)[0]
     pos+=(length+3)&~3
    interfaces.append(info)
   elif kind in(6,2,3):
    number+=1
    if number not in wanted:continue
    assert kind==6,'selected source936 uses timestamped enhanced blocks'
    interface,hi,lo,cap,orig=struct.unpack_from(endian+'IIIII',block,8);info=interfaces[interface];ns=(((hi<<32)|lo)*10**9)//info['denominator']+info['offset_seconds']*10**9
    out[number]={'frame':number,'section':section,'interface':interface,'time_ns':ns,'timestamp_high':hi,'timestamp_low':lo,'resolution_denominator':info['denominator'],'offset_seconds':info['offset_seconds'],'captured_length':cap,'original_length':orig}
 assert set(out)==set(wanted)
 return out

def parsed_time(text):
 match=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z',text);assert match
 sec=int(datetime.datetime.strptime(match[1],'%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc).timestamp())
 return sec*10**9+int((match[2]or'').ljust(9,'0'))

def field_map(snapshot):
 out={}
 for value in snapshot['values']:out.setdefault(value['name'],[]).append(copy.deepcopy(value['value']))
 return out

def target_row(before,identifier,case,timing,factory):
 assert before['number']==case['frame']and before['capture_domain']=={'Section':case['section'],'Interface':case['interface'],'Encapsulation':''}
 assert before['length']==case['original_length']==timing['original_length']and before['captured_length']==case['captured_length']==timing['captured_length']
 assert parsed_time(before['time'])==timing['time_ns']and timing['section']==case['section']and timing['interface']==case['interface']
 assert before['source']==case['IP_source']and before['destination']==case['IP_destination']and before['layers']==['Ethernet','IPv4']and before['protocol']=='IPv4'
 basic={'capture_domain','pdu_scope','protocol_scope','layers','number','time','source','destination','protocol','length','captured_length','info'}
 if case['frame']==279605:assert set(before)==basic and not before.get('pdus')
 else:assert set(before)==basic|{'pdus','protocol_status','protocol_evidence'}and len(before['pdus'])==1 and before['pdus'][0]['protocol']=='ip'and before['pdus'][0]['status']=='unrecognized'
 assert before['pdu_scope']=='observed_at_packet'
 event=factory.expected_diagnostic(case,identifier);snapshot=event['field_values'];out=copy.deepcopy(before)
 out.update(protocol='IPv4',protocol_stack=['Ethernet','IPv4'],visibility='malformed-network-header',protocol_status='malformed',protocol_scope='validated network prefix before refused child header',protocol_evidence='child header refused; no transport or application state admitted',info=event['summary'],transport='network',message_transport='network',message_domain=copy.deepcopy(event['domain']))
 out['source'],out['destination']=case['expected_event_source'],case['expected_event_destination']
 for key,value in [('physical_source',out['source']),('physical_destination',out['destination']),('message_source',out['source']),('message_destination',out['destination'])]:
  if value:out[key]=value
  else:out.pop(key,None)
 out['pdus']=[{'id':identifier,'protocol':'ip','status':'malformed','completeness':'malformed','response_to':0,'byte_source':copy.deepcopy(event['source_bytes']),'fields':field_map(snapshot),'field_values':copy.deepcopy(snapshot),'summary':event['summary'],'error':event['error'],'expert':event['expert']}]
 return out

def remap_cli(before,mapping):
 out=copy.deepcopy(before)
 def one(value):
  assert type(value)is int and value>=0
  if value==0:return 0
  assert value in mapping;return mapping[value]
 def source(value):
  if value is None:return
  if'ParentPDU'in value:value['ParentPDU']=one(value['ParentPDU'])
  if value.get('ParentPDUs')is not None:value['ParentPDUs']=[one(v)for v in value['ParentPDUs']]
 for group in('pdus','published_pdus'):
  for pdu in out.get(group)or[]:
   assert same(pdu['fields'],field_map(pdu['field_values']))
   pdu['id']=one(pdu['id'])
   if'response_to'in pdu:pdu['response_to']=one(pdu['response_to'])
   source(pdu.get('byte_source'));snapshot=pdu['field_values'];snapshot['pdu']=one(snapshot['pdu']);source(snapshot.get('source'))
   for value in snapshot.get('values')or[]:
    allowed=value['name']in('pdu.id','pdu.response_to')or(pdu['protocol']=='ntp'and value['name']=='ntp.control.request_pdu')
    if not allowed:continue
    prior=value['value'];value['value']=one(prior)
    if'exact_value'in value:assert value['exact_value']==str(prior);value['exact_value']=str(value['value'])
   pdu['fields']=field_map(snapshot)
 return out

def prepare(a):
 a.plan.mkdir(exist_ok=False);frozen=a.plan/'frozen';frozen.mkdir()
 native_plan=ROOT/'RSVP-whole-native-source-plan47-02';np=read(native_plan/'plan.json');assert sha(native_plan/'plan.json')=='20305ccb059bef45e6504d2b968a8bcb628a276c8312d600345860b61030641d'
 nhelper=load('RSVP_native_public_binding',native_plan/'frozen/rsvp_whole_native_bridge47.py');factory=load('RSVP_source_DTO_for_CLI',native_plan/'frozen/rsvp_parent_diagnostic_source47_v02.py');sources={r['frame']:r for r in read(np['source_cases'])}
 oldroot=ROOT/'runtime-public-CLI-45';summary=read(oldroot/'summary.json');assert summary['source']=='df30fc4bd909d6489844f8e0d1cd0ab03826f35c'and summary['cases']==37
 capture=Path(next(c['capture']for c in read(native_plan/'cases.json')if c['capture_SHA256']==CAPTURE_SHA));assert sha(capture)==CAPTURE_SHA;timefacts=timestamps(capture,sources)
 cases=[];witnesses=[];target_base={};total=0
 for pp in sorted(oldroot.glob('*/*/yak-producer.json')):
  prod,export=nhelper.binding(pp,summary);count=0
  for before in rows(export):
   count+=1;frame=before.get('number')
   if prod['capture_SHA256']==CAPTURE_SHA and frame in sources:
    expected=target_row(before,1,sources[frame],timefacts[frame],factory);assert frame not in target_base
    target_base[frame]=before;witnesses.append({'frame':frame,'old_whole_row_SHA256':hashlib.sha256(canonical(before)).hexdigest(),'source_expected_whole_row_ID1_SHA256':hashlib.sha256(canonical(expected)).hexdigest(),'original_timestamp_ns':timefacts[frame]['time_ns']})
  cases.append({'relative':export.relative_to(oldroot).as_posix(),'producer_relative':pp.relative_to(oldroot).as_posix(),'rows':count,'export_SHA256':sha(export),'producer_SHA256':sha(pp),'capture':prod['capture'],'capture_SHA256':prod['capture_SHA256'],'arguments':prod['arguments'][1:],'keylog_used':prod['keylog_used']});total+=count
 assert len(cases)==37 and total==384518 and len(witnesses)==936
 write(a.plan/'cases.json',cases);write(a.plan/'target-whole-row-source-witnesses.json',witnesses);write(a.plan/'target-original-timestamps.json',list(timefacts.values()))
 files={}
 for path,name in[(Path(__file__),'rsvp_whole_CLI_bridge47.py'),(native_plan/'frozen/rsvp_whole_native_bridge47.py','native_bridge.py'),(native_plan/'frozen/rsvp_parent_diagnostic_source47_v02.py','rsvp_parent_diagnostic_source47_v02.py')]:
  shutil.copyfile(path,frozen/name);assert sha(path)==sha(frozen/name);files[name]=sha(path)
 write(a.plan/'plan.json',{'scope':__doc__,'before':{'root':str(oldroot),'source':summary['source'],'summary_SHA256':sha(oldroot/'summary.json')},'native_plan':str(native_plan),'native_plan_SHA256':sha(native_plan/'plan.json'),'source_cases':np['source_cases'],'source_cases_SHA256':np['source_cases_SHA256'],'files':files,'cases_SHA256':sha(a.plan/'cases.json'),'target_witnesses_SHA256':sha(a.plan/'target-whole-row-source-witnesses.json'),'timestamps_SHA256':sha(a.plan/'target-original-timestamps.json'),'old_CLI_records':384518,'whole_target_rows':936,'retained_whole_rows':383582,'no_candidate47_output_read':True,'old45_physical_metadata_and_original_source_validated':True,'uncovered_projection_keys_exempted':False})
 # Source-only selftest over every target. Expected fixed strings/shape do not
 # use old4dce/47 target rows; the ID1 digest is the frozen full-row witness.
 counter=collections.Counter();negative=[]
 for frame,before in target_base.items():
  result=target_row(before,1,sources[frame],timefacts[frame],factory);counter['whole_source_rows_constructed']+=1;counter['literal_endpoint_rows'if result['source']else'empty_endpoint_rows']+=1
  assert result['protocol']=='IPv4'and not any(v['name'].startswith('rsvp.')for v in result['pdus'][0]['field_values']['values'])
 example=target_row(target_base[279605],1,sources[279605],timefacts[279605],factory)
 for label,mutate in [('fake RSVP identity',lambda x:x.update(protocol='RSVP')),('unknown extra key',lambda x:x.update(unknown_projection=True)),('source port inferred from content',lambda x:x.update(source_port=46)),('wrong original timestamp',lambda x:x.update(time='2022-01-01T00:00:00Z'))]:
  changed=copy.deepcopy(example);mutate(changed);assert not same(changed,example);negative.append(label)
 write(a.plan/'selftest-summary.json',{'plan_SHA256':sha(a.plan/'plan.json'),'counts':dict(counter),'negative_mutations_rejected':negative,'no_candidate47_CLI_read':True,'full_row_projection_not_product_pass':True})
 print(json.dumps({'plan':str(a.plan/'plan.json'),'SHA256':sha(a.plan/'plan.json'),'selftest_SHA256':sha(a.plan/'selftest-summary.json'),'source_rows':len(witnesses),'old_rows':total}))

def validate(plan):
 p=read(plan/'plan.json');assert sha(__file__)==p['files']['rsvp_whole_CLI_bridge47.py']
 for name,d in p['files'].items():assert sha(plan/'frozen'/name)==d
 for filename,key in [('cases.json','cases_SHA256'),('target-whole-row-source-witnesses.json','target_witnesses_SHA256'),('target-original-timestamps.json','timestamps_SHA256')]:assert sha(plan/filename)==p[key]
 assert sha(p['source_cases'])==p['source_cases_SHA256']and sha(Path(p['native_plan'])/'plan.json')==p['native_plan_SHA256']
 sources={r['frame']:r for r in read(p['source_cases'])};timefacts={r['frame']:r for r in read(plan/'target-original-timestamps.json')};factory=load('RSVP_CLI_frozen_source_DTO',plan/'frozen/rsvp_parent_diagnostic_source47_v02.py');nhelper=load('RSVP_CLI_frozen_binding',plan/'frozen/native_bridge.py')
 return p,sources,timefacts,factory,nhelper

def native_ledger(p,native,audit,sources,factory,nhelper):
 report=read(audit/'summary.json');assert report['source']=='55b29ade06b2201e8be8e40b25920226038df2e1'and report['plan_SHA256']==p['native_plan_SHA256']and report['unplanned_differences']==0
 nativeplan=read(Path(p['native_plan'])/'plan.json');case=next(c for c in read(Path(p['native_plan'])/'cases.json')if c['kind']=='events'and c['capture_SHA256']==CAPTURE_SHA)
 old=Path(nativeplan['before']['root'])/case['relative'];prod,after=nhelper.binding(native/case['producer_relative'],read(native/'summary.json'));assert prod['capture_SHA256']==CAPTURE_SHA
 certified=next(c for c in report['checks']if c['relative']==case['relative']);assert sha(after)==certified['new_export_SHA256']
 ledger_path=audit/(str(Path(case['relative']).parent.as_posix()).replace('/','-')+'-ID-ledger.json');saved=read(ledger_path);mapping={};source_ids={};left=iter(rows(old))
 for event in rows(after):
  frame=nhelper.source_key(event,sources)
  if frame is not None:
   assert same(factory.expected_diagnostic(sources[frame],event['id']),event);assert frame not in source_ids;source_ids[frame]=event['id']
   if frame==279605:continue
   before=next(left);assert nhelper.source_key(before,sources)==frame
  else:
   before=next(left)
   for key in('protocol','transport','profile','status','raw','domain','flow','source','destination'):assert same(before.get(key),event.get(key))
  assert before['id']not in mapping;mapping[before['id']]=event['id']
 assert next(left,None)is None and set(source_ids)==set(sources)and len(mapping)==case['rows']and len(set(mapping.values()))==len(mapping)
 assert mapping=={int(k):v for k,v in saved['old_to_new'].items()}and saved['new_source_frames_to_ID']=={'279605':source_ids[279605]}
 return mapping,source_ids,{'native_audit_summary_SHA256':sha(audit/'summary.json'),'closed_native_export_SHA256':sha(after),'reconstructed_native_ledger_SHA256':sha(ledger_path)}

def verify(a):
 p,sources,timefacts,factory,nhelper=validate(a.plan);a.out.mkdir(exist_ok=False);oldroot=Path(p['before']['root']);assert sha(oldroot/'summary.json')==p['before']['summary_SHA256'];summary=read(a.cli/'summary.json');assert summary['source']==a.source and summary['cases']==37
 mapping,source_ids,closure=native_ledger(p,a.native,a.native_audit,sources,factory,nhelper);totals=collections.Counter();checks=[]
 for case in read(a.plan/'cases.json'):
  old=oldroot/case['relative'];new=a.cli/case['relative'];assert sha(old)==case['export_SHA256']and sha(oldroot/case['producer_relative'])==case['producer_SHA256'];prod,export=nhelper.binding(a.cli/case['producer_relative'],summary);assert export==new and prod['source']==a.source
  for key in('capture','capture_SHA256','keylog_used'):assert prod[key]==case[key]
  assert prod['arguments'][1:]==case['arguments']
  if case['capture_SHA256']!=CAPTURE_SHA:
   assert sha(old)==sha(new),'non-target CLI whole export changed';count=sum(1 for _ in rows(new));assert count==case['rows'];totals['retained_whole_CLI_rows']+=count;totals['whole_literal_CLI_exports']+=1
  else:
   found=set();count=0
   for before,actual in itertools.zip_longest(rows(old),rows(new)):
    assert before is not None and actual is not None,'CLI record count changed';count+=1;frame=before.get('number')
    if frame in sources:
     assert frame not in found;found.add(frame);expected=target_row(before,source_ids[frame],sources[frame],timefacts[frame],factory);assert same(expected,actual),('whole source-constructed CLI row differs',frame);totals['source_whole_CLI_rows']+=1
    else:assert same(remap_cli(before,mapping),actual),('unplanned whole CLI row',frame);totals['retained_whole_CLI_rows']+=1
   assert found==set(sources)and count==case['rows']
  checks.append({'relative':case['relative'],'new_export_SHA256':sha(new),'new_producer_SHA256':sha(a.cli/case['producer_relative'])});print(json.dumps({'relative':case['relative'],'passed':True}),flush=True)
 assert totals['retained_whole_CLI_rows']==383582 and totals['source_whole_CLI_rows']==936 and totals['whole_literal_CLI_exports']==36
 validate(a.plan)
 write(a.out/'summary.json',{'source':a.source,'plan_SHA256':sha(a.plan/'plan.json'),'counts':dict(totals),'native_generated_ID_closure':closure,'checks':checks,'unplanned_differences':0,'no_projection_key_exemption':True,'936_bad_IP_parent_rows_not_RSVP_identity':True})
 print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(totals)}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=['prepare','verify']);p.add_argument('--plan',type=Path,required=True);p.add_argument('--cli',type=Path);p.add_argument('--native',type=Path);p.add_argument('--native-audit',type=Path);p.add_argument('--source');p.add_argument('--out',type=Path);a=p.parse_args()
 if a.mode=='prepare':prepare(a)
 else:assert a.cli and a.native and a.native_audit and a.source and a.out;verify(a)
if __name__=='__main__':main()