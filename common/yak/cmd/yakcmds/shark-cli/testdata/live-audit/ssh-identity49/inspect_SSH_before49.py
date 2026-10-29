"""Bounded old-SDK discovery; source plan stays immutable."""
from pathlib import Path
import collections,json
root=Path(__file__).parent
rows=[json.loads(line)for line in(root/'SSH-before-c04a49-01/events.jsonl').read_text('utf8').splitlines()]
print(json.dumps(dict(rows=len(rows),keys=list(rows[0]),statuses=dict(collections.Counter((r.get('protocol','?')+':'+r.get('status','?'))for r in rows)))))
for r in rows:
 session=r.get('session')or{}
 if session.get('Message Number')in(20,30,31,33):
  print(json.dumps({k:r.get(k)for k in('id','protocol','status','profile','source','destination','source_bytes')}|{'session':{k:v for k,v in session.items()if k in('Packet Name','Message Number','Kex Algorithm','Kex Family','Host Key Fingerprint','Host Key Format','Host Key SHA256','Payload Interpretation')}}))
