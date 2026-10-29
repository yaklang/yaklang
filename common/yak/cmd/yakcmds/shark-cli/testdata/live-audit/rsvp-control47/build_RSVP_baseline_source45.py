"""Independent RSVP baseline source grid and existing-PDML comparison.

Reads only the original official capture, its existing extracted frame/PDU files,
local primary RFC texts and already-closed Wireshark PDML. No Yak source, Go,
binary, native/CLI output or reference-derived repair is used. This is a
complete-source inventory for these nine frames, not a production RSVP parser
or proof of malformed/unknown-type handling. Intserv values deliberately remain
owned opaque bytes. Only this Python source is eligible for source-only archive;
all facts/expected/grid/PDML/captures stay external.
"""
import argparse
import base64
import collections
import copy
import hashlib
import ipaddress
import json
from pathlib import Path
import struct
import xml.etree.ElementTree as ET

MESSAGE_NAMES = {1:'Path', 2:'Resv', 3:'PathErr', 4:'ResvErr',
                 5:'PathTear', 6:'ResvTear', 7:'ResvConf'}
CLASS_NAMES = {1:'SESSION', 3:'RSVP_HOP', 5:'TIME_VALUES', 6:'ERROR_SPEC',
               8:'STYLE', 9:'FLOWSPEC', 10:'FILTER_SPEC', 11:'SENDER_TEMPLATE',
               12:'SENDER_TSPEC', 13:'ADSPEC', 15:'RESV_CONFIRM'}
OBJECT_WRAPPERS = {1:'rsvp.session', 3:'rsvp.hop', 5:'rsvp.time',
                   6:'rsvp.error', 8:'rsvp.style', 9:'rsvp.flowspec',
                   10:'rsvp.filter', 11:'rsvp.sender', 12:'rsvp.tspec',
                   13:'rsvp.adspec', 15:'rsvp.confirm'}
# name, body-relative byte offset, width, kind, existing Wireshark field name.
LAYOUTS = {
 1:[('Destination Address',0,4,'address','rsvp.session.ip'),
    ('Protocol ID',4,1,'uint','rsvp.session.proto'),
    ('Flags',5,1,'uint','rsvp.session.flags'),
    ('Destination Port',6,2,'uint','rsvp.session.port')],
 3:[('Hop Address',0,4,'address','rsvp.hop.neighbor_address_ipv4'),
    ('Logical Interface Handle',4,4,'uint','rsvp.hop.logical_interface')],
 5:[('Refresh Period Milliseconds',0,4,'uint','rsvp.refresh_interval')],
 6:[('Error Node Address',0,4,'address','rsvp.error.error_node_ipv4'),
    ('Flags',4,1,'uint','rsvp.error_flags'),
    ('Error Code',5,1,'uint','rsvp.error.error_code'),
    ('Error Value',6,2,'uint','rsvp.error_value')],
 8:[('Flags',0,1,'uint','rsvp.style.flags'),
    ('Option Vector',1,3,'uint','rsvp.style.style')],
 10:[('Source Address',0,4,'address','rsvp.sender.ip'),
     ('Unused',4,2,'uint',None),('Source Port',6,2,'uint','rsvp.sender.port')],
 11:[('Source Address',0,4,'address','rsvp.sender.ip'),
     ('Unused',4,2,'uint',None),('Source Port',6,2,'uint','rsvp.sender.port')],
 15:[('Receiver Address',0,4,'address','rsvp.confirm.receiver_address_ipv4')]
}
RFC_BASE = 'https://www.rfc-editor.org/rfc/'
PRIMARY_SECTIONS = {
 'header':{'rfc':2205,'section':'3.1.1','LF_lines':[1762,1825]},
 'object_header':{'rfc':2205,'section':'3.1.2','LF_lines':[1827,1917]},
 'unused_fields':{'rfc':2205,'section':'Appendix A','LF_lines':[4267,4270]},
 'SESSION':{'rfc':2205,'section':'A.1','LF_lines':[4272,4338]},
 'RSVP_HOP':{'rfc':2205,'section':'A.2','LF_lines':[4375,4427]},
 'TIME_VALUES':{'rfc':2205,'section':'A.4','LF_lines':[4437,4453]},
 'ERROR_SPEC':{'rfc':2205,'section':'A.5','LF_lines':[4487,4557]},
 'STYLE':{'rfc':2205,'section':'A.7','LF_lines':[4655,4718]},
 'FLOWSPEC':{'rfc':2205,'section':'A.8','LF_lines':[4767,4784]},
 'FILTER_SPEC':{'rfc':2205,'section':'A.9','LF_lines':[4823,4898]},
 'SENDER_TEMPLATE':{'rfc':2205,'section':'A.10','LF_lines':[4935,4948]},
 'SENDER_TSPEC':{'rfc':2205,'section':'A.11','LF_lines':[4950,4957]},
 'ADSPEC':{'rfc':2205,'section':'A.12','LF_lines':[4959,4966]},
 'RESV_CONFIRM':{'rfc':2205,'section':'A.14','LF_lines':[5047,5068]},
 'confirmation_error_fields':{'rfc':2205,'section':'3.1.9','LF_lines':[2603,2609]},
 'unknown_objects_not_in_fixture':{'rfc':2205,'section':'3.10','LF_lines':[3383,3458]},
 'Bundle_not_in_fixture':{'rfc':2961,'section':'3.1/3.2','LF_lines':[262,327]},
 'Integrity_not_in_fixture':{'rfc':2747,'section':'1/4.1','LF_lines':[95,97]},
}


