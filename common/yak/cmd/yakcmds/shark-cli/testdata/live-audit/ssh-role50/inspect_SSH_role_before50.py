"""Bounded discovery of four independently preselected sender contradictions."""
from pathlib import Path
import json,hashlib
root=Path(__file__).parent
p=root/'SSH-role-before-cc9d50-01/events.jsonl'
selected={24,36,89,103};out=[]
for line in p.read_text('utf8').splitlines():
 e=json.loads(line);refs=e['source_bytes'].get('PacketRefs')or[]
 if len(refs)!=1 or refs[0]['Number']not in selected:continue
 s=e.get('session')or{}
 out.append(dict(frame=refs[0]['Number'],protocol=e['protocol'],status=e['status'],
 Packet_Name=s.get('Packet Name'),method=s.get('Kex Algorithm'),
 derived_hostkey=bool(s.get('Host Key Fingerprint')),selected_method_fields='Initial Packet Fields'in s,
 fields_error=bool(e.get('fields_error')),error=e.get('error','')))
assert len(out)==4
result=dict(scope=__doc__,input_SHA256=hashlib.sha256(p.read_bytes()).hexdigest(),observations=out)
with(root/'SSH-role-before-cc9d50-01/discovery-role.json').open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
print(json.dumps(result))
