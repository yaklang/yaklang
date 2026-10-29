"""Authored OSPFv2 independent leaf/reference checks and retained RFC differences.

Only original authored bytes, independent source leaves, public reference schema
and official pinned dissector source are consumed. No native output is read.
"""
import argparse,collections,hashlib,importlib.util,json,re,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for key in('root','mapping','official-source','out'):ap.add_argument('--'+key,required=True,type=Path)
 a=ap.parse_args();assert not a.out.exists();a.out.mkdir();spec=importlib.util.spec_from_file_location('pure_reference_mapping',a.mapping);base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)
 sourcep=a.root/'authored-controls01/source-facts.json';refp=a.root/'authored-reference01/reference.pdml';paths=[Path(__file__),a.mapping,a.official_source,sourcep,refp,a.root/'authored-controls01/summary.json',a.root/'authored-reference01/producer.json'];binding={str(p):sha(p)for p in paths};rows={r['frame']:r for r in load(sourcep)};packets={int(p.find("proto[@name='geninfo']/field[@name='num']").get('show')):p for p in ET.parse(refp).getroot().findall('packet')};assert set(rows)==set(packets)
 count=collections.Counter();gaps=collections.Counter();fail=[];complete=[];differences=[]
 for frame,row in rows.items():
  rawp=Path(row['original_frame']);binding[str(rawp)]=sha(rawp);assert binding[str(rawp)]==row['original_frame_SHA256'];packet=packets[frame];by=collections.defaultdict(list)
  for field in packet.iter('field'):by[field.get('name','')].append(dict(field.attrib))
  if 'source'not in row:count['strict_source_extent_refusals']+=1;continue
  count['complete_source_rows']+=1;complete.append(frame)
  def unique(name,span):
   q=[f for f in by[name]if[int(f['pos']),int(f['pos'])+int(f['size'])]==span];assert len(q)==1,(name,span,len(q));return q[0]
  for leaf in row['source']['source_leaves']:
   count['source_leaves']+=1;pat=base.pattern(leaf['path']);span=leaf['physical_span'];name=base.alias(leaf,row)
   if pat in base.GAPS:gaps[base.GAPS[pat]]+=1;continue
   if pat in('LLS/TLVs/*/Opaque Value','LLS/TLVs/*/Padding'):
    gaps['unknown LLS TLV opaque value/padding are not exported in public reference schema']+=1;continue
   try:
    if pat.startswith('LSAs/*/Body/Links/*/TOS Metrics/'):
     parentpath=leaf['path'][:-1];parent=row['source']['fields']
     for key in parentpath:parent=parent[key]
     starts=[s[0]for s in parent['Wire Field Ranges'].values()];at=min(starts)+34;f=unique('ospf.ls.metric',[at,at+4]);wire=bytes.fromhex(f['value']);delta=span[0]-at;assert int.from_bytes(wire[delta:delta+span[1]-span[0]],'big')==leaf['value'];m=re.fullmatch(r'TOS: (\d+), Metric: (\d+)',f['showname']);assert m
     if leaf['path'][-1]=='TOS':assert int(m.group(1))==leaf['value'];count['compound_owner_semantic_subfields_verified']+=1
     elif leaf['path'][-1]=='Metric':assert int(m.group(2))==leaf['value'];count['compound_owner_semantic_subfields_verified']+=1
     else:gaps['Router optional metric Reserved has compound raw bytes but no public semantic alias']+=1
     count['wider_compound_owner_raw_subspans_verified']+=1;continue
    if pat=='LSAs/*/Body/Network Mask':
     typ=row['source']['fields']['LSAs'][leaf['path'][1]]['LS Type'];name='ospf.lsa.network.netmask'if typ==2 else'ospf.lsa.asbr.netmask'if typ in(3,4)else'ospf.lsa.asext.netmask'
    if pat=='LSAs/*/Body/TOS Metrics/*/TOS':name='ospf.lsa.tos'
    if pat=='Authentication/Observed Authentication Field':name='ospf.auth.simple'if row['source']['fields']['Header']['Authentication Type']==1 else'ospf.auth.none'
    if pat=='LSAs/*/Body/TOS Metrics/*/Flags and TOS':
     f=unique('ospf.lsa.asext.type',span);g=unique('ospf.lsa.tos',span);assert base.uint(f)==leaf['value']>>7 and base.uint(g)==leaf['value']&127;assert base.uint(f,True)==base.uint(g,True)==leaf['value'];count['scalar_bit_split_leaves_verified']+=1;count['reference_field_value_span_checks']+=2;continue
    assert name is not None,('no mapping',pat);f=unique(name,span)
    if leaf['encoding']in('ipv4','id','mask'):assert f['show']==leaf['value']
    elif leaf['encoding']=='opaque':
     wire=bytes.fromhex(f['value']);assert{'bytes':len(wire),'SHA256':hashlib.sha256(wire).hexdigest()}==leaf['value'];count['opaque_byte_hash_leaves_verified']+=1
    elif name=='ospf.lsa.age':assert base.uint(f,True)==leaf['value']and base.uint(f)==(leaf['value']&32767)
    else:assert base.uint(f)==leaf['value']
    if name=='ospf.lls.data_length':assert int(f['show'])==4*leaf['value'];count['LLS_derived_byte_length_verified']+=1
    count['direct_reference_scalar_span_leaves_verified']+=1;count['reference_field_value_span_checks']+=1
   except Exception as ex:fail.append({'frame':frame,'name':row['name'],'path':leaf['path'],'span':span,'reason':repr(ex)})
 # Retain exact owner: base checksum field is not the LLS checksum field.
 for n in(2,3,20,23,24):
  fs=[f.attrib for f in packets[n].iter('field')if f.get('name')=='ospf.checksum'];assert len(fs)==1 and fs[0]['pos']=='46'and fs[0]['size']=='2'
 pair=[Path(rows[n]['original_frame']).read_bytes()[34:]for n in(2,3)];length=int.from_bytes(pair[0][2:4],'big');assert pair[0][:length]==pair[1][:length];assert rows[2]['source']['fields']['Packet Checksum Verification']['verified']and rows[3]['source']['fields']['Packet Checksum Verification']['verified'];assert rows[2]['source']['fields']['LLS']['Checksum Verification']['verified']and not rows[3]['source']['fields']['LLS']['Checksum Verification']['verified']
 base2=next(f for f in packets[2].iter('field')if f.get('name')=='ospf.checksum');base3=next(f for f in packets[3].iter('field')if f.get('name')=='ospf.checksum');assert'[correct]'in base2.get('showname')and'[incorrect,'in base3.get('showname')
 differences.append({'kind':'LLS bad checksum contaminates displayed base checksum verdict','source_frames':[2,3],'same_base_PDU_SHA256':hashlib.sha256(pair[0][:length]).hexdigest(),'base_checksum_owner_physical':[46,48],'LLS_checksum_owner_physical':[82,84],'source_base_checksum_valid':[True,True],'source_LLS_checksum_valid':[True,False],'reference_base_rendering':['correct','incorrect'],'primary_source':'https://www.rfc-editor.org/rfc/rfc5613.html#section-2.2','official_code_lines':[1577,1595],'reference_disagreement_not_native_failure':True})
 assert not any(f.get('name')=='_ws.expert.message'for f in packets[2].iter('field'))
 expert=[f.get('showname','')for f in packets[22].iter('field')if f.get('name')=='_ws.expert.message'];assert any('Malformed Packet'in s for s in expert);unknown=rows[22]['source']['fields']['LLS']['TLVs'][1];assert unknown['Type']==300 and unknown['Length']==1 and unknown['Padding']['bytes']==3 and rows[22]['source']['fields']['LLS']['Checksum Verification']['verified']
 differences.append({'kind':'unknown LLS TLV padding step is not rounded to 32-bit word','source_frame':22,'public_reference_export_Malformed_exception':True,'source_unknown_value_bytes':1,'source_padding_bytes':3,'source_TLV_physical':[94,102],'source_block_checksum_valid':True,'primary_source':'https://www.rfc-editor.org/rfc/rfc5613.html#section-2.3','official_code_lines':[1774,1795],'reference_disagreement_not_native_failure':True})
 assert rows[24]['source']['fields']['Header']['Checksum']==0 and rows[24]['source']['fields']['Packet Checksum Verification']['verified'];z=next(f for f in packets[24].iter('field')if f.get('name')=='ospf.checksum');assert'(None)'in z.get('showname')
 differences.append({'kind':'mathematically valid checksum field zero is displayed as None without computation','source_frame':24,'Authentication_Type':0,'base_checksum_value':0,'source_base_checksum_valid':True,'primary_source':'https://www.rfc-editor.org/rfc/rfc2328.html#appendix-A.3.1','official_code_lines':[1566,1580],'scope':'reference rendering/checksum-verification limitation;not claim auth success'})
 assert binding=={p:sha(p)for p in binding};write(a.out/'failures.json',fail);write(a.out/'reference-differences.json',differences)
 report={'scope':__doc__,'counts':dict(count),'unexported_or_compound_reference_gaps':[{'reason':k,'leaves':v}for k,v in gaps.items()],'failure_count':len(fail),'failures_SHA256':sha(a.out/'failures.json'),'retained_reference_disagreements':len(differences),'reference_differences_SHA256':sha(a.out/'reference-differences.json'),'real_traffic_claim':False,'no_native_output_read':True,'official_source_URL':'https://raw.githubusercontent.com/wireshark/wireshark/v4.6.9/epan/dissectors/packet-ospf.c','inputs':binding,'inputs_unchanged':True,'auth_MAC_algorithm_SA_replay_LSDB_adjacency_route_verified':False}
 write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(count),'failures':len(fail),'reference_differences':len(differences),'gaps':report['unexported_or_compound_reference_gaps']}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
