"""Source-built whole-row EOL projections, with explicit uncovered rows.

The inputs are the frozen whole-CLI differential ledger and its parent summary.
This is a physical observer audit, not IP admission, reassembly or application
parity. Literal zero IPv4 lengths remain zero and do not establish an offload
or capture-loss cause. No PDU, flow, interface, sequence or wire value is waived.
"""
from pathlib import Path
import argparse, collections, copy, hashlib, ipaddress, json, struct, sys

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()

def same(x,y):
 if type(x)is not type(y):return False
 if isinstance(x,dict):return x.keys()==y.keys()and all(same(x[k],y[k])for k in x)
 if isinstance(x,list):return len(x)==len(y)and all(same(a,b)for a,b in zip(x,y))
 return x==y

def eol_ipv4(rec):
 w=rec['wire']
 if rec['link']!=1 or len(w)<34 or w[12:14]!=b'\x08\x00' or w[14]>>4!=4:return None
 ip=w[14:];n=(ip[0]&15)*4;declared=int.from_bytes(ip[2:4],'big')
 extent=min(len(ip),declared)if declared else len(ip)
 if n<20 or extent<n:return None
 at=20;has_eol=False
 while at<n:
  typ=ip[at]
  if typ==0:has_eol=True;break
  if typ==1:at+=1;continue
  if at+2>n or ip[at+1]<=2 or at+ip[at+1]>n:return None
  at+=ip[at+1]
 if not has_eol:return None
 return dict(source=str(ipaddress.ip_address(ip[12:16])),destination=str(ipaddress.ip_address(ip[16:20])),fragment=bool(int.from_bytes(ip[6:8],'big')&0x3fff),declared=declared,protocol=ip[9],header=ip[:n],body=ip[n:extent])

def remove_decode_suffix(text):
 assert text.count(' [incomplete/undecoded]')==1
 return text.replace(' [incomplete/undecoded]','',1)

