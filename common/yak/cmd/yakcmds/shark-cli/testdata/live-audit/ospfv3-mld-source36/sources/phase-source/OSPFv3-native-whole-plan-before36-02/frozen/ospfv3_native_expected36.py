"""Source-only whole observer object, with an explicit separately checked ID.

Fields/session and protocol aliases come from the source-frozen independent
oracle, never from a future native result. The small common public SDK schema
and physical MPacket/VLAN domain are explicit here. No wire integers, flow IDs,
timestamps or physical packet numbers are reinterpreted as generated PDU IDs.
"""
import base64,copy,hashlib,ipaddress,zlib
from pathlib import Path


def sha(raw):return hashlib.sha256(raw).hexdigest()


def source_domain(frozen):
    ident=frozen['source_identity']
    raw=Path(frozen['original_frame_file']).read_bytes()
    assert sha(raw)==frozen['original_frame_SHA256']
    link=ident['link'];at=0;encapsulation=''
    assert link in (1,274),'this whole-object constructor selects only the807 source-verified carriers'
    if link==274:
        if raw[0]==0x50:at=1
        while at<len(raw) and raw[at]==0x55:
            at+=1;assert at<=64
        assert at<len(raw) and raw[at]==0xd5
        at+=1
        assert len(raw)-at>=18 and zlib.crc32(raw[at:-4])==int.from_bytes(raw[-4:],'little')
        encapsulation='/mpacket:express'
    assert len(raw)-at>=14
    ether=int.from_bytes(raw[at+12:at+14],'big');at+=14
    while ether in (0x8100,0x88a8):
        assert len(raw)-at>=4
        encapsulation+='/vlan:'+str(int.from_bytes(raw[at:at+2],'big')&4095)
        ether=int.from_bytes(raw[at+2:at+4],'big');at+=4
    assert ether==0x86dd and len(raw)-at>=40 and raw[at]>>4==6
    declared_end=at+40+int.from_bytes(raw[at+4:at+6],'big')
    assert declared_end<=len(raw)
    source=str(ipaddress.IPv6Address(raw[at+8:at+24]))
    destination=str(ipaddress.IPv6Address(raw[at+24:at+40]))
    first,last=frozen['source_message_physical_range']
    assert at+40<=first<=last==declared_end
    wire=raw[first:last]
    assert len(wire)==frozen['source_message_length'] and sha(wire)==frozen['source_message_SHA256']
    return dict(Section=ident['section'],Interface=ident['interface'],Encapsulation=encapsulation),source,destination,wire


def event(identifier,frozen):
    assert type(identifier)is int and identifier>0
    domain,source,destination,wire=source_domain(frozen)
    frame=frozen['source_identity']['frame']
    ref=dict(Number=frame,Domain=copy.deepcopy(domain))
    bs=dict(Kind='captured',SHA256=sha(wire),ParentPDU=0,ParentPDUs=None,PacketRefs=[ref],CompletedIn=copy.deepcopy(ref))
    metadata={'destination':[destination],'flow.id':[0],'ip.addr':[source,destination],
              'pdu.id':[identifier],'pdu.length':[len(wire)],'pdu.completed_in':[frame],
              'protocol':['ospf'],'source':[source],'status':['decoded']}
    values=[]
    for name in sorted(metadata):
        for value in metadata[name]:
            item=dict(name=name,value=value,origin='event')
            if name in ('flow.id','pdu.id','pdu.length','pdu.completed_in'):item['exact_value']=str(value)
            values.append(item)
    for name,occurrences in sorted(frozen['expected_typed_values_by_name'].items()):
        for item in occurrences:
            assert item['name']==name and item['origin']=='pdu'
            assert 'exact_value'not in item,'OSPF32-bit aliases retain the existing public SDK policy'
            values.append(copy.deepcopy(item))
    snapshot=dict(schema_version=1,pdu=identifier,source=copy.deepcopy(bs),values=values,completeness='message')
    fields=copy.deepcopy(frozen['expected_fields'])
    assert fields==frozen['expected_session'] and fields['Checksum Verified']is True and fields['Authentication Verified']is False
    return dict(id=identifier,flow=0,protocol='ospf',status='decoded',profile='ospfv3-bounded-control',completeness='message',
                summary='OSPFv3 '+fields['Packet Name'],expert='',source_bytes=bs,fields=fields,session=copy.deepcopy(fields),
                field_values=snapshot,raw=base64.b64encode(wire).decode(),entry='OSPFv3ControlFields',
                admission='observed-IPv6-next-header-and-complete-control-grammar',source=source,destination=destination,
                direction=0,transport='network',domain=domain,carrier_layers=None,transaction=0,response_to=0)
