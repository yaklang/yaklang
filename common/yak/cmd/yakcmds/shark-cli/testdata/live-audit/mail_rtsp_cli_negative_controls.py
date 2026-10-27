"""Reject mutations of actual changed CLI rows and their byte-bound SDK evidence.

These controls use two declared original physical rows under the real SDK
messages. They test Info projection and deep binding, not a mutated full input.
"""
import argparse
import copy
import json
from pathlib import Path

from mail_rtsp_cli_invariants import audit
from mail_rtsp_export_invariants import canonical, rows, sha

ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','sdk','certificate','out-dir']:
    ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
proof=json.loads(a.certificate.read_text(encoding='utf8'))
assert proof['before_sha256']==sha(a.before) and proof['after_sha256']==sha(a.after) and proof['SDK_sha256']==sha(a.sdk)
chosen=[next(e for e in proof['changed_rows']if e['protocol']==p)for p in ['RTSP','IMAP']]
numbers={e['frame']for e in chosen}
before=[e for e in rows(a.before)if e['number']in numbers]
after=[e for e in rows(a.after)if e['number']in numbers]
ids={item['SDK_PDU']for e in chosen for item in e['evidence']}
sdk=[e for e in rows(a.sdk)if e['id']in ids]
assert len(before)==len(after)==len(numbers)==2
a.out_dir.mkdir(exist_ok=False)

def write(path,values):path.write_text(''.join(canonical(e)+'\n'for e in values),encoding='utf8')
def execute(label,old,new,native):
    paths=[a.out_dir/(label+'-'+name+'.jsonl')for name in ['before','after','SDK']]
    for p,values in zip(paths,[old,new,native]):write(p,values)
    return audit(*paths)

baseline=execute('baseline',before,after,sdk)
rtsp=next(i for i,e in enumerate(after)if e['protocol']=='RTSP')
imap=next(i for i,e in enumerate(after)if e['protocol']=='IMAP')

def simultaneous(old,new,index,fn):
    fn(old[index]);fn(new[index])

cases=[
    ('wrong-specific-method',lambda old,new,native:new[rtsp].__setitem__('info','RTSP FORGED CSeq=0')),
    ('wrong-CSeq',lambda old,new,native:new[rtsp].__setitem__('info',new[rtsp]['info'].replace('CSeq=0','CSeq=999'))),
    ('generic-IMAP-summary',lambda old,new,native:new[imap].__setitem__('info','IMAP')),
    ('label-change',lambda old,new,native:new[imap].__setitem__('protocol','TCP')),
    ('field-type-change',lambda old,new,native:new[imap].__setitem__('number',str(new[imap]['number']))),
    ('deep-endpoint',lambda old,new,native:simultaneous(old,new,rtsp,lambda e:e.__setitem__('message_source','192.0.2.1:1'))),
    ('deep-domain',lambda old,new,native:simultaneous(old,new,rtsp,lambda e:e['message_domain'].__setitem__('Interface',999))),
    ('deep-contributors',lambda old,new,native:simultaneous(old,new,rtsp,lambda e:e['pdus'][0]['byte_source'].__setitem__('PacketRefs',[]))),
    ('deep-status',lambda old,new,native:simultaneous(old,new,rtsp,lambda e:e['pdus'][0].__setitem__('status','context-required'))),
    ('deep-no-PDU',lambda old,new,native:simultaneous(old,new,rtsp,lambda e:e.__setitem__('pdus',[]))),
    ('duplicate-SDK-evidence',lambda old,new,native:native.append(copy.deepcopy(native[0]))),
    ('SDK-raw-byte',lambda old,new,native:native[0].__setitem__('raw','Zm9yZ2Vk')),
]
rejected=[]
for name,mutate in cases:
    old,new,native=map(copy.deepcopy,[before,after,sdk]);mutate(old,new,native)
    try:execute(name,old,new,native)
    except (AssertionError,KeyError,ValueError,IndexError,TypeError)as exc:rejected.append(dict(name=name,rejection=str(exc)))
    else:raise AssertionError('CLI negative accepted: '+name)
result=dict(scope=__doc__,full_CLI_certificate_sha256=sha(a.certificate),SDK_export_sha256=sha(a.sdk),
            selected_physical_frames=sorted(numbers),selected_rows=baseline['physical_rows_compared'],rejected=rejected,negative_controls_rejected=len(rejected))
(a.out_dir/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(selected_rows=baseline['physical_rows_compared'],negative_controls_rejected=len(rejected))))
