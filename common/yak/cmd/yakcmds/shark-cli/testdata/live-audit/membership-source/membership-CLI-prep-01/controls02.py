"""Pre-runtime whole CLI source/typed-object controls; authored rows are not executions."""
import argparse,collections,copy,datetime,hashlib,importlib.util,json,sys
from pathlib import Path
from membership_cli_expected import expected_row,same,legacy_membership_layer,clock_ns

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def module(name,p):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('cli-plan','native-plan','igmp-modules','mld-modules','carrier','audit-modules','capture-weber','capture-sharon','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path[:0]=[str(a.igmp_modules),str(a.mld_modules),str(a.audit_modules)]
 from vxlan_endpoint_projection_audit import capture_records
 igmp=module('igmpOracle',a.igmp_modules/'igmp_native_expected.py');mld=module('mldOracle',a.mld_modules/'mld_native_expected.py');carrier=module('mldCarrier',a.carrier)
 positive=[];negative=[];chosen={};all_checked=0;unknown_queries=[]
 def rejects(name,fn):
  try:fn()
  except(AssertionError,KeyError,ValueError,TypeError):negative.append(name);return
  raise AssertionError('negative accepted '+name)
 def differs(name,want,after):
  assert not same(want,after),'whole-object mutation accepted '+name;negative.append(name)
 for corpus,cap in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
  groups={f:{x['frame']:x for x in json.loads((a.native_plan/corpus/(f.upper()+'-source-plan.json')).read_text())}for f in('igmp','mld')};ts={x['frame']:x for x in map(json.loads,(a.cli_plan/corpus/'membership-rows.jsonl').read_text().splitlines())};rs={n:r for n,r in enumerate(capture_records(cap),1)if n in ts}
  def qualify(t,record,old=None):
   source=groups[t['family']][t['frame']];record=copy.deepcopy(record);record['frame']=t['frame']
   event=igmp.event(t['id'],record,source)if t['family']=='igmp'else mld.event(t['id'],record,source,carrier.locate)[0]
   loc=source['location']if t['family']=='igmp'else source['source_profile']
   return expected_row(t['old_row']if old is None else old,record,loc,event,t['family'])
  for frame,t in ts.items():
   record=rs[frame];record['frame']=frame;assert same(qualify(t,record),t['expected_row']);all_checked+=1
   carrierclass='GRE'if t['location']['raw_start']>0 else'MPacket'if record['link']==274 else'direct'
   key=(t['family'],t['native_event']['status'],carrierclass)
   if key not in chosen:chosen[key]=(t,record,qualify)
   source=groups[t['family']][frame];m=t['native_event']['session']['IGMP'if t['family']=='igmp'else'MLD']
   if m.get('Message')in('Membership Query','Multicast Listener Query')and t['native_event']['status']=='limited':
    assert 'Version'not in m and 'Source Count'not in m and t['expected_row']['protocol']==('IGMP'if t['family']=='igmp'else'MLD');unknown_queries.append(dict(corpus=corpus,frame=frame,family=t['family']))
  # Freeze closure-local source bindings before the next corpus changes them.
  for key,(t,record,fn)in list(chosen.items()):
   if t['corpus']!=corpus:continue
   label='/'.join(key);want=t['expected_row'];positive.append(label)
   changed=copy.deepcopy(record);w=bytearray(changed['wire']);w[0]^=1;changed['wire']=bytes(w);rejects(label+'/original-wire-change',lambda changed=changed:qualify(t,changed))
   changed=copy.deepcopy(record);changed['interface']+=1;rejects(label+'/record-interface-change',lambda changed=changed:qualify(t,changed))
   changed=copy.deepcopy(record);changed['captured_length']+=1;rejects(label+'/record-capture-extent-change',lambda changed=changed:qualify(t,changed))
   old=copy.deepcopy(t['old_row']);sec,ns=divmod(clock_ns(old['time'])+1,10**9);old['time']=(datetime.datetime(1970,1,1)+datetime.timedelta(seconds=sec)).strftime('%Y-%m-%dT%H:%M:%S')+('.%09dZ'%ns);rejects(label+'/timestamp-one-nanosecond',lambda old=old:qualify(t,record,old))
   old=copy.deepcopy(t['old_row']);old['layers'].append('TLS');rejects(label+'/wrong-physical-child',lambda old=old:qualify(t,record,old))
   changed=copy.deepcopy(want);changed['protocol']='TLS';differs(label+'/unobserved-application-label',want,changed)
   changed=copy.deepcopy(want);changed['pdus'][0]['id']=float(changed['pdus'][0]['id']);differs(label+'/PDU-integer-type',want,changed)
   changed=copy.deepcopy(want);changed['pdus'][0]['field_values']['values'][0]['value']='forged';differs(label+'/typed-native-snapshot',want,changed)
   changed=copy.deepcopy(want);v=next(v for v in changed['pdus'][0]['field_values']['values']if 'pdu_range'in v);v['pdu_range']['offset']+=1;differs(label+'/Source1-byte-range',want,changed)
   changed=copy.deepcopy(want);changed['capture_domain']['Interface']+=1;differs(label+'/physical-domain',want,changed)
 assert all_checked==388 and len(unknown_queries)==13
 # Explicit dependency-body positives test fixed minima and the known ignored
 # IGMPv3 parse errors; these are not claims of valid native protocol grammar.
 legacy=[('IGMP-zero',b'','igmp',(None,False)),('IGMP-query7',b'\x11'+bytes(6),'igmp',(None,True)),('IGMP-query8',b'\x11'+bytes(7),'igmp',('IGMP',False)),('IGMP-query9',b'\x11'+bytes(8),'igmp',(None,True)),('IGMP-query12-short-sources',b'\x11'+bytes(9)+b'\xff\xff','igmp',('IGMP',False)),('IGMP-v3report1',b'\x22','igmp',('IGMP',False)),('IGMP-v1report7',b'\x12'+bytes(6),'igmp',(None,True)),('MLD-report-no-body',b'\x8f'+bytes(3),'mld',(None,False)),('MLD-report-short-body',b'\x8f'+bytes(4),'mld',(None,True)),('MLD-report0',b'\x8f'+bytes(7),'mld',('MLDv2MulticastListenerReport',False)),('MLD-report1-short-record',b'\x8f'+bytes(5)+b'\0\1','mld',(None,True)),('MLD-query24',b'\x82'+bytes(23),'mld',('MLDv1MulticastListenerQuery',False)),('MLD-query25',b'\x82'+bytes(24),'mld',(None,True)),('MLD-query28',b'\x82'+bytes(27),'mld',('MLDv2MulticastListenerQuery',False)),('MLD-query43-short-source',b'\x82'+bytes(25)+b'\0\1'+bytes(15),'mld',(None,True))]
 for name,w,f,want in legacy:assert legacy_membership_layer(w,f)==want,name;positive.append('authored-dependency/'+name)
 rejects('foreign-ICMPv6-echo-is-not-MLD',lambda:legacy_membership_layer(b'\x80'+bytes(23),'mld'))
 result={'scope':__doc__,'original_source_whole_rows_checked':all_checked,'positive_controls':positive,'negative_controls':negative,'unknown_extent_queries':unknown_queries,'CLI_plan_SHA256':sha(a.cli_plan/'summary.json'),'constructor_SHA256':sha(Path(__file__).with_name('membership_cli_expected.py')),'verifier_SHA256':sha(Path(__file__).with_name('verify.py')),'auditor_SHA256':sha(Path(__file__))};f=a.out/'summary.json';f.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps({'summary':str(f),'SHA256':sha(f),'positive':len(positive),'negative':len(negative),'source_rows':all_checked}))
if __name__=='__main__':main()
