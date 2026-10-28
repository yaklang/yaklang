"""Mutate actual VXLAN wire/reference/ownership witnesses; require rejection."""
import argparse,copy,io,json,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import vxlan_parent_source_audit as a


class MemoryJSONL:
    def __init__(self,values):self.values=values
    def open(self,mode='r',encoding=None):
        value=''.join(a.canonical(v)+'\n'for v in self.values)
        return io.BytesIO(value.encode())if'b'in mode else io.StringIO(value)


def candidate_identity_negatives(capture,inventory,before,events,before_flows,flows):
    positive=a.public_candidates(capture,inventory,before,events,before_flows,flows);wanted={v['frame']for v in json.loads(inventory.read_text('utf8'))['observations']};old=[];new=[]
    for path,destination in[(before,old),(events,new)]:
        for event in a.rows(path):
            if any(r['Number']in wanted for r in event['source_bytes'].get('PacketRefs')or[]):destination.append(event)
    checks=[]
    def rejected(name,mutation):
        value=copy.deepcopy(new);mutation(value)
        try:a.public_candidates(capture,inventory,MemoryJSONL(old),MemoryJSONL(value),before_flows,flows)
        except(AssertionError,KeyError):checks.append(dict(case=name,rejected=True));return
        raise AssertionError('candidate ID forgery accepted: '+name)
    def assign(event,ident):
        event['id']=ident;event['field_values']['pdu']=ident
        for field in event['field_values']['values']:
            if field['name']=='pdu.id':field['value']=ident;field['exact_value']=str(ident)
    rejected('coherent own PDU and snapshot aliases renumbered without a new source publication',lambda v:assign(v[0],v[0]['id']+1))
    rejected('snapshot own PDU differs from event identity',lambda v:v[0]['field_values'].__setitem__('pdu',v[0]['id']+1))
    rejected('typed numeric PDU ID changed from integer to equal float',lambda v:next(f for f in v[0]['field_values']['values']if f['name']=='pdu.id').__setitem__('value',float(v[0]['id'])))
    rejected('event PDU ID changed from integer to equal float',lambda v:v[0].__setitem__('id',float(v[0]['id'])))
    rejected('two physical candidate sources reuse one PDU identity',lambda v:assign(v[1],v[0]['id']))
    return dict(scope='Actual original candidate35 paired sources publish no extra PDU; event/snapshot/typed own IDs and their types must remain literal',auditor_SHA256=a.sha(Path(a.__file__)),driver_SHA256=a.sha(Path(__file__)),capture_SHA256=a.sha(capture),before_SDK_SHA256=a.sha(before),SDK_SHA256=a.sha(events),positive_original_sources=positive['original_selected_frames'],negative_controls=len(checks),all_rejected=True,checks=checks)


