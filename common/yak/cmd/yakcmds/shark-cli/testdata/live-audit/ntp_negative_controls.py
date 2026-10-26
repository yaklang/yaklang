"""Require the live NTP certificate to reject damaged fields and assembly claims."""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','oracle','events','manifest','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True);assert not any((q/'.git').exists()for q in[a.out.resolve(),*a.out.resolve().parents]);base=[json.loads(s)for s in a.events.read_text(encoding='utf-8-sig').splitlines()];audit=Path(__file__).with_name('ntp_wire_audit.py')
def run(path,out):return subprocess.run([sys.executable,'-X','utf8',str(audit),'--require-association','--capture',str(a.capture),'--oracle',str(a.oracle),'--events',str(path),'--manifest',str(a.manifest),'--out',str(out)],capture_output=True)
r=run(a.events,a.out/'positive.json');assert r.returncode==0,r.stderr.decode('utf8','replace')
results=[];kinds=['version','mode','stratum','poll-sign','seconds','fraction','control-sequence','control-count','control-as-time','fragment-data','complete-flag','complete-alias','assembled-data','assembled-digest','source-hole','source-domain','source-parent','source-packet','source-completeness','request-owner','transaction','field-range','field-alias','raw-digest','authentication','replay-payload','missing-PDU','duplicate-PDU','protocol','forged-encapsulation-domain','clock-word-hardware-state']
if any((e.get('session')or{}).get('Version')==1 for e in base):kinds+=['legacy-modern-mode','legacy-distance-sign','legacy-drift-scale']
if any((e.get('session')or{}).get('Response Association Observation')for e in base):kinds+=['selected-peer-request-association']
for kind in kinds:
 es=copy.deepcopy(base);e=next(e for e in es if(e.get('session')or{}).get('Reassembly Complete'));s=e['session'];time_event=next(x for x in es if(x.get('session')or{}).get('Mode')==3);first=next(x for x in es if(x.get('session')or{}).get('Mode')==6)
 if kind=='version':s['Version']=7
 elif kind=='mode':s['Mode']=4
 elif kind=='stratum':time_event['session']['Stratum']=17
 elif kind=='poll-sign':time_event['session']['Precision']=236
 elif kind=='seconds':time_event['session']['Transmit Timestamp Seconds']+=1
 elif kind=='fraction':time_event['session']['Transmit Timestamp Fraction']-=1
 elif kind=='control-sequence':s['Sequence Number']+=1
 elif kind=='control-count':s['Data Count']+=1
 elif kind=='control-as-time':s['Transmit Timestamp']='AAAAAAAAAAA='
 elif kind=='fragment-data':s['Control Data']='AA=='
 elif kind=='complete-flag':s['Reassembly Complete']=False
 elif kind=='complete-alias':next(v for v in e['field_values']['values']if v['name']=='ntp.control.complete')['value']=False
 elif kind=='assembled-data':s['Reassembled Control Data']='AA=='
 elif kind=='assembled-digest':s['Reassembled Control SHA256']='0'*64
 elif kind=='source-hole':s['Reassembled Byte Source']['Ranges'][0]['Length']-=1
 elif kind=='source-domain':s['Reassembled Byte Source']['Capture Domain']['Interface']+=1
 elif kind=='source-parent':s['Reassembled Byte Source']['Ranges'][0]['Parent PDU']=first['id']
 elif kind=='source-packet':s['Reassembled Byte Source']['Ranges'][0]['Packet References'][0]['Packet']=999999
 elif kind=='source-completeness':s['Reassembled Byte Source']['Byte Coverage Complete']=False
 elif kind=='request-owner':e['response_to']=e['id']
 elif kind=='transaction':e['transaction']+=1
 elif kind=='field-range':next(v for v in e['field_values']['values']if v['name']=='ntp.ctrl.sequence')['pdu_range']['offset']+=1
 elif kind=='field-alias':next(v for v in e['field_values']['values']if v['name']=='ntp.ctrl.sequence')['value']+=1
 elif kind=='raw-digest':e['source_bytes']['SHA256']='0'*64
 elif kind=='authentication':next(x['session']for x in es if'Authentication Verified'in(x.get('session')or{}))['Authentication Verified']=True
 elif kind=='replay-payload':
  duplicate=next(x['session']for x in es if(x.get('session')or{}).get('Control Reassembly')=='duplicate-no-replay');duplicate['Reassembled Control Data']=s['Reassembled Control Data']
 elif kind=='missing-PDU':es.remove(e)
 elif kind=='duplicate-PDU':other=copy.deepcopy(e);other['id']=max(x['id']for x in es)+1;es.append(other)
 elif kind=='protocol':e['protocol']='udp'
 elif kind=='forged-encapsulation-domain':
  e['domain']['Encapsulation']='/vlan:999';e['source_bytes']['PacketRefs'][0]['Domain']['Encapsulation']='/vlan:999'
 elif kind=='clock-word-hardware-state':next(x['session']for x in es if(x.get('session')or{}).get('Status Word Type')=='clock')['Reported Clock Status']=1
 elif kind.startswith('legacy-'):
  legacy=next(x for x in es if(x.get('session')or{}).get('Version')==1);ls=legacy['session']
  if kind=='legacy-modern-mode':ls['Mode']=0
  elif kind=='legacy-distance-sign':ls['Synchronizing Distance Seconds']=-32768
  elif kind=='legacy-drift-scale':ls['Estimated Drift Rate']=32768
 elif kind=='selected-peer-request-association':next(x['session']for x in es if(x.get('session')or{}).get('Response Association Observation'))['Requested Association ID']=7
 path=a.out/(kind+'.jsonl');path.write_text(''.join(json.dumps(x)+'\n'for x in es),encoding='utf8');r=run(path,a.out/(kind+'-unexpected.json'));assert r.returncode!=0 and b'AssertionError'in r.stderr,(kind,r.returncode,r.stderr.decode('utf8','replace'));results.append({'corruption':kind,'detected':'rejected'})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'NTP corruptions rejected')
