"""Reject consistent-looking but incorrect RPC owner and operation projections.

Each control changes a complete SDK export and re-runs original-capture wire,
field, contributor, ordered-context and independently derived call-link checks.
Captures and mutated exports must remain outside Git.
"""
import argparse
import copy
import json
from pathlib import Path

from dcerpc_wire_audit import audit,canonical,sha

ap=argparse.ArgumentParser(description=__doc__)
for name in['capture','events','oracle','out-dir']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((q/'.git').exists()for q in[path.resolve(),*path.resolve().parents])
a.out_dir.mkdir(exist_ok=False)
baseline=audit(a.capture,a.events,a.oracle,None,True,True)
rows=[json.loads(line)for line in a.events.open(encoding='utf8')]
reply=next(i for i,e in enumerate(rows)if e['protocol']=='dcerpc'and e['response_to']and e['session']['PType']==2)
request=next(i for i,e in enumerate(rows)if e['id']==rows[reply]['response_to'])
other=next(e['id']for e in rows if e['protocol']=='dcerpc'and e.get('raw')and e['id']!=rows[reply]['response_to']and e['id']<rows[reply]['id'])

def owner(event,value):
    event['response_to']=value
    aliases=event['field_values']['values']
    for field in aliases:
        if field['name']=='pdu.response_to':field['value']=value;field['exact_value']=str(value)
    if value==0:aliases[:]=[v for v in aliases if v['name']!='pdu.response_to']

controls=[
    ('missing-owner-and-alias',reply,lambda e:owner(e,0)),
    ('wrong-original-request-and-alias',reply,lambda e:owner(e,other)),
    ('self-owner-and-alias',reply,lambda e:owner(e,e['id'])),
    ('future-owner-and-alias',reply,lambda e:owner(e,max(r['id']for r in rows)+1)),
    ('owner-string-type',reply,lambda e:owner(e,str(e['response_to']))),
    ('typed-owner-only',reply,lambda e:next(v for v in e['field_values']['values']if v['name']=='pdu.response_to').__setitem__('value',other)),
    ('typed-owner-exact-decimal',reply,lambda e:next(v for v in e['field_values']['values']if v['name']=='pdu.response_to').__setitem__('exact_value','000'+str(e['response_to']))),
    ('reply-connection-generation',reply,lambda e:e.__setitem__('flow',e['flow']+999)),
    ('reply-operation-owner',reply,lambda e:e['session'].__setitem__('Matched Request','EPM.ept_delete')),
    ('request-operation-name',request,lambda e:e['session'].__setitem__('Packet Name','EPM.ept_delete')),
    ('request-interface-alias',request,lambda e:e['session'].__setitem__('Interface','SRVSVC')),
    ('request-interface-version',request,lambda e:e['session'].__setitem__('Interface Version Major',99)),
    ('request-operation-number',request,lambda e:e['session'].__setitem__('OpNum',99)),
    ('reply-association-success',reply,lambda e:e['session'].__setitem__('Association Status','missing-request')),
    ('reply-context',reply,lambda e:e['session'].__setitem__('Context ID',999)),
    ('literal-wire-call-id',reply,lambda e:e['session'].__setitem__('Call ID',999)),
]
cached=next((link for link in baseline['numeric_call_links']['links']if link['request_frames']!=[link['request_stream_prefix_delivery_frame']]),None)
if cached:
    cached_request=next(i for i,e in enumerate(rows)if e['id']==cached['request_PDU'])
    def normalize_contributors_to_delivery(event):
        source=event['source_bytes'];source['PacketRefs']=[dict(source['PacketRefs'][0],Number=cached['request_stream_prefix_delivery_frame'])]
        source['CompletedIn']['Number']=cached['request_stream_prefix_delivery_frame']
    controls.append(('replace-original-contributors-with-Wireshark-delivery-frame',cached_request,normalize_contributors_to_delivery))
rejected=[]
for name,index,mutate in controls:
    changed=copy.deepcopy(rows);mutate(changed[index])
    path=a.out_dir/(name+'.jsonl');path.write_text(''.join(canonical(e)+'\n'for e in changed),encoding='utf8')
    try:audit(a.capture,path,a.oracle,None,True,True)
    except(AssertionError,KeyError,ValueError,IndexError,TypeError)as exc:rejected.append(dict(name=name,PDU=rows[index]['id'],rejection=str(exc),mutated_export_sha256=sha(path.read_bytes())))
    else:raise AssertionError('RPC numeric ownership negative accepted: '+name)
if cached:
    def replace_reference(node,name,value):
        changed=0
        if isinstance(node,dict):
            for key,child in node.items():
                if key==name:node[key]=[str(value)]*len(child)if isinstance(child,list)else str(value);changed+=1
                elif isinstance(child,(dict,list)):changed+=replace_reference(child,name,value)
        elif isinstance(node,list):
            for child in node:changed+=replace_reference(child,name,value)
        return changed
    for field in['dcerpc.request_in','dcerpc.response_in']:
        oracle=json.loads(a.oracle.read_text(encoding='utf8'))
        assert replace_reference(oracle,field,cached['request_frames'][0])>0
        path=a.out_dir/(field+'-uses-byte-arrival-instead-of-stream-delivery.json');path.write_text(json.dumps(oracle)+'\n',encoding='utf8')
        try:audit(a.capture,a.events,path,None,True,True)
        except(AssertionError,KeyError,ValueError,IndexError,TypeError)as exc:rejected.append(dict(name=field+'-wrong-stream-delivery',rejection=str(exc),mutated_oracle_sha256=sha(path.read_bytes())))
        else:raise AssertionError('incorrect reference delivery attribution accepted: '+field)
proof=dict(scope=__doc__,capture_sha256=baseline['capture_sha256'],events_sha256=baseline['events_sha256'],oracle_sha256=baseline['oracle_sha256'],baseline_links=baseline['numeric_call_links']['positive_links'],negative_controls_rejected=len(rejected),rejected=rejected)
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(baseline_links=proof['baseline_links'],negative_controls_rejected=len(rejected))))
