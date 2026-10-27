"""Reject declared SDK forgeries against independent PCAP/endpoint/reference evidence."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

from imap_rtsp_wire_audit import audit

ap=argparse.ArgumentParser(description=__doc__)
for name in ['capture','manifest','receipts','events','oracle','follow-dir','out-dir']:
    ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out_dir.mkdir(exist_ok=False)
rows=[json.loads(line)for line in a.events.open(encoding='utf-8-sig')]
baseline=audit(a.capture,a.manifest,a.receipts,a.events,a.oracle,a.follow_dir)
def index(predicate):return next(i for i,e in enumerate(rows)if predicate(e))
def stage(value):return index(lambda e:(e.get('session')or{}).get('SASL Stage')==value)
response,challenge,result,invalid=map(stage,['response','challenge','result','invalid-response'])
namespace=index(lambda e:(e.get('session')or{}).get('Namespace Stage')=='response')
ambiguous=index(lambda e:(e.get('session')or{}).get('Association')=='ambiguous-outstanding-namespace')
done=index(lambda e:(e.get('session')or{}).get('IDLE Stage')=='done')
flush=index(lambda e:(e.get('session')or{}).get('Extension Method')=='FLUSH')
mutations=[]
def change(name,position,fn):mutations.append((name,position,fn))
def both(e,fn):
    fn(e['session'])
    if isinstance(e.get('fields'),dict):fn(e['fields'])
change('decoded-payload',response,lambda e:both(e,lambda s:s['Transport Value'].__setitem__('Decoded Payload','Zm9yZ2Vk')))
change('wrong-challenge-association',response,lambda e:e.__setitem__('response_to',rows[result]['response_to']))
change('cross-exchange-tag',challenge,lambda e:both(e,lambda s:s.__setitem__('Tag','forged-tag')))
change('cross-exchange-mechanism',challenge,lambda e:both(e,lambda s:s.__setitem__('Mechanism','GSSAPI')))
change('contributor-loss',response,lambda e:e['source_bytes']['PacketRefs'].pop())
change('capture-domain',response,lambda e:e['domain'].__setitem__('Interface',999))
change('raw-sha256',response,lambda e:e['source_bytes'].__setitem__('SHA256','0'*64))
change('raw-body',response,lambda e:e.__setitem__('raw','Zm9yZ2VkDQo='))
change('invalid-value-accepted',invalid,lambda e:e.__setitem__('status','decoded'))
change('authentication-claim',result,lambda e:both(e,lambda s:s.__setitem__('Authentication Verified',True)))
change('security-layer-claim',result,lambda e:both(e,lambda s:s.__setitem__('Security Layer Verified',True)))
change('encoded-span',response,lambda e:both(e,lambda s:s['Transport Value']['Wire Field Ranges'].__setitem__('Encoded Value',[1,5])))
change('decoded-wire-span-claim',response,lambda e:both(e,lambda s:s['Transport Value'].__setitem__('Decoded Values Are Wire Spans',True)))
def swap_groups(s):
    m=s['IMAP Fields'];m['Personal Namespaces'],m['Shared Namespaces']=m['Shared Namespaces'],m['Personal Namespaces']
change('namespace-group-swap',namespace,lambda e:both(e,swap_groups))
change('namespace-extension-value-loss',namespace,lambda e:both(e,lambda s:s['IMAP Fields']['Shared Namespaces']['Entries'][0]['Extensions'][0]['Values'].pop()))
change('empty-prefix-erased',namespace,lambda e:both(e,lambda s:s['IMAP Fields']['Personal Namespaces']['Entries'][0].__setitem__('Prefix',None)))
change('ambiguous-untagged-associated',ambiguous,lambda e:e.__setitem__('response_to',rows[result]['response_to']))
change('idle-borrows-sasl-challenge',done,lambda e:e.__setitem__('response_to',rows[challenge]['id']))
change('flush-timestamp',flush,lambda e:both(e,lambda s:s['RTP Info'][0].__setitem__('rtptime',0)))
change('flush-media-reset-claim',flush,lambda e:both(e,lambda s:s.__setitem__('Media Effect Verified',True)))
change('selected-protocol-erased',flush,lambda e:e.__setitem__('protocol','tcp'))
rejected=[]
for name,position,mutate in mutations:
    changed=copy.deepcopy(rows)
    mutate(changed[position])
    path=a.out_dir/(name+'.jsonl')
    path.write_text(''.join(json.dumps(e,separators=(',',':'))+'\n'for e in changed),encoding='utf8')
    try:audit(a.capture,a.manifest,a.receipts,path,a.oracle,a.follow_dir)
    except (AssertionError,KeyError,ValueError,IndexError,TypeError)as exc:
        rejected.append(dict(name=name,PDU=rows[position]['id'],rejection=str(exc),mutated_export_sha256=hashlib.sha256(path.read_bytes()).hexdigest()))
    else:raise AssertionError('negative control accepted: '+name)
proof=dict(scope=__doc__,events_sha256=baseline['events_sha256'],capture_sha256=baseline['capture_sha256'],rejected=rejected,negative_controls_rejected=len(rejected),baseline_checks=baseline['checks'])
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(negative_controls_rejected=len(rejected),baseline_checks=baseline['checks'])))
