"""Derive selected clear RPC request owners from independently proven wire PDUs.

This runs after dcerpc_wire_audit has bound every Raw byte and contributor to
the original capture. Call IDs are literal wire values; generated PDU IDs are
only references. Refusals and opaque/fragment views never invent an owner.
"""
import base64
import collections
import json


def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'))


def leaves(node,name):
    if isinstance(node,dict):
        for key,value in node.items():
            if key==name:yield from value if isinstance(value,list)else[value]
            elif isinstance(value,(dict,list)):yield from leaves(value,name)
    elif isinstance(node,list):
        for value in node:yield from leaves(value,name)


def audit_call_links(native,reference_views,pdu_deliveries):
    pending={};contexts={};expected={};links=[];limits=[];references=0;response_references=0;checks=0;connections=set();names=collections.Counter()
    def check(ok,why):
        nonlocal checks
        checks+=1;assert ok,why
    for event in native:
        identifier=event['id'];want=0;raw=base64.b64decode(event.get('raw')or'')
        session=event.get('session')or{}
        check(type(event['response_to'])is int and event['response_to']>=0,'literal generated response identity')
        if raw and event['status']=='incomplete':
            check(not session and not event.get('entry')and bool(event.get('fields_error')),'captured partial bytes remain a diagnostic without call semantics')
            limits.append(dict(PDU=identifier,reason='incomplete captured RPC prefix has no complete call header/body'))
            raw=b''
        if raw:
            connection=canonical([event['domain'],event['flow'],sorted([event['source'],event['destination']])]);connections.add(connection)
            call=int.from_bytes(raw[12:16],'little');ptype=raw[2];key=(connection,call)
            full=raw[3]&3==3 and not session.get('Reassembled')
            authenticated=int.from_bytes(raw[10:12],'little')!=0
            if not full:
                # These logical bodies need a separate source/fragment-header
                # contract. No numeric-link coverage is claimed for them.
                limits.append(dict(PDU=identifier,reason='RPC fragment source ownership outside numeric-link profile'))
                check(event['response_to']==0,'RPC fragment view cannot assert this whole-PDU link profile')
                pending.pop(key,None)
            elif authenticated:
                check(event['response_to']==0,'an unverified security envelope cannot link calls')
            elif event['status']in['decoded','deferred']:
                check(session.get('Call ID')==call,'native literal wire Call ID')
                if ptype in[0,2,3]:
                    check(type(session.get('Context ID'))is int and session['Context ID']==int.from_bytes(raw[20:22],'little'),'native call context equals the original fixed header')
                if ptype==0:
                    check(type(session.get('OpNum'))is int and session['OpNum']==int.from_bytes(raw[22:24],'little'),'native operation number equals the original request header')
                if ptype==3:
                    check(type(session.get('Status'))is int and session['Status']==int.from_bytes(raw[24:28],'little'),'native fault status equals the original fault header')
                owner=pending.get(key)
                if ptype in[0,11,14]:
                    check(owner is None,'admitted clear request cannot replace an outstanding owner')
                    context=int.from_bytes(raw[20:22],'little')if ptype==0 else None
                    proposals=[];name={0:'Request',11:'Bind',14:'AlterContext'}[ptype]
                    if ptype==0:
                        accepted=contexts.get((connection,context))
                        if accepted:
                            interface=accepted['interface'];op=int.from_bytes(raw[22:24],'little')
                            name=(('EPM.'+{0:'ept_insert',1:'ept_delete',2:'ept_lookup',3:'ept_map',4:'ept_lookup_handle_free',5:'ept_inq_object',6:'ept_mgmt_delete'}[op])if interface=='EPM'and op<=6 else'SRVSVC.NetrShareEnum'if interface=='SRVSVC'and op==15 else f'{interface}.op_{op}')
                            check(session.get('Interface')==interface,'operation interface follows the original accepted context')
                            check((session.get('Interface Version Major'),session.get('Interface Version Minor'))==(accepted['major'],accepted['minor']),'operation interface version follows the accepted original proposal')
                        else:check('Interface'not in session,'an unobserved presentation context cannot invent an interface')
                    else:
                        at=28
                        for index in range(raw[24]):
                            major=int.from_bytes(raw[at+20:at+22],'little');minor=int.from_bytes(raw[at+22:at+24],'little');abstract=raw[at+4:at+20]
                            alias={bytes.fromhex('0883afe11f5dc91191a408002b14a0fa'):'EPM',bytes.fromhex('c84f324b7016d30112785a47bf6ee188'):'SRVSVC'}.get(abstract,'unknown')if(major,minor)==(3,0)else'unknown'
                            proposals.append(dict(id=int.from_bytes(raw[at:at+2],'little'),interface=alias,major=major,minor=minor))
                            at+=24+20*raw[at+2]
                    check(session.get('Packet Name')==name,'precise operation name follows the original UUID/version/opnum')
                    names[name]+=1
                    pending[key]=dict(id=identifier,ptype=ptype,source=event['source'],context=context,event=event,proposals=proposals,name=name)
                    check(session.get('Outstanding')is True,'admitted whole request remains outstanding')
                elif ptype in[2,3,12,13,15]:
                    request_type={2:0,3:0,12:11,15:14}.get(ptype)
                    if owner is None:association='missing-request'
                    elif (ptype==13 and owner['ptype']not in[11,14])or(ptype!=13 and owner['ptype']!=request_type):association='request-type-mismatch'
                    elif ptype in[2,3]and owner['context']!=int.from_bytes(raw[20:22],'little'):association='context-mismatch'
                    elif owner['source']==event['source']:association='direction-mismatch'
                    else:association='matched'
                    check(session.get('Association Status')==association,('association follows original request type/direction/context',identifier,association))
                    if association=='matched':
                        want=owner['id'];pending.pop(key)
                        check(session.get('Matched Request')==owner['name'],'reply operation name comes from the same original request')
                        if ptype in[12,15]:
                            at=(26+int.from_bytes(raw[24:26],'little')+3)&~3
                            count=raw[at];at+=4
                            check(count==len(owner['proposals']),'every reply result belongs to the original proposal ordinal')
                            for index,proposal in enumerate(owner['proposals']):
                                if int.from_bytes(raw[at+24*index:at+24*index+2],'little')==0:contexts[(connection,proposal['id'])]=proposal
                        check(want<identifier,'request must precede its immutable reply')
                        request=owner['event'];request_frames={r['Number']for r in request['source_bytes']['PacketRefs']}
                        response_frames={r['Number']for r in event['source_bytes']['PacketRefs']}
                        response_delivery=pdu_deliveries[identifier];request_delivery=pdu_deliveries[want]
                        views=[view for no in response_frames|{response_delivery}for wire,view in reference_views.get(no,[])if wire==raw]
                        observed=[int(v)for view in views for v in leaves(view,'dcerpc.request_in')]
                        request_raw=base64.b64decode(request['raw'])
                        request_views=[view for wire,view in reference_views.get(request_delivery,[])if wire==request_raw]
                        if observed:
                            check(set(observed)=={request_delivery},('Wireshark request frame equals independently reconstructed TCP prefix delivery',identifier))
                            check(bool(request_views),'reference request frame actually contains that complete original request PDU')
                            references+=1
                        else:limits.append(dict(PDU=identifier,reason='no uniquely paired Wireshark request_in field'))
                        response_in=[int(v)for view in request_views for v in leaves(view,'dcerpc.response_in')]
                        if response_in:
                            check(set(response_in)=={response_delivery},('Wireshark response frame equals independently reconstructed reply delivery',identifier))
                            check(bool(views),'reference response frame actually contains that complete original response PDU')
                            response_references+=1
                        else:limits.append(dict(PDU=identifier,reason='no uniquely paired Wireshark response_in field'))
                        links.append(dict(response_PDU=identifier,request_PDU=want,wire_Call_ID=call,request_PType=owner['ptype'],response_PType=ptype,request_frames=sorted(request_frames),response_frames=sorted(response_frames),request_stream_prefix_delivery_frame=request_delivery,response_stream_prefix_delivery_frame=response_delivery,Wireshark_request_frame_compared=bool(observed)))
            else:
                # A complete refusal must not overwrite or consume a request.
                check(event['response_to']==0,'refused or malformed message cannot claim a reply owner')
        check(event['response_to']==want,('exact original request PDU owner',identifier,want))
        aliases=[v for v in event['field_values']['values']if v['name']=='pdu.response_to']
        check(len(aliases)==int(want!=0),'exact presence of typed generated reply alias')
        if aliases:
            check(type(aliases[0]['value'])is int and aliases[0]['value']==want,'typed generated reply owner')
            check(aliases[0]['exact_value']==str(want),'exact decimal generated reply owner')
        expected[str(identifier)]=want
    return dict(scope=__doc__,checks=checks,SDK_records_compared=len(native),selected_connections=len(connections),verified_operation_names=dict(sorted(names.items())),verified_response_owner_PDU_IDs=expected,positive_links=len(links),Wireshark_request_frame_links_compared=references,Wireshark_response_frame_links_compared=response_references,links=links,explicit_limits=limits,outstanding_original_calls=len(pending),all_numeric_links_match_original_request_ownership=True)
