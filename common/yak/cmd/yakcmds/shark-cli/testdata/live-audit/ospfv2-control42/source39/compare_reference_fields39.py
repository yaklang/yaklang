"""Literal scalar/span OSPFv2 source-grid vs isolated pinned TShark comparison.

No native decoder is consulted. Reserved source leaves absent from the public
reference schema remain explicit unverified-by-reference coverage gaps.
"""
import argparse,collections,hashlib,json,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def pattern(path):return '/'.join('*'if isinstance(x,int)else x for x in path)
MAP={
'Header/Version':'ospf.version','Header/Type':'ospf.msg','Header/Packet Length':'ospf.packet_length','Header/Router ID':'ospf.srcrouter','Header/Area ID':'ospf.area_id','Header/Checksum':'ospf.checksum','Header/Instance ID':'ospf.instance_id','Header/Authentication Type':'ospf.auth.type',
'Authentication/Observed Authentication Field':'ospf.auth.none','Authentication/Key ID':'ospf.auth.crypt.key_id','Authentication/Authentication Data Length':'ospf.auth.crypt.data_length','Authentication/Sequence':'ospf.auth.crypt.seq_nbr','Authentication/Authentication Digest':'ospf.auth.crypt.data',
'Hello/Network Mask':'ospf.hello.network_mask','Hello/Hello Interval':'ospf.hello.hello_interval','Hello/Options':'ospf.v2.options','Hello/Router Priority':'ospf.hello.router_priority','Hello/Router Dead Interval':'ospf.hello.router_dead_interval','Hello/Designated Router':'ospf.hello.designated_router','Hello/Backup Designated Router':'ospf.hello.backup_designated_router','Hello/Neighbors/*/Neighbor Router ID':'ospf.hello.active_neighbor',
'Database Description/Interface MTU':'ospf.db.interface_mtu','Database Description/Options':'ospf.v2.options','Database Description/Flags':'ospf.dbd','Database Description/DD Sequence':'ospf.db.dd_sequence',
'Requests/*/LS Type':'ospf.lsa','Requests/*/Link State ID':'ospf.link_state_id','Requests/*/Advertising Router':'ospf.advrouter','LSA Count':'ospf.ls.number_of_lsas',
'LSAs/*/Body/Flags':'ospf.v2.router.lsa.flags','LSAs/*/Body/Link Count':'ospf.lsa.number_of_links','LSAs/*/Body/Links/*/Link ID':'ospf.lsa.router.linkid','LSAs/*/Body/Links/*/Link Data':'ospf.lsa.router.linkdata','LSAs/*/Body/Links/*/Link Type':'ospf.lsa.router.linktype','LSAs/*/Body/Links/*/TOS Count':'ospf.lsa.router.nummetrics','LSAs/*/Body/Links/*/Metric':'ospf.lsa.router.metric0','LSAs/*/Body/Attached Routers/*/Attached Router':'ospf.lsa.network.attchrtr','LSAs/*/Body/TOS Metrics/*/Metric':'ospf.metric','LSAs/*/Body/TOS Metrics/*/Forwarding Address':'ospf.lsa.asext.fwdaddr','LSAs/*/Body/TOS Metrics/*/External Route Tag':'ospf.lsa.asext.extrttag',
'LLS/Checksum':'ospf.lls.checksum','LLS/Length Words':'ospf.lls.data_length','LLS/TLVs/*/Type':'ospf.tlv_type','LLS/TLVs/*/Length':'ospf.tlv_length','LLS/TLVs/*/Extended Options and Flags':'ospf.lls.ext.options','LLS/TLVs/*/Sequence':'ospf.v2.lls.sequence_number','LLS/TLVs/*/Authentication Data':'ospf.v2.lls.auth_data'}
LSAH={'LS Age':'ospf.lsa.age','Options':'ospf.v2.options','LS Type':'ospf.lsa','Link State ID':'ospf.lsa.id','Advertising Router':'ospf.advrouter','Sequence':'ospf.lsa.seqnum','Checksum':'ospf.lsa.chksum','Length':'ospf.lsa.length'}
GAPS={'Authentication/Reserved':'reference public crypto header does not export reserved16..18','LSAs/*/Body/Reserved':'reference Router-LSA public schema does not export reserved byte'}
def alias(leaf,row):
 pat=pattern(leaf['path'])
 if pat in MAP:return MAP[pat]
 if leaf['path'][0]in('LSAs','LSA Headers')and len(leaf['path'])==3:return LSAH[leaf['path'][-1]]
 if leaf['path'][:2]==['Database Description','LSA Headers']and len(leaf['path'])==4:return LSAH[leaf['path'][-1]]
 if pat=='LSAs/*/Body/Network Mask':
  typ=row['source']['fields']['LSAs'][leaf['path'][1]]['LS Type'];return'ospf.lsa.network.netmask'if typ==2 else'ospf.lsa.asext.netmask'
 return None

