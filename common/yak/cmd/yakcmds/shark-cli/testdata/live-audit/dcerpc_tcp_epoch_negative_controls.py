"""Corrupt source/epoch/semantic metadata in the actual four-epoch RPC audit.

Temporary logical-control certificates deliberately re-seal only the export hash
to reach semantic guards. They are never independent physical certificates.
Unchanged original frame bytes remain authoritative for physical controls.
"""
import argparse
import base64
import copy
import json
import tempfile
from pathlib import Path

from dcerpc_fragment_messages import audit as logical_audit
from dcerpc_wire_audit import audit as wire_audit
from mail_rtsp_export_invariants import rows,sha


def controls(capture,events,oracle,wire_certificate):
    original=list(rows(events));proof=json.loads(wire_certificate.read_text(encoding='utf8'))
    assert proof['events_sha256']==sha(events)and proof['capture_sha256']==sha(capture)and proof['oracle_sha256']==sha(oracle)
    assert len(proof['original_TCP_epochs'])==4
    logical_audit(events,wire_certificate,oracle,True)
    rpc=[e for e in original if e['protocol']=='dcerpc'and e.get('raw')]
    nak=next(e for e in rpc if e['session']['PType']==13)
    unbound=next(e for e in rpc if e['flow']==nak['flow']and e['session']['PType']==0)
    complete=[e for e in rpc if e['session'].get('Reassembled Message')]
    assert len(complete)==2
    reply=complete[0];other=complete[1]
    partial=next(e for e in rpc if e['session']['PType']==0 and e['session'].get('Fragment'))
    old_frame=proof['compared_PDUs'][0]['physical_frames'][0]

    def one(values,identifier):return next(e for e in values if e['id']==identifier)
    def mutate(identifier,change):return lambda values:change(one(values,identifier))
    def session(identifier,key,value):return mutate(identifier,lambda e:e['session'].__setitem__(key,value))
    def all_one_flow(values):
        for e in values:
            if e['protocol']=='dcerpc':e['flow']=rpc[0]['flow']

    physical=[
        ('conflated-native-flows',all_one_flow),
        ('split-native-epoch',mutate(reply['id'],lambda e:e.__setitem__('flow',999999))),
        ('cross-epoch-byte-contributor',mutate(reply['id'],lambda e:e['source_bytes']['PacketRefs'].append(dict(Number=old_frame,Domain=e['domain'])))),
        ('false-bind-nak-reason',session(nak['id'],'Reject Reason',8)),
        ('false-supported-version',session(nak['id'],'Protocol Versions',['6.0'])),
        ('short-padding-projection',session(nak['id'],'Trailing Padding',base64.b64encode(b'\0\0').decode())),
        ('wrong-padding-interval',mutate(nak['id'],lambda e:e['session']['Wire Field Ranges'].__setitem__('Trailing Padding',[22,24]))),
        ('wrong-version-count',session(nak['id'],'Version Count',0)),
    ]
    logical=[
        ('old-interface-carried-to-unbound-epoch',session(unbound['id'],'Interface','EPM')),
        ('bind-nak-consumes-operation',mutate(nak['id'],lambda e:e.__setitem__('response_to',unbound['id']))),
        ('bind-nak-falsely-matched',session(nak['id'],'Association Status','matched')),
        ('partial-request-publishes-complete-message',session(partial['id'],'Reassembled Message',copy.deepcopy(reply['session']['Reassembled Message']))),
        ('wrong-complete-request-owner',mutate(reply['id'],lambda e:e.__setitem__('response_to',other['response_to']))),
        ('header-included-in-logical-stub',mutate(reply['id'],lambda e:e['session']['Reassembled Message'].__setitem__('Stub',base64.b64encode(base64.b64decode(e['raw'])[16:24]+base64.b64decode(e['session']['Reassembled Message']['Stub'])).decode()))),
        ('parent-from-another-epoch',mutate(reply['id'],lambda e:e['session']['Reassembled Message']['Fragment Sources'][0].__setitem__('PDU ID',other['id']))),
        ('decoded-NDR-without-service-proof',mutate(reply['id'],lambda e:e['session']['Reassembled Message'].__setitem__('Stub Decoded',True))),
    ]
    refused=[]
    with tempfile.TemporaryDirectory(prefix='rpc-epoch-negative-')as directory:
        temp=Path(directory)
        for kind,changes in [('physical',physical),('logical',logical)]:
            for name,change in changes:
                altered=copy.deepcopy(original);change(altered)
                changed=temp/'events.jsonl';changed.write_text(''.join(json.dumps(e)+'\n'for e in altered),encoding='utf8')
                try:
                    if kind=='physical':wire_audit(capture,changed,oracle,None,True,False,None,True)
                    else:
                        negative=copy.deepcopy(proof);negative['events_sha256']=sha(changed);negative['temporary_negative_control_metadata_only']=True
                        cert=temp/'negative-certificate.json';cert.write_text(json.dumps(negative),encoding='utf8')
                        logical_audit(changed,cert,oracle,True)
                except (AssertionError,KeyError):refused.append(dict(name=name,guard=kind))
                else:raise AssertionError(('corruption escaped original-wire/epoch certification',name))
    return dict(scope=__doc__,capture_SHA256=sha(capture),events_SHA256=sha(events),oracle_SHA256=sha(oracle),wire_certificate_SHA256=sha(wire_certificate),auditor_SHA256=sha(Path(__file__)),rejected_controls=refused,negative_controls=len(refused),original_frame_bytes_rewritten=False,temporary_logical_control_hash_resealing=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','events','oracle','wire-certificate','out']:ap.add_argument('--'+name,type=Path,required=True)
    args=ap.parse_args()
    for path in vars(args).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=controls(args.capture,args.events,args.oracle,args.wire_certificate)
    args.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(negative_controls=result['negative_controls'],all_rejected=True)),flush=True)
