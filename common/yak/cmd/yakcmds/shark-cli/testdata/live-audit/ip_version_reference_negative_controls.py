"""Require source-bound version-field checks to reject forged raw descriptors.

This is a focused negative check of the independent reference-field boundary,
not a substitute for the complete original-capture SDK/CLI source audit.
"""
import argparse,copy,json
from pathlib import Path
from ip_version_full_capture_audit import header,reference_versions,objects,sha


def run(oracle,out):
    records=json.loads(oracle.read_text('utf8'));base=None
    for record in records:
        layers=record['_source']['layers'];raw=bytes.fromhex(layers['frame_raw'][0])
        try:headers=header(raw)[3]
        except AssertionError:continue
        if len(headers)==1 and headers[0][1]not in[0,4]:base=layers;break
    assert base is not None
    raw=bytes.fromhex(base['frame_raw'][0]);headers=header(raw)[3];reference_versions(base,raw,headers)
    def field(layers):
        return next(n for n in objects(layers)if'ip.version_raw'in n and'ip.version_tree'in n)
    def remove_owner(layers):field(layers).pop('ip.version_tree')
    def misplaced_expert(layers):
        node=field(layers);expert=node.pop('ip.version_tree');layers['unrelated']={'ip.version':node['ip.version'],'ip.version_raw':[node['ip.version_raw'][0],15,1,0,4,0],'ip.version_tree':expert}
    cases=[
        ('changed original frame',lambda x:x['frame_raw'].__setitem__(0,'00'+x['frame_raw'][0][2:])),
        ('foreign data source',lambda x:field(x)['ip.version_raw'].__setitem__(5,1)),
        ('wrong byte offset',lambda x:field(x)['ip.version_raw'].__setitem__(1,15)),
        ('wrong field width',lambda x:field(x)['ip.version_raw'].__setitem__(2,2)),
        ('wrong bit mask',lambda x:field(x)['ip.version_raw'].__setitem__(3,1)),
        ('wrong raw byte',lambda x:field(x)['ip.version_raw'].__setitem__(0,'00')),
        ('wrong field value',lambda x:field(x).__setitem__('ip.version','4')),
        ('boolean version',lambda x:field(x).__setitem__('ip.version',True)),
        ('boolean mask',lambda x:field(x)['ip.version_raw'].__setitem__(3,False)),
        ('boolean width',lambda x:field(x)['ip.version_raw'].__setitem__(2,True)),
        ('boolean data source',lambda x:field(x)['ip.version_raw'].__setitem__(5,False)),
        ('boolean frame source',lambda x:x['frame_raw'].__setitem__(5,False)),
        ('missing owning expert',remove_owner),
        ('expert moved to unrelated byte',misplaced_expert),
    ];rejected=[]
    for name,mutate in cases:
        forged=copy.deepcopy(base);mutate(forged)
        try:reference_versions(forged,raw,headers)
        except (AssertionError,KeyError,ValueError,TypeError):rejected.append(name)
        else:raise AssertionError('forged reference accepted: '+name)
    proof=dict(scope=__doc__,reference_SHA256=sha(oracle),auditor_SHA256=sha(Path(__file__)),rejected=rejected,all_rejected=len(rejected)==len(cases),original_reference_unchanged=True)
    with out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(rejected=len(rejected),all_rejected=True)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--oracle',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);args=ap.parse_args()
    for path in[args.oracle,args.out]:assert not any((v/'.git').exists()for v in[path.resolve(),*path.resolve().parents])
    run(args.oracle,args.out)
