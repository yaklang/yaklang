"""Reject missing, relabelled, reordered or corrupted capture-close projections."""
import argparse
import copy
import json
import tempfile
from pathlib import Path

from close_publication_export_audit import audit
from mail_rtsp_export_invariants import rows,sha


def controls(before,after,sdk,temporary_root):
    baseline=audit(before,after,sdk)
    original=list(rows(after))
    index=next(i for i,r in enumerate(original)if r.get('record_type')=='protocol_publication')
    def pdu(value):return value[index]['published_pdus'][0]
    def alias(value):return next(v for v in pdu(value)['field_values']['values']if v['name']=='tcp.flow.id')
    def move_to_packet(value):
        publication=value.pop(index)
        value[index-1].setdefault('published_pdus',[]).extend(publication['published_pdus'])
    def early(value):value.insert(0,value.pop(index))
    plan=[
        ('missing-final-diagnostic',lambda v:v.pop(index)),
        ('invented-physical-number',lambda v:v[index].update(number=999999)),
        ('invented-physical-time',lambda v:v[index].update(time='2026-10-28T00:00:00Z')),
        ('wrong-publication-phase',lambda v:v[index].update(publication_phase='packet')),
        ('relocated-to-last-packet',move_to_packet),
        ('publication-before-physical-rows',early),
        ('duplicate-final-publication',lambda v:v.append(copy.deepcopy(v[index]))),
        ('changed-diagnostic-reason',lambda v:pdu(v).update(summary='invented successful recovery')),
        ('missing-explicit-TCP-link',lambda v:pdu(v).pop('tcp_flow_id')),
        ('floating-typed-TCP-ID',lambda v:alias(v).update(value=float(alias(v)['value']))),
        ('fabricated-wire-range',lambda v:alias(v).update(range=dict(start=0,length=8))),
        ('hidden-detail-refusal',lambda v:pdu(v).update(id=0,status='limited')),
    ]
    temporary_root=temporary_root.resolve()
    assert temporary_root.is_dir()and not any((p/'.git').exists()for p in[temporary_root,*temporary_root.parents])
    rejected=[]
    with tempfile.TemporaryDirectory(prefix='close-projection-controls-',dir=temporary_root)as folder:
        target=Path(folder)/'corrupted.jsonl'
        for name,change in plan:
            value=copy.deepcopy(original);change(value)
            target.write_text(''.join(json.dumps(r,separators=(',',':'))+'\n'for r in value),encoding='utf8')
            try:audit(before,target,sdk)
            except(AssertionError,KeyError,TypeError):rejected.append(name)
            else:raise AssertionError(('corrupted publication escaped',name))
    return dict(scope=__doc__,before_SHA256=sha(before),after_SHA256=sha(after),SDK_SHA256=sha(sdk),baseline_final_records=baseline['new_capture_close_records'],negative_controls=len(rejected),rejected_controls=rejected,auditor_SHA256=sha(Path(__file__)),metadata_controls_only=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['before','after','sdk','temporary-root','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=controls(a.before,a.after,a.sdk,a.temporary_root)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(result,out,indent=2);out.write('\n')
    print(json.dumps(dict(negative_controls=result['negative_controls'],all_rejected=True)),flush=True)
