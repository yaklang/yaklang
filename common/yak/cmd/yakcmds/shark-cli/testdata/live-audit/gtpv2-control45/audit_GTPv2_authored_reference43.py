"""Isolated Wireshark comparison after independently frozen authored wire plan.
Only known complete header/IE framing leaves are compared. Unrendered source
prefixes and reference overreach remain explicit, never converted to passes.
"""
import argparse,collections,hashlib,json,os,subprocess,time,xml.etree.ElementTree as ET
from pathlib import Path
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def put(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def main():
 ap=argparse.ArgumentParser();ap.add_argument('--plan',type=Path,required=True);ap.add_argument('--tshark',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir();paths=[Path(__file__),a.plan/'summary.json',a.plan/'source-grid.json',a.plan/'authored-controls.pcap',a.plan/'normative-contract.json',a.tshark];frozen={str(p):sha(p)for p in paths};doc=json.loads((a.plan/'source-grid.json').read_text('utf8'));grid={w['authored_capture_frame']:w for c in doc for w in c['wires']};config=a.out/'isolated-config';config.mkdir();env=os.environ.copy();env['APPDATA']=str(config);env['WIRESHARK_CONFIG_DIR']=str(config);env.pop('SSLKEYLOGFILE',None);cmd=[str(a.tshark),'-n','-2','-r',str(a.plan/'authored-controls.pcap'),'-T','pdml'];start=time.time()
 with(a.out/'reference.pdml').open('xb')as out,(a.out/'stderr.txt').open('xb')as err:r=subprocess.run(cmd,stdout=out,stderr=err,env=env,timeout=45)
 assert r.returncode==0;tree=ET.parse(a.out/'reference.pdml');rows=[];counts=collections.Counter();mapping={'gtpv2.flags':['Flags'],'gtpv2.version':['Version'],'gtpv2.p':['P'],'gtpv2.t':['T'],'gtpv2.mp':['MP Bit Observed','Message Priority'],'gtpv2.message_type':['Message Type'],'gtpv2.msg_length':['Message Length'],'gtpv2.teid':['TEID'],'gtpv2.seq':['Sequence Number'],'gtpv2.spare':['Priority or Spare Octet','Last Octet Spare'],'gtpv2.ie_type':['Type'],'gtpv2.ie_len':['Length'],'gtpv2.instance':['Instance'],'gtpv2.cr':['Spare']}
 for packet in tree.getroot().findall('packet'):
  num=int(next(x.attrib['show']for x in packet.iter('field')if x.attrib.get('name')=='frame.number'));source=grid[num];matched=[];unmapped=[];diffs=[];experts=[];protocols=[];observed=set()
  for proto in packet.findall('proto'):
   owner=proto.attrib.get('name');protocols.append(owner)
   for f in proto.iter('field'):
    name=f.attrib.get('name','')
    if name=='_ws.expert.message':experts.append({'owner':owner,'message':f.attrib.get('show'),'physical_pos':f.attrib.get('pos'),'size':f.attrib.get('size')})
    if name not in mapping:continue
    pos=int(f.attrib['pos'])-42;size=int(f.attrib['size']);value=int(f.attrib['show'],0)if f.attrib['show'].startswith('0x')else int(f.attrib['show']);candidates=[x for x in source['source_leaves']if x['path'][-1]in mapping[name]and x['span']==[pos,pos+size]and'value'in x]
    item={'field':name,'reference_value':value,'logical_span':[pos,pos+size],'owner':owner}
    if not candidates:unmapped.append(item);counts['reference_fields_without_source_leaf']+=1
    elif not any(x['value']==value for x in candidates):diffs.append({**item,'source_values':[x['value']for x in candidates]});counts['value_difference']+=1
    else:matched.append(item);counts['matched_value_and_span']+=1;observed.update(tuple(x['path'])for x in candidates if x['value']==value)
  eligible=[x for x in source['source_leaves']if'value'in x and x['path'][-1]in set(k for keys in mapping.values()for k in keys)];not_rendered=[{'path':x['path'],'span':x['span'],'value':x['value']}for x in eligible if tuple(x['path'])not in observed];counts['source_leaves_not_rendered_by_selected_alias']+=len(not_rendered);rows.append({'frame':num,'sourcecase':source['sourcecase'],'wire_index':source['wire_index'],'source_raw_SHA256':source['raw_SHA256'],'source_expectation':source['scenario_expectation'],'source_issues':source['source_issues'],'protocols':protocols,'matched':matched,'reference_without_source_framing_leaf':unmapped,'differences':diffs,'source_selected_leaves_not_rendered':not_rendered,'experts':experts})
 assert len(rows)==24 and counts['value_difference']==0;put(a.out/'comparisons.json',rows);assert frozen=={p:sha(p)for p in frozen};put(a.out/'summary.json',{'frozen_inputs':frozen,'frozen_inputs_unchanged':True,'rows':len(rows),'cases':20,'returncode':r.returncode,'seconds':round(time.time()-start,3),'command':cmd,'counts':dict(counts),'comparison_SHA256':sha(a.out/'comparisons.json'),'PDML_SHA256':sha(a.out/'reference.pdml'),'case8_P0_declared_short_reference_extra_fields':[x for x in rows if x['sourcecase']=='P0-unindicated-trailer'][0]['reference_without_source_framing_leaf'],'case6_illegal_secondP1_reference_extra_fields':[x for x in rows if x['sourcecase']=='second-P1-third-message'][0]['reference_without_source_framing_leaf'],'resource_controls_have_no_equivalent_Wireshark_collection_budget':True,'no_native_Go_or_repo_write':True});print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(counts)}))
if __name__=='__main__':main()
