"""Verify that exact export comparison rejects changed values, types and order.
These are declared small derived SDK controls, not complete-capture evidence.
"""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in['events','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in[a.events,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);records=[]
for line in a.events.open():
    e=json.loads(line);frames=[r['Number']for r in e['source_bytes']['PacketRefs']or[]]
    if e['protocol']=='tls'and 1994 in frames or e['protocol']=='dns'and any(f in[38951,38952]for f in frames):records.append(e)
assert len(records)==4,'two exact TLS records and one observed DNS request/response'
before=a.out/'before.jsonl';before.write_text(''.join(json.dumps(e)+'\n'for e in records))
def tls(es):return es[0]
def response(es):return es[-1]
def source_parent(es):es[0]['source_bytes']['ParentPDU']+=1
def raw(es):
    b=bytearray(base64.b64decode(es[0]['raw']));b[-1]^=1;es[0]['raw']=base64.b64encode(b).decode()
controls=[('generated-PDU-ID',lambda es:tls(es).__setitem__('id',tls(es)['id']+1)),('numeric-type',lambda es:tls(es).__setitem__('id',float(tls(es)['id']))),('boolean-numeric-type',lambda es:response(es)['session']['DNS'].__setitem__('Response',1)),('flow-ID',lambda es:tls(es).__setitem__('flow',tls(es)['flow']+1)),('endpoint-direction',lambda es:tls(es).__setitem__('direction',tls(es)['direction']^1)),('capture-domain',lambda es:tls(es)['domain'].__setitem__('Interface',9999)),('response-association',lambda es:response(es).__setitem__('response_to',response(es)['id'])),('source-parent',source_parent),('literal-message-bytes',raw),('typed-field',lambda es:next(v for v in tls(es)['field_values']['values']if v['name']=='tls.record.length').__setitem__('value','66')),('record-missing',lambda es:es.pop()),('record-duplicated',lambda es:es.append(copy.deepcopy(es[-1]))),('record-order',lambda es:es.reverse())]
audit=Path(__file__).with_name('exact_export_invariants.py');results=[]
for name,mutate in [('formatting-and-key-order-only',None),*controls]:
    out=a.out/name;out.mkdir();es=copy.deepcopy(records)
    if mutate:mutate(es)
    after=out/'after.jsonl';after.write_text(''.join(json.dumps({k:e[k]for k in reversed(list(e))},separators=(', ', ': '))+'\n'for e in es))
    r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--kind','SDK','--before',str(before),'--after',str(after),'--out',str(out/'audit.json')],capture_output=True,timeout=30);(out/'audit.log').write_bytes(r.stdout+r.stderr)
    assert(r.returncode==0)==(mutate is None),(name,(r.stdout+r.stderr).decode('utf8','replace')[-800:]);results.append({'control':name,'accepted':r.returncode==0})
summary={'scope':__doc__,'baseline_passed':True,'subset_records':len(records),'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary))
