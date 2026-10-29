"""Compare independently decoded source fields and byte ranges to current PDML.

Verifies the listed clear headers, OSPF envelopes/LSA headers and Intra-Area
prefixes. Other LSA bodies, IKE body semantics, ESP/AH authentication, OSPF
adjacency and association/replay state are deliberately outside this proof.
"""
from pathlib import Path
import collections,hashlib,importlib.util,ipaddress,json,sys
root=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-OSPFv3-source-readonly30-01')
out=root/'pdml-source-verification01';out.mkdir(exist_ok=False)
spec=importlib.util.spec_from_file_location('oracle',root/'audit_source02.py');o=importlib.util.module_from_spec(spec);spec.loader.exec_module(o)
base=json.loads((root/'source02/source-facts.json').read_text())
v3={r['frame']:r for r in json.loads((root/'source03/all-417-OSPFv3-source-facts.json').read_text())}
natt={r['frame']:r for r in json.loads((root/'source03/all-34-NATT-source-facts.json').read_text())}
rows={r['frame']:r for r in json.loads((root/'wireshark03/selected-fields.json').read_text())}
reader=Path(base['reader']);sys.path.insert(0,str(reader.parent))
spec=importlib.util.spec_from_file_location('reader',reader);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
source_paths=[Path(__file__),root/'audit_source02.py',root/'source02/source-facts.json',root/'source03/summary.json',
 root/'source03/all-417-OSPFv3-source-facts.json',root/'source03/all-34-NATT-source-facts.json',root/'wireshark03/selected-fields.json',
 root/'wireshark03/receipt.json',Path(base['capture']),reader]
