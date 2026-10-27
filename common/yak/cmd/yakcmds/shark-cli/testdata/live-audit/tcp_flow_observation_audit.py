"""Check every captured TCP packet, flow opener and timestamp, then compare streams.

Native packet assignment is verified against independently source-certified
original acquisition epochs. Wireshark stream IDs stay literal. Whole per-stream
frame sets are compared through each epoch's first captured SYN; any grouping
differences remain explicit instead of normalizing them away. Application/NDR
message completeness is a separate wire/logical certificate.
"""
import argparse
import calendar
import collections
import csv
import datetime
import hashlib
import json
import re
from pathlib import Path

from tcp_opening_wire_audit import capture_records,source_partition


def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def nanoseconds(value):
    match=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z',value)
    assert match,'literal UTC native capture timestamp'
    seconds=calendar.timegm(datetime.datetime.strptime(match[1],'%Y-%m-%dT%H:%M:%S').timetuple())
    return seconds*1000000000+int((match[2]or'').ljust(9,'0')or'0')


def audit(capture,source_capture,manifest,source_certificate,before,after,oracle):
    physical,by_frame,epochs,source=source_partition(capture,source_capture,manifest,source_certificate)
    _,records,_=capture_records(capture)
    states={v['epoch']:v for v in epochs};checks=0
    def check(value,why):
        nonlocal checks
        checks+=1;assert value,why
    def review(path,strict):
        native=json.loads(path.read_text(encoding='utf8'));check(native['capture_SHA256']==sha(capture),'native capture digest')
        flows={v['flow']:v for v in native['flows']};check(len(flows)==len(native['flows']),'unique native flow identities')
        observations={};diagnostics=[]
        for packet in native['packets']:
            check(packet['flow']in flows,'packet has a real created native flow')
            check(len(packet['refs'])==1,'selected original unfragmented TCP packet has one literal reference')
            ref=packet['refs'][0];no=ref['Number'];check(type(no)is int and no in physical,'literal current capture TCP packet number')
            check(no not in observations,'a captured TCP packet is observed at most once')
            check(ref['Domain']==physical[no]['domain'],'original capture domain')
            check(nanoseconds(packet['captured'])==physical[no]['time_ns'],'every retained original capture time is exact')
            observations[no]=packet
        missing=sorted(set(physical)-set(observations))
        payload_epochs=collections.defaultdict(set)
        for no,packet in observations.items():
            if physical[no]['payload']:payload_epochs[packet['flow']].add(by_frame[no])
        flow_epochs={}
        for flow,values in payload_epochs.items():
            check(len(values)==1,'one native flow cannot merge application packets from different original epochs')
            flow_epochs[flow]=next(iter(values))
        for no,packet in observations.items():
            if packet['flow']not in flow_epochs:
                diagnostics.append(dict(frame=no,kind='native flow has no observed application-byte epoch'));continue
            epoch=flow_epochs[packet['flow']];state=states[epoch]
            if epoch!=by_frame[no]:diagnostics.append(dict(frame=no,kind='packet assigned across original epochs',native_epoch=epoch,original_epoch=by_frame[no]))
            want=0 if physical[no]['source']==state['opening'][0]else 1
            if packet['direction']!=want:diagnostics.append(dict(frame=no,kind='direction does not follow the captured opener',native=packet['direction'],expected=want))
        for flow,value in flows.items():
            if flow not in flow_epochs:
                diagnostics.append(dict(flow=flow,kind='created flow has no original application epoch'));continue
            epoch=flow_epochs[flow];syn=physical[states[epoch]['SYN_frame']]
            for key,want in [('source',syn['source']),('destination',syn['destination']),('domain',syn['domain']),('half_open',False)]:
                if value[key]!=want:diagnostics.append(dict(flow=flow,kind='original opener metadata mismatch',field=key,native=value[key],expected=want))
        if strict:
            check(not native['error'],'complete selected TCP observation replay has no reassembly error')
            check(not missing,'every captured TCP packet, including opening copies, has an owner')
            check(not diagnostics,'all native packet directions and original opener metadata are exact')
            check(len(flows)==len(epochs)==len(set(flow_epochs.values())),'one native flow per original acquisition epoch')
            check(len(native['closed'])==len(flows)and all(not v['pending_gaps']for v in native['closed']),'all TCP closures are unique and have no invented TCP hole')
        return dict(capture_SHA256=native['capture_SHA256'],created_flows=len(flows),observed_TCP_packets=len(observations),missing_TCP_packets=missing,metadata_differences=diagnostics,native_flow_epochs=flow_epochs,reassembly_error=native['error']),observations,flow_epochs
    old,old_packets,old_flows=review(before,False)
    new,new_packets,new_flows=review(after,True)
    reference={};reference_groups=collections.defaultdict(set)
    for row in json.loads(oracle.read_text(encoding='utf8')):
        layers=row['_source']['layers'];no=int(layers['frame']['frame.number'])
        check(no in physical and no not in reference,'literal unique reference TCP packet')
        check(hashlib.sha256(bytes.fromhex(layers['frame_raw'][0])).hexdigest()==physical[no]['frame_sha256'],'reference uses the exact complete current physical frame')
        tcp=layers['tcp'];check(isinstance(tcp,dict),'selected reference frame contains one TCP header')
        stream=int(tcp['tcp.stream']);reference[no]=stream;reference_groups[stream].add(no)
    check(set(reference)==set(physical),'Wireshark observes every selected TCP packet')
    native_groups=collections.defaultdict(set)
    for no,packet in new_packets.items():native_groups[packet['flow']].add(no)
    comparisons=[];differences=[]
    for flow,epoch in sorted(new_flows.items()):
        anchor=states[epoch]['SYN_frame'];stream=reference[anchor];ours=native_groups[flow];theirs=reference_groups[stream]
        comparison=dict(original_epoch=epoch,native_flow=flow,Wireshark_stream=stream,opening_frame=anchor,native_packets=len(ours),reference_packets=len(theirs),native_only_frames=sorted(ours-theirs),reference_only_frames=sorted(theirs-ours),whole_packet_sets_equal=ours==theirs)
        comparisons.append(comparison)
        if ours!=theirs:differences.append(comparison)
    packet_rows=[]
    for no in sorted(physical):
        packet=new_packets[no];old_packet=old_packets.get(no);epoch=by_frame[no]
        anchor=reference[states[epoch]['SYN_frame']]
        packet_rows.append(dict(frame=no,original_frame=source['original_frame_by_observation_frame'][no],original_epoch=epoch,native_before=old_packet['flow']if old_packet else None,native_before_direction=old_packet['direction']if old_packet else None,native_after=packet['flow'],native_direction=packet['direction'],Wireshark_stream=reference[no],reference_stream_at_original_opener=anchor,reference_stream_matches_original_opener=reference[no]==anchor,capture_timestamp_ns=physical[no]['time_ns']))
    groups=[dict(Wireshark_stream=stream,frames=sorted(frames),original_acquisition_epochs=sorted({by_frame[n]for n in frames}))for stream,frames in sorted(reference_groups.items())]
    return dict(scope=__doc__,checks=checks,capture_SHA256=sha(capture),source_observation_proof=source,before_observer_SHA256=sha(before),after_observer_SHA256=sha(after),oracle_SHA256=sha(oracle),auditor_SHA256=sha(Path(__file__)),physical_frames=len(records),TCP_packets=len(physical),before=old,after=new,Wireshark_streams=len(reference_groups),reference_stream_groups=groups,whole_flow_packet_comparisons=comparisons,reference_grouping_differences=differences,packet_reference_stream_differences=sum(not p['reference_stream_matches_original_opener']for p in packet_rows),every_native_packet_timestamp_and_domain_exact=True,generated_stream_IDs_not_normalized=True),packet_rows


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','source-capture','manifest','source-certificate','before','after','oracle','out','csv']:ap.add_argument('--'+name,type=Path,required=True)
    args=ap.parse_args()
    for path in vars(args).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result,packets=audit(args.capture,args.source_capture,args.manifest,args.source_certificate,args.before,args.after,args.oracle)
    args.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    with args.csv.open('w',newline='',encoding='utf8')as file:
        writer=csv.DictWriter(file,fieldnames=list(packets[0]));writer.writeheader();writer.writerows(packets)
    print(json.dumps(dict(TCP_packets=result['TCP_packets'],missing_before=len(result['before']['missing_TCP_packets']),metadata_differences_before=len(result['before']['metadata_differences']),reference_grouping_differences=len(result['reference_grouping_differences']))),flush=True)
