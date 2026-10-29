"""Read-only audit of pure556 OSPFv2 field/alias contract, not native execution.

Fields use the independent frozen per-case checker. Expected typed alias values,
scopes and ranges are reconstructed from raw-validated source leaves and their
semantic ownership, not from any constructor aliases or candidate output.
"""
import argparse,collections,hashlib,importlib.util,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def load(name,path):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def alias_for(path):
 key=path[-1]
 if path[0]=='Header':return{'Version':'ospf.version','Type':'ospf.msg','Packet Length':'ospf.packet_length','Checksum':'ospf.checksum','Router ID':'ospf.srcrouter','Area ID':'ospf.area_id','Instance ID':'ospfv2.instance_id','Authentication Type':'ospfv2.auth.type'}.get(key)
 if path[0]=='Authentication':return{'Key ID':'ospfv2.auth.key_id','Authentication Data Length':'ospfv2.auth.data_length','Sequence':'ospfv2.auth.sequence'}.get(key)
 if path[0]=='Requests':return{'LS Type':'ospfv2.request.ls_type','Link State ID':'ospfv2.request.id','Advertising Router':'ospfv2.request.advertising_router'}.get(key)
 if path[0]=='LLS':return{'Checksum':'ospfv2.lls.checksum','Length Words':'ospfv2.lls.length_words'}.get(key)if len(path)==2 else{'TLV Type':'ospfv2.lls.tlv.type','Length':'ospfv2.lls.tlv.length','Extended Options and Flags':'ospfv2.lls.extended_options','Sequence':'ospfv2.lls.auth.sequence'}.get(key)
 header=(path[0]in('LSAs','LSA Headers')and len(path)==3)or(path[:2]==['Database Description','LSA Headers']and len(path)==4)
 if header and key!='Options':return{'LS Age':'ospfv2.lsa.age','LS Type':'ospfv2.lsa.type','Link State ID':'ospfv2.lsa.id','Advertising Router':'ospfv2.lsa.advertising_router','Sequence':'ospfv2.lsa.sequence','Checksum':'ospfv2.lsa.checksum','Length':'ospfv2.lsa.length'}.get(key)
 return{'Options':'ospfv2.options','Router Priority':'ospfv2.router_priority','Hello Interval':'ospfv2.hello_interval','Router Dead Interval':'ospfv2.dead_interval','Interface MTU':'ospfv2.interface_mtu','DD Sequence':'ospfv2.dd_sequence','Flags':'ospfv2.flags','LSA Count':'ospfv2.lsa_count','Link Count':'ospfv2.link_count','Link Type':'ospfv2.link.type','TOS/MT-ID Count':'ospfv2.link.selector_count','TOS/MT-ID Selector':'ospfv2.selector','Metric':'ospfv2.metric','Flags and TOS/MT-ID':'ospfv2.external.flags','External Route Tag':'ospfv2.external.tag','Network Mask':'ospfv2.network_mask','Designated Router':'ospfv2.designated_router','Backup Designated Router':'ospfv2.backup_designated_router','Neighbor Router ID':'ospfv2.neighbor','Link ID':'ospfv2.link.id','Link Data':'ospfv2.link.data','Attached Router':'ospfv2.attached_router','Forwarding Address':'ospfv2.forwarding_address'}.get(key)

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for n in('phase','contract','verifier','out'):ap.add_argument('--'+n,required=True,type=Path)
 a=ap.parse_args();assert not a.out.exists();plan=a.phase/'OSPFv2-direct-source-plan41-01';v2=a.phase/'OSPFv2-independent-source39-01';bound={str(p):sha(p)for p in(Path(__file__),a.contract,a.verifier,plan/'manifest.json')};assert sha(a.contract)=='b31be890882764b20dceefddf7e8e7fa4d10bbd9944a7e7d11552278521a0901';c=load('pure_OSPFv2_source_contract',a.contract);v=load('independent_OSPFv2_wire_checker',a.verifier);inputs,facts,patterns,_=v.load_plan(plan);by={f['sourcecase']:f for f in facts};totals=collections.Counter();fail=[];ranges=0
 for number in(35,37):
  path=v2/('source-grid03/input-%02d-source-grid.json'%number);bound[str(path)]=sha(path)
  for fact in read(path):
   case=by['real/input-%02d/frame-%d'%(number,fact['frame'])];raw,wire=c.source_wire(fact);constructed=c.fields(fact);stats=v.verify_case(case,wire,4096,constructed,known_patterns=patterns,check_errors=False);totals.update(stats);leaves=case['wire_leaves'];source_aliases=[]
   def derived(name,value,at):source_aliases.append((name,at,{'name':name,'value':value,'origin':'derived'}))
   for leaf in leaves:
    name=alias_for(leaf['path'])
    if name:
     start,end=leaf['PDU_span'];source_aliases.append((name,start,{'name':name,'value':leaf['value'],'origin':'pdu','pdu_range':{'offset':start,'length':end-start}}));totals['literal_aliases']+=1
   for item in case['metadata_checks']:
    p=item['path']
    if p==['Checksum Verified']:derived('ospfv2.checksum_verified',item['value'],0)
    elif p==['LLS','Checksum Verified']:derived('ospfv2.lls.checksum_verified',item['value'],0)
    elif p[-1]=='Checksum Verified'and p[0]in('LSAs','LSA Headers','Database Description'):
     row=fact['source']['fields']
     for k in p[:-1]:row=row[k]
     derived('ospfv2.lsa.checksum_verified',item['value'],row['Wire Field Ranges']['LS Age'][0])
   for leaf in leaves:
    p=leaf['path']
    if p[:2]==['LLS','TLVs']and p[-1]=='Sequence':
     header=next(l for l in leaves if l['path']==['Authentication','Sequence']);derived('ospfv2.lls.auth.sequence_matches_header',leaf['value']==header['value'],leaf['PDU_span'][0])
    if p[:2]==['LLS','TLVs']and p[-1]=='Authentication Data':
     header=next(l for l in leaves if l['path']==['Authentication','Authentication Data Length']);derived('ospfv2.lls.auth.length_matches_header',leaf['PDU_span'][1]-leaf['PDU_span'][0]==header['value'],leaf['PDU_span'][0])
    if p[-1]=='Flags and TOS/MT-ID':derived('ospfv2.external.metric_type',1+(leaf['value']>>7),leaf['PDU_span'][0])
   expected=[r[2]for r in sorted(source_aliases,key=lambda x:(x[0],x[1]))];actual=c.aliases(constructed)
   if expected!=actual:fail.append({'input':number,'frame':fact['frame'],'reason':'alias ownership/value/range/order differs from independently source-mapped atoms','expected_aliases':len(expected),'actual_aliases':len(actual)})
   assert not any(x['name'].startswith('ospfv3.')for x in actual);totals['typed_aliases']+=len(actual);totals['derived_aliases']+=sum(x['origin']=='derived'for x in actual);totals['source_frames']+=1
 assert totals['source_frames']==556;assert bound=={p:sha(p)for p in bound};report={'scope':__doc__,'contract_SHA256':sha(a.contract),'counts':dict(totals),'failure_count':len(fail),'failures':fail,'inputs':bound,'inputs_unchanged':True,'no_native_candidate_or_CLI_output_read':True,'no_complete_event_or_CLI_row_constructed':True,'no_Go_run_or_repo_write':True,'scope_limit':'556 complete Ethernet/IPv4 auth0/2 source observations only; this is not a598 native boundary/ingress or UI proof','observed_pure_contract_risks':[]}
 with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
 print(json.dumps({'proof':str(a.out),'SHA256':sha(a.out),'counts':dict(totals),'failures':len(fail)}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
