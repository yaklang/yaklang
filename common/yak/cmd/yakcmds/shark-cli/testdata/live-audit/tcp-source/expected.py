"""TCP145 whole CLI/worker expectations, frozen before candidate binary output.

All source values/coordinates come from reread original capture records. Text
and branch order implement the frozen CLI interface, not candidate responses.
No baseline row is copied or path-whitelisted to manufacture an expectation.
"""
import hashlib


def field(text,depth=0,branch=False,start=0,end=0,has_range=False):
    return dict(Text=text,Depth=depth,Branch=branch,Start=start,End=end,HasRange=has_range,Source=0)


def mptcp_failure(b):
    if len(b)<3:return 'MPTCP option subtype unavailable'
    sub=b[2]>>4;name=None;allowed=[]
    if sub==0:name='MP_CAPABLE';allowed=[4,12,20,22,24]
    elif sub==1:name='MP_JOIN';allowed=[12,16,24]
    elif sub==2:
        name='DSS'
        if len(b)<4:return 'MPTCP DSS flags unavailable'
        flags=b[3];n=4
        if flags&1:n+=8 if flags&2 else 4
        if flags&4:
            n+=(8 if flags&8 else 4)+4+2;allowed=[n,n+2]
        else:allowed=[n]
    elif sub==4:
        if len(b)>=4:return None
        name='REMOVE_ADDR';allowed=[4]
    elif sub==5:name='MP_PRIO';allowed=[3,4]
    elif sub==6:name='MP_FAIL';allowed=[12]
    elif sub==7:name='MP_FASTCLOSE';allowed=[12]
    elif sub==8:name='MP_TCPRST';allowed=[4]
    else:return None
    if len(b)in allowed:return None
    return f"MPTCP {name} option length {len(b)} inconsistent with its fields (expected ["+' '.join(map(str,allowed))+'])'


def own_failure(wire,loc):
    at,end=loc['own_offset'],loc['own_end'];body=wire[at:end];assert len(body)>=20
    required=(body[12]>>4)*4;options=[]
    if required<20:return dict(status='malformed',reason=f'TCP data offset {body[12]>>4} words is below the 5-word fixed header',required=required,complete=False,options=options)
    if required>len(body):return dict(status='incomplete',reason=f'TCP header incomplete ({len(body)}/{required} bytes)',required=required,complete=False,options=options)
    p=20
    while p<required:
        kind=body[p];remaining=required-p
        if kind in[0,1]:
            options.append(dict(label='End of option list'if kind==0 else'No-operation',start=at+p,end=at+p+1))
            if kind==0:break
            p+=1;continue
        if remaining<2:
            options.append(dict(label=f'Option {kind}: length byte unavailable',start=at+p,end=at+p+1))
            return dict(status='malformed',reason=f'TCP option {kind} length byte unavailable at header offset {p}',required=required,complete=True,options=options)
        n=body[p+1]
        if n<2 or n>remaining:
            options.append(dict(label=f'Rejected option prefix: kind {kind}, length {n}',start=at+p,end=at+p+2))
            return dict(status='malformed',reason=f'TCP option {kind} length {n} invalid at header offset {p} ({remaining} header bytes available)',required=required,complete=True,options=options)
        options.append(dict(label=f'Option {kind}: {n} bytes',start=at+p,end=at+p+n))
        why=mptcp_failure(body[p:p+n])if kind==30 else None
        if why:return dict(status='malformed',reason=why+f' at TCP header offset {p}',required=required,complete=True,options=options)
        p+=n
    return None


