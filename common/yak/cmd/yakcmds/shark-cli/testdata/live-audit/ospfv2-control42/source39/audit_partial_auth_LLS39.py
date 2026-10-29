"""Independent complete scalar prefixes and missing extents for authored controls.

Complete captured IP extent is distinct from snaplen truncation: both controls
have protocol-declared subordinate extents longer than a fully captured IPv4
packet. Only complete observed scalars are kept; missing digest/LLS bytes are not
invented and no full authentication/body verification is claimed.
"""
import argparse,hashlib,importlib.util,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(p):return json.loads(Path(p).read_text('utf8'))
def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for n in('source','codec','out'):ap.add_argument('--'+n,type=Path,required=True)
 a=ap.parse_args();assert not a.out.exists();sp=a.source/'source-facts.json';binding={str(p):sha(p)for p in(Path(__file__),a.codec,sp)};spec=importlib.util.spec_from_file_location('source_codec',a.codec);codec=importlib.util.module_from_spec(spec);spec.loader.exec_module(codec);result=[]
 for row in load(sp):
  if not row['strict_source_refusal_expected']:continue
  fp=Path(row['original_frame']);binding[str(fp)]=sha(fp);assert binding[str(fp)]==row['original_frame_SHA256'];raw=fp.read_bytes();start,end=row['PDU_range'];w=raw[start:end];n=int.from_bytes(w[2:4],'big');au=w[15];src=codec.Source(w,start)
  try:src.decode();raise AssertionError('expected source bounds refusal')
  except AssertionError:pass
  # These are complete IP packets with no missing captured Ethernet/IPv4 bytes.
  assert end==len(raw)==14+int.from_bytes(raw[16:18],'big')and n<=len(w)
  base={'verified':codec.checksum(w[:16]+w[24:n])==0,'scope':'OSPFn only excluding auth8'}if au in(0,1)else{'verified':False,'scope':'omitted by cryptographic authentication'}
  observed_prefix={};known=list(src.leaves)
  if au==2:
   wanted=w[19];extent=n+wanted;assert extent>len(w);opaque=w[n:]
   observed_prefix={'kind':'authentication digest','declared_PDU_span':[n,extent],'actual_PDU_span':[n,len(w)],'missing_PDU_span':[len(w),extent],'actual_bytes':len(opaque),'actual_SHA256':hashlib.sha256(opaque).hexdigest(),'complete_digest_field_present':False}
  else:
   words=int.from_bytes(w[n+2:n+4],'big');assert n+4*words>len(w);pos=n+4;tlv={};typ=src.leaf(tlv,['LLS','TLVs',0],'Type',pos,2);length=src.leaf(tlv,['LLS','TLVs',0],'Length',pos+2,2);value_start=pos+4;assert value_start+length>len(w)
   known=list(src.leaves);opaque=w[value_start:];observed_prefix={'kind':'LLS block and incomplete TLV value','declared_block_PDU_span':[n,n+4*words],'actual_block_PDU_span':[n,len(w)],'missing_PDU_span':[len(w),n+4*words],'first_TLV_type':typ,'declared_TLV_value_PDU_span':[value_start,value_start+length],'actual_TLV_value_PDU_span':[value_start,len(w)],'actual_value_bytes':len(opaque),'actual_value_SHA256':hashlib.sha256(opaque).hexdigest(),'complete_EOF_uint32_present':False,'full_LLS_checksum_verdict_available':False}
  assert all(leaf['PDU_span'][1]<=len(w)for leaf in known)
  result.append({'frame':row['frame'],'name':row['name'],'original_frame_SHA256':row['original_frame_SHA256'],'complete_captured_IP_extent':True,'capture_truncated':False,'protocol_declared_subordinate_extent_overruns_IP':True,'base_OSPF_body_complete':True,'base_checksum':base,'complete_source_scalar_prefix_leaves':known,'incomplete_source_extent':observed_prefix,'native_status_or_full_expected_event_defined':False})
 assert len(result)==2;assert binding=={p:sha(p)for p in binding}
 report={'scope':__doc__,'controls':result,'complete_prefix_scalar_leaves':sum(len(r['complete_source_scalar_prefix_leaves'])for r in result),'no_complete_fields_for_partial_values':True,'authentication_verified':False,'inputs':binding,'inputs_unchanged':True,'no_native_output_read':True}
 with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
 print(json.dumps({'proof':str(a.out),'SHA256':sha(a.out),'controls':len(result),'complete_prefix_leaves':report['complete_prefix_scalar_leaves'],'complete_IP_but_subordinate_extent_overrun':[r['frame']for r in result]}))
if __name__=='__main__':main()