before={str(p):o.sha(p) for p in source_paths}
counts=collections.Counter();results=[]
for number,record in enumerate(m.capture_records(Path(base['capture'])),1):
 if number not in v3 and number not in natt:continue
 source=(v3.get(number) or natt[number])['source'];raw=record['wire'];ws=rows[number];checks=[]
 def check(name,at,size,expected=None):
  fs=[f for f in ws['fields'] if f['name']==name and int(f['pos'])==at and int(f['size'])==size]
  assert fs,(number,name,at,size,'field absent')
  expected_hex=raw[at:at+size].hex()
  for f in fs:
   value=f.get('unmaskedvalue',f.get('value'))
   assert value==expected_hex,(number,name,'raw/range mismatch',value,expected_hex)
   if expected is not None:
    if isinstance(expected,int):assert int(value,16)==expected,(number,name,'integer mismatch')
    else:assert ipaddress.ip_address(f['show'])==ipaddress.ip_address(expected),(number,name,'address mismatch')
  counts['field_values_and_ranges']+=1;checks.append(dict(name=name,range=[at,at+size],value_hex=expected_hex))
 def marker(name,at,size):
  fs=[f for f in ws['fields'] if f['name']==name and int(f['pos'])==at and int(f['size'])==size]
  assert fs,(number,name,'marker missing');counts['marker_ranges']+=1
  checks.append(dict(name=name,range=[at,at+size]))
 if number in v3:
  p=source['OSPF'];at=p['declared_range'][0]
  protos=[p for p in ws['protocols'] if p['name']=='ospf']
  assert len(protos)==1 and int(protos[0]['pos'])==at and int(protos[0]['size'])==p['declared_length']
  for name,off,size,value in [('ospf.version',0,1,3),('ospf.msg',1,1,p['type']),('ospf.packet_length',2,2,p['declared_length']),
   ('ospf.srcrouter',4,4,p['router_ID']),('ospf.area_id',8,4,p['area_ID']),('ospf.checksum',12,2,p['checksum']),
   ('ospf.v3.instance_id',14,1,p['instance_ID']),('ospf.reserved',15,1,p['reserved'])]:check(name,at+off,size,value)
  cs=next(f for f in ws['fields'] if f['name']=='ospf.checksum')
  assert cs['showname'].endswith('[correct]') and p['checksum']==p['computed_checksum'] and p['checksum_residual']==0
  counts['OSPFv3_checksum_independent_and_Wireshark_valid']+=1
  if p['type']==1:
   h=p['Hello']
   for name,off,size,value in [('ospf.hello.interface_id',16,4,h['interface_ID']),('ospf.hello.router_priority',20,1,h['priority']),
    ('ospf.v3.options',21,3,h['options']),('ospf.hello.hello_interval',24,2,h['hello_interval']),('ospf.hello.router_dead_interval',26,2,h['dead_interval']),
    ('ospf.hello.designated_router',28,4,h['designated_router']),('ospf.hello.backup_designated_router',32,4,h['backup_designated_router'])]:check(name,at+off,size,value)
   for k,n in enumerate(h['neighbors']):check('ospf.hello.active_neighbor',at+36+4*k,4,n)
  elif p['type']==2:
   d=p['DBD']
   for name,off,size,value in [('ospf.reserved',16,1,d['reserved_1']),('ospf.v3.options',17,3,d['options']),
    ('ospf.db.interface_mtu',20,2,d['interface_MTU']),('ospf.reserved',22,1,d['reserved_2']),('ospf.dbd',23,1,d['flags']),('ospf.db.dd_sequence',24,4,d['DD_sequence'])]:check(name,at+off,size,value)
  elif p['type']==3:
   for r in p['requests']:
    n=r['source_range'][0]
    for name,off,size,value in [('ospf.reserved',0,2,r['reserved']),('ospf.v3.lsa',2,2,r['LS_type']),
     ('ospf.link_state_id',4,4,r['link_state_ID']),('ospf.advrouter',8,4,r['advertising_router'])]:check(name,n+off,size,value)
  elif p['type']==4:check('ospf.ls.number_of_lsas',at+16,4,p['LSA_count'])
  for l in p.get('LSAs',p.get('LSA_headers',[])):
   n=l['header_range'][0]
   for name,off,size,value in [('ospf.lsa.age',0,2,l['age']),('ospf.v3.lsa.fc',2,2,l['LS_type']),('ospf.link_state_id',4,4,l['link_state_ID']),
    ('ospf.advrouter',8,4,l['advertising_router']),('ospf.lsa.seqnum',12,4,l['sequence']),('ospf.lsa.chksum',16,2,l['checksum']),('ospf.lsa.length',18,2,l['length'])]:check(name,n+off,size,value)
   if 'fletcher' in l:assert l['fletcher']['valid'];counts['full_LSA_Fletcher_valid']+=1
  for l in v3[number]['IntraAreaPrefixes']:
   n=l['LSA_range'][0]
   for name,off,size,value in [('ospf.v3.lsa.num_prefixes',20,2,l['count']),('ospf.v3.lsa.referenced_ls_type',22,2,l['referenced_LS_type']),
    ('ospf.v3.lsa.referenced_link_state_id',24,4,l['referenced_link_state_ID']),('ospf.v3.lsa.referenced_advertising_router',28,4,l['referenced_advertising_router'])]:check(name,n+off,size,value)
   for pfx in l['prefixes']:
    n=pfx['header_range'][0]
    check('ospf.prefix_length',n,1,pfx['prefix_length']);check('ospf.v3.prefix.options',n+1,1,pfx['prefix_options']);check('ospf.metric',n+2,2,pfx['metric'])
    check('ospf.v3.address_prefix.ipv6',pfx['prefix_range'][0],pfx['encoded_octets'],pfx['prefix'].split('/')[0]);counts['IntraArea_prefix_values_and_ranges']+=1
  for ah in [x for x in source['extensions'] if x['kind']=='AH']:
   n=ah['range'][0]
   for name,off,size,value in [('ah.next_header',0,1,ah['next_header']),('ah.length',1,1,ah['payload_length_field']),('ah.spi',4,4,ah['SPI']),('ah.sequence',8,4,ah['sequence'])]:check(name,n+off,size,value)
   check('ah.icv',ah['ICV_range'][0],ah['ICV_range'][1]-ah['ICV_range'][0]);counts['AH_headers_clear_only']+=1
  counts['OSPFv3_frames']+=1
 if number in natt:
  u=source['UDP'];p=source['NATT'];n=u['header_range'][0]
  assert u['complete'] and u['parent_extent_valid']
  for name,off,size,value in [('udp.srcport',0,2,u['source_port']),('udp.dstport',2,2,u['destination_port']),('udp.length',4,2,u['length'])]:check(name,n+off,size,value)
  if p['kind']=='ESP-clear-header':
   e=p['ESP'];check('esp.spi',e['SPI_range'][0],4,e['SPI']);check('esp.sequence',e['sequence_range'][0],4,e['sequence'])
   esp=next(x for x in ws['protocols'] if x['name']=='esp')
   assert int(esp['pos'])==p['wire_range'][0] and int(esp['size'])==p['wire_length']
   assert u['checksum_residual']==0 and source['network_header_checksum_residual']==0
  elif p['kind']=='non-ESP-marker':
   marker('udpencap.non_esp_marker',p['wire_range'][0],4)
   assert raw[p['wire_range'][0]:p['wire_range'][0]+4]==b'\0'*4
   assert not any(x['name']=='esp' for x in ws['protocols'])
  elif p['kind']=='keepalive':
   marker('udpencap.nat_keepalive',p['wire_range'][0],1)
   assert raw[slice(*p['wire_range'])]==b'\xff'
   assert not any(x['name']=='esp' for x in ws['protocols'])
  counts['NATT_'+p['kind']]+=1
 results.append(dict(frame=number,verified_fields=checks))
after={p:o.sha(p) for p in before};assert before==after
(out/'field-checks.json').write_text(json.dumps(results,indent=2)+'\n',encoding='utf8')
summary=dict(scope=__doc__,source_before=before,source_after=after,sources_byte_equal=True,counts=dict(counts),
 all_assertions_passed=True,field_checks_SHA256=o.sha(out/'field-checks.json'),negative_marker_and_keepalive_not_ESP=True)
(out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(result=str(out/'summary.json'),SHA256=o.sha(out/'summary.json'),counts=dict(counts))))
