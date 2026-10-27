"""Challenge public-corpus wire checks and complete-record preservation separately.

Wire negatives use the full selected IMAP/RTSP record sequence and the original
capture/reference files. Preservation negatives use explicitly selected original
nonselected records under the full capture's validated PDU-reference bijection;
they are record-projection negatives, not mutated whole-capture comparisons.
Raw exports and detailed controls must remain outside Git.
"""
import argparse
import copy
import json
from pathlib import Path

from mail_rtsp_export_invariants import References, SELECTED, canonical, digest, rows, sha
from mail_rtsp_public_wire_audit import audit


def both(e, fn):
    fn(e['session'])
    if isinstance(e.get('fields'), dict):
        fn(e['fields'])


def main(a):
    a.out_dir.mkdir(exist_ok=False)
    selected = [e for e in rows(a.after) if e['protocol'] in SELECTED]
    selected_path = a.out_dir / 'selected-baseline.jsonl'

    def write(path, events):
        path.write_text(''.join(canonical(e)+'\n' for e in events), encoding='utf8')

    write(selected_path, selected)
    baseline = audit(a.capture, selected_path, a.oracle)
    def find(fn): return next(i for i,e in enumerate(selected) if fn(e))
    response = find(lambda e:(e.get('session') or {}).get('SASL Stage') == 'response')
    challenge = find(lambda e:(e.get('session') or {}).get('SASL Stage') == 'challenge')
    namespace = find(lambda e:(e.get('session') or {}).get('Namespace Stage') == 'response')
    gap = find(lambda e:e['status'] == 'context-required')
    flush = find(lambda e:(e.get('session') or {}).get('Extension Method') == 'FLUSH')
    reply = find(lambda e:e['protocol'] == 'rtsp' and (e.get('session') or {}).get('Status') == 200)
    cases = [
        ('SASL-value', response, lambda e:both(e, lambda s:s['Transport Value'].__setitem__('Decoded Payload','Zm9yZ2Vk'))),
        ('SASL-encoded-value', response, lambda e:both(e, lambda s:s['Transport Value'].__setitem__('Encoded Value','Zm9yZ2Vk'))),
        ('SASL-encoded-range', response, lambda e:both(e, lambda s:s['Transport Value']['Wire Field Ranges'].__setitem__('Encoded Value',[1,2]))),
        ('SASL-decoded-wire-span', response, lambda e:both(e, lambda s:s['Transport Value'].__setitem__('Decoded Values Are Wire Spans',True))),
        ('SASL-tag', challenge, lambda e:both(e, lambda s:s.__setitem__('Tag','wrong-tag'))),
        ('SASL-mechanism', challenge, lambda e:both(e, lambda s:s.__setitem__('Mechanism','GSSAPI'))),
        ('SASL-association', response, lambda e:e.__setitem__('response_to',0)),
        ('SASL-authentication-claim', response, lambda e:both(e, lambda s:s.__setitem__('Authentication Verified',True))),
        ('namespace-empty-prefix', namespace, lambda e:both(e, lambda s:s['IMAP Fields']['Personal Namespaces']['Entries'][0].__setitem__('Prefix',None))),
        ('namespace-delimiter', namespace, lambda e:both(e, lambda s:s['IMAP Fields']['Personal Namespaces']['Entries'][0].__setitem__('Hierarchy Delimiter','Lw=='))),
        ('namespace-range', namespace, lambda e:both(e, lambda s:s['IMAP Fields']['Personal Namespaces']['Entries'][0].__setitem__('Relative Byte Range',[0,1]))),
        ('namespace-association', namespace, lambda e:e.__setitem__('response_to',0)),
        ('peer-gap-marked-decoded', gap, lambda e:e.__setitem__('status','decoded')),
        ('peer-gap-marked-malformed', gap, lambda e:e.__setitem__('status','malformed')),
        ('peer-gap-accepted-fields', gap, lambda e:e.__setitem__('fields',{'Accepted':True})),
        ('FLUSH-timestamp', flush, lambda e:both(e, lambda s:s['RTP Info'][0].__setitem__('rtptime',0))),
        ('FLUSH-media-effect', flush, lambda e:both(e, lambda s:s.__setitem__('Media Effect Verified',True))),
        ('RTSP-CSeq', flush, lambda e:both(e, lambda s:s.__setitem__('CSeq',999))),
        ('RTSP-method', flush, lambda e:both(e, lambda s:s.__setitem__('Packet Name','OPTIONS'))),
        ('RTSP-body-length', flush, lambda e:both(e, lambda s:s.__setitem__('Content Length',1))),
        ('RTSP-reply-status', reply, lambda e:both(e, lambda s:s.__setitem__('Status',404))),
        ('contributor-erased', response, lambda e:e['source_bytes'].__setitem__('PacketRefs',[])),
        ('completion-frame', response, lambda e:e['source_bytes']['CompletedIn'].__setitem__('Number',1)),
        ('capture-domain', response, lambda e:e['domain'].__setitem__('Interface',999)),
        ('raw-source-SHA', response, lambda e:e['source_bytes'].__setitem__('SHA256','0'*64)),
        ('raw-PDU', response, lambda e:e.__setitem__('raw','Zm9yZ2VkDQo=')),
    ]
    rejected=[]
    for name,index,mutate in cases:
        changed=copy.deepcopy(selected);mutate(changed[index])
        path=a.out_dir/(name+'.jsonl');write(path,changed)
        try: audit(a.capture,path,a.oracle)
        except (AssertionError,KeyError,ValueError,IndexError,TypeError) as exc:
            rejected.append(dict(name=name,PDU=selected[index]['id'],rejection=str(exc),mutated_export_sha256=sha(path)))
        else: raise AssertionError('wire negative accepted: '+name)

    references=References(a.before,a.after)
    preserved=json.loads(a.preservation_certificate.read_text(encoding='utf8'))
    assert preserved['before_sha256']==sha(a.before) and preserved['after_sha256']==sha(a.after)
    assert preserved['nonselected_records_compared']==references.nonselected
    assert preserved['PDU_reference_bijection_sha256']==digest(sorted(references.ids.items()))
    predicates={
        'renumbered-id':lambda old,new:old['id']!=new['id'],
        'response-reference':lambda old,new:old['response_to']!=new['response_to'],
        'request-transaction':lambda old,new:old['transaction']!=new['transaction'],
        'literal-transaction':lambda old,new:old['protocol']=='dhcp' and 'Xid'in(old.get('session')or{}),
        'FTP-control':lambda old,new:old['protocol']=='ftp-data' and old['transaction'],
        'typed-alias':lambda old,new:old['id']!=new['id'] and isinstance(new.get('field_values'),dict),
        'source-parent':lambda old,new:bool((new.get('source_bytes')or{}).get('ParentPDU')),
    }
    examples={}
    old_rows=(e for e in rows(a.before) if e['protocol']not in SELECTED)
    new_rows=(e for e in rows(a.after) if e['protocol']not in SELECTED)
    for old,new in zip(old_rows,new_rows):
        for name,predicate in predicates.items():
            if name not in examples and predicate(old,new): examples[name]=(old,new)
    assert len(examples)==len(predicates),('actual preservation examples missing',predicates.keys()-examples.keys())
    old,new=examples['renumbered-id']
    projection_cases=[
        ('stale-PDU-id','renumbered-id',lambda x,o:x.__setitem__('id',o['id'])),
        ('stale-response-id','response-reference',lambda x,o:x.__setitem__('response_to',o['response_to'])),
        ('stale-request-transaction','request-transaction',lambda x,o:x.__setitem__('transaction',o['transaction'])),
        ('literal-wire-transaction','literal-transaction',lambda x,o:both(x,lambda s:s.__setitem__('Xid',s['Xid']+1))),
        ('FTP-wrong-control-flow','FTP-control',lambda x,o:x['session'].__setitem__('Control Flow ID',x['flow'])),
        ('typed-PDU-id-type','typed-alias',lambda x,o:next(f for f in x['field_values']['values']if f['name']=='pdu.id').__setitem__('value',str(x['id']))),
        ('typed-PDU-id-exact-value','typed-alias',lambda x,o:next(f for f in x['field_values']['values']if f['name']=='pdu.id').__setitem__('exact_value',str(o['id']))),
        ('wrong-parent-PDU','source-parent',lambda x,o:x['source_bytes'].__setitem__('ParentPDU',0)),
        ('literal-flow-id','renumbered-id',lambda x,o:x.__setitem__('flow',x['flow']+1)),
        ('literal-domain','renumbered-id',lambda x,o:x['domain'].__setitem__('Interface',999)),
        ('literal-protocol','renumbered-id',lambda x,o:x.__setitem__('protocol','tcp')),
        ('literal-wire-byte','renumbered-id',lambda x,o:x.__setitem__('raw','Zm9yZ2Vk')),
    ]
    record_rejected=[]
    for name,example,mutate in projection_cases:
        old,new=examples[example];expected=references.expected(old,new)
        assert canonical(expected)==canonical(new),'unaltered selected record satisfies complete projection'
        changed=copy.deepcopy(new);mutate(changed,old)
        path=a.out_dir/(name+'-record.jsonl');write(path,[changed])
        assert canonical(expected)!=canonical(changed),'record projection negative accepted: '+name
        record_rejected.append(dict(name=name,before_PDU=old['id'],after_PDU=new['id'],protocol=old['protocol'],mutated_record_sha256=sha(path)))
    result=dict(scope=__doc__,capture_sha256=baseline['capture_sha256'],events_sha256=sha(a.after),reference_sha256=sha(a.oracle),
                full_reference_map_sha256=preserved['PDU_reference_bijection_sha256'],full_preserved_record_count=references.nonselected,
                selected_wire_baseline_checks=baseline['checks'],wire_controls_rejected=rejected,wire_controls_count=len(rejected),
                full_map_record_projection_controls_rejected=record_rejected,record_projection_controls_count=len(record_rejected))
    (a.out_dir/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:result[k]for k in ['full_preserved_record_count','selected_wire_baseline_checks','wire_controls_count','record_projection_controls_count']}))


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['capture','before','after','oracle','preservation-certificate','out-dir']:
        ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    main(a)
