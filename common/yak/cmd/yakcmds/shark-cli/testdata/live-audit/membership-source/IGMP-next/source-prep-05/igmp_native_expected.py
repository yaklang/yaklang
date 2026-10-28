"""Whole events-observer object expected from original source + an explicit ID.

The ID is provided by a separately validated native publication bijection. It
is never inferred by endpoint/time windows or silently removed from comparison.
This constructor describes proposal-01, not an observed candidate success.
"""
import base64
import copy
import hashlib
import zlib

from igmp_expected import decode
from igmp_carrier_source import locate


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b


def literal_capture_encapsulation(record,location):
    """Physical MPacket express namespace from original link+delimiter+FCS."""
    if record['link']!=274:
        assert not any(h['kind']=='FPP'for h in location['headers'])
        return ''
    w=record['wire'];at=1 if w and w[0]==0x50 else 0
    while at<len(w)and w[at]==0x55:
        at+=1
        assert at<=64,'MPacket preamble bound'
    assert at<len(w)and w[at]==0xd5,'MPacket express delimiter'
    at+=1
    assert len(w)-at>=18,'MPacket complete Ethernet plus FCS'
    assert zlib.crc32(w[at:-4])==int.from_bytes(w[-4:],'little'),'MPacket literal FCS'
    assert same(location['headers'][0],dict(kind='FPP',offset=0,length=at))
    return '/mpacket:express'


def event(identifier,record,source):
    assert type(identifier)is int and identifier>0
    assert hashlib.sha256(record['wire']).hexdigest()==source['wire_SHA256']
    assert same({k:v for k,v in record.items()if k not in('wire','frame')},source['record'])
    assert source['selected_membership_profile']
    assert same(locate(record),source['location'])
    location=source['location'];start,end=location['offset'],location['end']
    result=decode(record['wire'][start:end],source['declared_IGMP_length'])
    assert same(result,source['whole_codec_expected'])
    raw_start,raw_end=source['source1_recipe']['raw_start'],source['source1_recipe']['raw_end']
    assert raw_start==0 and raw_end==len(record['wire']),'initial native profile is direct single IPv4'
    raw=record['wire'][raw_start:raw_end];status=result['status']
    ip=next(h for h in reversed(location['headers'])if h['kind']=='IPv4')
    domain=dict(Section=record['section'],Interface=record['interface'],Encapsulation=literal_capture_encapsulation(record,location))
    ref=dict(Number=source['frame'],Domain=copy.deepcopy(domain))
    byte_source=dict(Kind='captured',SHA256=hashlib.sha256(raw).hexdigest(),ParentPDU=0,ParentPDUs=None,PacketRefs=[ref])
    if status=='decoded':byte_source['CompletedIn']=copy.deepcopy(ref)
    fields=copy.deepcopy(result['fields'])
    def shift(value):
        if isinstance(value,dict):
            for key,child in value.items():
                if key=='Wire Field Ranges':
                    for word in child.values():word[0]+=start;word[1]+=start
                else:shift(child)
        elif isinstance(value,list):
            for child in value:shift(child)
    shift(fields)
    # Event facts are reserved before protocol arrays, each group name-sorted.
    metadata={'destination':[ip['destination']],'flow.id':[0],'ip.addr':[ip['source'],ip['destination']],
              'pdu.id':[identifier],'pdu.length':[len(raw)],'protocol':['igmp'],'source':[ip['source']],'status':[status]}
    if status=='decoded':metadata['pdu.completed_in']=[source['frame']]
    values=[]
    for name in sorted(metadata):
        for value in metadata[name]:
            item=dict(name=name,value=value,origin='event')
            if name in('flow.id','pdu.id','pdu.length','pdu.completed_in'):item['exact_value']=str(value)
            values.append(item)
    for v in sorted((v for v in result['values']if v['name']),key=lambda x:x['name']):
        values.append(dict(name=v['name'],value=v['value'],origin='pdu',pdu_range=dict(offset=start+v['offset'],length=v['length'])))
    completeness='message'if status=='decoded'else status
    snapshot=dict(schema_version=1,pdu=identifier,source=copy.deepcopy(byte_source),values=values,completeness=completeness)
    if completeness=='limited':snapshot['incomplete']=True
    expected=dict(id=identifier,flow=0,protocol='igmp',status=status,profile='igmp-membership-wire',completeness=completeness,summary=result['summary'],expert=result['expert'],
                  source_bytes=byte_source,fields=copy.deepcopy(fields),session=copy.deepcopy(fields),field_values=snapshot,raw=base64.b64encode(raw).decode(),
                  entry='IGMPFields',admission='observed-IPv4-protocol2-and-complete-membership-grammar'if status=='decoded'else'observed-IPv4-protocol2-only',
                  source=ip['source'],destination=ip['destination'],direction=0,transport='network',domain=domain,carrier_layers=None,transaction=0,response_to=0)
    if result['expert']:expected['error']=result['expert']
    return expected
