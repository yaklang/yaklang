"""Bounded whole-object oracle for the two newly exposed public parent refusals.
The source-byte predicates select the reason. No event label/frame-number is used.
"""
import copy

def expected_parent(identifier,record):
 wire=record['wire'];assert record['link']==1 and len(wire)>=34 and wire[12:14]==b'\x08\x00'
 ip=wire[14:];assert ip[0]>>4==4 and (ip[0]&15)*4==20 and int.from_bytes(ip[2:4],'big')==0
 if ip[9]==50:code='ESPParentLengthInvalid';reason='ESP parent IP length is invalid; clear ESP header not admitted'
 elif ip[9]==51:
  body=ip[20:];assert len(body)>=12 and body[1]==0;code='AHHeaderBounds';reason='AH payload length cannot be zero'
 else:raise AssertionError('unproven new parent refusal source')
 domain={'Section':record['section'],'Interface':record['interface'],'Encapsulation':''};source={'Kind':'captured','SHA256':'','ParentPDU':0,'ParentPDUs':None,'PacketRefs':[{'Number':record['frame'],'Domain':domain}]}
 values=[{'name':'flow.id','value':0,'exact_value':'0','origin':'event'},{'name':'pdu.id','value':identifier,'exact_value':str(identifier),'origin':'event'},{'name':'pdu.length','value':0,'exact_value':'0','origin':'event'},{'name':'protocol','value':'ip','origin':'event'},{'name':'status','value':'malformed','origin':'event'}]
 return {'id':identifier,'flow':0,'protocol':'ip','status':'malformed','profile':'network-header-refusal','completeness':'malformed','summary':reason+'; child identity not admitted','expert':code,'source_bytes':source,'fields':None,'session':{'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'validated network prefix before refused child header','Child Header Identity':'unconfirmed','Parent Header Refusal':code},'field_values':{'schema_version':1,'pdu':identifier,'source':copy.deepcopy(source),'values':values,'completeness':'malformed'},'raw':None,'entry':'','admission':'','source':'','destination':'','direction':0,'transport':'network','domain':domain,'carrier_layers':None,'transaction':0,'response_to':0,'error':reason,'fields_error':'protocol parser: no exact rule for this message'}
