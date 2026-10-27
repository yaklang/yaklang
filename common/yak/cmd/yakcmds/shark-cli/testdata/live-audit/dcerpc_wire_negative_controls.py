"""Reject actual owned-capture DCE/RPC field/source/security-claim forgeries.

Run the complete selected record sequence against the original TCP bytes,
reference file and actual endpoint receipts. Keep all mutated exports outside Git.
"""
import argparse
import copy
import json
from pathlib import Path

from dcerpc_wire_audit import audit,canonical,sha

ap=argparse.ArgumentParser(description=__doc__)
for name in ['capture','events','oracle','receipts','out-dir']:ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--require-scoped-ranges',action='store_true')
a=ap.parse_args()
for p in vars(a).values():
    if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out_dir.mkdir(exist_ok=False)
rows=[json.loads(line)for line in a.events.open(encoding='utf-8-sig')]
baseline=audit(a.capture,a.events,a.oracle,a.receipts,a.require_scoped_ranges)
index=next(i for i,e in enumerate(rows)if e.get('profile')=='dcerpc-security-envelope')

def both(e,fn):
    fn(e['session'])
    if isinstance(e.get('fields'),dict):fn(e['fields'])

cases=[
    ('token-octets',lambda e:both(e,lambda s:s.__setitem__('Authentication Token','Zm9yZ2Vk'))),
    ('opaque-body',lambda e:both(e,lambda s:s.__setitem__('Opaque PDU Body','Zm9yZ2Vk'))),
    ('padding',lambda e:both(e,lambda s:s.__setitem__('Authentication Padding',''))),
    ('token-length',lambda e:both(e,lambda s:s.__setitem__('Auth Length',999))),
    ('auth-context',lambda e:both(e,lambda s:s['Security Trailer'].__setitem__('Auth Context ID',999))),
    ('auth-type',lambda e:both(e,lambda s:s['Security Trailer'].__setitem__('Auth Type',0))),
    ('authentication-claim',lambda e:both(e,lambda s:s.__setitem__('Authentication Verified',True))),
    ('identity-claim',lambda e:both(e,lambda s:s.__setitem__('Peer Identity Verified',True))),
    ('body-decoding-claim',lambda e:both(e,lambda s:s.__setitem__('Stub Decoded',True))),
    ('alignment-claim',lambda e:both(e,lambda s:s.__setitem__('Trailer Alignment Validated',True))),
    ('token-wire-range',lambda e:both(e,lambda s:s['Wire Field Ranges'].__setitem__('Authentication Token',[0,1]))),
    ('call-id',lambda e:both(e,lambda s:s.__setitem__('Call ID',999))),
    ('packet-type',lambda e:both(e,lambda s:s.__setitem__('PType',0))),
    ('packet-name',lambda e:both(e,lambda s:s.__setitem__('Packet Name','Authenticated procedure call'))),
    ('identity-collision',lambda e:e.__setitem__('id',rows[0]['id'])),
    ('typed-identity-owner',lambda e:e['field_values'].__setitem__('pdu',1)),
    ('contributor-loss',lambda e:e['source_bytes'].__setitem__('PacketRefs',[])),
    ('completion-frame',lambda e:e['source_bytes']['CompletedIn'].__setitem__('Number',1)),
    ('capture-domain',lambda e:e['domain'].__setitem__('Interface',999)),
    ('raw-SHA',lambda e:e['source_bytes'].__setitem__('SHA256','0'*64)),
    ('raw-PDU',lambda e:e.__setitem__('raw','Zm9yZ2Vk')),
    ('protocol-erasure',lambda e:e.__setitem__('protocol','tcp')),
    ('query-refusal-hidden',lambda e:e.__setitem__('fields_error','forged refusal despite decoded label')),
]
controls=[(name,index,mutate)for name,mutate in cases]
proposal=next(i for i,e in enumerate(rows)if len((e.get('session')or{}).get('Presentation Contexts')or[])==3)
feature_ack=next(i for i,e in enumerate(rows)if 'Observed Bind Time Feature Bits'in(e.get('session')or{}))
controls.extend([
    ('proposal-order',proposal,lambda e:e['session']['Presentation Contexts'].reverse()),
    ('proposal-UUID',proposal,lambda e:e['session']['Presentation Contexts'][0].__setitem__('Abstract Syntax','Zm9yZ2Vk')),
    ('proposal-transfer',proposal,lambda e:e['session']['Presentation Contexts'][1].__setitem__('Transfer Version',999)),
    ('proposal-feature-role',proposal,lambda e:e['session']['Presentation Contexts'][2].__setitem__('Bind Time Feature Proposal',False)),
    ('result-ordinal',feature_ack,lambda e:e['session']['Presentation Context Results'][1].__setitem__('Ordinal',999)),
    ('result-loss',feature_ack,lambda e:e['session']['Presentation Context Results'].pop()),
    ('result-owner',feature_ack,lambda e:e['session']['Presentation Context Results'][2].__setitem__('Proposed Context ID',0)),
    ('result-transfer',feature_ack,lambda e:e['session']['Presentation Context Results'][1].__setitem__('Transfer Version',999)),
    ('feature-as-ordinary-context',feature_ack,lambda e:e['session']['Accepted Context IDs'].append(2)),
    ('feature-bits',feature_ack,lambda e:e['session'].__setitem__('Observed Bind Time Feature Bits',65535)),
    ('feature-endpoint-state',feature_ack,lambda e:e['session'].__setitem__('Bind Time Feature Endpoint State Verified',True)),
])
if a.require_scoped_ranges:
    controls.extend([
        ('proposal-range-loss',proposal,lambda e:e['session']['Presentation Contexts'][0].pop('Wire Field Ranges')),
        ('proposal-range-other-ordinal',proposal,lambda e:e['session']['Presentation Contexts'][1]['Wire Field Ranges'].__setitem__('Transfer Syntax',[52,68])),
        ('result-range-loss',feature_ack,lambda e:e['session']['Presentation Context Results'][0].pop('Wire Field Ranges')),
        ('result-range-other-ordinal',feature_ack,lambda e:e['session']['Presentation Context Results'][1]['Wire Field Ranges'].__setitem__('Transfer Syntax',[40,56])),
    ])
rejected=[]
for name,target,mutate in controls:
    changed=copy.deepcopy(rows);mutate(changed[target])
    path=a.out_dir/(name+'.jsonl');path.write_text(''.join(canonical(e)+'\n'for e in changed),encoding='utf8')
    try:audit(a.capture,path,a.oracle,a.receipts,a.require_scoped_ranges)
    except (AssertionError,KeyError,ValueError,IndexError,TypeError)as exc:
        rejected.append(dict(name=name,PDU=rows[target]['id'],rejection=str(exc),mutated_export_sha256=sha(path.read_bytes())))
    else:raise AssertionError('DCE/RPC negative control accepted: '+name)
proof=dict(scope=__doc__,capture_sha256=baseline['capture_sha256'],events_sha256=baseline['events_sha256'],reference_sha256=baseline['oracle_sha256'],baseline_checks=baseline['checks'],negative_controls_rejected=len(rejected),rejected=rejected)
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(baseline_checks=baseline['checks'],negative_controls_rejected=len(rejected))))
