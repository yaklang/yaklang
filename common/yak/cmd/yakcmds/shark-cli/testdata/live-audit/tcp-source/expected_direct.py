"""Fresh whole display objects for source-located direct TCP failures.

The fixed20B145 oracle remains frozen separately. In particular short fields
here have explicit per-word capture thresholds, never default zero words.
Discovery supplies only original frame numbers and record hashes; candidate
rows, event maps and Native.Raw never supply expected values or positions.
"""


def field(text,depth=0,branch=False,start=0,end=0,has_range=False):
    return dict(Text=text,Depth=depth,Branch=branch,Start=start,End=end,HasRange=has_range,Source=0)


def whole_row(record,loc,bad,projection,stamp):
    value,absent=projection(record,loc,bad,stamp)
    # This is the frozen --json capture publication mode, not a value copied
    # from a candidate row. No optional Native/PDU/connection fields are added.
    value['pdu_scope']='observed_at_packet'
    return value


def whole_worker(record,loc,bad,quick=False):
    w=record['wire'];start,end=loc['own_offset'],loc['own_end'];fixed=w[start:min(end,start+20)];available=end-start;required=bad['required']
    title=f"Frame #{record['frame']} · {record['original_length']} bytes · {len(w)} captured"if quick else f"Frame {record['frame']}: {record['original_length']} bytes on wire, {len(w)} captured"
    out=[field(title,branch=quick,end=len(w),has_range=bool(w)),field('Captured network prefix',branch=True)]
    for p in loc['network_prefix']:out.append(field(p['label']+' header',depth=1,branch=True,start=p['start'],end=p['end'],has_range=p['end']>p['start']))
    out.extend([field('Source: '+loc['source'],depth=2,start=loc['source_range'][0],end=loc['source_range'][1],has_range=True),field('Destination: '+loc['destination'],depth=2,start=loc['destination_range'][0],end=loc['destination_range'][1],has_range=True),
                field('TCP header observation',branch=True,start=start,end=min(end,start+max(20,required)),has_range=end>start),field('Status: '+bad['status'],depth=1),field('Error: '+bad['reason'],depth=1)])
    words=[('Source Port',0,2),('Destination Port',2,2),('Sequence Number',4,4),('Acknowledgment Number',8,4),('Window',14,2),('Checksum (unverified)',16,2),('Urgent Pointer',18,2)]
    for name,offset,size in words:
        if offset+size<=len(fixed):out.append(field(name+': '+str(int.from_bytes(fixed[offset:offset+size],'big')),depth=1,start=start+offset,end=start+offset+size,has_range=True))
    if len(fixed)>=13:out.append(field(f'Data Offset (32-bit words): {fixed[12]>>4}',depth=1,start=start+12,end=start+13,has_range=True))
    if len(fixed)>=14:
        out.extend([field(f'TCP Flags (reserved bits excluded): 0x{((fixed[12]&1)<<8)|fixed[13]:03x}',depth=1,start=start+12,end=start+14,has_range=True),field(f'Reserved Bits: {(fixed[12]>>1)&7}',depth=1,start=start+12,end=start+13,has_range=True)])
    out.extend([field(f'Available TCP Bytes: {available}',depth=1),field('Minimum TCP Fixed Header Bytes: 20',depth=1),field(f'Declared TCP Header Bytes: {required}'if len(fixed)>=13 else'Declared TCP Header Bytes: unavailable',depth=1)])
    if loc['zero_total_legacy']:out.append(field('Parent scope: IPv4 total length zero; legacy captured-prefix observation, not a valid declared IPv4 extent',depth=1))
    for option in bad.get('options',[]):out.append(field(option['label'],depth=1,start=option['start'],end=option['end'],has_range=True))
    if bad['complete']:out.append(field(f'Captured TCP Payload: {available-required} bytes; not admitted to reassembly',depth=1,start=start+required,end=end,has_range=end>start+required))
    else:
        out.append(field('TCP Payload Boundary: unknown; the fixed/header-options prefix is incomplete or invalid',depth=1))
        if required>available and available>20:out.append(field(f'Captured TCP option prefix: {available-20} bytes; header incomplete',depth=1,start=start+20,end=end,has_range=True))
    out.append(field('Observation scope: captured header fields only; no connection, application, MPTCP negotiation or checksum validation',depth=1))
    assert all(x['Source']==0 and(not x['HasRange']or 0<=x['Start']<x['End']<=len(w))for x in out)
    return dict(Number=record['frame'],Protocol='TCP',Sources=None,Fields=out)
