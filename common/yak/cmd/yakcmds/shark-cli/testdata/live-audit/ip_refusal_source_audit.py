"""Independently enumerate direct-IPv4 refusal sources, then check complete SDK events."""
from pathlib import Path
import argparse,collections,copy,hashlib,json,sys
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--events',required=True,type=Path);ap.add_argument('--producer',required=True,type=Path);ap.add_argument('--capture',required=True,type=Path)
ap.add_argument('--source',required=True);ap.add_argument('--out',required=True,type=Path);ap.add_argument('--audit-modules',type=Path,default=Path(__file__).parent)
a=ap.parse_args();a.out.mkdir();sys.path.insert(0,str(a.audit_modules))
from vxlan_endpoint_projection_audit import capture_records
import ip_refusal_expected
from ip_refusal_expected import literal,check
def check_source_events(events,rec):
 facts=literal(rec)
 assert facts is not None
 assert len(events)==1,('An independently selected physical source requires exactly one refusal and no child PDU',rec['frame'],len(events))
 assert check(events[0],rec)==facts
 return facts
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
producer=json.loads(a.producer.read_text('utf8'))
assert producer['source']==a.source and producer['exit_code']==0 and producer['capture_SHA256']==sha(a.capture)and producer['export_SHA256']==sha(a.events)
groups=collections.defaultdict(list)
for line in a.events.open(encoding='utf8'):
 e=json.loads(line);refs=e['source_bytes'].get('PacketRefs')or[]
 if len(refs)==1:groups[refs[0]['Number']].append(e)
counts=collections.Counter();selected=0;representative=None
with(a.out/'source-ledger.jsonl').open('x',encoding='utf8')as log:
 for frame,rec in enumerate(capture_records(a.capture),1):
  rec['frame']=frame;facts=literal(rec)
  if facts is None:continue
  selected+=1;events=groups.get(frame,[])
  assert check_source_events(events,rec)==facts
  event=events[0]
  if representative is None:representative=(event,rec)
  counts[event['profile']+'/'+event['expert']]+=1
  log.write(json.dumps(dict(frame=frame,event_id=event['id'],wire_SHA256=hashlib.sha256(rec['wire']).hexdigest(),facts=facts))+'\n')
assert selected>0
negatives=[];event,rec=representative
for name,mutate in [('wrong-protocol',lambda e:e.__setitem__('protocol','tls')),('invented-flow',lambda e:e.__setitem__('flow',1)),('invented-transaction',lambda e:e.__setitem__('transaction',1)),('wrong-frame',lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',rec['frame']+1)),('wrong-interface',lambda e:e['domain'].__setitem__('Interface',rec['interface']+1)),('invented-source',lambda e:e.__setitem__('source','203.0.113.1')),('invented-child-layer',lambda e:e['session']['Observed Header Layers'].append('TLS'))]:
 changed=copy.deepcopy(event);mutate(changed)
 try:check(changed,rec)
 except(AssertionError,KeyError,ValueError):negatives.append(dict(name=name,rejected=True))
 else:raise AssertionError('Semantic corruption accepted: '+name)
# This is the selector's independently observed cardinality, never an input
# event's presence check; dropping the whole event cannot escape the oracle.
assert literal(rec)is not None
try:check_source_events([],rec)
except AssertionError:negatives.append(dict(name='entire-refusal-event-missing',rejected=True))
summary=dict(scope=__doc__,source=a.source,physical_records=frame,independently_selected_sources=selected,whole_events_exact=True,counts=dict(counts),capture_SHA256=sha(a.capture),events_SHA256=sha(a.events),producer_SHA256=sha(a.producer),selector_SHA256=sha(Path(ip_refusal_expected.__file__)),auditor_SHA256=sha(Path(__file__)),reader_SHA256=sha(a.audit_modules/'vxlan_endpoint_projection_audit.py'),negative_controls=negatives)
(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');print(json.dumps(summary),flush=True)