def uint(field,raw=False):
 # FT_UINT masked fields use unmaskedvalue when caller explicitly requests raw.
 v=field.get('unmaskedvalue')if raw else None
 if v is None:v=field.get('value')
 if v is None:raise ValueError('missing public numeric value')
 return int(v,16)

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for key in('root','out'):ap.add_argument('--'+key,required=True,type=Path)
 a=ap.parse_args();assert not a.out.exists();a.out.mkdir();binding={str(Path(__file__)):sha(__file__)};count=collections.Counter();gap=collections.Counter();coverage=collections.Counter();fail=[];presentations=collections.Counter();details=[]
 def bad(row,leaf,reason):fail.append({'input':row['input'],'frame':row['frame'],'path':leaf['path'],'span':leaf['physical_span'],'reason':reason})
 for number in(35,37):
  gp=a.root/('source-grid02/input-%02d-source-grid.json'%number);rp=a.root/('reference01/input-%02d/reference.pdml'%number);producer=a.root/('reference01/input-%02d/producer.json'%number)
  for p in(gp,rp,producer):binding[str(p)]=sha(p)
  rows={r['frame']:r for r in load(gp)};seen=[]
  for _,packet in ET.iterparse(rp,events=['end']):
   if packet.tag!='packet':continue
   frame=int(packet.find("proto[@name='geninfo']/field[@name='num']").attrib['show']);assert frame in rows;row=rows[frame];seen.append(frame);fields=collections.defaultdict(list)
   for field in packet.iter('field'):
    x=field.attrib;n=x.get('name','')
    if n.startswith('ospf.'):fields[n].append(dict(x))
    if n in('ospf.checksum','ospf.lls.checksum','ospf.lsa.chksum'):
     text=x.get('showname','');presentations[(number,n,'incorrect'if'incorrect'in text else'correct'if'correct'in text else'None'if'(None)'in text else'plain')]+=1
   for leaf in row['source']['source_leaves']:
    count['source_leaves']+=1;pat=pattern(leaf['path']);coverage[pat]+=1;span=leaf['physical_span'];name=alias(leaf,row)
    if pat in GAPS:gap[pat]+=1;count['explicit_unexported_reference_gaps']+=1;continue
    if pat=='LSAs/*/Body/TOS Metrics/*/Flags and TOS':
     try:
      pair=[]
      for n in('ospf.lsa.asext.type','ospf.lsa.tos'):
       q=[f for f in fields[n]if[int(f['pos']),int(f['pos'])+int(f['size'])]==span];assert len(q)==1,(n,len(q));pair.append(q[0])
      assert uint(pair[0])==leaf['value']>>7;assert uint(pair[1])==leaf['value']&127;assert all(uint(f,True)==leaf['value']for f in pair)
      count['scalar_bit_split_leaves_verified']+=1;count['reference_field_value_span_checks']+=2
     except Exception as ex:bad(row,leaf,'bit split: '+repr(ex))
     continue
    if name is None:bad(row,leaf,'source leaf has no declared public-reference mapping');continue
    candidates=[f for f in fields[name]if[int(f['pos']),int(f['pos'])+int(f['size'])]==span]
    if len(candidates)!=1:bad(row,leaf,'expected exactly one reference alias/span;'+name+' count='+str(len(candidates)));continue
    f=candidates[0]
    try:
     if leaf['encoding']in('ipv4','id','mask'):assert f['show']==leaf['value']
     elif leaf['encoding']=='opaque':
      raw=bytes.fromhex(f['value']);assert {'bytes':len(raw),'SHA256':hashlib.sha256(raw).hexdigest()}==leaf['value'];count['opaque_byte_hash_leaves_verified']+=1
     elif name=='ospf.lsa.age':
      assert uint(f,True)==leaf['value'];assert uint(f)==(leaf['value']&0x7fff);count['LS_Age_raw_and_DoNotAge_mask_verified']+=1
     else:assert uint(f)==leaf['value']
     if name=='ospf.lls.data_length':assert int(f['show'])==4*leaf['value'];count['LLS_derived_byte_length_verified']+=1
     count['direct_reference_scalar_span_leaves_verified']+=1;count['reference_field_value_span_checks']+=1
    except Exception as ex:bad(row,leaf,'reference type/value mismatch;'+name+';'+repr(ex))
   count['source_physical_frames']+=1;packet.clear()
  assert set(seen)==set(rows)and len(seen)==len(rows)==278
 assert binding=={p:sha(p)for p in binding}
 write(a.out/'failures.json',fail)
 report={'scope':__doc__,'source_grid02_summary_SHA256':sha(a.root/'source-grid02/summary.json'),'reference01_summary_SHA256':sha(a.root/'reference01/summary.json'),'counts':dict(count),'coverage_by_source_path':dict(coverage),'explicit_reference_gaps':[{'path':k,'count':v,'reason':GAPS[k]}for k,v in gap.items()],'checksum_reference_presentations':[{'input':k[0],'field':k[1],'rendering':k[2],'count':v}for k,v in presentations.items()],'reference_checksum_limits':['LLS checksum value exported but official dissector has TODO verify checksum; no LLS verdict claimed','LSA checksum value exported but no Fletcher verification claimed','base OSPF checksum labels agree on these valid real samples; official full-IP-reported-length rather than OSPFn scope needs authored distinguishing control'],'failure_count':len(fail),'failures_SHA256':sha(a.out/'failures.json'),'inputs':binding,'inputs_unchanged':True,'native_output_read':False,'authentication_LSDB_neighbor_or_route_verified':False}
 write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(count),'failure_count':len(fail),'gaps':report['explicit_reference_gaps']}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
