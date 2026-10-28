"""Reject application/page restoration or unrelated changes in closed-TCP rows."""
import argparse,copy,hashlib,json
from pathlib import Path
from closed_tcp_projection_audit import audit


def run(capture,before,after,observer,out):
    out.mkdir(parents=True,exist_ok=False);positive=audit(capture,before,after,observer);assert positive['source_bound_projection_corrections']>0
    old=[json.loads(l)for l in before.open(encoding='utf-8-sig')];new=[json.loads(l)for l in after.open(encoding='utf-8-sig')];hooks=json.loads(observer.read_text('utf8'))
    header=next(c['frame']for c in positive['corrections']if c['kind']=='zero-payload header');payload=next(c['frame']for c in positive['corrections']if c['kind']=='unadmitted payload')
    def row(v,n=header):return next(r for r in v if r.get('record_type')!='protocol_publication'and r['number']==n)
    cases=[
        ('old-application-restored','after',lambda v:row(v).update(connection_protocol='TLSv1.2')),
        ('old-preview-restored','after',lambda v:row(v).update(stream_id=1)),
        ('payload-old-preview-restored','after',lambda v:row(v,payload).update(stream_id=1)),
        ('boolean-native-flow-id','after',lambda v:row(v).update(tcp_flow_id=True)),
        ('different-native-flow-id','after',lambda v:row(v).update(tcp_flow_id=999999)),
        ('different-native-domain','after',lambda v:row(v)['tcp_flow_domain'].update(Encapsulation='/foreign')),
        ('injected-PDU','after',lambda v:row(v).update(pdus=[dict(id=999,protocol='http',status='decoded')])),
        ('wire-length-forgery','after',lambda v:row(v).update(length=row(v)['length']+1)),
        ('protocol-label-forgery','after',lambda v:row(v).update(protocol='TLSv1.2')),
        ('application-breadcrumb-forgery','after',lambda v:row(v)['protocol_stack'].append('HTTP/1.1')),
        ('source-header-Info-erased','after',lambda v:row(v).update(info='TCP header after observed close; generation unconfirmed')),
        ('false-payload-classification','after',lambda v:row(v).update(visibility='encrypted')),
        ('unrelated-status-forgery','after',lambda v:row(v).update(protocol_status='malformed')),
        ('wrong-physical-frame','after',lambda v:row(v).update(number=payload)),
        ('ineligible-before-state','before',lambda v:row(v).update(tcp_flow_state='closed-direction-observation')),
        ('ineligible-after-state','after',lambda v:row(v).update(tcp_flow_state='active')),
        ('changed-unrelated-row','after',lambda v:row(v,14).update(info='changed unrelated valid HTTP message')),
        ('native-hook-foreign-owner','observer',lambda v:next(p for p in v['packets']if any(r['Number']==header for r in p['refs'])).update(flow=999999)),
        ('native-hook-foreign-capture','observer',lambda v:v.update(capture_SHA256='0'*64)),
    ]
    def restore_old_header(value):
        selected=row(value);selected.clear();selected.update(copy.deepcopy(row(old)))
    cases.append(('entire-old-ACK-row-restored','after',restore_old_header))
    originals=dict(before=old,after=new,observer=hooks);results=[]
    for name,kind,mutate in cases:
        value=copy.deepcopy(originals[kind]);mutate(value);assert json.dumps(value,sort_keys=True)!=json.dumps(originals[kind],sort_keys=True)
        dest=out/(name+('.json'if kind=='observer'else'.jsonl'));dest.write_text(json.dumps(value)if kind=='observer'else''.join(json.dumps(v)+'\n'for v in value),encoding='utf8')
        args=dict(capture=capture,before=before,after=after,observer=observer);args[kind]=dest
        try:audit(**args)
        except(AssertionError,KeyError,ValueError)as exc:results.append(dict(case=name,rejected=True,rejection=type(exc).__name__,forgery_SHA256=hashlib.sha256(dest.read_bytes()).hexdigest()))
        else:raise AssertionError('forgery accepted: '+name)
    proof=dict(scope=__doc__,positive_original_passed=True,auditor_SHA256=positive['auditor_SHA256'],negative_driver_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),positive_capture_SHA256=positive['capture_SHA256'],positive_before_CLI_SHA256=positive['before_CLI_SHA256'],positive_after_CLI_SHA256=positive['after_CLI_SHA256'],negative_controls=len(results),all_forgeries_rejected=True,cases=results)
    (out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(positive_original_passed=True,negative_controls=len(results),all_forgeries_rejected=True)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','before','after','observer','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    run(a.capture,a.before,a.after,a.observer,a.out)
