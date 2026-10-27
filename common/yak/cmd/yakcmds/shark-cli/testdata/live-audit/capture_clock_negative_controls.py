"""Reject forged provenance, clock declarations and fragment source intervals."""
import argparse,copy,json,tempfile
from pathlib import Path
from capture_clock_wire_audit import audit,sha


def controls(capture,manifest,primary,foreign,temporary_root):
    baseline=audit(capture,manifest,primary,foreign);original=json.loads(manifest.read_text('utf8'))
    fragment=next(i for i,r in enumerate(original['ledger'])if r['change']['kind']=='IPv4-fragment'and r['source']=='primary')
    exact=next(i for i,r in enumerate(original['ledger'])if r['change']['kind']=='exact')
    external=next(i for i,r in enumerate(original['ledger'])if r['source']=='CoAP')
    plan=[
        ('wrong-capture-digest',lambda d:d.update(capture_SHA256='0'*64)),
        ('wrong-physical-count',lambda d:d.update(physical_records=d['physical_records']-1)),
        ('missing-original-record',lambda d:d['ledger'].pop(exact)),
        ('wrong-output-number',lambda d:d['ledger'][exact].update(number=999999)),
        ('boolean-original-frame',lambda d:d['ledger'][exact].update(source_frame=True)),
        ('forged-original-record-digest',lambda d:d['ledger'][exact].update(source_wire_SHA256='0'*64)),
        ('forged-original-time',lambda d:d['ledger'][exact].update(source_time_ns=d['ledger'][exact]['source_time_ns']+1)),
        ('forged-observed-time',lambda d:d['ledger'][exact].update(time_ns=d['ledger'][exact]['time_ns']+1)),
        ('forged-source-domain',lambda d:d['ledger'][external].update(source='primary')),
        ('forged-physical-domain',lambda d:d['ledger'][external].update(interface=0)),
        ('false-fragment-interval',lambda d:d['ledger'][fragment]['change'].update(IP_payload_interval=[8,64])),
        ('disguised-envelope-mutation',lambda d:d['ledger'][fragment].update(change=dict(kind='exact'))),
    ]
    rejected=[]
    temporary_root=temporary_root.resolve()
    assert temporary_root.is_dir()and not any((p/'.git').exists()for p in[temporary_root,*temporary_root.parents])
    with tempfile.TemporaryDirectory(prefix='clock-provenance-controls-',dir=temporary_root)as folder:
        target=Path(folder)/'forged.json'
        for name,change in plan:
            value=copy.deepcopy(original);change(value)
            target.write_text(json.dumps(value),encoding='utf8')
            try:audit(capture,target,primary,foreign)
            except(AssertionError,KeyError,TypeError):rejected.append(name)
            else:raise AssertionError(('false original-wire declaration escaped',name))
    return dict(scope=__doc__,capture_SHA256=baseline['capture_SHA256'],manifest_SHA256=baseline['manifest_SHA256'],auditor_SHA256=sha(Path(__file__).read_bytes()),negative_controls=len(rejected),rejected_controls=rejected,semantic_metadata_corruptions_only=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','manifest','primary','foreign','temporary-root','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=controls(a.capture,a.manifest,a.primary,a.foreign,a.temporary_root)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(negative_controls=proof['negative_controls'],all_rejected=True)),flush=True)
