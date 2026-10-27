"""Reject forged original-record controls and false native packet/flow metadata.

Temporary capture/manifest hashes are explicitly re-sealed where needed to reach
record-shape and provenance guards. These fixtures are never wire certificates,
and no actual endpoint-acceptance claim is derived from them.
"""
import argparse
import collections
import copy
import hashlib
import json
import tempfile
from pathlib import Path

from tcp_flow_observation_audit import audit as flow_audit
from tcp_opening_wire_audit import capture_records,source_partition


def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def controls(capture,source_capture,manifest,source_certificate,before,after,oracle,temporary_root):
    physical,by_frame,epochs,source=source_partition(capture,source_capture,manifest,source_certificate)
    baseline,_=flow_audit(capture,source_capture,manifest,source_certificate,before,after,oracle)
    plan=json.loads(manifest.read_text('utf8'));native=json.loads(after.read_text('utf8'));cert=json.loads(source_certificate.read_text('utf8'))
    header,records,_=capture_records(capture)
    duplicate=next(n for n,p in physical.items()if p['syn']and not p['ack']and by_frame[n]>1)
    old_reset=source['delayed_old_reset_cursor_checks'][0]['observed_reset_frame']
    payload=next(n for n,p in physical.items()if p['payload'])
    payloads=[n for n,p in physical.items()if p['payload']]
    selected=[dict(record=records[n]['record'],parent=plan['original_frame_by_observation_frame'][str(n)])for n in records]
    mapping=baseline['after']['native_flow_epochs'];new_flow=next(int(flow)for flow,epoch in mapping.items()if epoch==2)
    def packet(value,no):return next(p for p in value['packets']if p['refs'][0]['Number']==no)
    def missing(value):value['packets'].remove(packet(value,duplicate))
    metadata=[
        ('missing-opening-copy',missing),
        ('late-old-reset-forced-into-reference-new-stream',lambda v:packet(v,old_reset).__setitem__('flow',new_flow)),
        ('opening-capture-time-replaced',lambda v:packet(v,duplicate).__setitem__('captured','2000-01-01T00:00:00Z')),
        ('foreign-capture-domain',lambda v:packet(v,duplicate)['refs'][0]['Domain'].__setitem__('Interface',999)),
        ('wrong-flow-opener',lambda v:next(f for f in v['flows']if f['flow']==new_flow).__setitem__('source','127.0.0.1:135')),
        ('false-half-open-with-observed-opening',lambda v:next(f for f in v['flows']if f['flow']==new_flow).__setitem__('half_open',True)),
        ('wrong-packet-direction',lambda v:packet(v,duplicate).__setitem__('direction',1)),
    ]
    simple=[
        ('false-source-digest',lambda p,c:p.__setitem__('source_capture_SHA256','0'*64)),
        ('false-source-certificate-digest',lambda p,c:p.__setitem__('source_certificate_SHA256','0'*64)),
        ('false-current-capture-digest',lambda p,c:p.__setitem__('capture_SHA256','0'*64)),
        ('wrong-original-record',lambda p,c:p['original_frame_by_observation_frame'].__setitem__(str(duplicate),1)),
        ('boolean-original-record',lambda p,c:p['original_frame_by_observation_frame'].__setitem__(str(duplicate),True)),
        ('undeclared-opening-copies',lambda p,c:p.__setitem__('duplicated_source_frames',[])),
        ('hidden-delayed-old-reset',lambda p,c:p.__setitem__('delayed_old_reset_source_frame',None)),
        ('false-network-injection-scope',lambda p,c:p.__setitem__('network_retransmission_or_loss_injection',True)),
        ('forged-source-epoch',lambda p,c:c['original_TCP_epochs'][0].__setitem__('epoch',999)),
    ]
    refused=[]
    with tempfile.TemporaryDirectory(prefix='opening-negative-',dir=temporary_root)as directory:
        temp=Path(directory).resolve();assert temporary_root.resolve()in temp.parents
        for name,change in simple:
            altered=copy.deepcopy(plan);certificate=copy.deepcopy(cert);change(altered,certificate)
            certificate_path=temp/'source-certificate.json';certificate_path.write_text(json.dumps(certificate),encoding='utf8')
            if name=='forged-source-epoch':altered['source_certificate_SHA256']=sha(certificate_path)
            else:certificate_path=source_certificate
            target=temp/'manifest.json';target.write_text(json.dumps(altered),encoding='utf8')
            try:source_partition(capture,source_capture,target,certificate_path)
            except (AssertionError,KeyError):refused.append(dict(name=name,guard='original record/handshake source'))
            else:raise AssertionError(('source corruption escaped',name))
        for name in['removed-application-record','duplicated-application-record','reordered-application-records','old-reset-before-original-data']:
            altered=copy.deepcopy(selected)
            if name=='removed-application-record':altered.pop(payload-1)
            elif name=='duplicated-application-record':altered.insert(payload,copy.deepcopy(altered[payload-1]))
            elif name=='reordered-application-records':
                a,b=payloads[:2];altered[a-1],altered[b-1]=altered[b-1],altered[a-1]
            else:altered.insert(0,altered.pop(old_reset-1))
            target_capture=temp/'altered.pcap';target_capture.write_bytes(header+b''.join(p['record']for p in altered))
            updated=copy.deepcopy(plan);updated['capture_SHA256']=sha(target_capture);updated['physical_frames']=len(altered)
            updated['original_frame_by_observation_frame']={str(i+1):p['parent']for i,p in enumerate(altered)}
            counts=collections.Counter(p['parent']for p in altered)
            _,source_records,_=capture_records(source_capture)
            updated['removed_source_frames']=sorted(set(source_records)-set(counts));updated['duplicated_source_frames']=sorted(n for n,count in counts.items()if count>1)
            target_manifest=temp/'manifest.json';target_manifest.write_text(json.dumps(updated),encoding='utf8')
            try:source_partition(target_capture,source_capture,target_manifest,source_certificate)
            except (AssertionError,KeyError):refused.append(dict(name=name,guard='re-sealed original record-shape control'))
            else:raise AssertionError(('record corruption escaped',name))
        for name,change in metadata:
            value=copy.deepcopy(native);change(value);target=temp/'native.json';target.write_text(json.dumps(value),encoding='utf8')
            try:flow_audit(capture,source_capture,manifest,source_certificate,before,target,oracle)
            except (AssertionError,KeyError):refused.append(dict(name=name,guard='native source/flow metadata'))
            else:raise AssertionError(('native metadata corruption escaped',name))
    return dict(scope=__doc__,capture_SHA256=sha(capture),source_capture_SHA256=sha(source_capture),source_certificate_SHA256=sha(source_certificate),manifest_SHA256=sha(manifest),after_observer_SHA256=sha(after),oracle_SHA256=sha(oracle),auditor_SHA256=sha(Path(__file__)),negative_controls=len(refused),rejected_controls=refused,temporary_control_hash_resealing=True,actual_endpoint_injection_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','source-capture','manifest','source-certificate','before','after','oracle','temporary-root','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=controls(a.capture,a.source_capture,a.manifest,a.source_certificate,a.before,a.after,a.oracle,a.temporary_root)
    a.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(negative_controls=result['negative_controls'],all_rejected=True)),flush=True)
