"""Complete CLI row constructor from frozen original sources and a pure contract.

The identifier argument is an explicit publisher-ledger parameter. ID1 used
while freezing is a placeholder, never a claim about a runtime PDU number.
No candidate decoder or candidate output is read by this module.
"""
import copy,hashlib,zlib
from pathlib import Path
import native_source_schema

BASIC_KEYS=frozenset(('capture_domain','pdu_scope','protocol_scope','layers',
                     'number','time','source','destination','protocol','length',
                     'captured_length','info'))
EVIDENCE='declared IPv6 next-header and bounded OSPFv3 wire observations'
SCOPE='observed control message; advertisements only'


def field_map(snapshot):
    result={}
    for value in snapshot['values']:
        # The native integer is represented exactly in JSON. Existing >53-bit
        # aliases additionally preserve exact_value; OSPF aliases are <=32bit.
        result.setdefault(value['name'],[]).append(copy.deepcopy(value['value']))
    return result


def physical_layers(frozen):
    ident=frozen['source_identity'];raw=Path(frozen['original_frame_file']).read_bytes()
    assert hashlib.sha256(raw).hexdigest()==frozen['original_frame_SHA256']
    at=0;names=[]
    if ident['link']==274:
        names.append('FPP')
        if raw[0]==0x50:at=1
        while at<len(raw) and raw[at]==0x55:
            at+=1;assert at<=64
        assert raw[at]==0xd5;at+=1
        assert zlib.crc32(raw[at:-4])==int.from_bytes(raw[-4:],'little')
    else:assert ident['link']==1
    names.append('Ethernet');assert len(raw)-at>=14
    ether=int.from_bytes(raw[at+12:at+14],'big');at+=14
    while ether in(0x8100,0x88a8):
        names.append('Dot1Q');assert len(raw)-at>=4
        ether=int.from_bytes(raw[at+2:at+4],'big');at+=4
    assert ether==0x86dd and raw[at]>>4==6
    names.append('IPv6');end=at+40+int.from_bytes(raw[at+4:at+6],'big')
    assert end<=len(raw);next_header=raw[at+6];at+=40
    # Exactly the extension forms independently observed among these807.
    while next_header==51:
        assert end-at>=12
        size=(raw[at+1]+2)*4;assert size>=12 and at+size<=end
        names.append('IPSecAH');next_header=raw[at];at+=size
    assert next_header==89 and [at,end]==frozen['source_message_physical_range']
    names.append('OSPF');return names


def pdu(identifier,frozen):
    e=native_source_schema.event(identifier,frozen)
    assert not e['expert'] and e['status']=='decoded' and e['response_to']==0
    values=copy.deepcopy(e['field_values'])
    row=dict(id=identifier,protocol='ospf',status='decoded',completeness='message',
             response_to=0,byte_source=copy.deepcopy(e['source_bytes']),
             fields=field_map(values),field_values=values)
    return row


def row(before,frozen,identifier):
    assert set(before)==BASIC_KEYS and before['pdu_scope']=='observed_at_packet'
    assert before['protocol_scope']=='packet'
    assert before['number']==frozen['source_identity']['frame']
    assert before['captured_length']==len(Path(frozen['original_frame_file']).read_bytes())
    domain,source,destination,_=native_source_schema.source_domain(frozen)
    assert before['capture_domain']==domain
    assert before['source']==source and before['destination']==destination
    layers=physical_layers(frozen)
    assert before['layers'] in(layers,layers[:-1])
    assert before['protocol']==before['layers'][-1]
    assert before['info'] in('',' [incomplete/undecoded]')
    assert (before['layers']==layers)==(before['info']=='')
    result=copy.deepcopy(before)
    result['pdus']=[pdu(identifier,frozen)]
    result['layers']=layers
    result['protocol_stack']=['OSPFv3'if x=='OSPF'else x for x in layers]
    result['protocol']='OSPFv3';result['visibility']='message'
    result['protocol_status']='decoded';result['protocol_evidence']=EVIDENCE
    result['protocol_scope']=SCOPE
    fields=frozen['expected_fields'];header=fields['Header']
    result['info']='OSPFv3 '+fields['Packet Name']+' · router='+header['Router ID']+' area='+header['Area ID']+' instance='+str(header['Instance ID'])
    # No transport/flow/transaction, ownership or endpoint association is
    # inferred from a router advertisement or an AH header.
    return result
