"""Independent selected direct-TCP DCE/RPC outer-field and provenance audit.

Decode original PCAP/PCAPNG and TCP sequence ownership without Yak codecs. Bind
every selected native byte, contributor and domain, compare literal reference
headers and security-envelope fields, and list unexported complete wire PDUs.
Opaque security bodies do not certify authentication, alignment or NDR semantics.
Reference carriers outside the selected direct TCP tuples remain separate.
"""
import argparse
import base64
import collections
import hashlib
import json
import struct
import uuid
from pathlib import Path

from mail_rtsp_public_wire_audit import records as pcapng_records, tcp_packet
from mail_rtsp_export_invariants import rows as sdk_rows


def sha(w):return hashlib.sha256(w).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def b64(w):return base64.b64encode(w).decode()


def invalid_clear_body(raw):
    """Locate an actual declared-array mismatch without applying Yak limits.

    Contexts contain a 24-byte fixed part plus their declared 20-byte transfer
    syntax array. Results are 24 bytes following the aligned secondary address.
    This is a wire-boundary check, not certification of arbitrary NDR semantics.
    """
    body=raw[16:]
    if raw[2]in[11,14]:
        if len(body)<12:return dict(reason='context-list fixed header exceeds body')
        count=body[8];at=12
        for index in range(count):
            if at+24>len(body):return dict(reason='declared context fixed part exceeds body',index=index,declared_contexts=count)
            transfers=body[at+2];end=at+24+20*transfers
            if transfers==0:return dict(reason='context has no transfer syntax',index=index)
            if end>len(body):return dict(reason='declared transfer array exceeds body',index=index,declared_transfers=transfers,required_end=end+16,available_end=len(raw))
            at=end
        if at!=len(body):return dict(reason='bytes after declared context array',extra_bytes=len(body)-at,declared_contexts=count)
    if raw[2]in[12,15]:
        if len(body)<10:return dict(reason='result-list fixed header exceeds body')
        address=int.from_bytes(body[8:10],'little');at=(10+address+3)&~3
        if at+4>len(body):return dict(reason='secondary address or result header exceeds body')
        count=body[at];end=at+4+24*count
        if end!=len(body):return dict(reason='declared result array does not occupy body',declared_results=count,required_end=end+16,available_end=len(raw),extra_bytes=len(body)-end)
        for index,pos in enumerate(range(at+4,end,24)):
            result=int.from_bytes(body[pos:pos+2],'little')
            if result>3:return dict(reason='invalid presentation context result code',index=index,result=result)
    return None


