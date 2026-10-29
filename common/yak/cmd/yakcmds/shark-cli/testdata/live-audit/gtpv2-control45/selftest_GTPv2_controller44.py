"""Source-only mechanical selftest; no binary/replay/Go execution permitted."""
import argparse,base64,copy,datetime,hashlib,importlib.util,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def write(path,rows):
 with Path(path).open('x',encoding='utf8')as f:
  for r in rows:f.write(json.dumps(r)+'\n')
def loadmod(path):
 s=importlib.util.spec_from_file_location('frozen_controller',path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--controller',required=True,type=Path);ap.add_argument('--plan',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args();assert not a.out.exists();a.out.mkdir();m=loadmod(a.controller);manifest,sources,contexts,checker,policy=m.load(a.plan);calls=[]
 def forbidden(*args,**kwargs):calls.append(True);raise AssertionError('binary execution forbidden in source-only selftest')
 m.subprocess.run=forbidden
 def timestamp(ns):
  sec,fraction=divmod(ns,10**9);s=datetime.datetime.fromtimestamp(sec,datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S');return s+('.'+str(fraction).zfill(9).rstrip('0')if fraction else'')+'Z'
 def compose(context):
  events=[];rows=[]
  for source in sources:
   if source['capture_key']!=context['capture_key']:continue
   anchor=source['anchor'];n=anchor['number'];raw=Path(source['PDU_file']).read_bytes();selected=source['explicit_selected_GTPv2']if context['explicit']else source['auto_selected_GTPv2'];tree,oracle=checker.expected_case(raw,4096,policy);decoded=selected and not oracle.issues;status='decoded'if decoded else('malformed'if selected else('incomplete'if not anchor['carrier_complete']else'unrecognized'));bs={'Kind':'captured','SHA256':m.dg(raw)if raw else'','ParentPDU':0,'ParentPDUs':None,'PacketRefs':[{'Number':n,'Domain':copy.deepcopy(anchor['domain'])}]}
   if decoded:bs['CompletedIn']=copy.deepcopy(bs['PacketRefs'][0])
   event={'id':n,'flow':0,'protocol':'gtpv2'if selected else'','status':status,'profile':'gtpv2-bounded-UDP-observation'if selected else'','completeness':'message'if decoded else status,'summary':('GTPv2-C '+' + '.join(source['source_message_names'])+' · wire observation; bearer/session unverified')if decoded else'source-only diagnostic','expert':'','source_bytes':bs,'fields':None,'session':None,'raw':base64.b64encode(raw).decode()if raw else None,'entry':'GTPv2ControlFields'if selected else'','admission':('explicit-decode-as'if context['explicit']else'complete-selected-wire-grammar-and-service-hint')if selected else'','source':anchor['source_endpoint'],'destination':anchor['destination_endpoint'],'direction':0,'transport':'udp','domain':copy.deepcopy(anchor['domain']),'carrier_layers':None,'transaction':0,'response_to':0}
   if selected:
    value=copy.deepcopy(tree);value.update({'Authentication Verified':False,'Procedure Semantics Verified':False,'Bearer State Inferred':False,'Wire Grammar Complete':decoded});event['session']=value
    if decoded:event['fields']=copy.deepcopy(value)
    else:event['fields_error']='source-only synthetic refusal'
   meta={'flow.id':0,'pdu.id':n,'pdu.length':len(raw),'source':anchor['source_endpoint'],'destination':anchor['destination_endpoint'],'status':status}
   if event['protocol']:meta['protocol']=event['protocol']
   if decoded:meta['pdu.completed_in']=n
   values=[]
   for key,value in sorted(meta.items()):
    x={'name':key,'value':value,'origin':'event'}
    if key in('flow.id','pdu.id','pdu.length','pdu.completed_in'):x['exact_value']=str(value)
    values.append(x)
   values+=[{'name':'ip.addr','value':value,'origin':'event'}for value in[anchor['IP_source'],anchor['IP_destination']]]
   if selected:values+=m.protocol_aliases(tree)
   snap={'schema_version':1,'pdu':n,'source':copy.deepcopy(bs),'values':values,'completeness':'message'if decoded else('limited'if selected else status)}
   if selected and not decoded:snap['incomplete']=True
   event['field_values']=snap;rows.append({'number':n,'captured_length':anchor['caplen'],'length':anchor['origlen'],'capture_domain':copy.deepcopy(anchor['domain']),'source':anchor['IP_source'],'destination':anchor['IP_destination'],'time':timestamp(anchor['time_ns']),'layers':copy.deepcopy(anchor['physical_required_prefix']),'protocol':'GTPv2-C'if decoded else'UDP','info':event['summary']if decoded else'source-only diagnostic'})
   if not raw and not context['explicit']:continue
   events.append(event);fmap={}
   for x in values:fmap.setdefault(x['name'],[]).append(copy.deepcopy(x['value']))
   rows[-1]['pdus']=[{'id':n,'protocol':event['protocol'],'status':status,'completeness':event['completeness'],'response_to':0,'byte_source':copy.deepcopy(bs),'field_values':copy.deepcopy(snap),'fields':fmap}]
   if decoded:rows[-1].update(protocol_status='decoded',protocol_stack=anchor['physical_required_prefix']+['GTPv2-C'])
  return events,rows
 gold={};counts={}
 for context in contexts:
  folder=a.out/context['name'];folder.mkdir();events,rows=compose(context);gold[context['name']]=(events,rows);write(folder/'events.jsonl',events);write(folder/'CLI.jsonl',rows);counts[context['name']]=m.verify_exports(a.plan,context,folder/'events.jsonl',folder/'CLI.jsonl')
 context=contexts[0];base_events,base_rows=gold[context['name']];negatives=[]
 def reject(name,mutate,ctx=context):
  events,rows=copy.deepcopy(gold[ctx['name']]);mutate(events,rows);folder=a.out/('negative-'+name);folder.mkdir();write(folder/'events.jsonl',events);write(folder/'CLI.jsonl',rows)
  try:m.verify_exports(a.plan,ctx,folder/'events.jsonl',folder/'CLI.jsonl')
  except(AssertionError,checker.VerifyError,ValueError):negatives.append({'name':name,'rejected':True});return
  raise AssertionError('source mutation accepted: '+name)
 reject('raw-wire-changed',lambda e,r:e[0].__setitem__('raw',base64.b64encode(b'X').decode()))
 reject('wrong-domain',lambda e,r:e[0]['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',1))
 reject('wrong-source-hash',lambda e,r:e[0]['source_bytes'].__setitem__('SHA256','0'*64))
 reject('session-association',lambda e,r:e[0].__setitem__('flow',1))
 reject('CLI-caption-inheritance',lambda e,r:r[0].__setitem__('protocol','TLS'))
 reject('CLI-physical-time',lambda e,r:r[0].__setitem__('time','2000-01-01T00:00:00Z'))
 reject('CLI-stack-fake-carrier',lambda e,r:r[0]['protocol_stack'].insert(3,'TLS'))
 reject('literal-value-changed',lambda e,r:e[0]['session']['Messages'][0]['Header'].__setitem__('TEID',999))
 reject('literal-source-range',lambda e,r:e[0]['session']['Messages'][0]['Header'][m.WR].__setitem__('Sequence Number',[1,4]))
 def alias_corrupt(e,r):
  x=next(v for v in e[0]['field_values']['values']if v['name']=='gtpv2.flags');x['pdu_range']['offset']+=42;r[0]['pdus'][0]['field_values']=copy.deepcopy(e[0]['field_values'])
  fmap={}
  for v in e[0]['field_values']['values']:fmap.setdefault(v['name'],[]).append(v['value'])
  r[0]['pdus'][0]['fields']=fmap
 reject('typed-alias-physical-offset',alias_corrupt)
 def native_low_budget(e,r):
  target=next(x for x in e if x['source_bytes']['PacketRefs'][0]['Number']==17);target['session']['Messages'][0]['IEs'].pop()
 reject('default-budget-misused-direct1',native_low_budget,contexts[1])
 def crosspeer(e,r):
  target=next(x for x in e if x['source_bytes']['PacketRefs'][0]['Number']==24);target['session']['Messages'][0]['IEs'][0]['Body']['Opaque Value']=base64.b64encode(b'A').decode()
 reject('same-TEID-cross-peer-inheritance',crosspeer,contexts[1])
 descriptors={'schema_version':1,'fields':[{'name':n,'protocol':'gtpv2','type':t,'origin':'derived'if d else'pdu',**({'bits':b}if b else{})}for n,k,sc,t,b,d in m.SPECS]};f=a.out/'descriptors.json';f.write_text(json.dumps(descriptors),'utf8');assert m.verify_descriptors(f)==30;assert not calls
 for path,h in manifest['frozen_inputs'].items():assert sha(path)==h
 summary={'source_only_not_candidate_output':True,'binary_execution_guard_calls':len(calls),'selftest_source_SHA256':sha(__file__),'controller_SHA256':sha(a.controller),'plan_manifest_SHA256':sha(a.plan/'manifest.json'),'source_contexts':counts,'negative_controls':negatives,'registered_schema_descriptors_checked':30,'frozen_inputs_unchanged':True};(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n','utf8');print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'contexts':len(contexts),'negative_controls':len(negatives)}))
if __name__=='__main__':main()
