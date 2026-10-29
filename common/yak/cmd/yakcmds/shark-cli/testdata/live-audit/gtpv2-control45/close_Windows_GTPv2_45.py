"""Close native GTPv2 cells, exact sources/highlights and root visual review.

Actual native console cells/attributes are checked; Windows Terminal pixels are
not claimed. The first failed IPC run remains immutable and is not a success.
"""
from pathlib import Path
import hashlib,importlib.util,json,re
root=Path(__file__).parent;out=root/'Windows-GTPv2-root-closure45-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
original=root/'Windows-GTPv2-source-plan44-01';plan=root/'Windows-GTPv2-source-plan45-02';actual=root/'windows-GTPv2-df30f-ack45-01'
assert read(plan/'plan.json')['only_helper_transport_changed']and read(actual/'summary.json')['passed']
old=read(original/'cases.json');cases=read(plan/'cases.json');assert len(cases)==len(old)==2
spec=importlib.util.spec_from_file_location('native_console_source_pane',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
results=[]
for c,o in zip(cases,old):
 assert{k:v for k,v in c.items()if k not in('helper','helper_SHA256')}=={k:v for k,v in o.items()if k not in('helper','helper_SHA256')}
 folder=actual/c['name'];producer=actual/(c['name']+'-producer.json');p=read(producer)
 assert p['source']=='df30fc4bd909d6489844f8e0d1cd0ab03826f35c'and p['exit_code']==0 and p['inputs_unchanged']
 for v in p['inputs_before'].values():assert sha(v['path'])==v['SHA256']and Path(v['path']).stat().st_size==v['bytes']
 shots={name:read(folder/(name+'.json'))for name in p['snapshots']}
 required={'00-capture-ready','00-filter-ready','01-protocol-fields','01-deep-protocol-fields','01-adaptive-fields-before','01-adaptive-bytes-click','01-adaptive-fields-click','01-selected-field','02-first-byte-source','02-byte-sources','03-conversation-messages','04-association-evidence','05-narrow','06-restored'};assert required<=shots.keys()
 for name,s in shots.items():
  assert sha(folder/(name+'.json'))==p['snapshots'][name]['cells_SHA256']and sha(folder/(name+'.png'))==p['snapshots'][name]['PNG_SHA256']
  assert len(s['Rows'])==len(s['Attributes'])==s['Height']and all(len(row)==len(at)==s['Width']for row,at in zip(s['Rows'],s['Attributes']))
  assert(folder/(name+'.json.ready')).is_file(),'snapshot completion acknowledgement missing'
 first=shots['01-protocol-fields'];text='\n'.join(first['Rows']);assert' EOF 'in first['Rows'][0]and c['query']in first['Rows'][1]and' › '.join(c['progressive_stack'])in text
 assert re.search(r'buffer\s+1\s*/\s*20000',text)and re.search(r'· 1 visible(?:\s|╮)',text)
 assert any(re.search(r'›1\s+\S+\s+GTPv2-C(?:\s|$)',row)for row in first['Rows'])
 raw=Path(c['physical_file']).read_bytes();wire=raw[slice(*c['PDU_range'])];assert sha(c['physical_file'])==c['physical_SHA256']and hashlib.sha256(wire).hexdigest()==c['PDU_SHA256']
 assert m.pane(first)[0]==raw and m.pane(shots['02-first-byte-source'])[0]==raw and m.pane(shots['02-byte-sources'])[0]==wire
 selected=shots['01-selected-field'];observed,pos=m.pane(selected);assert observed==wire
 lo,hi=c['field_PDU_range'];attrs=[selected['Attributes'][y][x]for y,x in pos];style=attrs[lo]
 assert all(v==style for v in attrs[lo:hi])and all(v!=style for i,v in enumerate(attrs)if not lo<=i<hi)
 assert all(selected['Attributes'][y][x+1]==style for y,x in pos[lo:hi])
 assert observed[lo:hi].hex()==c['field_hex']and c['field_name']+': '+c['field_value']in'\n'.join(selected['Rows'])
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 image=folder/'01-selected-field.png'
 results.append(dict(case=c['name'],original_source_frame=c['original_public_frame'],derived_inspection_frame=1,authored_control=c['authored_control'],field=c['field_name'],value=c['field_value'],PDU_range=[lo,hi],physical_range=c['field_physical_range'],physical_bytes=len(raw),PDU_bytes=len(wire),entire_visible_sources_exact=True,selected_highlight_exact=True,CP936_tabs_mouse_resize_checked=True,default_packet_cache=20000,default_history_mib=64,producer_SHA256=sha(producer),selected_image=str(image),image_SHA256=sha(image),root_viewed_actual_image=True))
result=dict(scope=__doc__,source='df30fc4bd909d6489844f8e0d1cd0ab03826f35c',cases=results,passed=True,source_plan_SHA256=sha(plan/'plan.json'),only_helper_binding_changed=True,navigation_and_source_expectations_unchanged=True,first_IPC_empty_request_failure_preserved=True,root_visual_review='Root viewed both selected PNGs; concrete names, progressive indentation, separate second-message bytes, exact highlights and dark pane backgrounds readable.',console_cells_not_terminal_pixels=True,checker_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),cases=2,passed=True)))
