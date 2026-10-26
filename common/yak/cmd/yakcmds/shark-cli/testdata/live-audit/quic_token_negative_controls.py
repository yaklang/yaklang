"""Require literal token projection checks to reject deliberately corrupted exports.
Original physical packets and TShark output are retained, never rewritten to
match a native value. This selected dataset must contain Initial, Retry, wide,
empty and binary tokens, plus short-header traffic.
"""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','oracle','events','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);original=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')];audit=Path(__file__).with_name('quic_header_wire_audit.py')
def pick(es,key,nonempty=False):return next(e for e in es if key in(e.get('session')or{})and(not nonempty or bool(e['session'][key])))
def alias(e,name):return next(v for v in e['field_values']['values']if v['name']==name)
def flip(es,key):
 e=pick(es,key,True);w=bytearray(base64.b64decode(e['session'][key]));w[0]^=1;e['session'][key]=base64.b64encode(w).decode()
def value(es,name,key,value):alias(pick(es,'Initial Token'if name in['quic.token','quic.token_length']else'Retry Token'),name)[key]=value
def range_start(es,key):pick(es,key)['session']['Wire Field Ranges'][key][0]+=1
def legacy_length(es):
 e=pick(es,'Initial Token Length Encoding Bytes');e['fields']['QUICToken']['Token Length']+=1
def legacy_prefix(es):
 e=next(e for e in es if(e.get('session')or{}).get('Initial Token Length Encoding Bytes',0)>1 and e.get('fields'));e['fields']['QUICToken']['Length Encoding']=0
def short_field(es,name):
 e=next(e for e in es if(e.get('session')or{}).get('Header Form')=='short');v=copy.deepcopy(alias(e,'quic.dcid'));v['name']=name;e['field_values']['values'].append(v)
def swap_kind(es):alias(pick(es,'Retry Token'),'quic.retry_token')['name']='quic.token'
controls=[('Initial-token-octets',lambda es:flip(es,'Initial Token')),('Retry-token-octets',lambda es:flip(es,'Retry Token')),('Retry-tag-octets',lambda es:flip(es,'Retry Integrity Tag')),('Initial-length-fact',lambda es:pick(es,'Initial Token Length')['session'].__setitem__('Initial Token Length',1<<40)),('Initial-width-fact',lambda es:pick(es,'Initial Token Length Encoding Bytes')['session'].__setitem__('Initial Token Length Encoding Bytes',8)),('Initial-length-alias',lambda es:value(es,'quic.token_length','value',1<<40)),('Initial-token-alias',lambda es:value(es,'quic.token','value','00')),('Retry-token-alias',lambda es:value(es,'quic.retry_token','value','00')),('Retry-tag-alias',lambda es:value(es,'quic.retry_integrity_tag','value','00')),('token-length-span',lambda es:range_start(es,'Initial Token Length')),('token-body-span',lambda es:range_start(es,'Initial Token')),('legacy-token-length',legacy_length),('legacy-width-prefix',legacy_prefix),('Retry-borrows-Initial-field',swap_kind)]
controls += [('short-borrows-'+name,lambda es,name=name:short_field(es,name))for name in['quic.token','quic.token_length','quic.retry_token','quic.retry_integrity_tag']]
controls += [('token-length-exact',lambda es:value(es,'quic.token_length','exact_value','999999999999999999'))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(original)
 if mutate:mutate(es)
 ep=d/'events.jsonl';ep.write_text(''.join(json.dumps(v)+'\n'for v in es),encoding='utf8');r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--oracle',str(a.oracle),'--events',str(ep),'--out',str(d/'audit.json'),'--require-no-native-surplus','--require-decoded-reference-coverage'],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-900:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