def clear_wire_fields(raw):
    """Decode selected clear fixed fields independently of shared/native rules.

    The historical result projection keeps its first result structured and the
    remaining results as exact bytes. This check verifies that public contract;
    it does not count those bytes as interpreted NDR result semantics.
    """
    ptype=raw[2];body=raw[16:];ref={};f={}
    u16=lambda at:int.from_bytes(body[at:at+2],'little')
    u32=lambda at:int.from_bytes(body[at:at+4],'little')
    guid=lambda value:str(uuid.UUID(bytes_le=value))
    if ptype in[11,14]:
        assert invalid_clear_body(raw)is None
        count=body[8];contexts=[];at=12
        f={'Max Xmit Frag':u16(0),'Max Recv Frag':u16(2),'Assoc Group':u32(4),'Num Ctx Items':count,'Reserved':body[9],'Reserved2':u16(10)}
        ref={'dcerpc.cn_max_xmit':u16(0),'dcerpc.cn_max_recv':u16(2),'dcerpc.cn_assoc_group':u32(4),'dcerpc.cn_num_ctx_items':count}
        array=collections.defaultdict(list)
        for _ in range(count):
            assert body[at+2]==1,'selected clear profile has one transfer syntax per context'
            contexts.append({'Context ID':u16(at),'Num Trans Items':body[at+2],'Reserved':body[at+3],'Abstract Syntax':b64(body[at+4:at+20]),'Abstract Version':u32(at+20),'Transfer Syntax':b64(body[at+24:at+40]),'Transfer Version':u32(at+40)})
            for name,value in [('dcerpc.cn_ctx_id',u16(at)),('dcerpc.cn_num_trans_items',body[at+2]),('dcerpc.cn_bind_to_uuid',guid(body[at+4:at+20])),('dcerpc.cn_bind_if_ver',u16(at+20)),('dcerpc.cn_bind_if_ver_minor',u16(at+22)),('dcerpc.cn_bind_trans_id',guid(body[at+24:at+40])),('dcerpc.cn_bind_trans_ver',u32(at+40))]:array[name].append(value)
            at+=44
        if contexts:f['Contexts']=contexts
        ref.update(array);name='Bind'
    elif ptype in[12,15]:
        assert invalid_clear_body(raw)is None
        address=u16(8);at=(10+address+3)&~3;count=body[at]
        f={'Max Xmit Frag':u16(0),'Max Recv Frag':u16(2),'Assoc Group':u32(4),'Sec Addr Len':address}
        if address:f['Sec Addr']=b64(body[10:10+address])
        if at>10+address:f['Pad']=b64(body[10+address:at])
        results={'Num Results':count,'Reserved':body[at+1],'Reserved2':u16(at+2)}
        if count:
            results.update({'Ack Result':u16(at+4),'Ack Reason':u16(at+6),'Transfer Syntax':b64(body[at+8:at+24]),'Transfer Version':u32(at+24)})
        if count>1:results['Additional Results']=b64(body[at+28:])
        f['DCERPCResults']=results;name='BindAck'
        ref={'dcerpc.cn_max_xmit':u16(0),'dcerpc.cn_max_recv':u16(2),'dcerpc.cn_assoc_group':u32(4),'dcerpc.cn_sec_addr_len':address,'dcerpc.cn_num_results':count,
             'dcerpc.cn_ack_result':[u16(pos)for pos in range(at+4,len(body),24)],'dcerpc.cn_ack_trans_id':[guid(body[pos+4:pos+20])for pos in range(at+4,len(body),24)],'dcerpc.cn_ack_trans_ver':[u32(pos+20)for pos in range(at+4,len(body),24)]}
    elif ptype in[0,2,3]:
        assert len(body)>=8
        f={'Alloc Hint':u32(0),'Context ID':u16(4)}
        ref={'dcerpc.cn_alloc_hint':u32(0),'dcerpc.cn_ctx_id':u16(4)}
        if ptype==0:
            f['OpNum']=u16(6);ref['dcerpc.opnum']=u16(6);name='Request';at=8
            if raw[3]&0x80:
                assert len(body)>=24
                f['Object UUID']=b64(body[8:24]);at=24
        else:
            f.update({'Cancel Count':body[6],'Reserved':body[7]});name='Response';at=8
            if ptype==3:
                assert len(body)>=16
                name='Fault';f.update({'Status':u32(8),'Reserved2':u32(12)});at=16
        if at<len(body):f['Stub']=b64(body[at:])
    else:return None,{}
    root={'RPC Vers':raw[0],'RPC Vers Minor':raw[1],'PType':ptype,'PFC Flags':raw[3],'Packed Drep':b64(raw[4:8]),'Frag Length':len(raw),'Auth Length':0,'Call ID':int.from_bytes(raw[12:16],'little'),'PDU':{name:f}}
    return root,ref