def digest(data):
 return hashlib.sha256(data).hexdigest()


def sha(path):
 return digest(Path(path).read_bytes())


def read(path):
 return json.loads(Path(path).read_text(encoding='utf-8-sig'))


def put(path, obj):
 Path(path).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')


def unsigned(raw, start, width):
 assert width > 0 and 0 <= start <= start+width <= len(raw)
 return int.from_bytes(raw[start:start+width],'big')


def checksum(raw):
 data=raw+b'\0'*(len(raw)%2)
 value=sum(int.from_bytes(data[n:n+2],'big') for n in range(0,len(data),2))
 while value>>16:
  value=(value&65535)+(value>>16)
 return (~value)&65535


def pcap_frames(path):
 raw=Path(path).read_bytes()
 variants={b'\xd4\xc3\xb2\xa1':('<',1000),b'\xa1\xb2\xc3\xd4':('>',1000),
           b'\x4d\x3c\xb2\xa1':('<',1),b'\xa1\xb2\x3c\x4d':('>',1)}
 endian,scale=variants[raw[:4]]
 assert len(raw)>=24
 major,minor,zone,sigfigs,snaplen,network=struct.unpack_from(endian+'HHiIII',raw,4)
 assert (major,minor,network)==(2,4,1)
 pos=24
 rows=[]
 while pos<len(raw):
  assert pos+16<=len(raw)
  sec,fraction,caplen,origlen=struct.unpack_from(endian+'IIII',raw,pos)
  assert caplen<=snaplen and pos+16+caplen<=len(raw)
  rows.append({'frame':len(rows)+1,'time_ns':sec*10**9+fraction*scale,
               'captured_length':caplen,'original_length':origlen,
               'bytes':raw[pos+16:pos+16+caplen]})
  pos+=16+caplen
 assert pos==len(raw)
 return rows