def public_cli_negatives(capture,inventory,before_events,events,before_cli,cli):
    positive=a.public_candidate_cli(capture,inventory,before_events,events,before_cli,cli);wanted={v['frame']for v in json.loads(inventory.read_text('utf8'))['observations']};sides=[]
    for path in[before_events,events]:sides.append([e for e in a.rows(path)if any(r['Number']in wanted for r in e['source_bytes'].get('PacketRefs')or[])])
    for path in[before_cli,cli]:sides.append([r for r in a.rows(path)if r.get('number')in wanted])
    def event(values,n):return next(e for e in values if e['source_bytes']['PacketRefs'][0]['Number']==n)
    def row(values,n):return next(r for r in values if r['number']==n)
    checks=[]
    def rejected(name,mutation):
        values=copy.deepcopy(sides);mutation(values)
        try:a.public_candidate_cli(capture,inventory,*[MemoryJSONL(v)for v in values])
        except(AssertionError,KeyError):checks.append(dict(case=name,rejected=True));return
        raise AssertionError('negative accepted: '+name)
    rejected('CLI source endpoint replaced by another plausible IP',lambda v:row(v[3],48568).__setitem__('source','192.0.2.1'))
    rejected('original source-only observed VNI namespace forged coherently as a different legacy tag',lambda v:row(v[2],48568)['capture_domain'].__setitem__('Encapsulation','/vxlan:1.2.3.4~4.3.2.1:42'))
    rejected('new physical capture domain keeps the unselected source-only tunnel',lambda v:row(v[3],48568).__setitem__('capture_domain',copy.deepcopy(row(v[2],48568)['capture_domain'])))
    rejected('wire UDP port integer replaced by equal float',lambda v:row(v[3],48568).__setitem__('source_port',4789.0))
    rejected('wire-zero destination port serialized as an invented explicit scalar',lambda v:row(v[3],133116).__setitem__('destination_port',0))
    rejected('physical frame time changed while source reference stays plausible',lambda v:row(v[3],48568).__setitem__('time','2022-02-13T06:06:47Z'))
    rejected('carrier overrun diagnostic changes its literal declared bound',lambda v:row(v[3],48568).__setitem__('info',row(v[3],48568)['info'].replace('121','122')))
    rejected('precise carrier error is erased along with the generic observer suffix',lambda v:row(v[3],48568).__setitem__('info','UDP payload unrecognized'))
    rejected('corrected opaque source gains a DNS child PDU',lambda v:row(v[3],48644)['pdus'].append(dict(id=999999,protocol='dns',status='decoded')))
    def native_change(values,key,value):
        e=event(values[1],48644);e[key]=value;row(values[3],48644)['pdus']=[a.native_cli_pdu(e)]
    rejected('corrected native event changes a preserved direction value',lambda v:native_change(v,'direction',1))
    rejected('corrected native event invents an extra metadata value',lambda v:native_change(v,'invented_wire_version',1))
    def foreign_parent(values):
        e=event(values[1],48644)
        for source in[e['source_bytes'],e['field_values']['source']]:source['ParentPDU']=1
        row(values[3],48644)['pdus']=[a.native_cli_pdu(e)]
    rejected('corrected captured payload coherently invents a decoded parent contribution',foreign_parent)
    def renumber(values):
        e=event(values[1],48644);e['id']+=100000;e['field_values']['pdu']=e['id']
        for f in e['field_values']['values']:
            if f['name']=='pdu.id':f['value']=e['id'];f['exact_value']=str(e['id'])
        row(values[3],48644)['pdus']=[a.native_cli_pdu(e)]
    rejected('coherent own PDU renumbering is not a permitted correction',renumber)
    def closed_child(values):values[3].append(dict(record_type='protocol_publication',publication_phase='capture_close',published_pdus=[a.native_cli_pdu(event(values[1],48644))]))
    rejected('unselected datagram acquires a capture-close publication contribution',closed_child)
    return dict(scope='Independent source-derived complete candidate CLI/SDK expectations reject wire/source/typed identity/diagnostic and child-contribution forgeries; no broad path waiver',auditor_SHA256=a.sha(Path(a.__file__)),driver_SHA256=a.sha(Path(__file__)),capture_SHA256=a.sha(capture),positive_original_sources=positive['original_sources'],positive_complete_expected_row_corrections=positive['source_derived_complete_CLI_row_corrections'],negative_controls=len(checks),all_rejected=True,checks=checks)


