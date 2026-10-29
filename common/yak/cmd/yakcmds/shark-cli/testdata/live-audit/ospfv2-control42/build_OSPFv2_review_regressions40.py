"""Minimum authored regressions for the agreed OSPFv2 draft review scope.

Original wire and complete scalar spans are constructed independently before
candidate execution. The old real source-grid03 is read only by hash; no native,
Go or candidate protocol output is consumed. These are17 tiny authored PCAP
records, not live traffic; one no-CA wire has both full/budgeted observation
scenarios. Unknown authentication trailer placement is intentionally opaque.
"""
import argparse,base64,collections,hashlib,importlib.util,json,shutil,struct
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def load(name,p):
 spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

def known_prefix(codec,w,physical_start):
 s=codec.Source(w,physical_start);fields={};h={};at=0
 schema=[('Version',1,'uint'),('Type',1,'uint'),('Packet Length',2,'uint'),('Router ID',4,'id'),('Area ID',4,'id'),('Checksum',2,'uint'),('Instance ID',1,'uint'),('Authentication Type',1,'uint')]
 for name,n,kind in schema:
  if at+n>len(w):break
  s.leaf(h,['Header'],name,at,n,kind);at+=n
 fields['Header']=h
 if at==16:
  auth={};kind=h['Authentication Type']
  if kind==2:
   for name,at,n in[('Reserved',16,2),('Key ID',18,1),('Authentication Data Length',19,1),('Sequence',20,4)]:
    if at+n>len(w):break
    s.leaf(auth,['Authentication'],name,at,n)
  elif len(w)>=24:s.leaf(auth,['Authentication'],'Observed Authentication Field',16,8,'opaque')
  if auth:fields['Authentication']=auth
 return s,fields

