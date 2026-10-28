"""Frozen exact ESP event/CLI constructors; raw source and source-bound IDs are inputs.

No performance, SA validation, decryption or full-protocol parity is inferred.
"""
import base64,copy,hashlib
SCOPE='clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict'
NEED='NeedMore: ESP capture is incomplete; only captured clear-header facts are available'
def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b

def expected_esp(identifier,record,literal):
 assert type(identifier)is int and 0<identifier<1<<64
 assert literal['selection']=='explicit-IP-next-header-50'
 wire=record['wire'][literal['esp_offset']:literal['esp_end']]
 incomplete=len(wire)<8 or not literal['ip_extent_complete']or record['captured_length']<record['original_length']
 status='incomplete'if incomplete else'limited';complete='incomplete'if incomplete else'headers'
 domain={'Section':record['section'],'Interface':record['interface'],'Encapsulation':''}
 source={'Kind':'captured','SHA256':hashlib.sha256(wire).hexdigest()if wire else'','ParentPDU':0,'ParentPDUs':None,'PacketRefs':[{'Number':record['frame'],'Domain':domain}]}
 fields={'Header Complete':len(wire)>=8,'Algorithm Context Available':False,'Authentication Verified':False,'Extended Sequence Reconstructed':False,'Observation Scope':SCOPE,'Wire Field Ranges':{}}
 if len(wire)>=4:fields['SPI']=int.from_bytes(wire[:4],'big');fields['Wire Field Ranges']['SPI']=[0,4]
 if len(wire)>=8:
  fields['Sequence']=int.from_bytes(wire[4:8],'big');fields['Opaque Body']=base64.b64encode(wire[8:]).decode();fields['Wire Field Ranges'].update(Sequence=[4,8]);fields['Wire Field Ranges']['Opaque Body']=[8,len(wire)]
  assert fields['SPI']!=0,'this bounded public schema has no complete zero-SPI observation'
 values=[{'name':'destination','value':literal['destination'],'origin':'event'},{'name':'flow.id','value':0,'exact_value':'0','origin':'event'},{'name':'ip.addr','value':literal['source'],'origin':'event'},{'name':'ip.addr','value':literal['destination'],'origin':'event'},{'name':'pdu.id','value':identifier,'exact_value':str(identifier),'origin':'event'},{'name':'pdu.length','value':len(wire),'exact_value':str(len(wire)),'origin':'event'},{'name':'protocol','value':'esp','origin':'event'},{'name':'source','value':literal['source'],'origin':'event'},{'name':'status','value':status,'origin':'event'}]
 # Registry protocol values are sorted by descriptor name, not by wire offset.
 if len(wire)>=8:values.append({'name':'esp.sequence','value':fields['Sequence'],'origin':'pdu','pdu_range':{'offset':4,'length':4}})
 if len(wire)>=4:values.append({'name':'esp.spi','value':fields['SPI'],'origin':'pdu','pdu_range':{'offset':0,'length':4}})
 summary='ESP · clear header · opaque body; SA context unavailable'
 if len(wire)>=8:summary='ESP · SPI=0x%08x · seq=%d · opaque=%d bytes; SA context unavailable'%(fields['SPI'],fields['Sequence'],len(wire)-8)
 event={'id':identifier,'flow':0,'protocol':'esp','status':status,'profile':'esp-observed-header','completeness':complete,'summary':summary,'expert':'NeedMore'if incomplete else'','source_bytes':source,'fields':None if incomplete else copy.deepcopy(fields),'session':fields,'field_values':{'schema_version':1,'pdu':identifier,'source':copy.deepcopy(source),'values':values,'completeness':complete},'raw':base64.b64encode(wire).decode()if wire else None,'entry':'ESPHeaderFields','admission':'observed-IP-next-header','source':literal['source'],'destination':literal['destination'],'direction':0,'transport':'network','domain':domain,'carrier_layers':None,'transaction':0,'response_to':0}
 if incomplete:event.update(error=NEED,fields_error=NEED)
 return event

def expected_pdu(event):
 fields={}
 for value in event['field_values']['values']:fields.setdefault(value['name'],[]).append(value['value'])
 pdu={'byte_source':event['source_bytes'],'completeness':event['completeness'],'field_values':event['field_values'],'fields':fields,'id':event['id'],'protocol':event['protocol'],'response_to':event['response_to'],'status':event['status']}
 if event.get('error')or event.get('expert'):pdu.update(error=event.get('error',''),expert=event['expert'],summary=event['summary'])
 if event.get('tcp_flow_id'):pdu['tcp_flow_id']=event['tcp_flow_id']
 return pdu

def expected_esp_cli(event,record,literal,clock_text):
 # The caller separately checks clock_text against the exact captured instant;
 # local RFC3339 timezone spelling is not a protocol field or an ID mapping.
 raw=base64.b64decode(event['raw'])if event['raw']else b''
 path=['IPSecAH'if name=='AH'else name for name in literal['path']]
 incomplete=event['status']=='incomplete'
 info=event['summary'];scope='observed clear ESP header and opaque remainder';evidence='SA context unavailable; authentication, decryption and replay not verified'
 if incomplete:
  evidence='captured prefix only; no complete ESP PDU, SA, authentication or replay verdict'
  if len(raw)<8:
   scope='declared ESP; clear header incomplete';info='ESP clear header incomplete · %d/8 bytes observed; SA context unavailable'%len(raw)
  else:scope='ESP clear header observed; declared carrier extent incomplete';info='ESP clear header observed · declared carrier extent incomplete; SA context unavailable'
 if record['original_length']>record['captured_length']:info+=' [truncated capture: %d/%d bytes]'%(record['captured_length'],record['original_length'])
 return {'capture_domain':event['domain'],'pdus':[expected_pdu(event)],'pdu_scope':'observed_at_packet','protocol_stack':path+['ESP'],'visibility':'partial-header'if incomplete else'headers','protocol_status':event['status'],'protocol_scope':scope,'protocol_evidence':evidence,'layers':path+['IPSecESP'],'number':record['frame'],'time':clock_text,'source':event['source'],'destination':event['destination'],'protocol':'ESP','length':record['original_length'],'captured_length':record['captured_length'],'info':info}