def audit(capture,oracle,events,flows,control_root):
    positive=a.audit(capture,oracle,events,flows);physical={n:r for n,r in enumerate(a.physical(capture),1)};scans={n:a.scan(r)for n,r in physical.items()};reference=json.loads(oracle.read_text('utf8'));native=list(a.rows(events));state=json.loads(flows.read_text('utf8'));checks=[]
    n=next(n for n,s in scans.items()if s['headers']);header=scans[n]['headers'][0];layers=reference[n-1]['_source']['layers'];wire=physical[n]['wire'];a.reference_header(layers,header,wire)
    def rejected(name,action):
        try:action()
        except(AssertionError,KeyError):checks.append(dict(case=name,rejected=True));return
        raise AssertionError('negative accepted: '+name)
    def reference_forgery(name,field,mutation):
        value=copy.deepcopy(layers);owner=next(o for o in a.objects(value)if field in o);mutation(owner[field],owner);rejected(name,lambda:a.reference_header(value,header,wire))
    reference_forgery('VNI raw descriptor moved into another source', 'vxlan.vni_raw',lambda v,o:v.__setitem__(5,1))
    reference_forgery('VNI raw descriptor shifted into adjacent header', 'vxlan.vni_raw',lambda v,o:v.__setitem__(1,v[1]+1))
    reference_forgery('VNI scalar differs from its original bytes', 'vxlan.vni_raw',lambda v,o:o.__setitem__('vxlan.vni',str(int(o['vxlan.vni'])+1)))
    reference_forgery('I flag mask changed while scalar remains plausible','vxlan.flag_i_raw',lambda v,o:v.__setitem__(3,0x0400))
    reference_forgery('I flag scalar disagrees with original header','vxlan.flag_i_raw',lambda v,o:o.__setitem__('vxlan.flag_i','0'))
    changed=bytearray(wire);changed[header['offset']]^=8;rejected('record I bit altered without a matching literal reference',lambda:a.reference_header(layers,header,bytes(changed)))
    def state_forgery(name,mutate):
        value=copy.deepcopy(state);mutate(value);rejected(name,lambda:a.native_ownership(physical,scans,native,value))
    state_forgery('TCP packet ownership rebound to foreign original interface',lambda v:v['packets'][0]['refs'][0]['Domain'].__setitem__('Interface',v['packets'][0]['refs'][0]['Domain']['Interface']+1))
    first=state['packets'][0];foreign=next(f['flow']for f in state['flows']if f['flow']!=first['flow']and(f['source'],f['destination'],f['domain'])!=(state['flows'][0]['source'],state['flows'][0]['destination'],state['flows'][0]['domain']))
    state_forgery('TCP packet assigned to a different literal endpoint/domain flow',lambda v:v['packets'][0].__setitem__('flow',foreign))
    quoted=next(n for n,s in scans.items()if s['quoted_headers']and s['leaf']['kind']=='icmp')
    def quoted_TCP(value):
        packet=copy.deepcopy(value['packets'][0]);packet['refs']=[dict(Number=quoted,Domain=a.domain(physical[quoted],scans[quoted]['leaf']['domain']))];value['packets'].append(packet)
    state_forgery('quoted VXLAN/TCP bytes incorrectly create transport ownership',quoted_TCP)
    def event_forgery(name,mutate):
        value=copy.deepcopy(native);mutate(value);rejected(name,lambda:a.native_ownership(physical,scans,value,state))
    bound=next(i for i,e in enumerate(native)if e['domain']['Encapsulation']and e['source_bytes'].get('PacketRefs'))
    def change_domain(value):
        event=value[bound];event['domain']['Encapsulation']+='/vxlan:forged:43'
        for source in[event['source_bytes'],event['field_values']['source']]:
            for ref in source['PacketRefs']:ref['Domain']=copy.deepcopy(event['domain'])
            if source.get('CompletedIn'):source['CompletedIn']['Domain']=copy.deepcopy(event['domain'])
    event_forgery('coherent but invented VNI source domain',change_domain)
    def invented_parent(value):
        for source in[value[0]['source_bytes'],value[0]['field_values']['source']]:source['ParentPDU']=1
    event_forgery('captured/reassembled bytes invent a decoded parent PDU',invented_parent)
    def changed_digest(value):
        for source in[value[0]['source_bytes'],value[0]['field_values']['source']]:source['SHA256']='0'*64
    event_forgery('coherent message byte digest changed without changing wire bytes',changed_digest)
    # Use a real before-fix false DNS child plus its actual fixed refusal. The
    # correction cannot pass merely by adding a label while retaining the child.
    d=control_root/'I-missing';receipts=json.loads((d/'CLI-producers.json').read_text());get=lambda k:Path(receipts[k]['export']);manifest=json.loads((control_root.parent/'controls-before-01/manifest.json').read_text());entry=next(v for v in manifest['cases']if v['name']=='I-missing');capture_control=Path(entry['capture']);assert a.sha(capture_control)==entry['capture_SHA256']==receipts['new-events']['capture_SHA256']
    record=a.physical(capture_control)[0];current=list(a.rows(get('new-events')));a.header_refusal(record,current[0],1)
    def parent_forgery(name,mutation):
        value=copy.deepcopy(current[0]);mutation(value);rejected(name,lambda:a.header_refusal(record,value,1))
    parent_forgery('I-clear refusal falsely reports I set',lambda v:v['session'].__setitem__('VXLAN I Bit',True))
    parent_forgery('I-clear refusal invents a native TCP connection',lambda v:v.__setitem__('tcp_flow_id',1))
    parent_forgery('I-clear refusal advances source VNI without changing bytes',lambda v:v['session'].__setitem__('Observed VXLAN VNI',43))
    parent_forgery('I-clear refusal source rebound to another frame',lambda v:v['source_bytes']['PacketRefs'][0].__setitem__('Number',2))
    child=list(a.rows(get('old-events')))[0];assert child['protocol']=='dns'and child['status']=='decoded';extra=copy.deepcopy(child);extra['id']=current[0]['id']+1
    rejected('adding a refusal without removing its false DNS contribution',lambda:a.controls(capture_control,get('old-events'),MemoryJSONL(current+[extra]),get('old-flows'),get('new-flows'),get('new-yak')))
    return dict(scope=__doc__,auditor_SHA256=a.sha(Path(a.__file__)),driver_SHA256=a.sha(Path(__file__)),capture_SHA256=a.sha(capture),reference_SHA256=a.sha(oracle),SDK_SHA256=a.sha(events),flows_SHA256=a.sha(flows),positive_source_bound_headers=positive['source_bound_selected_VXLAN_header_occurrences']+positive['source_bound_quoted_VXLAN_header_occurrences'],positive_source_bound_TCP_callbacks=positive['native']['source_bound_TCP_packet_callbacks'],positive_source_bound_PDUs=positive['native']['source_bound_PDUs'],negative_controls=len(checks),all_rejected=True,checks=checks,corrected_export_fabricated=False)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    for name in['capture','oracle','events','flows','control-root','out']:parser.add_argument('--'+name,type=Path,required=True)
    args=parser.parse_args();proof=audit(args.capture,args.oracle,args.events,args.flows,args.control_root)
    with args.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','checks']}),flush=True)