def records(capture):
    wire=capture.read_bytes()
    if wire[:4]==b'\x0a\x0d\x0d\x0a':yield from pcapng_records(capture);return
    assert wire[:4]in[b'\xd4\xc3\xb2\xa1',b'\xa1\xb2\xc3\xd4',b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d'],'classic microsecond/nanosecond PCAP'
    endian='<'if wire[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1']else'>'
    assert len(wire)>=24
    assert struct.unpack_from(endian+'HH',wire,4)==(2,4)
    link=struct.unpack_from(endian+'I',wire,20)[0];at=24;number=0
    while at<len(wire):
        assert at+16<=len(wire);_,_,n,original=struct.unpack_from(endian+'IIII',wire,at);at+=16
        assert n<=original and at+n<=len(wire);number+=1
        yield number,0,0,link,wire[at:at+n];at+=n
    assert at==len(wire)


def audit(capture,events,oracle,receipts=None,require_scoped_ranges=False,require_call_links=False,control_manifest=None):
    checks=0
    def check(ok,why):
        nonlocal checks
        checks+=1;assert ok,why
    def eq(a,b,why):check(canonical(a)==canonical(b),why)
    def connection(e):return canonical([e['domain'],sorted([e['source'],e['destination']])])
    def direction(e):return canonical([e['domain'],e['source'],e['destination']])
    def leaves(node,name):
        if isinstance(node,dict):
            for key,value in node.items():
                if key==name:yield from value if isinstance(value,list)else[value]
                elif isinstance(value,(dict,list)):yield from leaves(value,name)
        elif isinstance(node,list):
            for value in node:yield from leaves(value,name)
    control_refusals={}
    if control_manifest is not None:
        from dcerpc_generation_control_audit import original_control_refusals
        control_refusals=original_control_refusals(capture,control_manifest)
    native=[];identities=set();SDK_records=0
    for e in sdk_rows(events):
        check(type(e['id'])is int and e['id']>0,'literal positive SDK identity')
        check(e['id']not in identities,'unique SDK identities across all protocols')
        identities.add(e['id']);SDK_records+=1
        if e['protocol']=='dcerpc':native.append(e)
    wanted={connection(e)for e in native if e.get('raw')};check(bool(wanted),'nonempty selected scope')
    streams={};physical={};total_frames=0
    for no,section,interface,link,frame in records(capture):
        total_frames=no;decoded=frame;decode_link=link
        if link==0:
            check(len(frame)>=4,'complete loopback family')
            if frame[:4]not in[b'\x02\0\0\0',b'\0\0\0\x02']:continue
            decoded=bytes(12)+b'\x08\0'+frame[4:];decode_link=1
        packet=tcp_packet(decoded,decode_link,section,interface)
        if packet is None or connection(packet)not in wanted:continue
        physical[no]=dict(packet,frame_sha256=sha(frame))
        state=streams.setdefault(direction(packet),dict(meta={k:packet[k]for k in ['domain','source','destination']},packets=[],syns=[],cursor=0))
        if packet['syn']:state['syns'].append((packet['sequence']+1)&0xffffffff)
        if packet['payload']:state['packets'].append((packet['sequence']+int(packet['syn']),packet['payload'],no))
    for state in streams.values():
        if not state['packets']:
            state.update(cells={},owners={},contiguous=0);continue
        anchor=state['packets'][0][0]
        signed=lambda n:((n-anchor+(1<<31))&0xffffffff)-(1<<31)
        # With no captured SYN the observed origin is the first arriving byte.
        # Later fuzz samples may reuse the tuple with a lower sequence number;
        # they cannot retroactively establish a previously unobserved prefix.
        base=state['syns'][0]if state['syns']else anchor
        check(all(v==base for v in state['syns']),'no TCP generation conflation')
        cells={};owners={};before_origin=[];delivered=0;deliveries=[]
        for seq,data,no in state['packets']:
            start=((seq-base+(1<<31))&0xffffffff)-(1<<31)
            if start<0:
                before_origin.append(dict(frame=no,relative_sequence=start,bytes_before_origin=min(-start,len(data)),payload_sha256=sha(data)))
                data=data[min(-start,len(data)):];start=0
            check(start+len(data)<=4<<20,'bounded independent stream')
            for at,byte in enumerate(data,start):
                check(at not in cells or cells[at]==byte,'no conflicting selected TCP overlaps')
                cells[at]=byte;owners.setdefault(at,no)
            previous_delivery=delivered
            while delivered in cells:delivered+=1
            if delivered>previous_delivery:deliveries.append((delivered,no))
        contiguous=0
        while contiguous in cells:contiguous+=1
        state.update(cells=cells,owners=owners,contiguous=contiguous,prefix_deliveries=deliveries,before_origin=before_origin,captured_SYN=bool(state['syns']))
    reference_views=collections.defaultdict(list);outside=[];aggregate_reference_limits=[]
    for row in json.loads(oracle.read_text(encoding='utf-8-sig')):
        layers=row['_source']['layers'];no=int(layers['frame']['frame.number'])
        if no not in physical:outside.append(no);continue
        eq(sha(bytes.fromhex(layers['frame_raw'][0])),physical[no]['frame_sha256'],'literal reference physical frame')
        raw=layers.get('dcerpc_raw')or[]
        if raw and isinstance(raw[0],str):raw=[raw]
        decoded=layers.get('dcerpc',{})
        if isinstance(decoded,list):
            check(len(decoded)==len(raw),'reference per-PDU raw/tree cardinality')
            paired=zip(raw,decoded)
        elif len(raw)==1:paired=[(raw[0],decoded)]
        else:
            aggregate_reference_limits.append(dict(frame=no,raw_views=len(raw),reason='reference fields are aggregated without a unique per-PDU tree'))
            paired=[]
        for span,view in paired:reference_views[no].append((bytes.fromhex(span[0]),view))
    compared=[];closures=[];prefixes=[];tails=[];reference_fields=collections.Counter();reference_limits=[];used_views=set();status=collections.Counter();receipt_matches=[];adjudications=[];complete_clear_trees=0;reference_field_limits=[];proposals={};association_checks=0;pdu_deliveries={}
    for e in native:
        status[e['status']]+=1
        if not e.get('raw'):
            check(e['status']in ['incomplete','context-required'],'empty event is an explicit diagnostic');closures.append(e['id']);continue
        raw=base64.b64decode(e['raw'],validate=True);state=streams[direction(e)];start=state['cursor'];wire_start=start;end=start+len(raw)
        snapshot=e['field_values'];eq(snapshot['pdu'],e['id'],'typed field snapshot belongs to this PDU')
        identity_fields=[v for v in snapshot['values']if v['name']=='pdu.id']
        eq(len(identity_fields),1,'one typed PDU identity');eq(identity_fields[0]['value'],e['id'],'typed identity literal value');eq(identity_fields[0]['exact_value'],str(e['id']),'typed identity exact decimal')
        check(end<=state['contiguous'],('no native bytes cross a captured TCP hole',e['id'],start,end,state['contiguous']))
        eq(bytes(state['cells'][i]for i in range(start,end)).hex(),raw.hex(),'complete PDU bytes equal independent captured TCP bytes')
        delivery=next(frame for prefix,frame in state['prefix_deliveries']if prefix>=end)
        pdu_deliveries[e['id']]=delivery
        state['cursor']=end;source=e['source_bytes'];owners=sorted({state['owners'][i]for i in range(start,end)})
        eq(source['SHA256'],sha(raw),'literal source raw SHA')
        eq(sorted(r['Number']for r in source['PacketRefs']),owners,'complete original contributor frames')
        for ref in source['PacketRefs']:
            eq(ref['Domain'],e['domain'],'native contributor domain');eq(physical[ref['Number']]['domain'],e['domain'],'original VLAN/interface/section domain')
        if source.get('CompletedIn'):
            eq(source['CompletedIn']['Number'],max(owners),'actual last byte-arrival frame');eq(source['CompletedIn']['Domain'],e['domain'],'completion domain')
        if e['status']=='incomplete'and not e.get('entry'):
            check(bool(e.get('fields_error'))and not e.get('fields')and not e.get('session'),'captured prefix remains an explicit field-query refusal')
            check(raw[:min(2,len(raw))]==b'\x05\0'[:min(2,len(raw))],'captured incomplete RPC prefix is not an unrelated message')
            check(len(raw)<16 or int.from_bytes(raw[8:10],'little')>len(raw),'incomplete prefix cannot contain a whole declared RPC PDU')
            eq(e['response_to'],0,'an incomplete prefix has no fabricated request link')
            prefixes.append(dict(PDU=e['id'],physical_frames=owners,captured_bytes=len(raw),raw_sha256=sha(raw)));continue
        check(len(raw)>=16 and raw[:2]==b'\x05\0'and raw[4:8]==b'\x10\0\0\0','selected connection-oriented little-endian header')
        size,auth,call=struct.unpack_from('<HHI',raw,8)
        if size!=len(raw):
            check(e['status']=='context-required'and not e.get('entry'),'aggregate stopped-direction bytes are explicit non-PDU diagnostics')
            tails.append(dict(PDU=e['id'],bytes=len(raw),physical_frames=owners));continue
        s=e.get('session')or{}
        if e['status']in ['decoded','deferred']:
            check(isinstance(e['fields'],dict)and not e.get('error')and not e.get('fields_error'),'accepted native record has a successful actual field query')
            for name,value in [('PType',raw[2]),('PFC Flags',raw[3]),('Call ID',call),('First Frag',bool(raw[3]&1)),('Last Frag',bool(raw[3]&2))]:eq(s[name],value,'literal native common field '+name)
        required={'dcerpc.ver':5,'dcerpc.ver_minor':0,'dcerpc.pkt_type':raw[2],'dcerpc.cn_flags':raw[3],'dcerpc.cn_frag_len':size,'dcerpc.cn_auth_len':auth,'dcerpc.cn_call_id':call};body_reference={}
        invalid=None;unsupported=None
        control_refusal=control_refusals.get(delivery)
        if control_refusal:
            eq(owners,[delivery],'control semantic refusal belongs to its one exact original TCP PDU')
            eq(control_refusal['raw_sha256'],sha(raw),'independently derived control refusal binds literal original bytes')
            eq(e['status'],control_refusal['expected_status'],'original-wire control cause determines refusal status')
        if auth:
            trailer=len(raw)-auth-8
            if trailer<16:invalid=dict(reason='authentication declaration exceeds fragment',auth_bytes=auth,available_body_bytes=len(raw)-16)
            else:
                header=raw[trailer:trailer+8];pad=header[2]
                if pad>trailer-16:invalid=dict(reason='authentication padding overlaps common header',declared_padding=pad,available_body_bytes=trailer-16)
                elif header[0]==0 or header[1]not in range(2,7):unsupported=dict(reason='authentication type/level outside selected envelope profile',auth_type=header[0],auth_level=header[1])
        elif e['status']=='malformed':invalid=invalid_clear_body(raw)
        if invalid:
            eq(e['status'],'malformed','a provably invalid declared byte range remains malformed')
            eq(e['fields'],None,'invalid bounds have no successful field tree')
            check(bool(e.get('fields_error')),'invalid byte-range field query preserves refusal')
            if s.get('Error Scope')=='message':
                eq(s['Presentation Context Updated'],False,'isolated malformed message cannot install a presentation context')
                if raw[2]in[11,14]:
                    eq(s['Declared Presentation Context Count'],raw[24],'original malformed context declaration')
                    eq(s['Unparsed Context List Tail Bytes'],invalid['extra_bytes'],'complete isolated context-list tail')
                elif raw[2]in[12,15]:
                    eq(s['Declared Presentation Result Count'],invalid['declared_results'],'original malformed result declaration')
                    eq(s['Unparsed Result List Tail Bytes'],invalid['extra_bytes'],'complete isolated result-list tail')
            adjudications.append(dict(PDU=e['id'],physical_frames=owners,**invalid))
        elif unsupported:
            eq(e['status'],'context-required','bounded unsupported authentication semantics stay distinct from malformed bounds')
            eq(e['fields'],None,'unsupported security semantics have no successful full field tree')
            check('UnsupportedFeature'in(e.get('fields_error')or''),'explicit unsupported profile refusal')
            adjudications.append(dict(PDU=e['id'],physical_frames=owners,**unsupported))
        elif auth:
            check(e['status']in ['decoded','deferred'],'observed selected security envelope remains available')
            eq(e['profile'],'dcerpc-security-envelope','explicit outer-envelope profile');eq(e['completeness'],'envelope','no full body-semantics claim')
            eq(s['Profile'],'dcerpc-security-envelope','native outer-envelope scope');eq(s['Context Level'],'unverified-security-envelope','no established security/presentation context claim')
            eq(s['RPC Vers'],raw[0],'exact major version');eq(s['RPC Vers Minor'],raw[1],'exact minor version');eq(s['Packed Drep'],b64(raw[4:8]),'complete data representation')
            names={0:'Request',2:'Response',3:'Fault',11:'Bind',12:'BindAck',13:'BindNak',14:'AlterContext',15:'AlterContextResp'}
            if raw[2]in names:eq(s['Packet Name'],names[raw[2]],'exact observed opaque-envelope PDU kind')
            eq(s['Auth Length'],auth,'token declaration');eq(s['Frag Length'],size,'fragment declaration')
            eq(s['Opaque PDU Body'],b64(raw[16:trailer-pad]),'complete opaque body')
            eq(s['Authentication Padding'],b64(raw[trailer-pad:trailer]),'exact captured padding')
            eq(s['Authentication Token'],b64(raw[trailer+8:]),'complete opaque authentication token')
            fields={'Auth Type':header[0],'Auth Level':header[1],'Auth Padding Length':pad,'Auth Reserved':header[3],'Auth Context ID':int.from_bytes(header[4:8],'little')}
            eq(s['Security Trailer'],fields,'all security-trailer fields')
            for flag in ['Authentication Verified','Peer Identity Verified','Stub Decoded','Trailer Alignment Validated']:eq(s[flag],False,'no unobserved security/body/alignment claim')
            for name,span in {'Opaque PDU Body':[16,trailer-pad],'Authentication Padding':[trailer-pad,trailer],'Security Trailer':[trailer,trailer+8],'Authentication Token':[trailer+8,len(raw)]}.items():eq(s['Wire Field Ranges'][name],span,'exact owned security wire range')
            for key,value in [('dcerpc.auth_type',header[0]),('dcerpc.auth_level',header[1]),('dcerpc.auth_pad_len',pad),('dcerpc.auth_rsrvd',header[3]),('dcerpc.auth_ctx_id',fields['Auth Context ID'])]:required[key]=value
            eq(e['fields'],s,'complete owned security-envelope field-query projection')
        elif control_refusal:
            eq(e['fields'],None,'semantically refused control has no successful field-query projection')
            eq(s['Error Scope'],'message','control semantic failure is isolated at its exact framed boundary')
            check(bool(e.get('fields_error')),'semantic control field query retains its refusal')
            adjudications.append(dict(PDU=e['id'],physical_frames=owners,**control_refusal))
        elif e['status']=='context-required':
            eq(e['fields'],None,'missing association does not become accepted generic fields')
            eq(s['Error Scope'],'message','complete message context refusal is isolated')
            check(bool(e.get('fields_error')),'context query preserves a refusal')
        elif e['status']=='malformed':
            check(False,'malformed record needs an independently established original-wire cause')
        elif e['status']in['decoded','deferred']:
            expected,body_reference=clear_wire_fields(raw)
            check(expected is not None,'accepted clear PDU needs an independently decoded selected layout')
            eq(e['fields'],expected,'complete clear field tree equals original-wire fields, values and types')
            complete_clear_trees+=1
            if raw[2]in[11,14]:
                contexts=expected['PDU']['Bind'].get('Contexts')or[];views=[]
                aliases={'e1af8308-5d1f-11c9-91a4-08002b14a0fa':'EPM','4b324fc8-1670-01d3-1278-5a47bf6ee188':'SRVSVC'}
                for ordinal,context in enumerate(contexts):
                    abstract=base64.b64decode(context['Abstract Syntax']);transfer=base64.b64decode(context['Transfer Syntax']);version=context['Abstract Version'];major=version&65535;minor=version>>16
                    name=aliases.get(str(uuid.UUID(bytes_le=abstract)),'unknown')if(major,minor)==(3,0)else'unknown'
                    view={'Context ID':context['Context ID'],'Abstract Syntax':b64(abstract),'Interface':name,'Interface Version Major':major,'Interface Version Minor':minor,'Transfer Syntax':b64(transfer),'Transfer Version':context['Transfer Version'],'Bind Time Feature Proposal':transfer[:8]==bytes.fromhex('2c1cb76c12984045')and context['Transfer Version']==1}
                    if require_scoped_ranges or 'Wire Field Ranges'in s['Presentation Contexts'][ordinal]:
                        at=28+44*ordinal
                        view['Wire Field Ranges']={'Context ID':[at,at+2],'Abstract Syntax':[at+4,at+20],'Interface Version Major':[at+20,at+22],'Interface Version Minor':[at+22,at+24],'Transfer Syntax':[at+24,at+40],'Transfer Version':[at+40,at+44]}
                    views.append(view)
                eq(s['Presentation Contexts'],views,'all ordered native presentation context proposals')
                proposals[(connection(e),call)]=dict(PType=raw[2],source=e['source'],contexts=contexts)
            elif raw[2]in[12,15]:
                body=raw[16:];at=(10+int.from_bytes(body[8:10],'little')+3)&~3;views=[]
                owner=proposals.get((connection(e),call))
                matched=owner is not None and owner['source']!=e['source']and owner['PType']==(11 if raw[2]==12 else 14)
                if matched:eq(s['Association Status'],'matched','original observed opposite-direction proposal owns ACK')
                else:check(s.get('Association Status')!='matched','missing or incompatible original proposal cannot own ACK')
                accepted=[];rejected=[]
                for index,pos in enumerate(range(at+4,len(body),24)):
                    result=int.from_bytes(body[pos:pos+2],'little');reason=int.from_bytes(body[pos+2:pos+4],'little');transfer=body[pos+4:pos+20];version=int.from_bytes(body[pos+20:pos+24],'little')
                    view={'Ordinal':index,'Result':result,'Reason':reason,'Transfer Syntax':b64(transfer),'Transfer Version':version}
                    if require_scoped_ranges or 'Wire Field Ranges'in s['Presentation Context Results'][index]:
                        start=16+pos
                        view['Wire Field Ranges']={'Result':[start,start+2],'Reason':[start+2,start+4],'Transfer Syntax':[start+4,start+20],'Transfer Version':[start+20,start+24]}
                    if matched:
                        check(index<len(owner['contexts']),'original ACK has a proposal at the same ordinal')
                        context=owner['contexts'][index];view['Proposed Context ID']=context['Context ID']
                        if result==0:
                            eq((b64(transfer),version),(context['Transfer Syntax'],context['Transfer Version']),'accepted transfer syntax was actually offered at the same ordinal');accepted.append(context['Context ID'])
                        elif result in[1,2]:rejected.append(context['Context ID'])
                        elif result==3:
                            offered=base64.b64decode(context['Transfer Syntax'])
                            check(offered[:8]==bytes.fromhex('2c1cb76c12984045')and context['Transfer Version']==1,'feature ACK has an original feature proposal')
                            eq((transfer.hex(),version),('00'*16,0),'feature response cannot select an ordinary syntax')
                            eq(s['Observed Bind Time Feature Bits'],reason,'exact observed response feature bits');eq(s['Bind Time Feature Endpoint State Verified'],False,'negotiation observation does not verify endpoint state')
                    views.append(view)
                eq(s['Presentation Context Results'],views,'every ordered result remains structured with its actual proposal identity')
                address=int.from_bytes(body[8:10],'little')
                if address:
                    eq(s['Secondary Address Bytes'],b64(body[10:10+address]),'lossless native secondary address');eq(s['Wire Field Ranges']['Secondary Address Bytes'],[26,26+address],'exact secondary-address PDU range')
                if matched:
                    eq(len(views),len(owner['contexts']),'all original proposal ordinals have a result')
                    eq(s.get('Accepted Context IDs')or[],accepted,'feature/rejected entries cannot become accepted transfer contexts')
                    eq(s.get('Rejected Context IDs')or[],rejected,'ordered rejected context IDs')
                    proposals.pop((connection(e),call));association_checks+=1
            elif raw[2]==0 and raw[3]&0x80:
                eq(s['Object UUID'],b64(raw[24:40]),'native object UUID is separate from the stub');eq(s['Stub Bytes'],len(raw)-40,'exact stub length after object identity');eq(s['Wire Field Ranges']['Object UUID'],[24,40],'object identity owns its actual PDU interval')
        # TCP can deliver several cached PDUs when an earlier hole is filled.
        # Wireshark's request_in names that delivery frame; byte contributors
        # still keep their exact original frames and are never rewritten.
        candidates=[(no,index,view)for no in sorted(set(owners)|{delivery})for index,(span,view)in enumerate(reference_views[no])if len(span)>=16 and raw.startswith(span)]
        if candidates:
            no,index,view=max(candidates,key=lambda item:len(reference_views[item[0]][item[1]][0]));used_views.add((no,index))
            for key,value in required.items():
                observed=list(leaves(view,key))
                check(bool(observed),'available reference common/security field '+key)
                check(value in[int(v,0)if str(v).startswith('0x')else int(v)for v in observed],'literal reference field value '+key)
                reference_fields[key]+=1
            for key,value in body_reference.items():
                observed=list(leaves(view,key))
                if not observed:
                    reference_field_limits.append(dict(PDU=e['id'],frame=no,field=key,reason='reference omits this selected body field'));continue
                expected=value if isinstance(value,list)else[value]
                values=[str(v)if isinstance(expected[0],str)else int(v,0)if str(v).startswith('0x')else int(v)for v in observed]
                eq(values,expected,'ordered literal reference body fields '+key)
                reference_fields[key]+=len(expected)
        else:reference_limits.append(dict(PDU=e['id'],physical_frames=owners,reason='no exact complete/prefix raw-view attribution; independent full wire bytes still checked'))
        compared.append(dict(PDU=e['id'],protocol=e['protocol'],status=e['status'],physical_frames=owners,stream_offset=wire_start,stream_prefix_delivery_frame=delivery,raw_sha256=sha(raw),auth_bytes=auth))
    if receipts:
        for r in json.loads(receipts.read_text(encoding='utf-8-sig')):
            raw=base64.b64decode(r['wire_base64'],validate=True);eq(sha(raw),r['sha256'],'endpoint receipt digest')
            matches=[v for v in compared if v['raw_sha256']==r['sha256']]
            eq(len(matches),1,'each actual sent/received complete PDU has one native wire record')
            receipt_matches.append(dict(case=r['case'],direction=r['direction'],PDU=matches[0]['PDU'],raw_sha256=r['sha256']))
        eq(len(compared),len(receipt_matches),'no extra selected PDU in owned endpoint capture')
    details=[];unexported=[]
    for state in streams.values():
        at=state['cursor'];available=state['contiguous']
        while at+16<=available:
            header=bytes(state['cells'][i]for i in range(at,at+16))
            if header[:2]!=b'\x05\0'or header[4:8]!=b'\x10\0\0\0':break
            size=int.from_bytes(header[8:10],'little')
            if size<16 or at+size>available:break
            data=bytes(state['cells'][i]for i in range(at,at+size))
            unexported.append(dict(**state['meta'],start=at,length=size,raw_sha256=sha(data)));at+=size
        holes=[];previous=0
        for offset in sorted(state['cells']):
            if offset>previous:holes.append([previous,offset])
            previous=offset+1
        details.append(dict(**state['meta'],native_consumed_bytes=state['cursor'],captured_contiguous_bytes=available,remaining_captured_bytes=available-state['cursor'],captured_sequence_bytes=len(state['cells']),captured_sequence_holes=holes,bytes_captured_beyond_first_hole=len(state['cells'])-available,captured_SYN=state.get('captured_SYN',False),later_pre_origin_segments=state.get('before_origin',[])))
    numeric_links=None
    if require_call_links:
        from dcerpc_call_links import audit_call_links
        numeric_links=audit_call_links(native,reference_views,pdu_deliveries)
    sources=['dcerpc_wire_audit.py','mail_rtsp_public_wire_audit.py','mail_rtsp_export_invariants.py']+(['dcerpc_call_links.py']if require_call_links else[])+(['dcerpc_generation_control_audit.py']if control_manifest is not None else[])
    audit_sources={name:sha((Path(__file__).parent/name).read_bytes())for name in sources}
    return dict(scope=__doc__,audit_sources_sha256=audit_sources,control_manifest_sha256=sha(control_manifest.read_bytes())if control_manifest is not None else None,require_scoped_field_ranges=require_scoped_ranges,numeric_call_links=numeric_links,checks=checks,capture_sha256=sha(capture.read_bytes()),events_sha256=sha(events.read_bytes()),oracle_sha256=sha(oracle.read_bytes()),physical_frames=total_frames,SDK_records_with_unique_positive_IDs=SDK_records,
                compared_PDUs=compared,status_counts=dict(status),complete_clear_field_trees_checked=complete_clear_trees,matched_bind_or_alter_associations_checked=association_checks,reference_field_counts=dict(reference_fields),reference_layout_limits=reference_limits,reference_field_limits=reference_field_limits,
                outside_selected_reference_frames=outside,aggregate_reference_limits=aggregate_reference_limits,unused_selected_reference_views=[dict(frame=no,view=index)for no,values in reference_views.items()for index,_ in enumerate(values)if(no,index)not in used_views],
                closure_diagnostics=closures,incomplete_captured_prefix_diagnostics=prefixes,aggregate_direction_diagnostics=tails,diagnostic_wire_adjudications=adjudications,actual_endpoint_receipt_matches=receipt_matches,streams=details,unexported_complete_wire_PDUs=unexported,wire_or_capture_domain_normalization=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['capture','events','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--receipts',type=Path);ap.add_argument('--control-manifest',type=Path);ap.add_argument('--require-scoped-ranges',action='store_true');ap.add_argument('--require-call-links',action='store_true');a=ap.parse_args()
    for p in vars(a).values():
        if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.events,a.oracle,a.receipts,a.require_scoped_ranges,a.require_call_links,a.control_manifest)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(checks=proof['checks'],physical_frames=proof['physical_frames'],native_PDUs=len(proof['compared_PDUs']),statuses=proof['status_counts'],reference_fields=sum(proof['reference_field_counts'].values()),unexported_complete_PDUs=len(proof['unexported_complete_wire_PDUs']))))
