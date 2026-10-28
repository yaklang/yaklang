"""Freeze whole CLI membership expectations before any new public CLI export."""
import argparse,collections,hashlib,importlib.util,json,sys
from pathlib import Path
from membership_cli_expected import same,compact_pdu,expected_row,legacy_membership_layer,physical_prefix

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1048576),b''):h.update(b)
 return h.hexdigest()
def rows(p):
 with Path(p).open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
def module(name,p):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('before-cli','native-after','native-proof','native-plan','igmp-modules','mld-modules','carrier','audit-modules','capture-weber','capture-sharon','repo','dependency-layers','out'):p.add_argument('--'+n,type=Path,required=True)
 p.add_argument('--source-cli',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
 sys.path[:0]=[str(a.igmp_modules),str(a.mld_modules),str(a.audit_modules)]
 from vxlan_endpoint_projection_audit import capture_records
 igmp=module('igmpWholeOracle',a.igmp_modules/'igmp_native_expected.py');mld=module('mldWholeOracle',a.mld_modules/'mld_native_expected.py');carrier=module('mldCarrier',a.carrier)
 proof=json.loads((a.native_proof/'summary.json').read_text());assert proof['whole_object_differences']==0 and proof['source_after']==a.source_cli and proof['source_plan_SHA256']==sha(a.native_plan/'summary.json')
 prooffiles={x['case']:x for x in proof['files']};summary={'scope':__doc__,'candidate_CLI_consumed':False,'source_cli':a.source_cli,'native_whole_proof_SHA256':sha(a.native_proof/'summary.json'),'native_plan_SHA256':sha(a.native_plan/'summary.json'),'cases':[]};frozen={};total=collections.Counter()
 for corpus,cap in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
  groups={f:{x['frame']:x for x in json.loads((a.native_plan/corpus/(f.upper()+'-source-plan.json')).read_text())}for f in('igmp','mld')};needed=set(groups['igmp'])|set(groups['mld']);records={n:r for n,r in enumerate(capture_records(cap),1)if n in needed}
  native=a.native_after/'preservation'/corpus/'events.jsonl';assert sha(native)==prooffiles[corpus]['after_SHA256']
  mapping={int(k):v for k,v in json.loads((a.native_proof/corpus/'pdu-map.json').read_text()).items()};assert sha(a.native_proof/corpus/'pdu-map.json')==prooffiles[corpus]['map_SHA256']and len(set(mapping.values()))==len(mapping)
  added={}
  # This only accelerates an already whole-certified compact JSONL format;
  # the parsed protocol/source identity and exact count remain mandatory.
  with native.open('rb')as handle:
   for line in handle:
    if b'"protocol":"igmp"'not in line:continue
    e=json.loads(line)
    if e['protocol']!='igmp':continue
    rs=e['source_bytes']['PacketRefs'];assert len(rs)==1;frame=rs[0]['Number'];assert frame in groups['igmp']and frame not in added;added[frame]=e
  assert added.keys()==groups['igmp'].keys()
  wanted={}
  for frame,s in groups['igmp'].items():
   r=records[frame];event=igmp.event(added[frame]['id'],r,s);assert same(event,added[frame]);wanted[frame]=('igmp',event,s['location'])
  for frame,s in groups['mld'].items():
   r=records[frame];r['frame']=frame;event,_,_=mld.event(mapping[s['id']],r,s,carrier.locate);wanted[frame]=('mld',event,s['source_profile'])
  frozen[corpus]=(wanted,records,groups)
 # All37 old exports are individually receipted. No candidate CLI path exists
 # in this program's arguments or read graph.
 for prpath in sorted(a.before_cli.glob('*/*/yak-producer.json')):
  c=prpath.parent.name;rel=prpath.relative_to(a.before_cli);data=prpath.with_name('yak.jsonl');pr=json.loads(prpath.read_text());assert pr['exit_code']==0 and pr['source']=='4e664ebaed542ffb80024ad70d797695975cee77'and sha(data)==pr['export_SHA256']
  wanted,records,groups=frozen.get(c,({}, {}, {}));selected=[];old_count=0;seen=set()
  for old in rows(data):
   old_count+=1;frame=old.get('number')
   if frame not in wanted:continue
   assert frame not in seen;seen.add(frame);family,event,loc=wanted[frame];r=records[frame];r['frame']=frame
   if family=='igmp':assert not old.get('pdus')and not groups[family][frame]['baseline_native_events']
   else:assert same(old['pdus'],[compact_pdu(groups[family][frame]['old_event'])]),'old CLI/native compact source disagrees'
   want=expected_row(old,r,loc,event,family)
   # Independent whole-row derivation records literal source binding rather
   # than saving a patch or accepting a path-list exemption.
   selected.append(dict(corpus=c,frame=frame,family=family,id=event['id'],wire_SHA256=hashlib.sha256(r['wire']).hexdigest(),native_event=event,location=loc,expected_row=want,old_row=old));total[family]+=1;total[family+'/'+event['status']]+=1
  assert seen==wanted.keys();out=a.out/c;out.mkdir()
  with(out/'membership-rows.jsonl').open('w',encoding='utf8')as f:
   for e in selected:f.write(json.dumps(e,separators=(',',':'))+'\n')
  native_entry=prooffiles[c];summary['cases'].append(dict(case=c,relative_producer=str(rel).replace('\\','/'),old_rows=old_count,old_export_SHA256=sha(data),old_producer_SHA256=sha(prpath),capture_SHA256=pr['capture_SHA256'],native_after_SHA256=native_entry['after_SHA256'],native_map_SHA256=native_entry['map_SHA256'],membership_count=len(selected),membership_plan_SHA256=sha(out/'membership-rows.jsonl'),all_other_rows='whole typed equality after explicit known_cli_pdu_reference_paths only'))
 assert len(summary['cases'])==37 and total['igmp']==151 and total['mld']==237
 sourcefiles=[Path(__file__),Path(__file__).with_name('membership_cli_expected.py'),a.audit_modules/'known_cli_pdu_reference_paths.py']
 sourcefiles += [a.repo/'common/yak/cmd/yakcmds/shark-cli'/name for name in('capture.go','decode.go','presentation.go','membership_presentation.go','packet_observation.go','conversation.go','protocol.go','command.go')]
 sourcefiles += [a.repo/'common/pcapx/pcaputil'/name for name in('capture_transport.go','capture_vlan.go','capture_protocol_decoder.go','capture_mpacket.go','protocol_evidence.go')]
 sourcefiles += [a.dependency_layers/name for name in('igmp.go','mldv1.go','mldv2.go','icmp6.go','base.go')]+[a.dependency_layers.parent/'packet.go']
 summary.update(membership_counts=dict(total),old_rows=sum(c['old_rows']for c in summary['cases']),source_files={str(p):sha(p)for p in sourcefiles},auditor_SHA256=sha(Path(__file__)))
 f=a.out/'summary.json';f.write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps({'summary':str(f),'SHA256':sha(f),'counts':dict(total),'old_rows':summary['old_rows']}))
if __name__=='__main__':main()
