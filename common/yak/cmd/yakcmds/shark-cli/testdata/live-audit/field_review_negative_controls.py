"""Challenge field-review proofs using declared small subsets of real exports.

The original capture/oracle bytes and case byte hashes stay unchanged. Reduced
case inventories and mutated SDK projections are synthetic audit controls,
never newly observed traffic or replacements for the complete 94/39 review.
"""
import argparse,base64,collections,copy,hashlib,json,subprocess,sys
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
for name in['review','weber-events','sharon-events','weber-capture','sharon-capture','baseline','tshark','weber-tls-oracle','sharon-tls-oracle','out']:
    ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():
    if p==a.review:continue
    assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents]),'raw controls and detailed output remain outside Git'
a.out.mkdir(parents=True,exist_ok=False)
review=json.loads(a.review.read_bytes());reduced={};records={}
sha=lambda b:hashlib.sha256(b).hexdigest()
for name in['weber','sharon']:
    rows=review[name]['rows'];chosen=[next(r for r in rows if r['field']=='tls.record.length'),next(r for r in rows if r['classification']=='readable DNS name display fixed')]
    chosen.extend(r for r in rows if r['classification'].startswith('invalid UTF-8'))
    if name=='weber':chosen.append(next(r for r in rows if any(49573 in v['contributors']and 49574 in v['contributors']for v in r.get('records',[]))))
    reduced[name]={**review[name],'rows':chosen,'original_difference_count':len(chosen)}
    hashes={r['sha256']for c in chosen for r in c.get('records',[])};frames={r['frame']for r in chosen if r['field']=='dns.qry.name'};records[name]=[]
    for line in getattr(a,name+'_events').open():
        e=json.loads(line)
        if e['protocol']=='tls'and sha(base64.b64decode(e['raw']or''))in hashes or e['protocol']in['dns','mdns','llmnr']and any(r['Number']in frames for r in e['source_bytes']['PacketRefs']or[]):records[name].append(e)
    assert records[name]
review_path=a.out/'derived-case-inventory.json';review_path.write_text(json.dumps(reduced)+'\n')

def tls(es):return next(e for e in es['weber']if e['protocol']=='tls')
def dns(es):return next(e for e in es['weber']if e['protocol']=='dns'and e['response_to']==0)
def response(es):return next(e for e in es['weber']if e['protocol']=='dns'and e['response_to'])
def mdns(es):return next(e for e in es['weber']if e['protocol']=='mdns')
def fragmented_tunnel(es):return next(e for e in es['weber']if e['protocol']=='tls'and 49573 in [r['Number']for r in e['source_bytes']['PacketRefs']or[]]and 49574 in [r['Number']for r in e['source_bytes']['PacketRefs']or[]])
def typed(e,name):return next(v for v in e['field_values']['values']if v['name']==name)
def domain(es):
    e=tls(es);e['domain']['Interface']+=1
    for r in e['source_bytes']['PacketRefs']:r['Domain']=copy.deepcopy(e['domain'])
def wire(es):
    e=tls(es);w=bytearray(base64.b64decode(e['raw']));w[-1]^=1;e['raw']=base64.b64encode(w).decode()
controls=[
 ('TLS-typed-length',lambda es:typed(tls(es),'tls.record.length').__setitem__('value',1)),
 ('TLS-session-length',lambda es:tls(es)['session'].__setitem__('Record Length',1)),
 ('TLS-literal-wire',wire),
 ('TLS-reference-source-endpoint',lambda es:tls(es).__setitem__('source','192.0.2.1:443')),
 ('TLS-reference-destination-endpoint',lambda es:tls(es).__setitem__('destination','192.0.2.2:1234')),
 ('TLS-reference-section-interface',domain),
 ('TLS-byte-completion-frame',lambda es:tls(es)['source_bytes']['CompletedIn'].__setitem__('Number',1)),
 ('TLS-original-record-missing',lambda es:es['weber'].remove(tls(es))),
 ('fragmented-tunnel-outer-IP-as-inner-endpoint',lambda es:fragmented_tunnel(es).__setitem__('source','[2a00:6020:ad0b:8300::d061:443]:443')),
 ('DNS-readable-display',lambda es:typed(mdns(es),'dns.qry.name').__setitem__('value','invented.example')),
 ('DNS-invalid-octets-replaced',lambda es:typed(dns(es),'dns.qry.name').__setitem__('value','replacement-\ufffd.example')),
 ('DNS-escaped-name',lambda es:typed(dns(es),'dns.qry.name.escaped').__setitem__('value','invented.example')),
 ('DNS-expanded-wire-name',lambda es:dns(es)['session']['DNS']['Questions'][0].__setitem__('Name Wire',base64.b64encode(b'\x01x\0').decode())),
 ('DNS-wire-transaction-ID',lambda es:dns(es)['session']['DNS'].__setitem__('ID',1)),
 ('DNS-wire-question-type',lambda es:dns(es)['session']['DNS']['Questions'][0].__setitem__('Type',65535)),
 ('DNS-reference-endpoint',lambda es:dns(es).__setitem__('source','192.0.2.9:50000')),
 ('DNS-response-transaction',lambda es:response(es).__setitem__('transaction',response(es)['id'])),
 ('DNS-response-domain',lambda es:response(es)['domain'].__setitem__('Interface',9999)),
 ('mDNS-invented-request-association',lambda es:mdns(es).__setitem__('transaction',mdns(es)['id'])),
]
results=[];audit=Path(__file__).with_name('field_difference_audit.py')
for name,mutate in [('baseline',None),*controls]:
    out=a.out/name;out.mkdir();es=copy.deepcopy(records)
    if mutate:mutate(es)
    inputs={}
    for capture,rows in es.items():
        path=out/(capture+'.jsonl');path.write_text(''.join(json.dumps(e)+'\n'for e in rows));inputs[capture]=path
    args=[sys.executable,'-X','utf8',str(audit),'--baseline',str(a.baseline),'--original-review',str(review_path),'--weber-tls-oracle',str(a.weber_tls_oracle),'--sharon-tls-oracle',str(a.sharon_tls_oracle),'--tshark',str(a.tshark),'--out',str(out/'audit')]
    for capture in['weber','sharon']:args+=['--'+capture+'-events',str(inputs[capture]),'--'+capture+'-capture',str(getattr(a,capture+'_capture'))]
    r=subprocess.run(args,capture_output=True,timeout=300);(out/'audit.log').write_bytes(r.stdout+r.stderr)
    assert(r.returncode==0)==(mutate is None),(name,(r.stdout+r.stderr).decode('utf8','replace')[-900:])
    results.append({'control':name,'accepted':r.returncode==0});print(name,'accepted'if r.returncode==0 else'rejected',flush=True)
summary={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'case_counts':{n:len(v['rows'])for n,v in reduced.items()},'subset_record_counts':{n:len(v)for n,v in records.items()},'controls':results}
(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary),flush=True)
