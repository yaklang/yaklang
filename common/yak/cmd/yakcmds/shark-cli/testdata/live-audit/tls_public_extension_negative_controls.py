"""Exercise semantic preservation failures without changing producer hashes."""
import argparse
import copy
import json
from pathlib import Path

from tls_public_extension_preservation_audit import inspect, rows, sha


def run(before_events, after_events, before_cli, after_cli, out):
    out.mkdir(exist_ok=False)
    original = [list(rows(p)) for p in [before_events, after_events, before_cli, after_cli]]
    paths = [out/(name+'.jsonl') for name in ['before-events','after-events','before-cli','after-cli']]
    def execute(values):
        for path, entries in zip(paths, values):
            with path.open('w',encoding='utf8') as f:
                for entry in entries:
                    f.write(json.dumps(entry,separators=(',',':'))+'\n')
        return inspect(*paths)
    baseline = execute(original)
    target = next(i for i,e in enumerate(original[1]) if e['protocol']=='tls' and
                  e.get('session',{}).get('Session Establishment') and e.get('fields') is not None)
    event_id = original[1][target]['id']
    row_index = next(i for i,r in enumerate(original[3]) if any(p['id']==event_id for p in r.get('pdus')or[]))
    pdu_index = next(i for i,p in enumerate(original[3][row_index]['pdus']) if p['id']==event_id)
    def event(v): return v[1][target]
    def row(v): return v[3][row_index]
    def pdu(v): return row(v)['pdus'][pdu_index]
    def source_refs(v):
        for source in [event(v)['source_bytes'],event(v)['field_values']['source']]:
            source['PacketRefs'][0]['Number'] += 1
    def source_domain(v):
        event(v)['domain']['Interface'] += 1
        for source in [event(v)['source_bytes'],event(v)['field_values']['source']]:
            for ref in source['PacketRefs']or[]:
                ref['Domain']['Interface'] += 1
    def old_alias(v):
        value=next(x for x in event(v)['field_values']['values'] if x['name']=='tls.record.length')
        value['value']+=1
    def wrong_origin(v):
        next(x for x in event(v)['field_values']['values'] if x['name'].startswith('tls.session.'))['origin']='pdu'
    def fake_range(v):
        next(x for x in event(v)['field_values']['values'] if x['name'].startswith('tls.session.'))['pdu_range']={'offset':0,'length':1}
    def changed_id(v):
        event(v)['id']+=100000
        event(v)['field_values']['pdu']=event(v)['id']
        next(x for x in event(v)['field_values']['values'] if x['name']=='pdu.id')['value']=event(v)['id']
    def reordered(v):
        values=event(v)['field_values']['values']
        values[0],values[1]=values[1],values[0]
    def cli_source(v):
        for source in [pdu(v)['byte_source'],pdu(v)['field_values']['source']]:
            source['PacketRefs'][0]['Number']+=1
    controls = [
        ('old-flow-id',lambda v:event(v).__setitem__('flow',event(v)['flow']+1)),
        ('old-id-and-snapshot-synchronized',changed_id),
        ('literal-source-ref-and-snapshot-synchronized',source_refs),
        ('literal-domain-and-source-synchronized',source_domain),
        ('old-record-length-alias',old_alias),
        ('old-alias-removed',lambda v:event(v)['field_values']['values'].pop(0)),
        ('old-alias-reordered',reordered),
        ('old-alias-duplicated',lambda v:event(v)['field_values']['values'].append(copy.deepcopy(event(v)['field_values']['values'][0]))),
        ('old-status',lambda v:event(v).__setitem__('status','deferred')),
        ('new-map-key-outside-contract',lambda v:event(v)['fields'].__setitem__('Invented Protocol',True)),
        ('new-alias-outside-contract',lambda v:event(v)['field_values']['values'].append({'name':'tls.resumed','value':True,'origin':'context'})),
        ('context-alias-wrong-origin',wrong_origin),
        ('context-alias-fabricated-ciphertext-range',fake_range),
        ('new-GetFields-only-metadata',lambda v:event(v)['fields']['Session Establishment'].__setitem__('Certificate Trust','verified')),
        ('CLI-capture-domain',lambda v:row(v)['capture_domain'].__setitem__('Interface',row(v)['capture_domain']['Interface']+1)),
        ('CLI-old-info',lambda v:row(v).__setitem__('info','invented')),
        ('CLI-pdu-id',lambda v:pdu(v).__setitem__('id',100000000)),
        ('CLI-alias-map-only',lambda v:pdu(v)['fields'].__setitem__('tls.session.id_echo',[False])),
        ('CLI-source-and-snapshot-synchronized',cli_source),
        ('SDK-event-duplicated',lambda v:v[1].append(copy.deepcopy(event(v)))),
        ('CLI-row-duplicated',lambda v:v[3].append(copy.deepcopy(row(v)))),
        ('old-int-bool-type-confusion',lambda v:event(v).__setitem__('direction',bool(event(v)['direction']))),
    ]
    results=[]
    for name,mutate in controls:
        values=copy.deepcopy(original)
        mutate(values)
        try:
            execute(values)
        except AssertionError as error:
            results.append({'control':name,'rejected':True,'reason':str(error)[:700]})
        else:
            raise AssertionError(('semantic negative accepted',name))
    execute(original)
    proof={'scope':__doc__,'auditor_SHA256':sha(Path(__file__).with_name('tls_public_extension_preservation_audit.py')),
           'negative_driver_SHA256':sha(Path(__file__)),'baseline_passed':True,'negative_count':len(results),
           'all_negatives_rejected':True,'results':results,'baseline':baseline,
           'inputs':{str(p):sha(p)for p in [before_events,after_events,before_cli,after_cli]}}
    with (out/'proof.json').open('x',encoding='utf8')as f:
        json.dump(proof,f,indent=2);f.write('\n')
    return proof


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for key in ['before-events','after-events','before-cli','after-cli','out']:
        ap.add_argument('--'+key,type=Path,required=True)
    args=ap.parse_args()
    proof=run(args.before_events,args.after_events,args.before_cli,args.after_cli,args.out)
    print(json.dumps({k:v for k,v in proof.items()if k not in ['baseline','inputs','results']}),flush=True)