def expected(rec,old,new):
 p=eol_ipv4(rec)
 if p is None:return None
 assert old['number']==new['number'] and old.get('pdus')==new.get('pdus')and old.get('published_pdus')==new.get('published_pdus')
 # These three profiles have direct Ethernet/IPv4 source coordinates. Nested
 # carriers, domain changes and other child grammars remain uncovered.
 base=copy.deepcopy(old)
 if old['protocol']=='IPv4'and old['layers']==['Ethernet','IPv4']and p['fragment']and old['capture_domain']==new['capture_domain']:
  observed=['Ethernet','IPv4']+(['Fragment']if p['body']else[])
  base.update(source=p['source'],destination=p['destination'],protocol=observed[-1],layers=observed,info=remove_decode_suffix(old['info']))
  return 'direct-fragment',base,p
 address_only=copy.deepcopy(old);address_only.update(source=p['source'],destination=p['destination'])
 if not p['fragment']and old['layers']==['Ethernet','IPv4']and same(address_only,new):
  return 'fixed-header-endpoints-only',address_only,p
 view=old.get('transport_view')
 if not p['fragment']and p['protocol']==6 and old['layers']==['Ethernet','IPv4']and view and view['protocol']=='tcp':
  b=p['body'];assert len(b)>=20
  n=(b[12]>>4)*4;assert n>=20 and n<=len(b)
  src,dst,seq,ack=struct.unpack_from('!HHII',b)
  assert view['source']==p['source']and view['destination']==p['destination']and view['source_port']==src and view['destination_port']==dst
  # The existing observation field carries all twelve literal low bits,
  # including reserved bits; it is not a peer-validity assertion.
  assert view.get('sequence',0)==seq and view.get('acknowledgment',0)==ack and view.get('tcp_flags',0)==int.from_bytes(b[12:14],'big')&0xfff,(old['number'],seq,ack,view)
  assert view['header_sha256']==hashlib.sha256(b[:n]).hexdigest()and view['header_length']==n and view['payload_length']==len(b)-n
  assert view['layers']==['Ethernet','IPv4','TCP']and view['layers_complete']is True
  domain=dict(Section=rec['section'],Interface=rec['interface'],Encapsulation='')
  assert view['domain']==domain and view['packet_refs']==[dict(Domain=domain,Number=old['number'])]
  assert old['source']==p['source']and old['destination']==p['destination']
  flags=','.join(name for mask,name in[(2,'SYN'),(16,'ACK'),(1,'FIN'),(4,'RST'),(8,'PSH'),(32,'URG')]if b[13]&mask)
  info=f'{src} → {dst} [{flags}] Seq={seq} Ack={ack} Payload={len(b)-n}'
  assert old['info'].count(view['info'])==1
  base.update(layers=['Ethernet','IPv4','TCP'],protocol_scope='packet',info=remove_decode_suffix(old['info'].replace(view['info'],info,1)))
  for key in['physical_source','physical_destination','transport_view','protocol_stack']:base.pop(key,None)
  return 'direct-complete-TCP-observation',base,p
 return None

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for name in['capture','diff','audit-modules','out']:ap.add_argument('--'+name,type=Path,required=True)
 a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.audit_modules.resolve()))
 from vxlan_endpoint_projection_audit import capture_records
 sources={i:r for i,r in enumerate(capture_records(a.capture),1)}
 manifest=json.loads((a.diff/'sharon.json').read_text());ledger=a.diff/'sharon-changed.jsonl'
 counts=collections.Counter();uncovered=[];positives={};checked=0;zero=0
 for line in ledger.open(encoding='utf8'):
  v=json.loads(line);number=v['frame'];old,new=v['old'],v['new'];rec=sources[number]
  assert number==old['number']==new['number'] and rec['original_length']==new['length']and rec['captured_length']==new['captured_length']
  result=expected(rec,old,new)
  if result is None:
   uncovered.append(dict(frame=number,protocol_before=old['protocol'],protocol_after=new['protocol'],layers_before=old['layers'],layers_after=new['layers']));continue
  category,want,facts=result
  assert same(want,new),(number,category,{k:(want.get(k),new.get(k))for k in want.keys()|new.keys()if not same(want.get(k),new.get(k))})
  counts[category]+=1;checked+=1;zero+=facts['declared']==0
  positives.setdefault(category,(copy.deepcopy(rec),copy.deepcopy(old),copy.deepcopy(new)))
 assert checked+len(uncovered)==manifest['changed']==16342
 assert counts=={'direct-fragment':15109,'direct-complete-TCP-observation':276,'fixed-header-endpoints-only':654}
 assert len(uncovered)==303 and sha(a.capture)=='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'
 # Corrupt semantic results through the same whole-row comparison as public
 # positives. These controls are auditor checks, not extra codec executions.
 negative=[]
 for category,(rec,old,new)in positives.items():
  for mutation in['source','native-PDU','layer','wire-source']:
   r,x,y=copy.deepcopy((rec,old,new))
   if mutation=='source':y['source']='203.0.113.254'
   elif mutation=='native-PDU':y['pdus']=[{'id':True}]
   elif mutation=='layer':y['layers']=y['layers'][:-1]
   else:
    w=bytearray(r['wire']);w[26]^=1;r['wire']=bytes(w)
   rejected=False
   try:
    result=expected(r,x,y)
    rejected=result is None or not same(result[1],y)
   except AssertionError:rejected=True
   assert rejected,(category,mutation)
   negative.append(category+'/'+mutation)
 (a.out/'uncovered.json').write_text(json.dumps(uncovered,indent=2)+'\n',encoding='utf8')
 summary=dict(scope=__doc__,capture_SHA256=sha(a.capture),capture_records=len(sources),before_CLI_SHA256=manifest['before_SHA256'],after_CLI_SHA256=manifest['after_SHA256'],parent_diff_summary_SHA256=sha(a.diff/'sharon.json'),complete_changed_ledger_SHA256=sha(ledger),changed=16342,whole_source_expected=checked,categories=dict(counts),literal_zero_IPv4_length=zero,uncovered_count=len(uncovered),uncovered_SHA256=sha(a.out/'uncovered.json'),negative_controls=negative,auditor_SHA256=sha(Path(__file__)))
 (a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');print(json.dumps(summary),flush=True)

if __name__=='__main__':main()
