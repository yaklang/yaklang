"""Bind QUIC CID/version/decoded-PN projections to native PDU bytes and TShark.
Clear header fields do not prove a sender. Decoded packet numbers are separately
gated by native authentication and compared with the reference when available.
Unmatched/derived reference views stay visible rather than becoming matches.
"""
import argparse,base64,collections,hashlib,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','oracle','events','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--require-reference-coverage',action='store_true')
ap.add_argument('--require-no-native-surplus',action='store_true')
ap.add_argument('--require-decoded-reference-coverage',action='store_true')
a=ap.parse_args();checks=0
for p in[a.capture,a.oracle,a.events,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))

def header_view(view):
 # TShark nests short-header fields but emits long-header fields directly.
 # Never recursively search frame bodies/ACKs for a header value.
 if not isinstance(view,dict):return {'untyped_reference_view':type(view).__name__}
 short=view.get('quic.short')or{}
 check(isinstance(short,dict),'one short-header subtree per raw packet')
 out=dict(view)
 for k,v in short.items():
  check(k not in out or out[k]==v,'header subtree has no conflicting scalar')
  out[k]=v
 return out
def frames(p):
 w=p.read_bytes();out={};at=0;section=-1;interfaces=[]
 if w[:4]in[bytes.fromhex(h)for h in['d4c3b2a1','4d3cb2a1','a1b2c3d4','a1b23c4d']]:
  endian='<'if w[:4]in[bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')]else'>';at=24
  while at<len(w):
   check(at+16<=len(w),'classic record header');n=struct.unpack_from(endian+'I',w,at+8)[0];at+=16;check(at+n<=len(w),'classic captured extent');out[len(out)+1]=(w[at:at+n],0,0);at+=n
  return out
 while at<len(w):
  check(at+12<=len(w),'PCAPNG block header')
  if w[at:at+4]==bytes.fromhex('0a0d0d0a'):endian='<'if w[at+8:at+12]==bytes.fromhex('4d3c2b1a')else'>';section+=1;interfaces=[]
  typ,n=struct.unpack_from(endian+'II',w,at);check(n>=12 and n%4==0 and at+n<=len(w),'bounded PCAPNG block');eq(struct.unpack_from(endian+'I',w,at+n-4)[0],n,'block footer');body=w[at+8:at+n-4]
  if typ==1:interfaces.append(struct.unpack_from(endian+'H',body)[0])
  if typ in[2,6]:
   iface=struct.unpack_from(endian+('I'if typ==6 else'H'),body)[0];size=struct.unpack_from(endian+'I',body,12)[0];check(iface<len(interfaces)and size<=len(body)-20,'packet interface and extent');out[len(out)+1]=(body[20:20+size],section,iface)
  check(typ!=3,'simple packet blocks are outside this certificate');at+=n
 return out
physical=frames(a.capture);ref=collections.defaultdict(list);derived=[];nonroot=[];untyped=[];raw_views=0;rows=load(a.oracle);reference_connections={};reference_versions=collections.Counter()
for row in rows:
 l=row['_source']['layers'];no=int(l['frame']['frame.number']);eq(bytes.fromhex(l['frame_raw'][0]),physical[no][0],'reference physical bytes');raws=l.get('quic_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws;views=l.get('quic')or[];views=views if isinstance(views,list)else[views]
 connections={str(v['QUIC Connection information']['quic.connection.number'])for v in views if isinstance(v,dict)and'quic.connection.number'in(v.get('QUIC Connection information')or{})}
 if len(connections)==1:reference_connections[no]=next(iter(connections))
 if not raws:nonroot.append(no)
 for i,r in enumerate(raws):
  raw_views+=1;w=bytes.fromhex(r[0]);eq(len(w),int(r[2]),'reference raw extent')
  if len(r)>5 and int(r[5])!=0:derived.append({'frame':no,'data_source':int(r[5]),'bytes':len(w),'sha256':sha(w)});continue
  at=int(r[1]);eq(physical[no][0][at:at+len(w)],w,'reference literal packet span');view=views[i]if i<len(views)else{}
  if not isinstance(view,dict):untyped.append({'frame':no,'reference_occurrence':i,'JSON_type':type(view).__name__})
  ref[no].append((w,view,at,i));reference_versions['0x'+w[1:5].hex()if len(w)>=5 and w[0]&0x80 else'short-or-diagnostic']+=1
native=0;matched=set();seen_ids=set();missing_reference=[];differences=[];counts=collections.Counter();alias_counts=collections.Counter();zero_omissions=collections.Counter();logical=0;unmatched=[]
token_names={'quic.token_length','quic.token','quic.retry_token','quic.retry_integrity_tag'}
header_names={'quic.version','quic.dcid','quic.scid','quic.dcil','quic.scil','quic.long.packet_type','quic.long.packet_type_v2','quic.packet_number'}|token_names
flow_connections={};connection_flows={};association_checks=0;pn_reference_comparisons=0;pn_reference_omissions=[]
for line in a.events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 if e['protocol']not in['quic','http3','doq']:continue
 check(e['id']not in seen_ids,'duplicate native QUIC event ID');seen_ids.add(e['id'])
 w=base64.b64decode(e['raw']or'',validate=True);m=e.get('session')or{};values=e['field_values']['values'];aliases={v['name']:v for v in values};ranges=m.get('Wire Field Ranges')or{};native+=1;eq(e['source_bytes']['SHA256'],sha(w),'native PDU digest');refs=e['source_bytes']['PacketRefs'];candidates=[]
 for name in header_names:check(sum(v['name']==name for v in values)<=1,'duplicate typed header field '+name)
 for r in refs:
  packet,sec,iface=physical[r['Number']];eq((r['Domain']['Section'],r['Domain']['Interface']),(sec,iface),'physical domain');eq(r['Domain'],e['domain'],'native witness domain')
  candidates +=[(r['Number'],rw,v,start,index)for rw,v,start,index in ref[r['Number']]if rw==w]
 unused=[v for v in candidates if(v[0],v[4])not in matched]
 check(not candidates or bool(unused),'duplicate native PDU for the same reference packet occurrence')
 if unused:no,rw,view,start,index=unused[0];view=header_view(view);matched.add((no,index))
 else:unmatched.append({'PDU':e['id'],'frames':[r['Number']for r in refs],'bytes':len(w),'status':e['status'],'has_header_projection':bool(ranges)});view={};start=None
 expected={}
 if ranges:
  form=m.get('Header Form');check(form in['long','short'],'explicit native header form');eq(bool(w[0]&128),form=='long','wire header form');dcid=base64.b64decode(m.get('DCID')or'');scid=base64.b64decode(m.get('SCID')or'');counts[(form,len(dcid),len(scid))]+=1
  if form=='long':
   check(len(w)>=7,'long fixed header');dl=w[5];check(dl<=20 and 7+dl<=len(w),'long DCID bound');sl=w[6+dl];check(sl<=20 and 7+dl+sl<=len(w),'long SCID bound');eq(dcid,w[6:6+dl],'literal DCID');eq(scid,w[7+dl:7+dl+sl],'literal SCID');version=int.from_bytes(w[1:5],'big');expected={'quic.version':(version,1,5,'Version'),'quic.dcil':(dl,5,6,'DCID Length'),'quic.dcid':(dcid.hex(),6,6+dl,'DCID'),'quic.scil':(sl,6+dl,7+dl,'SCID Length'),'quic.scid':(scid.hex(),7+dl,7+dl+sl,'SCID')}
   if version in[1,0x6b3343cf]:expected['quic.long.packet_type_v2'if version==0x6b3343cf else'quic.long.packet_type']=(w[0]>>4&3,0,1,'Long Packet Type')
   for name in{'quic.long.packet_type','quic.long.packet_type_v2'}-set(expected):check(name not in aliases,'version-specific packet type cannot borrow another version alias')
   kind=w[0]>>4&3;kind=(kind+3)&3 if version==0x6b3343cf else kind;off=7+dl+sl
   if version in[1,0x6b3343cf]and kind==0:
    check(off<len(w),'Initial token-length prefix');width=1<<(w[off]>>6);check(off+width<=len(w),'complete Initial token length');n=int.from_bytes(w[off:off+width],'big')&((1<<(width*8-2))-1);check(n<=20480 and off+width+n<=len(w),'complete selected Initial token');expected['quic.token_length']=(n,off,off+width,'Initial Token Length');expected['quic.token']=(w[off+width:off+width+n].hex(),off+width,off+width+n,'Initial Token');eq(m.get('Initial Token Length'),n,'retained literal token length');eq(m.get('Initial Token Length Encoding Bytes'),width,'retained literal token encoding width');eq(base64.b64decode(m.get('Initial Token')or''),w[off+width:off+width+n],'retained literal Initial token')
   elif version in[1,0x6b3343cf]and kind==3:
    check(off+16<=len(w),'complete Retry integrity field');expected['quic.retry_token']=(w[off:-16].hex(),off,len(w)-16,'Retry Token');expected['quic.retry_integrity_tag']=(w[-16:].hex(),len(w)-16,len(w),'Retry Integrity Tag');eq(base64.b64decode(m.get('Retry Token')or''),w[off:-16],'retained literal Retry token');eq(base64.b64decode(m.get('Retry Integrity Tag')or''),w[-16:],'retained literal public Retry integrity tag')
   for name in token_names-set(expected):check(name not in aliases,'packet type cannot borrow another token field')
  else:
   check(len(dcid)<=20 and 1+len(dcid)<=len(w),'observed short CID size');eq(dcid,w[1:1+len(dcid)],'literal known-context short CID');expected={'quic.dcid':(dcid.hex(),1,1+len(dcid),'DCID')}
   for name in{'quic.version','quic.scid','quic.scil','quic.dcil','quic.long.packet_type','quic.long.packet_type_v2'}|token_names:check(name not in aliases,'short header cannot invent '+name)
  eq(set(ranges),{v[3]for v in expected.values()},'only literal fields carried by this header have ranges')
  for key,(value,lo,hi,name)in expected.items():
   eq(ranges[name],[lo,hi],'owned clear-header range');eq(aliases[key]['value'],value,'typed '+key);eq(aliases[key]['origin'],'pdu','literal origin');eq(aliases[key]['pdu_range'],{'offset':lo,'length':hi-lo},'logical PDU span');alias_counts[key]+=1
   if key=='quic.token_length':eq(aliases[key]['exact_value'],str(value),'lossless 62-bit token length projection')
   if view and key in view:
    rv=view[key];check(not isinstance(rv,list),'one value per QUIC header instance');rv=bytes.fromhex(rv.replace(':','')).hex()if key in['quic.dcid','quic.scid','quic.token','quic.retry_token','quic.retry_integrity_tag']else int(rv,0)
    if rv!=value:differences.append({'PDU':e['id'],'frame':no,'field':key,'native':value,'reference':rv})
    rr=view.get(key+'_raw')
    if rr and not isinstance(rr[0],list)and int(rr[5])==0:eq((int(rr[1]),int(rr[2])),(start+lo,hi-lo),'reference field span')
   elif view and value=='':zero_omissions[key]+=1
   elif view:missing_reference.append({'PDU':e['id'],'frame':no,'field':key})
  fields=e.get('fields')or{}
  if fields:
   if'First Byte'in fields:eq(fields['First Byte'],w[0],'legacy envelope literal first byte')
   if form=='long':
    for name,value in[('Version',version),('DCID Length',dl),('SCID Length',sl)]:
     if name in fields:eq(fields[name],value,'legacy envelope literal '+name)
  for name in['DCID','SCID']:
   if name in fields:eq(base64.b64decode(fields[name]),dcid if name=='DCID'else scid,'binary legacy-shaped field survives JSON')
  if'QUICToken'in fields:
   check(form=='long'and'quic.token_length'in expected,'Initial token envelope belongs to Initial');off=7+len(dcid)+len(scid);width=1<<(w[off]>>6);n=int.from_bytes(w[off:off+width],'big')&((1<<(width*8-2))-1);check(off+width+n<=len(w),'bounded full-varint token envelope');tf=fields['QUICToken'];eq(tf['Token Length'],n,'legacy-shaped decoded token length');eq(base64.b64decode(tf.get('Token')or''),w[off+width:off+width+n],'literal binary token');eq('Token'in tf,n>0,'empty token does not invent a legacy leaf')
   if width>1:eq(tf['Length Encoding'],w[off]>>6,'wide length encoding prefix')
   else:check('Length Encoding'not in tf,'one-byte legacy shape preserved')
   eq(set(tf),{'Token Length'}|({'Length Encoding'}if width>1 else set())|({'Token'}if n else set()),'only literal token envelope fields');eq(fields.get('Protected Payload'),list(w[off+width+n:]),'opaque legacy body starts after complete token')
 else:
  for key in{'quic.version','quic.dcid','quic.scid','quic.dcil','quic.scil','quic.long.packet_type','quic.long.packet_type_v2'}|token_names:check(key not in aliases,'unparsed remainder cannot inherit a header field')
 if m.get('Authentication Verified')and'Packet Number'in m:
  pn=m['Packet Number'];eq(aliases['quic.packet_number']['value'],pn,'authenticated decoded number');eq(aliases['quic.packet_number']['exact_value'],str(pn),'lossless decoded number');check('pdu_range'not in aliases['quic.packet_number'],'decoded number is not a literal ciphertext value span');logical+=1
  if'quic.packet_number'in view:
   pn_reference_comparisons+=1
   if int(view['quic.packet_number'])!=pn:differences.append({'PDU':e['id'],'frame':no,'field':'quic.packet_number','native':pn,'reference':int(view['quic.packet_number'])})
  elif view:pn_reference_omissions.append({'PDU':e['id'],'frame':no,'native_authenticated_PN':pn})
  if candidates and no in reference_connections and e['flow']:
   domain=json.dumps(e['domain'],sort_keys=True);fk=(domain,e['flow']);ck=(domain,reference_connections[no]);flow_connections.setdefault(fk,ck);connection_flows.setdefault(ck,fk);eq(flow_connections[fk],ck,'one authenticated reference connection per native flow/domain');eq(connection_flows[ck],fk,'one native generation per reference connection/domain');association_checks+=1
 else:check('quic.packet_number'not in aliases,'unverified payload cannot publish a decoded number')
missing=[{'frame':no,'reference_occurrence':index,'bytes':len(w),'sha256':sha(w),'wire_version':'0x'+w[1:5].hex()if len(w)>=5 and w[0]&128 else None,'reference_decoded_packet_number':'quic.packet_number'in header_view(v),'reference_connection':reference_connections.get(no)}for no,vs in ref.items()for w,v,at,index in vs if(no,index)not in matched]
out={
 'scope':__doc__,'checks':checks,'physical_frames':len(physical),
 'reference_filter_rows':len(rows),'reference_raw_views':raw_views,
 # This includes fuzz/undissected wire words; it is not a count of valid versions.
 'reference_long_candidate_version_words':dict(reference_versions),
 'reference_nonroot_frames':nonroot,'reference_derived_views':derived,'reference_untyped_views':untyped,
 'native_QUIC_carrier_PDUs':native,'matched_reference_views':len(matched),
 'header_CID_profiles':{str(k):v for k,v in counts.items()},'typed_header_occurrences':dict(alias_counts),
 'authenticated_packet_numbers':logical,'authenticated_flow_association_checks':association_checks,
 'authenticated_packet_number_reference_comparisons':pn_reference_comparisons,'authenticated_packet_number_reference_omissions':pn_reference_omissions,
 'reference_zero_length_omissions':dict(zero_omissions),'reference_value_differences':differences,
 'reference_field_omissions':missing_reference,'unmatched_native_views':unmatched,'unmatched_reference_views':missing,
 'capture_sha256':sha(a.capture.read_bytes()),'oracle_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes())
}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8')
hidden={'scope','reference_value_differences','reference_field_omissions','unmatched_native_views','unmatched_reference_views','reference_nonroot_frames','reference_derived_views','reference_untyped_views','reference_long_candidate_version_words'}
print(json.dumps({k:v for k,v in out.items()if k not in hidden}));print('reference differences / omissions / unmatched native / unmatched reference',len(differences),len(missing_reference),len(unmatched),len(missing))
check(not differences,'decoded header/reference value differences remain; inspect the saved report')
if a.require_reference_coverage:check(not missing,'reference packet occurrences were not all covered')
if a.require_no_native_surplus:check(not unmatched,'native packet occurrences missing from the reference')
if a.require_decoded_reference_coverage:check(not any(v['reference_decoded_packet_number']for v in missing),'decoded reference packet occurrences were not all covered')