def decode_wire(raw):
 """Return neutral source fields/leaves; the names are not a Yak API contract."""
 assert len(raw)>=8
 leaves=[]
 header={}
 def literal(target,path,name,start,width,kind='uint',alias=None):
  end=start+width
  assert 0<=start<=end<=len(raw)
  wire=raw[start:end]
  if kind=='uint':value=int.from_bytes(wire,'big')
  elif kind=='address':value=str(ipaddress.ip_address(wire))
  elif kind=='bytes':value=base64.b64encode(wire).decode('ascii')
  else:raise AssertionError(kind)
  target[name]=value
  leaves.append({'path':path+'.'+name,'kind':kind,'value':value,
                 'literal':True,'range':[start,end],
                 'raw_SHA256':digest(wire),'reference_alias':alias})
  return value
 literal(header,'Header','Version and Flags',0,1)
 literal(header,'Header','Message Type',1,1,alias='rsvp.msg')
 literal(header,'Header','Checksum',2,2,alias='rsvp.message_checksum')
 literal(header,'Header','Send TTL',4,1,alias='rsvp.sending_ttl')
 literal(header,'Header','Reserved',5,1)
 literal(header,'Header','Message Length',6,2,alias='rsvp.message_length')
 for name,mask,shift,alias in [('Version',240,4,'rsvp.version'),('Flags',15,0,'rsvp.flags')]:
  value=(raw[0]&mask)>>shift
  header[name]=value
  leaves.append({'path':'Header.'+name,'kind':'uint','value':value,
                 'literal':False,'source_range':[0,1], 'mask':mask,'shift':shift,
                 'reference_alias':alias})
 assert header['Version']==1
 declared=header['Message Length']
 assert declared>=8 and declared==len(raw)
 fields={'Header':header,'Objects':[],
         'Message Name':MESSAGE_NAMES.get(header['Message Type'],'Unselected'),
         'Checksum Omitted':header['Checksum']==0,
         'Checksum Verified':header['Checksum']!=0 and checksum(raw)==0,
         'Authentication Verified':False,
         'Observation Scope':'wire observations only; no authenticated neighbor, reservation, installed flow or replay verdict'}
 assert fields['Checksum Verified']
 offset=8
 while offset<declared:
  assert offset+4<=declared
  length=unsigned(raw,offset,2)
  cls=unsigned(raw,offset+2,1)
  ctype=unsigned(raw,offset+3,1)
  assert length>=4 and length%4==0 and offset+length<=declared
  index=len(fields['Objects'])
  path=f'Objects[{index}]'
  obj={'Object Name':CLASS_NAMES.get(cls,'Unselected Object'),
       'Object Range':[offset,offset+length], 'Header':{},'Body':{}}
  literal(obj['Header'],path+'.Header','Length',offset,2,alias='rsvp.length')
  literal(obj['Header'],path+'.Header','Class',offset+2,1,alias='rsvp.object')
  literal(obj['Header'],path+'.Header','C-Type',offset+3,1,alias='rsvp.ctype')
  start=offset+4
  body=obj['Body']
  if ctype==1 and cls in LAYOUTS:
   spec=LAYOUTS[cls]
   assert max(a+w for _,a,w,_,_ in spec)==length-4
   for name,rel,width,kind,alias in spec:
    literal(body,path+'.Body',name,start+rel,width,kind,alias)
   obj['Source Layout Selected']=True
   if cls==8:
    # Only a display name for a wire option vector, no reservation state.
    obj['Style Name']={0x11:'Wildcard Filter',0x0a:'Fixed Filter',0x12:'Shared Explicit'}.get(body['Option Vector']&31,'Unselected')
   if cls==5:obj['Units']='milliseconds'
  else:
   literal(body,path+'.Body','Opaque Body',start,length-4,'bytes')
   obj['Source Layout Selected']=False
   obj['Observation Scope']='Intserv object body not semantically decoded in this baseline source grid' if cls in (9,12,13) else 'unselected class or C-Type; opaque bytes only'
  fields['Objects'].append(obj)
  offset+=length
 assert offset==declared
 return fields,leaves


def find_reference(proto,alias,pos,width):
 matches=[x for x in proto.iter('field') if x.get('name')==alias and
          x.get('pos')==str(pos) and x.get('size')==str(width)]
 assert len(matches)==1, f'{alias} span[{pos},{pos+width}) matches={len(matches)}'
 return matches[0]


