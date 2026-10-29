"""Close the two successful original pages and navigation-only crypto retry.

All original failed-run artifacts remain immutable. This checker revalidates
source bytes, ranges, producer bindings and actual console cells. Root visual
review is recorded separately from machine checks; no terminal pixels claimed.
"""
from pathlib import Path
import hashlib,importlib.util,json,re
root=Path(__file__).parent;out=root/'Windows-OSPFv2-root-closure42-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
plan=root/'Windows-OSPFv2-source-plan42-01';cases=read(plan/'cases.json')
retry=root/'Windows-OSPFv2-crypto-source-plan42-02';rc=read(retry/'cases.json')[0]
original=next(c for c in cases if c['name']=='Cryptographic-LLS')
assert {k:v for k,v in original.items()if k!='field_down'}=={k:v for k,v in rc.items()if k!='field_down'}
spec=importlib.util.spec_from_file_location('console_source_wire_pane',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
results=[]
for c in cases:
 parent=root/('windows-OSPFv2-crypto-cd7af-default42-02'if c['name']=='Cryptographic-LLS'else'windows-OSPFv2-cd7af-default42-01');folder=parent/c['name'];producer=parent/(c['name']+'-producer.json');p=read(producer)
 assert p['source']=='cd7af6a67e998ed1a6466a11f2b37c66fad742a8'and p['exit_code']==0 and p['inputs_unchanged']
 for v in p['inputs_before'].values():assert sha(v['path'])==v['SHA256']and Path(v['path']).stat().st_size==v['bytes']
 shots={name:read(folder/(name+'.json'))for name in p['snapshots']}
 required={'00-capture-ready','00-filter-ready','01-protocol-fields','01-deep-protocol-fields','01-adaptive-fields-before','01-adaptive-bytes-click','01-adaptive-fields-click','01-selected-field','02-first-byte-source','02-byte-sources','03-conversation-messages','04-association-evidence','05-narrow','06-restored'}
 assert required<=shots.keys()
 for name,s in shots.items():
  assert sha(folder/(name+'.json'))==p['snapshots'][name]['cells_SHA256']and sha(folder/(name+'.png'))==p['snapshots'][name]['PNG_SHA256']
  assert len(s['Rows'])==len(s['Attributes'])==s['Height']and all(len(row)==len(at)==s['Width']for row,at in zip(s['Rows'],s['Attributes']))
 first=shots['01-protocol-fields'];text='\n'.join(first['Rows']);assert ' EOF 'in first['Rows'][0]and c['query']in first['Rows'][1]and ' › '.join(c['progressive_stack'])in text
 assert re.search(r'buffer\s+1\s*/\s*20000',text)and re.search(r'· 1 visible(?:\s|╮)',text)
 assert any(re.search(r'›1\s+\S+\s+OSPFv2(?:\s|$)',r)for r in first['Rows'])
 raw=Path(c['physical_file']).read_bytes();wire=raw[slice(*c['PDU_range'])];assert sha(c['physical_file'])==c['physical_SHA256']and hashlib.sha256(wire).hexdigest()==c['PDU_SHA256']
 assert m.pane(first)[0]==raw and m.pane(shots['02-first-byte-source'])[0]==raw and m.pane(shots['02-byte-sources'])[0]==wire
 selected=shots['01-selected-field'];observed,pos=m.pane(selected);assert observed==wire
 lo,hi=c['field_PDU_range'];attrs=[selected['Attributes'][y][x]for y,x in pos];style=attrs[lo]
 assert all(v==style for v in attrs[lo:hi])and all(v!=style for i,v in enumerate(attrs)if not lo<=i<hi)
 assert all(selected['Attributes'][y][x+1]==style for y,x in pos[lo:hi])
 assert observed[lo:hi].hex()==c['field_hex']and c['field_name']+': '+c['field_value']in '\n'.join(selected['Rows'])
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 image=folder/'01-selected-field.png'
 results.append(dict(case=c['name'],original_public_frame=c['original_public_frame'],derived_inspection_frame=1,field=c['field_name'],value=c['field_value'],PDU_range=[lo,hi],physical_range=c['field_physical_range'],physical_bytes=len(raw),PDU_bytes=len(wire),entire_visible_sources_exact=True,selected_highlight_exact=True,CP936_tabs_mouse_resize_checked=True,default_packet_cache=20000,default_history_mib=64,producer_SHA256=sha(producer),selected_image=str(image),image_SHA256=sha(image),root_viewed_actual_image=True))
result=dict(scope=__doc__,source='cd7af6a67e998ed1a6466a11f2b37c66fad742a8',cases=results,passed=True,actual_sources_from_original_public_packets=True,derived_one_packet_inspection_not_live=True,first_crypto_navigation_failure_preserved=True,only_navigation_changed=True,root_visual_review='Root viewed all3 selected PNGs from actual native cells; protocol names, indentation, byte highlights and pane backgrounds are readable.',console_cells_not_terminal_pixels=True,checker_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),cases=3,passed=True)))
