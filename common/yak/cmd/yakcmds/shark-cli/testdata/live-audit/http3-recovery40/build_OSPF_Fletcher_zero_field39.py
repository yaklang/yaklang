"""Authored OSPFv3 zero-checksum/zero-residue pair and old807 source audit.

RFC5340 sections4.4.1/A.4.2 preserve/reference RFC2328 section12.1.7.
Fletcher residual zero is necessary but an LSA checksum field zero is forbidden.
Both tiny packets have valid IPv6 OSPF packet checksums; no key/auth/LSDB implied.
"""
import argparse,collections,hashlib,ipaddress,json,shutil,struct
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def sum16(w):
 if len(w)%2:w+=b'\0'
 total=sum(int.from_bytes(w[n:n+2],'big')for n in range(0,len(w),2))
 while total>>16:total=(total&65535)+(total>>16)
 return(~total)&65535

def residues(w):
 c0=c1=0
 for b in w[2:]:c0=(c0+b)%255;c1=(c1+c0)%255
 return[c0,c1]
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--grid',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();binding={str(Path(__file__)):sha(__file__),str(a.grid):sha(a.grid)}
 # Network-LSA28: attached RouterID last two octets are solved modulo255.
 wire=bytearray(struct.pack('!HHIIIHH',1,0x2002,1,0xc0000201,0x80000001,0,28)+bytes.fromhex('00000013c0000000'));assert len(wire)==28
 c0,c1=residues(wire);x=(c0-c1)%255;y=(-c0-x)%255;wire[26:28]=bytes([x,y]);assert residues(wire)==[0,0]and wire[16:18]==b'\0\0'
 positive=bytearray(wire);positive[16:18]=b'\xff\xff';assert residues(positive)==[0,0]
 src=ipaddress.IPv6Address('fe80::1').packed;dst=ipaddress.IPv6Address('ff02::5').packed;cases=[];pcap=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
 for index,(name,lsa,legal)in enumerate([('zero-field-zero-residues',bytes(wire),False),('nonzero-field-zero-residues',bytes(positive),True)],1):
  pdu=bytearray(struct.pack('!BBHIIHBB',3,4,48,0xc0000201,0,0,0,0)+struct.pack('!I',1)+lsa);pseudo=src+dst+struct.pack('!I',len(pdu))+b'\0\0\0\x59';pdu[12:14]=struct.pack('!H',sum16(pseudo+pdu));assert sum16(pseudo+pdu)==0
  ipv6=struct.pack('!IHBB',0x60000000,len(pdu),89,1)+src+dst;raw=bytes.fromhex('33330000000502000000000186dd')+ipv6+pdu;framep=a.out/('authored-frame-%02d.bin'%index);framep.write_bytes(raw);(a.out/(name+'.lsa.bin')).write_bytes(lsa);pcap+=struct.pack('<IIII',1700000000+index,0,len(raw),len(raw))+raw
  cases.append({'frame':index,'name':name,'wire_origin':'authored deterministic public control, not real capture','LSA_hex':lsa.hex(),'LSA_SHA256':digest(lsa),'LSA_length':len(lsa),'LS_Type':0x2002,'LSA_checksum_field':int.from_bytes(lsa[16:18],'big'),'Fletcher_residues':residues(lsa),'normative_LSA_checksum_valid':legal,'packet_checksum_value':int.from_bytes(pdu[12:14],'big'),'packet_checksum_pseudoheader_residue':0,'PDU_range':[54,102],'PDU_SHA256':digest(pdu),'LSA_PDU_range':[20,48],'LSA_checksum_PDU_range':[36,38],'LSA_checksum_physical_range':[90,92],'original_frame':str(framep),'original_frame_SHA256':sha(framep),'source':str(ipaddress.IPv6Address(src)),'destination':str(ipaddress.IPv6Address(dst)),'source_Options':19,'source_Attached_Router':str(ipaddress.IPv4Address(lsa[24:28])),'Authentication_Verified':False,'no_native_expected_event_built':True})
 cap=a.out/'authored-zero-Fletcher-pair.pcap';cap.write_bytes(pcap);assert cases[0]['packet_checksum_value']==cases[1]['packet_checksum_value']
 counts=collections.Counter();by=collections.defaultdict(collections.Counter);zeros=[];checks=[];rows=0
 for line in a.grid.read_text().splitlines():
  row=json.loads(line);rows+=1;identity=row['source_identity'];number=identity['input'];fp=Path(row['original_frame_file']);binding[str(fp)]=sha(fp);assert binding[str(fp)]==row['original_frame_SHA256'];data=fp.read_bytes();start,end=row['source_message_physical_range'];pdu=data[start:end];assert digest(pdu)==row['source_message_SHA256'];full=row['expected_fields'].get('LSAs',[])
  for index,lsa in enumerate(full):
   lstart=lsa['Wire Field Ranges']['LS Age'][0];length=lsa['Length'];w=pdu[lstart:lstart+length];assert len(w)==length;value=int.from_bytes(w[16:18],'big');assert value==lsa['Checksum'];assert residues(w)==[0,0];counts['full_LSAs']+=1;counts['checksum_field_nonzero']+=value!=0;by[number]['full_LSAs']+=1;by[number]['checksum_field_nonzero']+=value!=0
   item={'identity':identity,'LSA_index':index,'LS_Type':lsa['LS Type'],'checksum_field':value,'PDU_span':[lstart,lstart+length],'LSA_SHA256':digest(w),'residues':[0,0]};checks.append(item)
   if value==0:zeros.append(item)
 assert rows==807 and not zeros;assert binding=={p:sha(p)for p in binding};write(a.out/'controls.json',cases);write(a.out/'old807-full-LSA-source-checks.json',checks)
 report={'scope':__doc__,'primary_sources':[{'URL':'https://www.rfc-editor.org/rfc/rfc5340.html#section-4.4.1','meaning':'LS age/sequence/checksum/length unchanged from OSPFv2 and references2328 section12.1.7'},{'URL':'https://www.rfc-editor.org/rfc/rfc5340.html#appendix-A.4.2','meaning':'LS checksum Fletcher excluding LS Age, references2328 section12.1.7'},{'URL':'https://www.rfc-editor.org/rfc/rfc2328.html#section-12.1.7','meaning':'LSA checksum field zero is failure; verification not optional'}],'authored_controls':cases,'capture':str(cap),'capture_SHA256':sha(cap),'old_source807_messages':rows,'old_source_full_LSA_counts':dict(counts),'old_source_per_input':{str(k):dict(v)for k,v in by.items()},'old_source_zero_field_full_LSAs':zeros,'old807_declared_source_scope_unchanged_by_field_nonzero_check':True,'source_full_LSA_checks_SHA256':sha(a.out/'old807-full-LSA-source-checks.json'),'inputs':binding,'inputs_unchanged':True,'no_candidate_or_native_output_read':True,'no_keys_or_actual_capture_embedded_in_source':True,'whole_native_equivalence_claim':False}
 write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'old807_full_LSA_counts':dict(counts),'per_input':report['old_source_per_input'],'controls':[{'name':r['name'],'LSA_hex':r['LSA_hex'],'LSA_SHA256':r['LSA_SHA256'],'packet_checksum_value':r['packet_checksum_value'],'LSA_checksum_valid':r['normative_LSA_checksum_valid']}for r in cases]}))
if __name__=='__main__':main()
