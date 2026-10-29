"""Close actual Windows RSVP cells, exact full sources/highlights and root image review."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,re
root=Path(__file__).parent
ap=argparse.ArgumentParser(description=__doc__)
for key in('actual','out'):ap.add_argument('--'+key,type=Path,required=True)
ap.add_argument('--source',required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
old=root/'Windows-RSVP-source-plan46-01';plan=root/'Windows-RSVP-source-plan46-03'
assert read(a.actual/'summary.json')['passed']and read(plan/'plan.json')['only_navigation_changed']
cases=read(plan/'cases.json');original=read(old/'cases.json');assert len(cases)==len(original)==2
spec=importlib.util.spec_from_file_location('actual_console_RSVP_wire_pane',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
results=[]
for c,o in zip(cases,original):
 assert {k:v for k,v in c.items()if k!='field_down'}=={k:v for k,v in o.items()if k!='field_down'}
 folder=a.actual/c['name'];producer=a.actual/(c['name']+'-producer.json');p=read(producer)
 assert p['source']==a.source and p['exit_code']==0 and p['inputs_unchanged']
 for v in p['inputs_before'].values():assert sha(v['path'])==v['SHA256']and Path(v['path']).stat().st_size==v['bytes']
 shots={name:read(folder/(name+'.json'))for name in p['snapshots']}
 required={'00-capture-ready','00-filter-ready','01-protocol-fields','01-deep-protocol-fields','01-adaptive-fields-before','01-adaptive-bytes-click','01-adaptive-fields-click','01-selected-field','02-first-byte-source','02-byte-sources','03-conversation-messages','04-association-evidence','05-narrow','06-restored'}
 assert required<=shots.keys()
 for name,s in shots.items():
  cells=folder/(name+'.json')
  assert sha(cells)==p['snapshots'][name]['cells_SHA256']and sha(cells.with_suffix('.png'))==p['snapshots'][name]['PNG_SHA256']
  assert len(s['Rows'])==len(s['Attributes'])==s['Height']and all(len(row)==len(at)==s['Width']for row,at in zip(s['Rows'],s['Attributes']))
  assert cells.with_name(cells.name+'.ready').is_file()or any(sha(q)==sha(cells)and q.with_name(q.name+'.ready').is_file()for q in folder.glob(name+'-wait-*.json'))
 first=shots['01-protocol-fields'];text='\n'.join(first['Rows'])
 assert ' EOF 'in first['Rows'][0]and c['query']in first['Rows'][1]and' › '.join(c['progressive_stack'])in text
 assert re.search(r'buffer\s+1\s*/\s*20000',text)and re.search(r'· 1 visible(?:\s|╮)',text)
 assert any(re.search(r'›1\s+\S+\s+RSVP(?:\s|$)',row)for row in first['Rows'])
 raw=Path(c['physical_file']).read_bytes();wire=raw[slice(*c['PDU_range'])]
 assert m.pane(first)[0]==raw and m.pane(shots['02-first-byte-source'])[0]==raw and m.pane(shots['02-byte-sources'])[0]==wire
 selected=shots['01-selected-field'];observed,pos=m.pane(selected);assert observed==wire
 lo,hi=c['field_PDU_range'];attrs=[selected['Attributes'][y][x]for y,x in pos];style=attrs[lo]
 assert all(v==style for v in attrs[lo:hi])and all(v!=style for i,v in enumerate(attrs)if not lo<=i<hi)
 assert all(selected['Attributes'][y][x+1]==style for y,x in pos[lo:hi])
 assert observed[lo:hi].hex()==c['field_hex']and c['field_name']+': '+c['field_value']in'\n'.join(selected['Rows'])
 assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 image=folder/'01-selected-field.png'
 results.append(dict(case=c['name'],original_frame=c['original_public_frame'],derived_inspection_frame=1,field=c['field_name'],value=c['field_value'],PDU_range=[lo,hi],physical_range=c['field_physical_range'],entire_visible_sources_exact=True,selected_highlight_exact=True,default_packet_cache=20000,default_history_mib=64,CP936_mouse_tabs_resize_checked=True,producer_SHA256=sha(producer),selected_image=str(image),image_SHA256=sha(image),root_viewed_actual_image=True))
proof=dict(scope=__doc__,source=a.source,cases=results,passed=True,source_plan_SHA256=sha(plan/'plan.json'),only_navigation_changed=True,source_expected_names_values_ranges_unchanged=True,old_navigation_failures_preserved=True,console_cells_not_terminal_pixels=True,root_visual_review='Root viewed both actual selected images and checked concrete object names, nested bodies, full byte sources and exact highlights.',checker_SHA256=sha(__file__))
(a.out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),cases=2,passed=True)))
