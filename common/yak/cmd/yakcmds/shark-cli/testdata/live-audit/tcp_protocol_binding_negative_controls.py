"""Reject forged TCP-generation/session/page links on a source-bound capture.

Inputs are temporary metadata corruptions, never new wire certificates. Source
captures and producer exports stay immutable. No decoder sees these sidecars.
"""
import argparse
import copy
import json
import tempfile
from pathlib import Path

from mail_rtsp_export_invariants import rows,sha
from tcp_protocol_binding_audit import audit


def controls(capture,observer,sdk,cli,temporary_root):
    baseline=audit(capture,observer,sdk,cli)
    native=list(rows(sdk));page=list(rows(cli));hooks=json.loads(observer.read_text('utf8'))
    selected=next(i for i,e in enumerate(native)if e.get('tcp_flow_id')==2 and e['source_bytes'].get('PacketRefs'))
    first=next(i for i,e in enumerate(native)if e.get('tcp_flow_id')==1 and e['source_bytes'].get('PacketRefs'))
    row_index=next(i for i,r in enumerate(page)if r.get('tcp_flow_id')==2 and r.get('stream_id'))
    row_one=next(i for i,r in enumerate(page)if r.get('tcp_flow_id')==1 and r.get('stream_id'))
    pdu_row=next(i for i,r in enumerate(page)if any(p.get('tcp_flow_id')==1 for p in(r.get('pdus')or[])))
    selected_ref=native[selected]['source_bytes']['PacketRefs'][0]['Number']
    def alias(e):return next(v for v in e['field_values']['values']if v['name']=='tcp.flow.id')
    def pdu(value):return next(p for p in value[pdu_row]['pdus']if p.get('tcp_flow_id')==1)
    def sdk_change(fn):
        def change(n,p,h):fn(n)
        return change
    def cli_change(fn):
        def change(n,p,h):fn(p)
        return change
    def hook_change(fn):
        def change(n,p,h):fn(h)
        return change
    def wrong_native(n):n[selected]['tcp_flow_id']=3;alias(n[selected]).update(value=3,exact_value='3')
    def missing_native(n):
        n[selected].pop('tcp_flow_id');n[selected]['field_values']['values']=[v for v in n[selected]['field_values']['values']if v['name']!='tcp.flow.id']
    def false_deferred_payload(p):
        for name in['tcp_flow_id','tcp_flow_domain','stream_id']:p[pdu_row].pop(name,None)
    plan=[
        ('SDK-foreign-generation-same-tuple',sdk_change(wrong_native)),
        ('SDK-missing-native-binding-and-alias',sdk_change(missing_native)),
        ('SDK-boolean-generation',sdk_change(lambda n:n[first].__setitem__('tcp_flow_id',True))),
        ('SDK-boolean-typed-alias',sdk_change(lambda n:alias(n[first]).__setitem__('value',True))),
        ('SDK-floating-typed-alias',sdk_change(lambda n:alias(n[first]).__setitem__('value',1.0))),
        ('SDK-invented-wire-range-on-event-ID',sdk_change(lambda n:alias(n[first]).__setitem__('range',{'start':0,'length':8}))),
        ('SDK-boolean-contributor',sdk_change(lambda n:n[first]['source_bytes']['PacketRefs'][0].__setitem__('Number',True))),
        ('SDK-boolean-capture-domain',sdk_change(lambda n:n[first]['domain'].__setitem__('Section',False))),
        ('SDK-wrong-observer-direction',sdk_change(lambda n:n[selected].__setitem__('direction',1-n[selected]['direction']))),
        ('SDK-wrong-protocol-namespace',sdk_change(lambda n:n[selected].__setitem__('flow',999999))),
        ('CLI-boolean-row-generation',cli_change(lambda p:p[row_one].__setitem__('tcp_flow_id',True))),
        ('CLI-missing-native-preview',cli_change(lambda p:p[row_index].pop('stream_id'))),
        ('CLI-payload-cannot-masquerade-as-deferred-opening',cli_change(false_deferred_payload)),
        ('CLI-boolean-preview-ID',cli_change(lambda p:p[row_index].__setitem__('stream_id',True))),
        ('CLI-floating-row-generation',cli_change(lambda p:p[row_index].__setitem__('tcp_flow_id',float(p[row_index]['tcp_flow_id'])))),
        ('CLI-preview-splices-two-generations',cli_change(lambda p:p[row_index].__setitem__('stream_id',page[row_one]['stream_id']))),
        ('CLI-boolean-PDU-generation',cli_change(lambda p:pdu(p).__setitem__('tcp_flow_id',True))),
        ('CLI-boolean-field-map-ID',cli_change(lambda p:pdu(p)['fields'].__setitem__('tcp.flow.id',[True]))),
        ('CLI-invented-wire-range-on-event-ID',cli_change(lambda p:alias(pdu(p)).__setitem__('range',{'start':0,'length':8}))),
        ('hook-contributor-in-another-generation',hook_change(lambda h:next(p for p in h['packets']if any(r['Number']==selected_ref for r in p['refs'])).__setitem__('flow',3))),
        ('hook-wrong-original-opener',hook_change(lambda h:next(f for f in h['flows']if f['flow']==2).__setitem__('source','127.0.0.1:1'))),
        ('hook-failed-replay-cannot-certify',hook_change(lambda h:h.__setitem__('error','controlled replay failure'))),
        ('hook-wrong-capture-digest',hook_change(lambda h:h.__setitem__('capture_SHA256','0'*64))),
    ]
    temporary_root=temporary_root.resolve()
    assert temporary_root.is_dir()and not any((p/'.git').exists()for p in[temporary_root,*temporary_root.parents])
    rejected=[]
    with tempfile.TemporaryDirectory(prefix='TCP-binding-controls-',dir=temporary_root)as directory:
        temp=Path(directory).resolve();assert temp.parent==temporary_root
        for name,change in plan:
            n,p,h=copy.deepcopy(native),copy.deepcopy(page),copy.deepcopy(hooks);change(n,p,h)
            native_file=temp/'events.jsonl';page_file=temp/'page.jsonl';hook_file=temp/'hooks.json'
            native_file.write_text(''.join(json.dumps(v,separators=(',',':'))+'\n'for v in n),encoding='utf8')
            page_file.write_text(''.join(json.dumps(v,separators=(',',':'))+'\n'for v in p),encoding='utf8')
            hook_file.write_text(json.dumps(h),encoding='utf8')
            try:audit(capture,hook_file,native_file,page_file)
            except (AssertionError,KeyError,TypeError):rejected.append(name)
            else:raise AssertionError(('forged generation/type/provenance escaped',name))
    return dict(scope=__doc__,capture_SHA256=sha(capture),observer_SHA256=sha(observer),SDK_SHA256=sha(sdk),CLI_SHA256=sha(cli),baseline_TCP_events=baseline['source_bound_TCP_events'],negative_controls=len(rejected),rejected_controls=rejected,auditor_SHA256=sha(Path(__file__)),semantic_corruption_only=True,wire_or_endpoint_injection_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','observer','sdk','cli','temporary-root','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=controls(a.capture,a.observer,a.sdk,a.cli,a.temporary_root)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(result,out,indent=2);out.write('\n')
    print(json.dumps(dict(negative_controls=result['negative_controls'],all_rejected=True)),flush=True)
