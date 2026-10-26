"""Require QUIC header/epoch audit to reject deliberately corrupted exports.
Uses the original real capture and reference, then mutates one evidence item.
Full captures, manifests, oracle output and mutated JSONL stay outside Git.
"""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','oracle','events','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in [a.capture,a.oracle,a.events,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False)
original=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')]
reference=json.loads(a.oracle.read_text(encoding='utf-8-sig'))
audit=Path(__file__).with_name('quic_header_wire_audit.py')

def session(e):return e.get('session')or{}
def pick(es,predicate):return next(e for e in es if e['protocol']in['quic','http3','doq']and predicate(e))
def long(es):return pick(es,lambda e:session(e).get('Header Form')=='long'and bool(session(e).get('DCID'))and bool(session(e).get('SCID'))and bool(e.get('fields')))
def short(es):return pick(es,lambda e:session(e).get('Header Form')=='short'and bool(session(e).get('DCID')))
def empty(es):return pick(es,lambda e:session(e).get('Header Form')=='long'and not session(e).get('DCID')and not e.get('error'))
def verified(es):return pick(es,lambda e:session(e).get('Authentication Verified')is True)
def alias(e,name):return next(v for v in e['field_values']['values']if v['name']==name)
def flip_b64(value):
 w=bytearray(base64.b64decode(value));w[0]^=1;return base64.b64encode(w).decode()
def set_alias(es,name,key,value):alias(long(es),name)[key]=value
def set_session_cid(es,name):long(es)['session'][name]=flip_b64(long(es)['session'][name])
def raw_corruption(es):
 e=verified(es);e['raw']=flip_b64(e['raw'])
def duplicate(es):
 e=copy.deepcopy(verified(es));e['id']=max(v['id']for v in es)+1;es.append(e)
def duplicate_id(es):es.append(copy.deepcopy(verified(es)))
def missing(es):es.remove(verified(es))
def wrong_witness(es):verified(es)['source_bytes']['PacketRefs'][0]['Number']=1
def wrong_domain(es):verified(es)['source_bytes']['PacketRefs'][0]['Domain']['Interface']=1
def wrong_native_epoch(es):
 fs=sorted({e['flow']for e in es if session(e).get('Authentication Verified')});pick(es,lambda e:e['flow']==fs[-1]and session(e).get('Header Form')=='short')['flow']=fs[0]
def extra_short(es,name):
 e=short(es);v=copy.deepcopy(alias(e,'quic.dcid'));v['name']=name;e['field_values']['values'].append(v)
def extra_range(es):short(es)['session']['Wire Field Ranges']['Version']=[1,5]
def shifted_range(es):long(es)['session']['Wire Field Ranges']['DCID'][0]+=1
def missing_alias(es):
 e=long(es);e['field_values']['values'].remove(alias(e,'quic.dcid'))
def duplicate_alias(es):
 e=long(es);e['field_values']['values'].append(copy.deepcopy(alias(e,'quic.dcid')))
def wrong_token(es):
 e=empty(es);e['fields']['QUICToken']['Token']=base64.b64encode(b'not-a-token').decode()
def wrong_reference(oracle,key,range_offset=False):
 for r in oracle:
  vs=r['_source']['layers'].get('quic',[]);vs=vs if isinstance(vs,list)else[vs]
  for v in vs:
   if key in v:
    if range_offset:v[key][1]+=1
    elif key=='quic.dcid':v[key]='00'*len(bytes.fromhex(v[key].replace(':','')))
    else:v[key]=str(int(v[key])+1)
    return
 raise AssertionError('missing selected reference control field')

controls=[
 ('raw-octet',raw_corruption),('raw-digest',lambda es:verified(es)['source_bytes'].__setitem__('SHA256','00'*32)),
 ('physical-domain',wrong_domain),('physical-witness',wrong_witness),('missing-actual-pdu',missing),('duplicate-event-id',duplicate_id),('duplicate-native-occurrence',duplicate),
 ('wrong-generation',wrong_native_epoch),('DCID-binary',lambda es:set_session_cid(es,'DCID')),('SCID-binary',lambda es:set_session_cid(es,'SCID')),
 ('legacy-CID-binary',lambda es:long(es)['fields'].__setitem__('DCID','not base64')),('legacy-token-binary',wrong_token),
 ('legacy-CID-length',lambda es:long(es)['fields'].__setitem__('DCID Length',0)),('legacy-first-byte',lambda es:long(es)['fields'].__setitem__('First Byte',0)),
 ('owned-range',shifted_range),('missing-alias',missing_alias),('duplicate-alias',duplicate_alias),
 ('CID-alias-value',lambda es:set_alias(es,'quic.dcid','value','00')),('CID-alias-origin',lambda es:set_alias(es,'quic.dcid','origin','derived')),
 ('CID-value-span',lambda es:alias(long(es),'quic.dcid')['pdu_range'].__setitem__('offset',0)),
 ('version-alias',lambda es:set_alias(es,'quic.version','value',0)),('empty-CID-not-zero-octet',lambda es:alias(empty(es),'quic.dcid').__setitem__('value','00')),
 ('short-version',lambda es:extra_short(es,'quic.version')),('short-SCID',lambda es:extra_short(es,'quic.scid')),('short-CID-length',lambda es:extra_short(es,'quic.dcil')),('short-long-type',lambda es:extra_short(es,'quic.long.packet_type')),('short-forged-range',extra_range),
 ('unverified-PN',lambda es:verified(es)['session'].__setitem__('Authentication Verified',False)),
 ('decoded-PN-value',lambda es:alias(verified(es),'quic.packet_number').__setitem__('value',1<<40)),('decoded-PN-exact',lambda es:alias(verified(es),'quic.packet_number').__setitem__('exact_value','999999')),
 ('decoded-PN-not-ciphertext-span',lambda es:alias(verified(es),'quic.packet_number').__setitem__('pdu_range',{'offset':1,'length':2})),
]
results=[]
def run(name,es,oracle,must_pass=False):
 d=a.out/name;d.mkdir();ep=d/'events.jsonl';op=d/'oracle.json';ep.write_text(''.join(json.dumps(e,separators=(',',':'))+'\n'for e in es),encoding='utf8');op.write_text(json.dumps(oracle),encoding='utf8')
 args=[sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--oracle',str(op),'--events',str(ep),'--out',str(d/'audit.json'),'--require-no-native-surplus','--require-decoded-reference-coverage']
 r=subprocess.run(args,capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr)
 assert(r.returncode==0)==must_pass,(name,'unexpected audit acceptance/failure',(r.stdout+r.stderr).decode('utf8','replace')[-1200:])
 results.append({'control':name,'accepted':r.returncode==0})
run('baseline',original,reference,True)
for name,change in controls:
 es=copy.deepcopy(original);change(es);run(name,es,reference)
for name,key,range_offset in [('reference-CID-value','quic.dcid',False),('reference-PN-value','quic.packet_number',False),('reference-CID-range','quic.dcid_raw',True)]:
 oracle=copy.deepcopy(reference);wrong_reference(oracle,key,range_offset);run(name,original,oracle)
out={'scope':__doc__,'baseline_passed':True,'corruptions_rejected':len(results)-1,'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
