"""Reject missing/forged late publications using a complete owned export.

Require the baseline to contain every decoded nonempty SDK message, then change
one full export per control. Byte ownership, physical rows, aliases and numeric
identities remain independent of the row that published the result.
"""
import argparse
import copy
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical,rows,sha
from protocol_publication_audit import audit

ap=argparse.ArgumentParser(description=__doc__)
for name in['before','after','sdk','out-dir']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((q/'.git').exists()for q in[path.resolve(),*path.resolve().parents])
baseline=audit(a.before,a.after,a.sdk);assert baseline['all_decoded_nonempty_SDK_messages_present']is True
data=list(rows(a.after));index=next(i for i,row in enumerate(data)if row.get('published_pdus'))
frame=data[index]['number'];other=next(p['id']for row in data for p in row.get('pdus')or[]if p['id']>0)
a.out_dir.mkdir(exist_ok=False)
def published(row):return row['published_pdus'][0]
controls=[
    ('physical-byte-count',lambda r:r.__setitem__('captured_length',r['captured_length']+1)),
    ('physical-protocol-label',lambda r:r.__setitem__('protocol','forged publication protocol')),
    ('publication-promoted-to-ordinary-contributor',lambda r:r.setdefault('pdus',[]).append(published(r))),
    ('original-byte-owner-replaced-by-publication-frame',lambda r:published(r)['byte_source']['PacketRefs'][0].__setitem__('Number',frame)),
    ('source-SHA',lambda r:published(r)['byte_source'].__setitem__('SHA256','0'*64)),
    ('typed-owner-ID',lambda r:published(r)['field_values'].__setitem__('pdu',other)),
    ('numeric-request-owner',lambda r:published(r).__setitem__('response_to',other)),
    ('missing-complete-late-message',lambda r:r['published_pdus'].pop(0)),
    ('ordinary-PDU-identity-reused',lambda r:published(r).__setitem__('id',other)),
    ('published-protocol',lambda r:published(r).__setitem__('protocol','tls')),
    ('publication-claimed-as-byte-contribution',lambda r:r.__setitem__('published_pdu_scope','physical byte contributor')),
    ('typed-snapshot-completeness',lambda r:published(r)['field_values'].__setitem__('incomplete',True)),
]
rejected=[]
for name,mutate in controls:
    changed=copy.deepcopy(data);mutate(changed[index]);path=a.out_dir/(name+'.jsonl')
    path.write_text(''.join(canonical(row)+'\n'for row in changed),encoding='utf8')
    try:
        proof=audit(a.before,path,a.sdk)
        assert proof['all_decoded_nonempty_SDK_messages_present']is True,'removing a complete publication cannot pass decoded-message coverage'
    except(AssertionError,KeyError,TypeError,ValueError,IndexError)as exc:rejected.append(dict(name=name,frame=frame,rejection=str(exc),mutated_export_sha256=sha(path)))
    else:raise AssertionError('publication negative control accepted: '+name)
proof=dict(scope=__doc__,before_sha256=sha(a.before),after_sha256=sha(a.after),SDK_sha256=sha(a.sdk),baseline_publications=baseline['positive_publications_checked'],negative_controls_rejected=len(rejected),rejected=rejected)
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(baseline_publications=proof['baseline_publications'],negative_controls_rejected=len(rejected))))