def unknown_auth_source(codec,w,physical_start):
 s,fields=known_prefix(codec,w,physical_start);h=fields['Header'];assert h['Authentication Type']>2;n=h['Packet Length'];assert 44<=n<=len(w);hello={};at=24
 for name,size,kind in[('Network Mask',4,'mask'),('Hello Interval',2,'uint'),('Options',1,'uint'),('Router Priority',1,'uint'),('Router Dead Interval',4,'uint'),('Designated Router',4,'id'),('Backup Designated Router',4,'id')]:s.leaf(hello,['Hello'],name,at,size,kind,n);at+=size
 neighbors=[]
 while at<n:
  c={};s.leaf(c,['Hello','Neighbors',len(neighbors)],'Neighbor Router ID',at,4,'id',n);neighbors.append(c);at+=4
 hello['Neighbors']=neighbors;fields['Hello']=hello
 fields['L Bit']=bool(hello['Options']&16);fields['Trailer Observation']={'PDU_span':[n,len(w)],'bytes':len(w)-n,'SHA256':digest(w[n:]),'scope':'unknown authentication layout; no LLS position, fields or checksum verdict inferred'}
 return{'fields':fields,'source_leaves':s.leaves,'source_observed_bytes':len(w),'source_OSPF_packet_length':n,'no_LSDB_neighbor_SA_replay_or_installed_route_inferred':True}

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for n in('phase','builder','codec','out'):ap.add_argument('--'+n,type=Path,required=True)
 a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();b=load('authored_public_OSPFv2_builder',a.builder);codec=load('frozen_OSPFv2_source03',a.codec)
 real=a.phase/'OSPFv2-independent-source39-01/source-grid03/summary.json';assert sha(real)=='2aaf89759d9b4a2e44ff1b075778694d2b2c9eea8db3a3f12b19bae0ffdbbc65';binding={str(p):sha(p)for p in(Path(__file__),a.builder,a.codec,real)}
 seq=0x10203040;opaque16=bytes((i*7+3)&255 for i in range(16));opaque8=bytes((i*11+5)&255 for i in range(8));auth=b'\0\0'+bytes([7,16])+b.u32(seq);eof=(1,b.u32(1));fullca=[eof,(2,b.u32(seq)+opaque16)];cases=[]
 def add(name,wire,kind,requirements,budget=4096,original=None,variants=None):
  cases.append({'name':name,'wire':wire,'kind':kind,'requirements':requirements,'budget':budget,'original_PDU':wire if original is None else original,'variants':variants or[]})
 badhead=bytes([2,1,0,0])+b.ip('192.0.2.10')+b.ip('0.0.0.0')+b'\0\0'+bytes([0,2])+auth;assert len(badhead)==24
 for cut in[4,*range(16,24)]:
  add('short-header-cut%d-declared-n0'%cut,badhead[:cut],'known-bad-length-plus-capture-missing',{'wire_format_identity_required':True,'missing_captured_header_does_not_erase_bad_length':True,'Packet_Length_observed':0,'Packet_Length_PDU_span':[2,4],'complete_scalar_prefix_only':True,'absent_or_partial_values_must_not_appear_as_zero':True},original=badhead)
 network=b.lsa(2,b.ip('255.255.255.0'),identifier='192.0.2.254');assert codec.fletcher(network)['valid']
 add('Network-LSA-mask-only',b.packet(4,b.u32(1)+network),'complete-Network-LSA-empty-router-list',{'wire_format_identity_required':True,'base_packet_checksum_valid':True,'full_LSA_Fletcher_valid':True,'Network_Mask_retained':True,'Attached_Router_count':0,'no_inferred_DR_or_LSDB_state':True})
 nonzero=bytearray(b.packet(1,b.hello(),2,auth,opaque16));nonzero[12:14]=b.u16(1)
 add('Auth2-nonzero-packet-checksum',bytes(nonzero),'nonzero-omitted-packet-field',{'wire_format_identity_required':True,'packet_checksum_scope':'omitted','packet_mathematical_checksum_not_verified':True,'Authentication_Verified':False,'not_MAC_authentication_failure':True})
 nonzerolls=bytearray(b.lls(fullca,True));nonzerolls[0:2]=b.u16(1)
 add('Auth2-nonzero-LLS-checksum',b.packet(1,b.hello(0x12),2,auth,opaque16,bytes(nonzerolls)),'nonzero-omitted-LLS-field',{'wire_format_identity_required':True,'LLS_checksum_scope':'omitted','LLS_mathematical_checksum_not_verified':True,'Authentication_Verified':False,'not_MAC_authentication_failure':True})
 add('Auth2-complete-LLS-no-CA',b.packet(1,b.hello(0x12),2,auth,opaque16,b.lls([eof],True)),'complete-LLS-missing-authenticator',{'complete_TLV_coverage_required_before_CA_absence':True,'CA_count':0,'missing_CA_must_be_explicit':True,'Authentication_Verified':False},variants=[{'name':'same-wire-budget1-no-CA-absence-proof','budget':1,'requirements':{'budget_identity_required':True,'LLS_header_can_be_retained':True,'complete_TLV_coverage_available':False,'missing_CA_format_identity_must_not_be_inferred_from_budgeted_array':True}}])
 add('Auth2-CA-data-length8-header16',b.packet(1,b.hello(0x12),2,auth,opaque16,b.lls([eof,(2,b.u32(seq)+opaque8)],True)),'captured-authenticator-length-contradiction',{'header_digest_bytes':16,'LLS_CA_data_bytes':8,'length_mismatch_must_be_explicit':True,'same_sequence_does_not_verify_MAC':True,'Authentication_Verified':False})
 shaped=bytearray(b.lls([eof],False));shaped[0:2]=b.u16(1)
 add('unknown-Auth3-Lbit-opaque-shaped-tail',b.packet(1,b.hello(0x12),3,b'\xa5'*8,tail=bytes(shaped)),'unknown-auth-no-trailer-position',{'context_identity_required':True,'source_L_bit':True,'LLS_position_known':False,'LLS_fields_must_not_be_constructed':True,'LLS_integrity_identity_must_not_be_invented':True,'opaque_trailer_owned':True,'Authentication_Verified':False})
 hello2=b.hello(0x12)+b.ip('192.0.2.13');badlls=bytearray(b.lls([eof],False));badlls[0]^=1
 add('Hello-neighbor-budget1-independent-bad-LLS',b.packet(1,hello2,tail=bytes(badlls)),'budget-plus-independent-LLS-integrity',{'budget_identity_required':True,'LLS_integrity_identity_required':True,'base_packet_checksum_valid':True,'LLS_block_checksum_valid':False,'all_captured_neighbor_count':2,'budgeted_observed_neighbor_count':1,'LLS_fixed_header_retained':True,'known_bad_LLS_not_pure_resource_limit':True},budget=1)
 goodlsa=b.lsa(2,b.ip('255.255.255.0')+b.ip('192.0.2.10'),identifier='192.0.2.254');badlsa=bytearray(goodlsa);badlsa[-1]^=1;assert not codec.fletcher(badlsa)['valid']
 add('Auth2-bad-LSA-complete-digest',b.packet(4,b.u32(1)+bytes(badlsa),2,auth,opaque16),'independent-digest-after-hard-body-error',{'LSA_integrity_identity_required':True,'packet_checksum_scope':'omitted','complete_digest_retained':True,'digest_bytes':16,'Authentication_Verified':False,'bad_LSA_does_not_require_scanning_for_sibling':True})
 assert len(cases)==17;pcap=bytearray(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,1));rows=[];scenarios=[];all_leaves=0
 write(a.out/'plan-before.json',{'scope':__doc__,'inputs':binding,'before_any_new_candidate':True,'fixed_categories':['short4/16..23 with n<24','mask-only Network-LSA','Auth2 packet/LLS nonzero checksum','Auth2 full LLS noCA','CA length differs header','unknown auth3 Lbit opaque tail','Hello neighbor budget with independent badLLS','bad Auth2 LSA plus complete digest'],'source_grid03_unchanged':True,'future_native_whole_objects_built':False,'real_traffic_claim':False})
 for number,case in enumerate(cases,1):
  w=case.pop('wire');original=case.pop('original_PDU');full=b.frame(original,number);captured=full[:34+len(w)];assert captured[34:]==w;pcap+=struct.pack('<IIII',1700000000+number,0,len(captured),len(full))+captured;rawp=a.out/('PDU-%02d.bin'%number);rawp.write_bytes(w);fp=a.out/('captured-frame-%02d.bin'%number);fp.write_bytes(captured)
  if case['kind']=='known-bad-length-plus-capture-missing':
   src,fields=known_prefix(codec,w,34);decoded={'fields':fields,'source_leaves':src.leaves,'source_observed_bytes':len(w),'source_OSPF_packet_length':0,'no_complete_packet_checksum_verdict':True}
  elif case['kind']=='unknown-auth-no-trailer-position':decoded=unknown_auth_source(codec,w,34)
  else:decoded=codec.decode(w,34)
  tree=decoded['fields'];derived={'all_complete_scalar_or_opaque_leaves':len(decoded['source_leaves'])}
  if 'LLS'in tree:
   lls=tree['LLS'];ca=[t for t in lls['TLVs']if t['Type']==2];derived['complete_LLS_CA_count_from_full_original_wire']=len(ca);derived['LLS_declared_PDU_span']=lls['Observed Extent'];derived['LLS_header_Source_ranges']=lls['Wire Field Ranges']
   if ca:derived['LLS_CA_authenticator_bytes']=[t['Authentication Data']['bytes']for t in ca]
  if case['kind']=='budget-plus-independent-LLS-integrity':assert tree['Packet Checksum Verification']['verified']and not tree['LLS']['Checksum Verification']['verified']
  if case['kind']=='independent-digest-after-hard-body-error':assert not tree['LSAs'][0]['Checksum Verification']['valid'];assert tree['Authentication']['Authentication Digest']['bytes']==16;derived['digest_PDU_span']=tree['Authentication']['Wire Field Ranges']['Authentication Digest'];derived['digest_SHA256']=tree['Authentication']['Authentication Digest']['SHA256']
  if case['kind']=='nonzero-omitted-packet-field':assert tree['Header']['Checksum']==1 and not tree['Packet Checksum Verification']['field_zero']
  if case['kind']=='nonzero-omitted-LLS-field':assert tree['LLS']['Checksum']==1 and not tree['LLS']['Checksum Verification']['field_zero']
  row={**case,'frame':number,'PDU_file':str(rawp),'PDU_SHA256':sha(rawp),'PDU_captured_bytes':len(w),'captured_frame':str(fp),'captured_frame_SHA256':sha(fp),'captured_frame_length':len(captured),'original_frame_length':len(full),'source_capture_truncated':len(captured)<len(full),'declared_IPv4_extent_complete_in_capture':len(captured)==len(full),'PDU_physical_start':34,'source':decoded,'derived_source_witnesses':derived,'origin':'tiny authored deterministic control; not real captured traffic','no_native_output_read':True};rows.append(row);all_leaves+=len(decoded['source_leaves']);scenarios.append({'name':case['name'],'frame':number,'PDU_file':str(rawp),'PDU_SHA256':sha(rawp),'budget':case['budget'],'requirements':case['requirements']})
  for variant in case['variants']:scenarios.append({'name':variant['name'],'frame':number,'PDU_file':str(rawp),'PDU_SHA256':sha(rawp),'budget':variant['budget'],'requirements':variant['requirements']})
 cap=a.out/'authored-review-regressions.pcap';cap.write_bytes(pcap);write(a.out/'source-witnesses.json',rows);write(a.out/'direct-codec-scenarios.json',scenarios)
 refs={'packet_and_auth':'https://www.rfc-editor.org/rfc/rfc2328.html#appendix-A.3.1','packet_crypt_checksum_zero':'https://www.rfc-editor.org/rfc/rfc2328.html#appendix-D.4.3','Network_router_minimum':'https://www.rfc-editor.org/rfc/rfc2328.html#appendix-A.4.3','Fletcher':'https://www.rfc-editor.org/rfc/rfc2328.html#section-12.1.7','LLS_extent_and_omitted_checksum':'https://www.rfc-editor.org/rfc/rfc5613.html#section-2.2','LLS_CA_same_key_algorithm_sequence':'https://www.rfc-editor.org/rfc/rfc5613.html#section-2.5','instance8_auth8':'https://datatracker.ietf.org/doc/html/rfc6549#section-2'}
 assert binding=={p:sha(p)for p in binding};report={'scope':__doc__,'plan_before_SHA256':sha(a.out/'plan-before.json'),'source_frames':len(rows),'direct_codec_observation_scenarios':len(scenarios),'complete_scalar_or_opaque_source_leaves':all_leaves,'capture_truncated_bad_length_frames':[r['frame']for r in rows if r['source_capture_truncated']],'fully_captured_controls':sum(not r['source_capture_truncated']for r in rows),'capture':str(cap),'capture_SHA256':sha(cap),'capture_bytes':len(pcap),'source_witnesses_SHA256':sha(a.out/'source-witnesses.json'),'direct_scenarios_SHA256':sha(a.out/'direct-codec-scenarios.json'),'primary_sources':refs,'inputs':binding,'inputs_unchanged':True,'unknown_auth_trailer_not_decoded_by_legacy_fixed_scheme_source_helper':True,'old_grid03_not_modified':True,'no_Go_native_or_candidate_execution':True,'no_future_native_whole_objects_or_typed_alias_expectations_built':True,'no_auth_MAC_peer_neighbor_LSDB_or_route_verified':True};write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'source_frames':len(rows),'direct_scenarios':len(scenarios),'source_leaves':all_leaves,'capture_bytes':len(pcap),'source_witnesses_SHA256':report['source_witnesses_SHA256'],'direct_scenarios_SHA256':report['direct_scenarios_SHA256']}))
if __name__=='__main__':main()