def compare_reference(frame, proto):
 matches=[]
 missing=[]
 wrappers=[]
 base=frame['PDU_physical_range'][0]
 for leaf in frame['leaves']:
  alias=leaf['reference_alias']
  if alias is None:
   missing.append({'path':leaf['path'],'range':leaf.get('range'),
                   'reason':'no selected existing PDML primitive alias; not counted as reference value coverage'})
   continue
  start,end=leaf['range'] if leaf['literal'] else leaf['source_range']
  ref=find_reference(proto,alias,base+start,end-start)
  if leaf['kind']=='uint':assert int(ref.get('show'),0)==leaf['value'], leaf['path']
  elif leaf['kind']=='address':assert str(ipaddress.ip_address(ref.get('show')))==leaf['value']
  else:raise AssertionError('No opaque body reference comparison was selected')
  if leaf['literal']:
   expected=frame['_raw'][start:end].hex()
   assert ref.get('value').lower()==expected, leaf['path']+' full raw value'
  matches.append({'source_path':leaf['path'],'alias':alias,
                  'range':[start,end],'physical_range':[base+start,base+end],
                  'literal':leaf['literal'],'value':leaf['value']})
 for index,obj in enumerate(frame['fields']['Objects']):
  cls=obj['Header']['Class']
  start,end=obj['Object Range']
  alias=OBJECT_WRAPPERS[cls]
  ref=find_reference(proto,alias,base+start,end-start)
  wrappers.append({'object':index,'alias':alias,'range':[start,end],
                    'physical_range':[base+start,base+end],
                    'scope':'envelope/owner only; no opaque Intserv semantic proof'})
 # The existing PDML explicitly annotates checksums as correct. This is
 # reported separately from the independently proved checksum arithmetic.
 ref=find_reference(proto,'rsvp.message_checksum',base+2,2)
 assert '[correct]' in ref.get('showname','')
 return {'frame':frame['frame'],'value_and_span_matches':matches,
         'unrendered_source_leaves':missing,'object_envelope_owners':wrappers,
         'reference_checksum_annotation':'correct',
         'reference_semantics_not_claimed_for_opaque_bodies':True}


def verify_original_census(old, frame, raw):
 source=old['source']
 h=frame['fields']['Header']
 assert source['raw_SHA256']==digest(raw)
 for oldkey,newkey in [('version','Version'),('flags','Flags'),('type','Message Type'),
                       ('checksum','Checksum'),('send_TTL','Send TTL'),
                       ('reserved','Reserved'),('length','Message Length')]:
  assert source[oldkey]==h[newkey]
 assert source['message_span']==[0,len(raw)] and source['unselected_tail_bytes']==0
 assert source['checksum_arithmetic']=='valid' and source['object_envelopes_complete'] is True
 assert len(source['objects'])==len(frame['fields']['Objects'])
 for oldobj,obj in zip(source['objects'],frame['fields']['Objects']):
  start,end=obj['Object Range']
  assert oldobj['length']==obj['Header']['Length']
  assert oldobj['class']==obj['Header']['Class'] and oldobj['C-Type']==obj['Header']['C-Type']
  assert oldobj['header_span']==[start,start+4] and oldobj['value_span']==[start+4,end]
  assert oldobj['value_SHA256']==digest(raw[start+4:end])


