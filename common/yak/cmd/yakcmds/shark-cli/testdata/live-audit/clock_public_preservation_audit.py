"""Compare complete public SDK events across the capture-clock correction.

Pair by literal bytes, contributors, capture domain, transport endpoints and both
flow namespaces. Only generated event/PDU-ID aliases may be renumbered. The six
new unprotected zero tails and source-bound fragment diagnostics are adjudicated
separately. No application field or arbitrary wire integer is normalized.
"""
import argparse,base64,collections,copy,hashlib,ipaddress,json
from pathlib import Path
from mail_rtsp_public_wire_audit import records


def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))
def rows(p):
    for line in p.open(encoding='utf8'):yield json.loads(line)
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def key(e):
    values=[e.get(k)for k in['protocol','raw','flow','tcp_flow_id','source','destination','direction','transport','domain']]
    return hashlib.sha256(canonical(values+[e['source_bytes'].get('PacketRefs')]).encode()).hexdigest()
def without_generated_id(e):
    e=copy.deepcopy(e);e.pop('id');e['field_values'].pop('pdu')
    for f in e['field_values'].get('values')or[]:
        if f['name']=='pdu.id':f['value']='<generated-event-reference>';f.pop('exact_value',None)
    return e


def network(raw,link):
    assert link==1 and len(raw)>=14
    at=14;kind=int.from_bytes(raw[12:14],'big');encapsulation=''
    while kind in[0x8100,0x88a8,0x9100]:
        assert at+4<=len(raw);encapsulation+='/vlan:'+str(int.from_bytes(raw[at:at+2],'big')&4095);kind=int.from_bytes(raw[at+2:at+4],'big');at+=4
    return ip_network(raw,at,kind,encapsulation,0)


def ip_network(raw,at,kind,encapsulation,depth):
    assert depth<=4
    if kind==0x800:
        assert at+8<=len(raw)
        flags=int.from_bytes(raw[at+6:at+8],'big');fragment=bool(flags&0x3fff)
        end=min(len(raw),at+int.from_bytes(raw[at+2:at+4],'big'));header=(raw[at]&15)*4
        if not fragment and at+20<=len(raw)and raw[at+9]in[4,41]:
            assert header>=20 and at+header<len(raw)
            peers=sorted([str(ipaddress.ip_address(raw[at+12:at+16])),str(ipaddress.ip_address(raw[at+16:at+20]))])
            inner=0x800 if raw[at+9]==4 else 0x86dd;label='/ipip:'if inner==0x800 else'/6in4:'
            return ip_network(raw,at+header,inner,encapsulation+label+'~'.join(peers),depth+1)
        return dict(encapsulation=encapsulation,fragment=fragment,observed_version=raw[at]>>4,expected_version=4,UDP=raw[at+header:end]if not fragment and at+20<=len(raw)and raw[at+9]==17 else None)
    assert kind==0x86dd and at+40<=len(raw)
    version=raw[at]>>4
    end=min(len(raw),at+40+int.from_bytes(raw[at+4:at+6],'big'));next_header=raw[at+6];at+=40
    for _ in range(16):
        if next_header==44:return dict(encapsulation=encapsulation,fragment=True,observed_version=version,expected_version=6,UDP=None)
        if next_header not in[0,43,60]:return dict(encapsulation=encapsulation,fragment=False,observed_version=version,expected_version=6,UDP=raw[at:end]if next_header==17 else None)
        assert at+2<=end;size=(raw[at+1]+1)*8;next_header=raw[at];at+=size
    raise AssertionError('bounded independent extension chain')


