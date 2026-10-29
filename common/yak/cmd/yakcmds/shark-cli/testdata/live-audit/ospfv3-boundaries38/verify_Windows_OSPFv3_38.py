"""Check actual native console cells against independently frozen OSPF wire."""
import hashlib,json,re
from pathlib import Path
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def pane(s):
 data=bytearray();positions=[]
 for y,row in enumerate(s['Rows']):
  if '││'not in row:continue
  begin=row.index('││')+2;m=re.match(r'\s*([0-9a-f]{6})  ',row[begin:])
  if not m:continue
  assert int(m[1],16)==len(data)
  x=begin+m.start(1)
  for i in range(16):
   col=x+8+3*i+i//8;word=row[col:col+2]
   if not re.fullmatch('[0-9a-f]{2}',word):break
   data.append(int(word,16));positions.append((y,col))
 return bytes(data),positions
def verify(folder,case):
 shots={}
 for p in folder.glob('*.json'):
  s=read(p)
  if 'Rows'not in s:continue
  assert len(s['Rows'])==len(s['Attributes'])==s['Height']
  assert all(len(a)==len(r)==s['Width']for r,a in zip(s['Rows'],s['Attributes']))
  shots[p.stem]=s
 required={'00-capture-ready','00-filter-ready','01-protocol-fields','01-deep-protocol-fields','01-adaptive-fields-before','01-adaptive-bytes-click','01-adaptive-fields-click','01-selected-field','02-first-byte-source','02-byte-sources','03-conversation-messages','04-association-evidence','05-narrow','06-restored'}
 assert required<=shots.keys()
 all_text='\n'.join(row for s in shots.values() for row in s['Rows'])
 for label in case['required_human_type_names']:assert label in all_text,label
 first=shots['01-protocol-fields'];text='\n'.join(first['Rows'])
 assert ' EOF 'in first['Rows'][0] and case['query']in first['Rows'][1] and '▏'not in first['Rows'][1]
 assert re.search(r'buffer\s+20000\s*/\s*20000',text) and re.search(r'· 1 visible(?:\s|╮)',text)
 assert any(re.search(r'›48906\s+\S+\s+OSPFv3(?:\s|$)',row)for row in first['Rows'])
 assert '2 OSPFv3 FIELDS'in text and '#48906'in text and 'OSPFv3 · PDU #40097'in text
 assert ' › '.join(case['progressive_stack'])in text
 assert 'AH header · authentication unverified'in text
 raw=Path(case['physical_file']).read_bytes();assert hashlib.sha256(raw).hexdigest()==case['physical_SHA256']
 wire=raw[slice(*case['PDU_range'])];assert hashlib.sha256(wire).hexdigest()==case['PDU_SHA256']
 observed,_=pane(first);assert observed==raw,('Source0',len(observed),len(raw))
 selected=shots['01-selected-field'];text='\n'.join(selected['Rows']);assert case['field']in text
 assert '3 OSPFv3 · captured PDU'in text and '0x90–0x97 · 8 bytes'in text
 observed,pos=pane(selected);assert observed==wire,('Source1',len(observed),len(wire))
 attrs=[selected['Attributes'][y][x]for y,x in pos];a,b=case['field_PDU_range'];style=attrs[a]
 assert len(set(attrs[a:b]))==1 and all(v!=style for i,v in enumerate(attrs)if not a<=i<b),'exact distinct prefix highlight'
 for y,x in pos[a:b]:assert selected['Attributes'][y][x+1]==style
 assert observed[a:b].hex()==case['field_wire_hex']
 assert pane(shots['02-first-byte-source'])[0]==raw and pane(shots['02-byte-sources'])[0]==wire
 # Global history loss does not erase the selected retained observation.
 assert 'Field filter partial: incomplete field evidence'in '\n'.join(first['Rows'][-3:])
 assert 'PDU #0'not in text and 'YAML dissector incomplete'not in text
 restored=read(folder/'restored.json');assert restored==dict(error='<nil>',input_cp=936,output_cp=936)
 return dict(physical_frame=48906,PDU=40097,actual_sources_and_prefix_highlight_exact=True,physical_Source0_bytes=len(raw),PDU_Source1_bytes=len(wire),prefix_PDU_range=[a,b],prefix_physical_range=case['field_physical_range'],default_history_mib=64,default_packet_cache=20000,global_partial_footer_retained=True,AH_authentication_not_claimed=True,restored=restored,snapshots=len(shots))