def run(phase,out):
 assert not out.exists()
 out.mkdir()
 original=phase/'RSVP-official-source45-01'
 primary=phase/'RSVP-primary-download45-01'
 cap=primary/'rsvp-PATH-RESV.pcap'
 originals=read(original/'source-facts.json')
 pcap=pcap_frames(cap)
 pdml=ET.parse(original/'reference.pdml').getroot().findall('packet')
 assert len(originals)==len(pcap)==len(pdml)==9
 inputs=[Path(__file__),original/'source-facts.json',original/'summary.json',
         original/'reference.pdml',original/'reference-fields.json',cap,
         primary/'download-manifest.json']
 inputs += [primary/f'rfc{n}.txt' for n in (2205,2961,2747)]
 for old in originals:
  inputs += [Path(old['physical_file']),Path(old['PDU_file'])]
 before={str(p):sha(p) for p in inputs}
 expected_constants={str(original/'source-facts.json'):'99436adc7179b876259cb2b179e70e0fe1c157b3feb343a74e9774864b3ee1b2',
                     str(original/'reference.pdml'):'f6c1226dd1b832651a30470e2ab8d8575b3eea44cd8f969d891c5879b2831d20',
                     str(cap):'aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361'}
 assert all(before[p]==d for p,d in expected_constants.items())
 frames=[]
 references=[]
 stats=collections.Counter()
 class_counts=collections.Counter()
 type_counts=collections.Counter()
 unique=set()
 for old,record,packet in zip(originals,pcap,pdml):
  wire=record['bytes']
  assert old['frame']==record['frame']
  assert old['domain']=={'section':0,'interface':0,'link':1}
  assert old['time_ns']==record['time_ns']
  assert old['captured_length']==record['captured_length']==old['original_length']==record['original_length']==len(wire)
  assert wire==Path(old['physical_file']).read_bytes() and digest(wire)==old['physical_SHA256']
  assert wire[12:14]==b'\x08\x00' and wire[14]>>4==4
  ihl=(wire[14]&15)*4
  iplen=unsigned(wire,16,2)
  ipstart=14
  end=ipstart+iplen
  start=ipstart+ihl
  assert 20<=ihl<=60 and end==len(wire) and checksum(wire[ipstart:start])==0
  assert wire[23]==46 and (unsigned(wire,20,2)&0x3fff)==0
  raw=wire[start:end]
  assert raw==Path(old['PDU_file']).read_bytes() and digest(raw)==old['PDU_SHA256']
  assert old['PDU_physical_range']==[start,end]
  fields,leaves=decode_wire(raw)
  frame={'frame':old['frame'],'time_ns':record['time_ns'],'domain':old['domain'],
         'captured_length':len(wire),'original_length':len(wire),
         'physical_path':old['physical_file'],'physical_SHA256':digest(wire),
         'PDU_path':old['PDU_file'],'PDU_SHA256':digest(raw),
         'PDU_physical_range':[start,end],
         'carrier':{'transport':'ip','IP_protocol':46,'IP_source':str(ipaddress.ip_address(wire[26:30])),
                    'IP_destination':str(ipaddress.ip_address(wire[30:34])),
                    'IP_header_range':[ipstart,start],'IP_declared_end':end,
                    'IP_available_end':len(wire),'IP_checksum_verified':True,
                    'fragment_offset':0,'more_fragments':False},
         'fields':fields,'leaves':leaves,'_raw':raw}
  verify_original_census(old,frame,raw)
  proto=next(p for p in packet.findall('proto') if p.get('name')=='rsvp')
  references.append(compare_reference(frame,proto))
  unique.add(digest(raw))
  stats['physical_frames']+=1
  stats['objects']+=len(fields['Objects'])
  stats['literal_wire_leaves']+=sum(x['literal'] for x in leaves)
  stats['derived_bit_fields']+=sum(not x['literal'] for x in leaves)
  stats['source_checksum_verified']+=fields['Checksum Verified']
  for obj in fields['Objects']:
   class_counts[str(obj['Header']['Class'])]+=1
   stats['selected_baseline_objects' if obj['Source Layout Selected'] else 'opaque_Intserv_objects']+=1
  type_counts[str(fields['Header']['Message Type'])]+=1
  frames.append(frame)
 # Checker-only fault injections. No packet is changed or authored, no new
 # protocol scenario/capture is introduced, and no reference file is modified.
 testframe=frames[0]
 testproto=next(p for p in pdml[0].findall('proto') if p.get('name')=='rsvp')
 negative=[]
 for label,mutator in [('wrong scalar',lambda f:f['leaves'][1].update(value=255)),
                       ('wrong span',lambda f:f['leaves'][1].update(range=[2,3]))]:
  mutated=copy.deepcopy(testframe)
  mutator(mutated)
  try:compare_reference(mutated,testproto)
  except AssertionError:negative.append(label)
  else:raise AssertionError('checker accepted '+label)
 changed=copy.deepcopy(testproto)
 target=find_reference(changed,'rsvp.message_length',testframe['PDU_physical_range'][0]+6,2)
 changed.append(copy.deepcopy(target))
 try:compare_reference(testframe,changed)
 except AssertionError:negative.append('duplicate same-owner reference')
 else:raise AssertionError('checker accepted duplicate reference')
 for frame in frames:del frame['_raw']
 put(out/'source-grid.json',frames)
 put(out/'reference-comparison.json',references)
 contract={'source_schema':'neutral baseline field names; not a native/Go API expected-object schema',
           'range_domain':'half-open absolute logical RSVP-PDU offsets; physical offset is independently bound per record',
           'literal_ranges':'only complete literal fields; Version/Flags have mask+shift source ranges and are separately counted as derived',
           'unused_fields':'preserve wire values; Appendix A unused fields ignored on receipt, not an automatic format error',
           'checksum':'whole declared RSVP message, no pseudo-header; zero means omitted, never authenticated',
           'SESSION':'advertised flow selector, not the observed packet transport and not a confirmed session/flow',
           'RSVP_HOP':'unverified advertised hop and LIH; no authenticated neighbor/actual interface ownership inference',
           'TIME_VALUES':'Refresh Period R is milliseconds, no inferred liveness/retransmission state',
           'STYLE':'one Flags byte plus 24-bit Option Vector; low5 WF=17/FF=10/SE=18, unused bits not authentication',
           'ERROR_SPEC':'raw node/code/value; code/value0 in ResvConf confirms wire intent, not proof a reservation exists',
           'opaque_Intserv':'16 complete C-Type2 FLOWSPEC/SENDER_TSPEC/ADSPEC bodies owned raw bytes, not decoded rates/services',
           'not_covered':['malformed/short messages','unknown classes/C-Types','IPv6 object C-Types',
                          'RFC2961 Bundle/reduction state','RFC2747 Integrity/MAC/key/replay',
                          'Intserv parameter semantics','confirmed reservation/flow/link/neighbor state'],
           'primary_sections':PRIMARY_SECTIONS,
           'primary_urls':{str(n):RFC_BASE+f'rfc{n}.txt' for n in (2205,2961,2747)}}
 put(out/'source-contract.json',contract)
 assert before=={p:sha(p) for p in before}
 counts=dict(stats)
 counts.update({'distinct_PDU_SHA256':len(unique),
                'reference_literal_value_and_span_matches':sum(x['literal'] for r in references for x in r['value_and_span_matches']),
                'reference_derived_bit_value_and_span_matches':sum(not x['literal'] for r in references for x in r['value_and_span_matches']),
                'unrendered_source_leaves_not_reference_passes':sum(len(r['unrendered_source_leaves']) for r in references),
                'reference_object_envelope_owners':sum(len(r['object_envelope_owners']) for r in references),
                'reference_correct_checksum_annotations':len(references),
                'checker_only_negative_mutations_rejected':len(negative)})
 outputs={str(p):sha(p) for p in out.iterdir() if p.is_file()}
 manifest={'inputs':before,'outputs':outputs,'all_original_inputs_unchanged':True,
           'controller':str(Path(__file__)),'controller_SHA256':sha(__file__),
           'no_Yak_repo_source_read':True,'no_candidate_output_read':True,
           'no_Go_binary_native_CLI_or_TShark_process_started':True,
           'existing_reference_only':True,'reference_binary_SHA256':'662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b',
           'negative_mutations':negative,'source_census_bridge':'Original common/object census values, envelopes and value SHA256 are literally unchanged',
           'scope_limits':contract['not_covered']}
 put(out/'manifest.json',manifest)
 summary={'counts':counts,'message_types':dict(type_counts),'object_classes':dict(class_counts),
          'manifest_SHA256':sha(out/'manifest.json'),'source_grid_SHA256':sha(out/'source-grid.json'),
          'reference_comparison_SHA256':sha(out/'reference-comparison.json'),
          'contract_SHA256':sha(out/'source-contract.json'),
          'reference_differences':0,'unrendered_does_not_mean_wrong_or_unsupported':True,
          'original_source_census_unmodified':True,'future_native_correctness_not_yet_claimed':True}
 put(out/'summary.json',summary)
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':counts},ensure_ascii=False))


def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--phase',type=Path,required=True)
 parser.add_argument('--out',type=Path,required=True)
 args=parser.parse_args()
 run(args.phase,args.out)

if __name__=='__main__':main()