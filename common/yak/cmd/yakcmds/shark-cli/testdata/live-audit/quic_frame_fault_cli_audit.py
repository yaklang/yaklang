"""Check real CLI rows against independently certified QUIC control observations.
Packet authentication, refused payload visibility, connection ownership and the
application label must agree. This certifies selected captured control packets,
not a complete UI implementation or endpoint acceptance of the drained controls.
"""
import argparse,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['fault-audit','header-audit','cli','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--before',type=Path)
a=ap.parse_args();checks=0
for p in vars(a).values():
 if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def rows(p):
 es=[json.loads(l)for l in p.open(encoding='utf-8-sig')];out={e['number']:e for e in es};eq(len(out),len(es),'one CLI row per physical packet');return out
fault=load(a.fault_audit);header=load(a.header_audit);eq(fault['events_sha256'],header['events_sha256'],'same certified native replay');es=rows(a.cli);eq(len(es),header['physical_frames'],'complete physical CLI replay');old=rows(a.before)if a.before else{};duplicate_before=0;records=[]
for r in fault['records']:
 e=es[r['physical_frame']];eq(e['protocol'],'QUIC','refused transport content cannot become application label');eq(e['conversation_id'],r['flow'],'same observed native connection');eq(e['protocol_status'],r['native_status'],'packet row and native detail status');check('QUIC'in e['protocol_stack'],'progressive transport path retained')
 if r['native_error_class']:
  eq(e['visibility'],'authenticated-plaintext','verified refused payload remains inspectable');eq(e['protocol_scope'],'authenticated packet diagnostic','diagnostic does not claim application delivery');eq(e['info'].count(r['native_error_class']),1,'single specific diagnostic instead of duplicated error summary');check(e['info'].startswith('QUIC · '+r['native_error_class']+' · '),'error starts visibly');eq(e['protocol_evidence'],'packet AEAD verified; payload not delivered','authentication and delivery claims explicit')
  if old:duplicate_before+=old[r['physical_frame']]['info'].count(r['native_error_class'])>1
 else:check('packet AEAD verified'in e['info'],'valid contrast remains verified')
 records.append({'physical_frame':r['physical_frame'],'case':r['case'],'epoch':r['epoch'],'status':e['protocol_status'],'visibility':e['visibility'],'flow':e['conversation_id']})
out={'scope':__doc__,'checks':checks,'physical_rows':len(es),'controlled_rows':len(records),'baseline_duplicate_diagnostics':duplicate_before,'fault_certificate_sha256':sha(a.fault_audit),'header_certificate_sha256':sha(a.header_audit),'CLI_sha256':sha(a.cli),'before_CLI_sha256':sha(a.before)if a.before else None,'records':records};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','records']}))
