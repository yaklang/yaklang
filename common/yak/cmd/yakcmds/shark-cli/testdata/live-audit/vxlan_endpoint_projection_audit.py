"""Literal native-prefix endpoint projection; complete expected CLI rows.

This verifies the CLI117->CLI743 address-only correction, not native admission,
application identity or Wireshark parity. Native event values/IDs/domains remain
literal. Physical IP endpoints are never obtained from an ICMP quotation or an
unselected child. PCAP/NG packet lengths, source clocks and interfaces are read
independently; a capture-short record is never padded to its original length.
"""
import argparse,collections,copy,hashlib,ipaddress,itertools,json,struct,unicodedata
from pathlib import Path
from pptp_parent_source_audit import instant


def sha(path):
    h=hashlib.sha256()
    with path.open('rb')as f:
        for block in iter(lambda:f.read(1<<20),b''):h.update(block)
    return h.hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))
def rows(path):
    with path.open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)
def integer(value):assert type(value)is int;return value
def options(body,at,endian):
    while at+4<=len(body):
        code,size=struct.unpack_from(endian+'HH',body,at);at+=4
        if code==0:return
        assert at+size<=len(body);yield code,body[at:at+size];at+=(size+3)&~3


def capture_records(path):
    data=path.read_bytes()
    if data[:4]in[b'\xd4\xc3\xb2\xa1',b'\xa1\xb2\xc3\xd4',b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d']:
        endian='<'if data[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1']else'>';assert len(data)>=24 and struct.unpack_from(endian+'HH',data,4)==(2,4);link=struct.unpack_from(endian+'I',data,20)[0];scale=1 if data[:4]in[b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d']else 1000;at=24
        while at<len(data):
            assert at+16<=len(data);seconds,fraction,count,original=struct.unpack_from(endian+'IIII',data,at);at+=16;assert count<=original and at+count<=len(data)
            yield dict(wire=data[at:at+count],section=0,interface=0,link=link,time_ns=seconds*1000000000+fraction*scale,original_length=original,captured_length=count);at+=count
        assert at==len(data);return
    at=0;section=-1;interfaces=[];endian=None
    while at<len(data):
        assert at+12<=len(data)
        if data[at:at+4]==b'\x0a\x0d\x0d\x0a':
            magic=data[at+8:at+12];assert magic in[b'\x4d\x3c\x2b\x1a',b'\x1a\x2b\x3c\x4d'];endian='<'if magic[0]==0x4d else'>';section+=1;interfaces=[]
        assert endian is not None;kind,size=struct.unpack_from(endian+'II',data,at);assert size>=12 and size%4==0 and at+size<=len(data)and struct.unpack_from(endian+'I',data,at+size-4)[0]==size;body=data[at+8:at+size-4]
        if kind==1:
            assert len(body)>=8;link=struct.unpack_from(endian+'H',body)[0];ticks=1000000;offset=0
            for code,value in options(body,8,endian):
                if code==9:assert len(value)==1;ticks=(2**(value[0]&127))if value[0]&128 else 10**value[0]
                if code==14:assert len(value)==8;offset=struct.unpack(endian+'q',value)[0]
            interfaces.append((link,ticks,offset))
        elif kind in[2,6]:
            assert len(body)>=20
            if kind==6:interface,high,low,count,original=struct.unpack_from(endian+'IIIII',body)
            else:interface,_,high,low,count,original=struct.unpack_from(endian+'HHIIII',body)
            assert interface<len(interfaces)and count<=original and 20+count<=len(body);link,ticks,offset=interfaces[interface]
            yield dict(wire=body[20:20+count],section=section,interface=interface,link=link,time_ns=((high<<32)|low)*1000000000//ticks+offset*1000000000,original_length=original,captured_length=count)
        elif kind==3:raise AssertionError('unclocked simple packet block needs a separate explicit source profile')
        at+=size
    assert at==len(data)


def ip_string(w):
    address=ipaddress.ip_address(w)
    return str(address.ipv4_mapped)if isinstance(address,ipaddress.IPv6Address)and address.ipv4_mapped is not None else str(address)
def names(value):
    if type(value)is not list or not 1<=len(value)<=128:return[]
    for name in value:
        if type(name)is not str or not name or len(name.encode('utf8',errors='surrogatepass'))>128 or any(unicodedata.category(char)in['Cc','Cf']for char in name):return[]
    return value
def contract(event):
    s=event.get('session')or{}
    if event['protocol']!='ip':return None
    if event['status']=='incomplete'and event['expert']=='IPHeaderTruncated':
        version=s.get('Expected IP Version');required=s.get('Required Header Bytes');available=s.get('Available Header Bytes')
        if version in[4,6]and required==(20 if version==4 else 40)and type(available)is int and 0<=available<required:
            value=names(s.get('Observed Header Layers'));label='IPv'+str(version);return dict(headers=value if value and value[-1]==label else[label],native_headers=True)
    if event['status']=='limited'and event['profile']=='network-encapsulation-refusal'and s.get('Child Header Identity')=='unconfirmed':
        h=names(s.get('Observed Header Layers'))
        gpe=event['completeness']=='headers'and event['expert']=='VXLANProfileUnselected'and s.get('Unselected Carrier')=='VXLAN-GPE'and type(s.get('VXLAN-GPE Version'))is int and s['VXLAN-GPE Version']==0 and type(s.get('VXLAN-GPE Flags'))is int and 0<=s['VXLAN-GPE Flags']<=255 and s['VXLAN-GPE Flags']&8 and not s['VXLAN-GPE Flags']&48 and type(s.get('VXLAN-GPE VNI'))is int and 0<=s['VXLAN-GPE VNI']<1<<24 and type(s.get('VXLAN-GPE Next Protocol'))is int and 0<=s['VXLAN-GPE Next Protocol']<256 and type(s.get('VXLAN-GPE Next Protocol Present'))is bool and bool(s['VXLAN-GPE Flags']&4)==s['VXLAN-GPE Next Protocol Present']==bool(s['VXLAN-GPE Next Protocol'])and s.get('Payload Profile Implemented')is False
        return dict(headers=h if gpe or h and len(h)<=128 and h[-1]in['IPv4','IPv6']else['IP'],native_headers=not gpe)
    if event['status']=='malformed'and event['expert']=='IPVersionMismatch':
        expected=s.get('Expected IP Version');observed=s.get('Observed IP Version')
        if expected in[4,6]and expected!=observed:
            value=names(s.get('Observed Header Layers'));label='IPv'+str(expected);return dict(headers=value if value and len(value)<=128 and value[-1]==label else[label],native_headers=True)
    if event['profile']=='pptp-acknowledgment'and event['status']=='limited'and event['completeness']=='headers'and event['expert']=='PPTPAcknowledgment'and s.get('Observed GRE Version')==1 and s.get('PPTP Payload Length')==0 and s.get('Child Header Identity')=='not present':return dict(headers=names(s.get('Observed Header Layers')),native_headers=False)
    return None


def prefix_headers(record,observed):
    w=record['wire'];at=0;limit=len(w);found=[];link=record['link'];kind={1:'Ethernet',0:'Loopback',108:'Loopback',101:'raw',228:'IPv4',229:'IPv6',113:'Linux SLL',276:'Linux SLL2'}.get(link)
    eth=lambda v:{0x800:'IPv4',0x86dd:'IPv6',0x8100:'Dot1Q',0x88a8:'Dot1Q',0x9100:'Dot1Q',0x8847:'MPLS',0x8848:'MPLS'}.get(v,'LLC'if v<=1500 else None)
    ipnext=lambda v:{4:'IPv4',41:'IPv6',17:'UDP',6:'TCP',47:'GRE',1:'ICMPv4',58:'ICMPv6',0:'IPv6HopByHop',43:'IPv6Routing',44:'IPv6Fragment',60:'IPv6Destination',51:'IPSecAH'}.get(v)
    if kind=='raw':kind='IPv4'if w and w[0]>>4==4 else'IPv6'if w and w[0]>>4==6 else None
    for name in observed:
        if name!=kind or kind is None or at>limit:break
        start=at
        if kind in['ICMPv4','ICMPv6']:break
        if kind=='Ethernet':
            if at+14>limit:break
            code=int.from_bytes(w[at+12:at+14],'big');at+=14
            if code<=1500:limit=min(limit,at+code)
            kind=eth(code)
        elif kind=='Loopback':
            if at+4>limit:break
            family=int.from_bytes(w[at:at+4],'big'if link==108 or w[at:at+2]==b'\0\0'else'little');kind='IPv4'if family==2 else'IPv6'if family in[10,24,28,30]else None;at+=4
        elif kind in['Linux SLL','Linux SLL2']:
            size=16 if kind=='Linux SLL'else 20
            if at+size>limit:break
            code=int.from_bytes(w[at+14:at+16]if size==16 else w[at:at+2],'big');at+=size;kind=eth(code)
        elif kind=='Dot1Q':
            if at+4>limit:break
            code=int.from_bytes(w[at+2:at+4],'big');at+=4
            if code<=1500:limit=min(limit,at+code)
            kind=eth(code)
        elif kind=='IPv4':
            if at+20>limit or w[at]>>4!=4:break
            size=(w[at]&15)*4;total=int.from_bytes(w[at+2:at+4],'big')
            if size<20 or at+size>limit or total<size:break
            found.append(dict(version=4,source=ip_string(w[at+12:at+16]),destination=ip_string(w[at+16:at+20]),start=at,end=at+size,source_range=[at+12,at+16],destination_range=[at+16,at+20]));limit=min(limit,at+total);flags=int.from_bytes(w[at+6:at+8],'big');kind=ipnext(w[at+9]);at+=size
            if flags&0x3fff:break
        elif kind=='IPv6':
            if at+40>limit or w[at]>>4!=6:break
            found.append(dict(version=6,source=ip_string(w[at+8:at+24]),destination=ip_string(w[at+24:at+40]),start=at,end=at+40,source_range=[at+8,at+24],destination_range=[at+24,at+40]));size=int.from_bytes(w[at+4:at+6],'big');kind=ipnext(w[at+6]);at+=40
            if size==0:break
            limit=min(limit,at+size)
        elif kind=='UDP':
            if at+8>limit:break
            destination=int.from_bytes(w[at+2:at+4],'big');size=int.from_bytes(w[at+4:at+6],'big')
            if size<8 or at+size>limit:break
            limit=at+size;at+=8
            if destination!=4789 or limit-at<22 or not w[at]&8:break
            kind='VXLAN'
        elif kind=='VXLAN':
            if at+8>limit or not w[at]&8 or limit-at-8<14:break
            at+=8;kind='Ethernet'
        elif kind=='GRE':
            if at+4>limit:break
            flags=int.from_bytes(w[at:at+2],'big');protocol=int.from_bytes(w[at+2:at+4],'big');at+=4
            if flags&7:break
            if flags&0xc000:at+=4
            if flags&0x2000:at+=4
            if flags&0x1000:at+=4
            if at>limit:break
            if flags&0x4000:
                while at+4<=limit:
                    family=int.from_bytes(w[at:at+2],'big');size=w[at+3];at+=4
                    if family==0 and size==0:break
                    at+=size
                else:break
                if at>limit:break
            kind={0x800:'IPv4',0x86dd:'IPv6',0x6558:'Ethernet'}.get(protocol)
        elif kind in['IPv6HopByHop','IPv6Routing','IPv6Destination','IPSecAH']:
            if at+2>limit:break
            size=(w[at+1]+(2 if kind=='IPSecAH'else 1))*(4 if kind=='IPSecAH'else 8)
            if at+size>limit:break
            kind=ipnext(w[at]);at+=size
        elif kind=='IPv6Fragment':
            if at+8>limit:break
            flags=int.from_bytes(w[at+2:at+4],'big');kind=ipnext(w[at]);at+=8
            if flags&0xfff9:break
        elif kind=='MPLS':
            if at+4>limit:break
            bottom=w[at+2]&1;at+=4;kind='MPLS'if not bottom else'IPv4'if at<limit and w[at]>>4==4 else'IPv6'if at<limit and w[at]>>4==6 else None
        else:break
        assert at>start
    return found


def expected_row(before,events_by_id,record):
    expected=copy.deepcopy(before);active=None;event=None
    for pdu in before.get('pdus')or[]:
        ident=integer(pdu['id'])
        if ident and ident in events_by_id:
            candidate=events_by_id[ident];selected=contract(candidate)
            if selected is not None:active,event=selected,candidate;break
    if active is None:
        assert before.get('visibility')not in['unselected-network-profile','truncated-network-header','invalid-network-header','tunnel-header','acknowledgment-header'],'native refusal metadata must be present in the observed PDU, not inferred from a label'
        return expected,None
    refs=event['source_bytes'].get('PacketRefs')or[];assert any(r['Number']==before['number']and r['Domain']['Section']==record['section']and r['Domain']['Interface']==record['interface']for r in refs)
    headers=prefix_headers(record,active['headers']);source=destination=physical_source=physical_destination=''
    if headers:physical_source,physical_destination=headers[0]['source'],headers[0]['destination'];source,destination=headers[-1]['source'],headers[-1]['destination']
    expected['source'],expected['destination']=source,destination
    for key,value in[('physical_source',physical_source),('physical_destination',physical_destination)]:
        if value:expected[key]=value
        else:expected.pop(key,None)
    if active['native_headers']:
        for key,value in[('message_source',source),('message_destination',destination)]:
            if value:expected[key]=value
            else:expected.pop(key,None)
    return expected,dict(native_PDU=event['id'],expert=event['expert'],observed_prefix=active['headers'],literal_headers=headers,wire_SHA256=hashlib.sha256(record['wire']).hexdigest())


def audit(capture,events,before_cli,cli):
    native={}
    for event in rows(events):
        if contract(event)is not None:ident=integer(event['id']);assert ident not in native;native[ident]=event
    records=iter(capture_records(capture));sentinel=object();counts=collections.Counter();corrections=[];number=0
    for before,after in itertools.zip_longest(rows(before_cli),rows(cli),fillvalue=sentinel):
        assert before is not sentinel and after is not sentinel
        if 'number'not in before:assert canonical(before)==canonical(after)and before.get('record_type')=='protocol_publication';counts['literal_publications']+=1;continue
        number+=1;record=next(records,sentinel);assert record is not sentinel;assert integer(before['number'])==integer(after['number'])==number and instant(before['time'])==record['time_ns']and integer(before['length'])==record['original_length']and integer(before['captured_length'])==record['captured_length']
        expected,evidence=expected_row(before,native,record);assert canonical(expected)==canonical(after),('entire CLI row differs from literal-prefix expected projection',number,sorted(k for k in set(expected)|set(after)if canonical(expected.get(k))!=canonical(after.get(k))or(k in expected)!=(k in after)))
        counts['physical_rows']+=1
        if evidence is not None:counts['source_bound_refusal_rows']+=1;counts['expert:'+evidence['expert']]+=1
        if canonical(before)!=canonical(after):
            counts['corrected_rows']+=1;assert evidence is not None;corrections.append(dict(frame=number,section=record['section'],interface=record['interface'],time_ns=record['time_ns'],before_row_SHA256=hashlib.sha256(canonical(before).encode()).hexdigest(),after_row_SHA256=hashlib.sha256(canonical(after).encode()).hexdigest(),**evidence))
        else:counts['literal_rows']+=1
    assert next(records,sentinel)is sentinel
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),SDK_SHA256=sha(events),before_CLI_SHA256=sha(before_cli),CLI_SHA256=sha(cli),counts=dict(counts),all_rows_equal_complete_literal_expected_projection=True,all_other_values_types_IDs_domains_and_PDUs_literal=True,corrections=corrections)


if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for name in['capture','events','before-cli','cli','out']:p.add_argument('--'+name,type=Path,required=True)
    args=p.parse_args();value=audit(args.capture,args.events,args.before_cli,args.cli)
    with args.out.open('x',encoding='utf8')as f:json.dump(value,f,indent=2);f.write('\n')
    print(json.dumps(dict(counts=value['counts'],proof_SHA256=sha(args.out))),flush=True)