def audit(capture,before,after):
    old=collections.defaultdict(collections.deque);old_ip=[];new_ip=[];paired=0;exact=0;renumbered=0;padding=[];mapping=[]
    for e in rows(before):
        if e['protocol']=='ip'and e.get('error','').startswith('Fragment'):old_ip.append(e)
        else:old[key(e)].append(e)
    for e in rows(after):
        if e['protocol']=='ip'and e.get('error','').startswith('Fragment'):new_ip.append(e);continue
        q=old.get(key(e));assert q,'every non-fragment observation retains literal bytes, source and both connection identities'
        a=q.popleft();paired+=1;mapping.append((a['id'],e['id']))
        if canonical(a)==canonical(e):exact+=1;continue
        if e['completeness']=='datagram-padding':
            raw=base64.b64decode(e['raw']);assert raw and not any(raw)and a['protocol']==e['protocol']=='quic'and a['status']=='malformed'
            assert e['admission']=='observed-datagram-padding'and e['session']['Authentication Verified']is False and 'Frames'not in e['session']
            assert e['session']['Datagram Padding Length']==len(raw)
            padding.append(e);continue
        assert canonical(without_generated_id(a))==canonical(without_generated_id(e)),'all application fields and non-generated references stay literal'
        assert a['id']!=e['id'];renumbered+=1
    assert not any(old.values()),'no prior non-fragment PDU is removed'
    wanted={r['Number']for e in old_ip+new_ip+padding for r in e['source_bytes'].get('PacketRefs')or[]};physical={}
    for n,section,interface,link,raw in records(capture):
        if n in wanted:physical[n]=(section,interface,link,raw)
    assert set(physical)==wanted
    diagnostics=[]
    for label,events in[('before',old_ip),('after',new_ip)]:
        for e in events:
            assert e['transport']=='network'and e['flow']==e.get('tcp_flow_id',0)==0 and not e['raw']and not e['source']and not e['destination']and not e['fields']
            refs=e['source_bytes'].get('PacketRefs')or[];assert refs
            checked=[]
            for ref in refs:
                section,interface,link,raw=physical[ref['Number']];p=network(raw,link)
                assert ref['Domain']==e['domain']==dict(Section=section,Interface=interface,Encapsulation=p['encapsulation'])and p['fragment'],'every changed fragment diagnostic owns an actual fragment in its domain'
                checked.append(dict(frame=ref['Number'],raw_SHA256=hashlib.sha256(raw).hexdigest(),observed_IP_version=p['observed_version'],carrier_expected_IP_version=p['expected_version']))
            diagnostics.append(dict(side=label,PDU=e['id'],status=e['status'],reason=e['error'],source_kind=e['source_bytes']['Kind'],domain=e['domain'],source_frames=checked))
    tails=[]
    for e in padding:
        refs=e['source_bytes']['PacketRefs'];assert len(refs)==1
        section,interface,link,raw=physical[refs[0]['Number']];p=network(raw,link);udp=p['UDP'];assert udp and len(udp)==int.from_bytes(udp[4:6],'big')
        w=base64.b64decode(e['raw']);offset=e['session']['Datagram Offset'];assert udp[8+offset:]==w
        tails.append(dict(PDU=e['id'],frame=refs[0]['Number'],offset=offset,length=len(w),raw_SHA256=hashlib.sha256(w).hexdigest()))
    changed_mapping=[dict(before=a,after=b)for a,b in mapping if a!=b]
    invalid=[dict(side=d['side'],PDU=d['PDU'],reason=d['reason'],source=s)for d in diagnostics for s in d['source_frames']if s['observed_IP_version']!=s['carrier_expected_IP_version']]
    return dict(scope=__doc__,capture_SHA256=sha(capture),before_SDK_SHA256=sha(before),after_SDK_SHA256=sha(after),auditor_SHA256=sha(Path(__file__)),source_bound_paired_nonfragment_events=paired,literal_complete_events_unchanged=exact,events_with_only_generated_ID_changes=renumbered,generated_ID_mapping_SHA256=hashlib.sha256(canonical(changed_mapping).encode()).hexdigest(),all_other_application_values_and_connections_literal=True,unprotected_zero_tails=tails,fragment_diagnostic_counts=[dict(side=side,status=status,reason=reason,records=count)for(side,status,reason),count in sorted(collections.Counter((v['side'],v['status'],v['reason'])for v in diagnostics).items())],fragment_diagnostics=diagnostics,known_invalid_IP_version_diagnostics=invalid,scope_limit='Fragment diagnostics are source-bound observations and conservative resource/expiry outcomes; invalid IP version rows remain explicit defects and do not certify valid fragment admission or full Wireshark parity.')


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.before,a.after)
    with a.out.open('x',encoding='utf8',newline='\n')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['fragment_diagnostics','known_invalid_IP_version_diagnostics','scope']}),flush=True)
