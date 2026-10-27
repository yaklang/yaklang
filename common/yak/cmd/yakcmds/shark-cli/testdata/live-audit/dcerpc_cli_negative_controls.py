"""Reject mutations of two actual CLI rows under full original PDU mappings.

The full SDK/wire and CLI certificates are mandatory inputs. Controls reuse the
full immutable ID/source mapping but compare two declared row subsets; they do
not pretend to replay a modified whole public capture.
"""
import argparse
import copy
import json
from pathlib import Path

from dcerpc_cli_invariants import References,compare_row,identity,native_identity,physical_tuple,selected_projection
from mail_rtsp_export_invariants import canonical,rows,sha

ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','sdk','certificate','wire-certificate','out-dir']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
certificate=json.loads(a.certificate.read_text(encoding='utf8'));wire=json.loads(a.wire_certificate.read_text(encoding='utf8'))
assert certificate['before_sha256']==sha(a.before)and certificate['after_sha256']==sha(a.after)and certificate['SDK_sha256']==sha(a.sdk)
assert certificate['wire_certificate_sha256']==sha(a.wire_certificate)and wire['events_sha256']==sha(a.sdk)
refs=References(a.before,a.after)
native={native_identity(e):e for e in rows(a.sdk)if e['protocol']=='dcerpc'and e.get('raw')}
targets={change['frame']for change in certificate['changed_RPC_scope_rows']}
inside=next(r for r in rows(a.after)if r['number']in targets and any(p['protocol']=='dcerpc'and p['byte_source'].get('SHA256')for p in r.get('pdus')or[]))
tuples={physical_tuple(r)for r in rows(a.after)if any(p['protocol']=='dcerpc'for p in r.get('pdus')or[])}
outside=next(r for r in rows(a.after)if physical_tuple(r)not in tuples and r.get('pdus'))
chosen={inside['number'],outside['number']}
before={r['number']:r for r in rows(a.before)if r['number']in chosen}
after={r['number']:r for r in[inside,outside]}
for no in chosen:compare_row(before[no],after[no],refs,tuples)
selected=next(p for p in inside['pdus']if p['protocol']=='dcerpc'and identity(p)in native)
e=native[identity(selected)];selected_projection(selected,e)
a.out_dir.mkdir(exist_ok=False)
rejected=[]
row_controls=[
    ('physical-length',inside['number'],lambda r:r.__setitem__('length',r['length']+1)),
    ('physical-frame-type',inside['number'],lambda r:r.__setitem__('number',str(r['number']))),
    ('physical-endpoint',inside['number'],lambda r:r.__setitem__('source','192.0.2.99')),
    ('capture-domain',inside['number'],lambda r:r['capture_domain'].__setitem__('Interface',999)),
    ('outside-protocol-label',outside['number'],lambda r:r.__setitem__('protocol','DCE/RPC')),
    ('outside-summary',outside['number'],lambda r:r.__setitem__('info','forged RPC connection')),
    ('unrelated-PDU-metadata',outside['number'],lambda r:r['pdus'][0].__setitem__('summary','forged independent protocol summary')),
    ('unrelated-PDU-response',outside['number'],lambda r:r['pdus'][0].__setitem__('response_to',999999999)),
]
for name,no,mutate in row_controls:
    changed=copy.deepcopy(after[no]);mutate(changed)
    (a.out_dir/(name+'.json')).write_text(json.dumps(changed,indent=2)+'\n',encoding='utf8')
    try:compare_row(before[no],changed,refs,tuples)
    except (AssertionError,KeyError,TypeError,ValueError,IndexError)as exc:rejected.append(dict(name=name,frame=no,rejection=str(exc)))
    else:raise AssertionError('CLI row negative accepted: '+name)
selected_controls=[
    ('selected-status',lambda p:p.__setitem__('status','unrecognized')),
    ('selected-response',lambda p:p.__setitem__('response_to',999)),
    ('selected-source-SHA',lambda p:p['byte_source'].__setitem__('SHA256','0'*64)),
    ('selected-source-contributors',lambda p:p['byte_source'].__setitem__('PacketRefs',[])),
    ('selected-typed-owner',lambda p:p['field_values'].__setitem__('pdu',999)),
    ('selected-typed-snapshot',lambda p:p['field_values']['values'][0].__setitem__('value','forged typed fact')),
    ('selected-flat-status',lambda p:p['fields'].__setitem__('status',['forged status'])),
]
for name,mutate in selected_controls:
    changed=copy.deepcopy(selected);mutate(changed)
    (a.out_dir/(name+'.json')).write_text(json.dumps(changed,indent=2)+'\n',encoding='utf8')
    try:selected_projection(changed,e)
    except (AssertionError,KeyError,TypeError,ValueError,IndexError)as exc:rejected.append(dict(name=name,CLI_PDU=selected['id'],SDK_PDU=e['id'],rejection=str(exc)))
    else:raise AssertionError('CLI PDU negative accepted: '+name)
proof=dict(scope=__doc__,full_CLI_certificate_sha256=sha(a.certificate),wire_certificate_sha256=sha(a.wire_certificate),full_before_sha256=sha(a.before),full_after_sha256=sha(a.after),SDK_sha256=sha(a.sdk),selected_physical_rows=sorted(chosen),full_nonselected_PDU_mapping=refs.nonselected,negative_controls_rejected=len(rejected),rejected=rejected)
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(selected_rows=len(chosen),negative_controls_rejected=len(rejected))))