def cli(record,loc,stamp):
    w=record['wire'];at,end=loc['own_offset'],loc['own_end'];body=w[at:end];bad=own_failure(w,loc);assert bad
    ip=loc['ip'];info=bad['reason']
    info+=f"; captured TCP payload={len(body)-bad['required']} bytes (header boundary observed)"if bad['complete']else'; payload boundary unknown'
    if ip['total']==0:info+='; IPv4 total length zero: legacy captured-prefix observation'
    if record['original_length']>record['captured_length']:info+=f" [truncated capture: {record['captured_length']}/{record['original_length']} bytes]"
    value=dict(capture_domain=dict(Section=record['section'],Interface=record['interface'],Encapsulation=''),pdu_scope='observed_at_packet',protocol_scope='captured TCP header diagnostic',layers=['Ethernet','IPv4','TCP'],protocol_stack=['Ethernet','IPv4','TCP'],
               number=record['frame'],time=stamp(record['time_ns']),source=ip['source'],destination=ip['destination'],protocol='TCP',transport='tcp',length=record['original_length'],captured_length=record['captured_length'],info=info,protocol_status=bad['status'],visibility='headers',protocol_evidence='captured TCP header failure; segment not admitted to a connection or application')
    for name,offset in [('source_port',0),('destination_port',2)]:
        n=int.from_bytes(body[offset:offset+2],'big')
        if n:value[name]=n
    return value


def worker(record,loc,quick=False):
    w=record['wire'];at,end=loc['own_offset'],loc['own_end'];body=w[at:end];bad=own_failure(w,loc);assert bad
    ip=loc['ip'];required=bad['required'];available=len(body)
    title=f"Frame #{record['frame']} · {record['original_length']} bytes · {len(w)} captured"if quick else f"Frame {record['frame']}: {record['original_length']} bytes on wire, {len(w)} captured"
    values=[field(title,branch=quick,end=len(w),has_range=bool(w)),field('Captured network prefix',branch=True),field('Ethernet header',depth=1,branch=True,start=0,end=14,has_range=True),field('IPv4 header',depth=1,branch=True,start=14,end=at,has_range=True),
            field('Source: '+ip['source'],depth=2,start=26,end=30,has_range=True),field('Destination: '+ip['destination'],depth=2,start=30,end=34,has_range=True),
            field('TCP header observation',branch=True,start=at,end=min(end,at+max(20,required)),has_range=True),field('Status: '+bad['status'],depth=1),field('Error: '+bad['reason'],depth=1)]
    for key,offset,size in [('Source Port',0,2),('Destination Port',2,2),('Sequence Number',4,4),('Acknowledgment Number',8,4),('Window',14,2),('Checksum (unverified)',16,2),('Urgent Pointer',18,2)]:
        values.append(field(f'{key}: '+str(int.from_bytes(body[offset:offset+size],'big')),depth=1,start=at+offset,end=at+offset+size,has_range=True))
    values.extend([field(f'Data Offset (32-bit words): {body[12]>>4}',depth=1,start=at+12,end=at+13,has_range=True),field(f'TCP Flags (reserved bits excluded): 0x{((body[12]&1)<<8)|body[13]:03x}',depth=1,start=at+12,end=at+14,has_range=True),field(f'Reserved Bits: {(body[12]>>1)&7}',depth=1,start=at+12,end=at+13,has_range=True),
                   field(f'Available TCP Bytes: {available}',depth=1),field('Minimum TCP Fixed Header Bytes: 20',depth=1),field(f'Declared TCP Header Bytes: {required}',depth=1)])
    if ip['total']==0:values.append(field('Parent scope: IPv4 total length zero; legacy captured-prefix observation, not a valid declared IPv4 extent',depth=1))
    for option in bad['options']:values.append(field(option['label'],depth=1,start=option['start'],end=option['end'],has_range=True))
    if bad['complete']:values.append(field(f'Captured TCP Payload: {available-required} bytes; not admitted to reassembly',depth=1,start=at+required,end=end,has_range=end>at+required))
    else:
        values.append(field('TCP Payload Boundary: unknown; the fixed/header-options prefix is incomplete or invalid',depth=1))
        if required>available and available>20:values.append(field(f'Captured TCP option prefix: {available-20} bytes; header incomplete',depth=1,start=at+20,end=end,has_range=True))
    values.append(field('Observation scope: captured header fields only; no connection, application, MPTCP negotiation or checksum validation',depth=1))
    assert all(x['Source']==0 and (not x['HasRange']or 0<=x['Start']<x['End']<=len(w)) for x in values)
    return dict(Protocol='TCP',Number=record['frame'],Sources=None,Fields=values)
