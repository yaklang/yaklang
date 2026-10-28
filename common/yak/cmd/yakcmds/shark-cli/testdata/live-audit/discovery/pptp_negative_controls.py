"""Reject PPTP source/field/flow forgeries only after a real positive audit.

No fabricated corrected export is a positive control. Supply current replay
exports and unchanged source/reference fixtures; a vulnerable old binary must
fail the prerequisite rather than receive a passing negative-control receipt.
"""
import argparse,copy,json,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import pptp_parent_source_audit as a


def audit(manifest,exports,baseline,ack_capture,ack_exports):
    positive=a.audit_controls(manifest,exports,baseline,ack_capture,ack_exports)
    cases={c['name']:c for c in json.loads(manifest.read_text('utf8'))['cases']};bad='GRE-v1-IPv4-no-PPTP';record=a.physical(Path(cases[bad]['capture']))[0];claim=a.profile(record);event=list(a.rows(exports/bad/'events.jsonl'))[0];row=next(a.rows(exports/bad/'yak.jsonl'));checks=[]
    def rejected(name,fn):
        try:fn()
        except AssertionError:checks.append(dict(name=name,rejected=True));return
        raise AssertionError(('forgery accepted',name))
    def event_forgery(name,mutate):
        e=copy.deepcopy(event);mutate(e);rejected(name,lambda:a.refusal(e,1,record,claim))
    def row_forgery(name,mutate):
        r=copy.deepcopy(row);mutate(r);rejected(name,lambda:a.refusal_row(r,event,1,record))
    event_forgery('wrong literal GRE version',lambda e:e['session'].__setitem__('Observed GRE Version',2))
    event_forgery('floating literal GRE flags',lambda e:e['session'].__setitem__('GRE Header Flags',float(claim['flags'])))
    event_forgery('wrong literal GRE protocol type',lambda e:e['session'].__setitem__('GRE Protocol Type',0x880b))
    event_forgery('wrong available parent extent',lambda e:e['session'].__setitem__('Available GRE Bytes',claim['available']-1))
    event_forgery('false source frame number',lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',2))
    event_forgery('boolean source interface',lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',False))
    event_forgery('restore obsolete unadmitted GRE key domain',lambda e:e['domain'].__setitem__('Encapsulation','/gre:198.51.100.1~198.51.100.2:42'))
    event_forgery('claim decoded child DNS fields',lambda e:e.__setitem__('fields',{'DNS':{'ID':4660}}))
    event_forgery('claim child transport flow',lambda e:e.__setitem__('tcp_flow_id',1))
    event_forgery('claim a negotiated PPTP call',lambda e:e['session'].__setitem__('PPTP Negotiated',True))
    event_forgery('replace physical source with reassembly',lambda e:e['source_bytes'].__setitem__('Kind','reassembled'))
    extra=copy.deepcopy(event);extra['id']+=100;extra['protocol']='dns'
    rejected('second child contribution through refused physical source',lambda:a.exclusive([event,extra],{1:event}))
    changed=copy.deepcopy(record);wire=bytearray(changed['wire']);wire[claim['offset']+3]^=6;changed['wire']=bytes(wire);changed_claim=a.profile(changed)
    rejected('independently changed captured GRE protocol bytes',lambda:a.refusal(event,1,changed,changed_claim))
    row_forgery('CLI child DNS label',lambda r:r.__setitem__('protocol','DNS'))
    row_forgery('CLI speculative PPP stack',lambda r:r['protocol_stack'].append('PPP'))
    row_forgery('CLI obsolete source interpretation tag',lambda r:r['message_domain'].__setitem__('Encapsulation','/gre:198.51.100.1~198.51.100.2:42'))
    row_forgery('CLI foreign PDU ownership',lambda r:r['pdus'][0].__setitem__('id',event['id']+1))
    row_forgery('CLI hides precise refusal reason',lambda r:r.__setitem__('info','[incomplete/undecoded]'))
    name='PPTP-v1-TCP-variable-length';records=a.physical(Path(cases[name]['capture']));events=list(a.rows(exports/name/'events.jsonl'));flow=json.loads((exports/name/'flows.json').read_text());cli=list(a.rows(exports/name/'yak.jsonl'));reference=json.loads((baseline/name/'oracle.json').read_text());control=list(a.rows(exports/'GRE-v0-TCP-constant-key/events.jsonl'))[0]
    def TCP_forgery(name,mutate):
        values=copy.deepcopy(dict(records=records,events=events,flow=flow,cli=cli,reference=reference));mutate(values)
        rejected(name,lambda:a.TCP_call(values['records'],values['events'],values['flow'],values['cli'],values['reference'],control))
    TCP_forgery('same-call HTTP source loses a contributing packet',lambda v:v['events'][0]['source_bytes']['PacketRefs'].pop())
    TCP_forgery('same-call HTTP body byte changed',lambda v:v['events'][0].__setitem__('raw','AA=='))
    TCP_forgery('typed HTTP field changes while label remains',lambda v:next(f for f in v['events'][0]['field_values']['values']if f['name']=='http.request.method').__setitem__('value','POST'))
    TCP_forgery('same-call split into another TCP generation',lambda v:v['flow']['flows'].append(copy.deepcopy(v['flow']['flows'][0])))
    TCP_forgery('cross-call packet assigned to same TCP flow',lambda v:v['flow']['packets'][1]['refs'][0]['Domain'].__setitem__('Encapsulation','/pptp:198.51.100.1>198.51.100.2:43'))
    TCP_forgery('reversed outer direction falsely merged',lambda v:v['flow']['flows'][0]['domain'].__setitem__('Encapsulation','/pptp:198.51.100.2>198.51.100.1:42'))
    TCP_forgery('boolean transport flow counter',lambda v:v['events'][0].__setitem__('tcp_flow_id',True))
    TCP_forgery('reference stream splice on one contributor',lambda v:v['reference'][1]['_source']['layers']['tcp'].__setitem__('tcp.stream','99'))
    TCP_forgery('reference HTTP method disagrees with bytes',lambda v:v['reference'][-1]['_source']['layers']['http'].__setitem__('http.request.method','POST'))
    # Each raw descriptor remains coupled to its own scalar, offset and source.
    TCP_forgery('GRE raw field rebound to transformed source',lambda v:v['reference'][1]['_source']['layers']['gre']['gre.key.call_id_raw'].__setitem__(5,1))
    TCP_forgery('GRE raw field shifted into another header byte',lambda v:v['reference'][1]['_source']['layers']['gre']['gre.key.payload_length_raw'].__setitem__(1,40))
    ack_record=a.physical(ack_capture)[0];ack_event=list(a.rows(ack_exports/'events.jsonl'))[0];ack_flow=json.loads((ack_exports/'flows.json').read_text());ack_cli=list(a.rows(ack_exports/'yak.jsonl'))
    def ACK_forgery(name,mutate):
        v=copy.deepcopy(dict(event=ack_event,flow=ack_flow,cli=ack_cli));mutate(v);rejected(name,lambda:a.ACK_header(ack_record,v['event'],v['flow'],v['cli']))
    ACK_forgery('ACK-only incorrectly creates TCP flow',lambda v:v['flow'].__setitem__('flows',[{'flow':1}]))
    ACK_forgery('ACK-only falsely claims full call establishment',lambda v:v['event']['session'].__setitem__('PPTP Negotiated',True))
    ACK_forgery('ACK-only reports unobserved acknowledgment number',lambda v:v['event']['session'].__setitem__('PPTP Acknowledgment Number',8))
    ACK_forgery('ACK-only mislabeled as application TLS',lambda v:v['cli'][0].__setitem__('protocol','TLSv1.3'))
    return dict(scope=__doc__,auditor_SHA256=a.sha(Path(a.__file__)),driver_SHA256=a.sha(Path(__file__)),manifest_SHA256=a.sha(manifest),positive_primary_cases=positive['authored_primary_cases'],positive_primary_frames=positive['authored_primary_frames'],positive_additional_ACK_control=True,positive_control_exports=[{k:v for k,v in c.items()if k in['case','capture_SHA256','SDK_SHA256','CLI_SHA256','flows_SHA256']}for c in positive['controls']],positive_ACK_exports=positive['additional_ACK_only_control'],negative_controls=len(checks),all_rejected=all(c['rejected']for c in checks),checks=checks,corrected_export_fabricated=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['manifest','exports','baseline','ack-capture','ack-exports','out']:ap.add_argument('--'+name,type=Path,required=True)
    args=ap.parse_args()
    for p in vars(args).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(args.manifest,args.exports,args.baseline,args.ack_capture,args.ack_exports)
    with args.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','checks']}),flush=True)
