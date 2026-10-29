"""Whole NAT-T ESP object expectations from old identities and original wire.

The complete event constructor below targets the existing events/main.go JSON
API. Rule/Timestamp/Length are not exported by that API and are checked by the
separate API probe plan. No current/future native output is used as an oracle.
"""
import base64,copy,hashlib,ipaddress,json
SCOPE='clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict'
PROFILE='esp-natt-observed-header'
ADMISSION='observed-UDP4500-nonzero-SPI'

def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys() and all(same(a[k],b[k]) for k in a)
    if isinstance(a,list):return len(a)==len(b) and all(same(x,y) for x,y in zip(a,b))
    return a==b
def sha_bytes(b):return hashlib.sha256(b).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=False,allow_nan=False).encode('utf8')
def field_tree(wire):
    assert len(wire)==96
    spi=int.from_bytes(wire[:4],'big');seq=int.from_bytes(wire[4:8],'big')
    assert spi==0xf487bfcf and 2<=seq<=8
    return {'Header Complete':True,'Algorithm Context Available':False,'Authentication Verified':False,
        'Extended Sequence Reconstructed':False,'Observation Scope':SCOPE,
        'Wire Field Ranges':{'SPI':[0,4],'Sequence':[4,8],'Opaque Body':[8,len(wire)]},
        'SPI':spi,'Sequence':seq,'Opaque Body':base64.b64encode(wire[8:]).decode()}
def byte_source(before,wire):
    value=copy.deepcopy(before['source_bytes'])
    assert value['Kind']=='captured' and value['ParentPDU']==0 and value['ParentPDUs']is None
    assert len(value['PacketRefs'])==1 and 'CompletedIn' not in value
    value['SHA256']=sha_bytes(wire)
    return value
def snapshot(before,wire,source,fields):
    # DisplayFieldValues visits event bindings in lexical name order, then
    # protocol bindings in lexical order. 32-bit SPI/sequence have no exact_value.
    identifier=before['id'];flow=before['flow']
    def address(endpoint):
        host=endpoint.rsplit(':',1)[0].strip('[]');return str(ipaddress.ip_address(host))
    values=[{'name':'destination','value':before['destination'],'origin':'event'},
        {'name':'flow.id','value':flow,'exact_value':str(flow),'origin':'event'},
        {'name':'ip.addr','value':address(before['source']),'origin':'event'},
        {'name':'ip.addr','value':address(before['destination']),'origin':'event'},
        {'name':'pdu.id','value':identifier,'exact_value':str(identifier),'origin':'event'},
        {'name':'pdu.length','value':len(wire),'exact_value':str(len(wire)),'origin':'event'},
        {'name':'protocol','value':'esp','origin':'event'},
        {'name':'source','value':before['source'],'origin':'event'},
        {'name':'status','value':'limited','origin':'event'},
        {'name':'esp.sequence','value':fields['Sequence'],'origin':'pdu','pdu_range':{'offset':4,'length':4}},
        {'name':'esp.spi','value':fields['SPI'],'origin':'pdu','pdu_range':{'offset':0,'length':4}}]
    return {'schema_version':1,'pdu':identifier,'source':copy.deepcopy(source),'values':values,'completeness':'headers'}
def event(before,wire):
    assert before['protocol']=='' and before['transport']=='udp' and before['status']=='unrecognized'
    assert before['profile']==before['entry']==before['admission']=='' and before['expert']=='ProtocolNotRecognized'
    assert before['fields']is None and before['session']is None and 'error' not in before
    assert before['flow']==before['transaction']==before['response_to']==before['direction']==0
    assert not before.get('tcp_flow_id')
    assert before['fields_error']=='protocol parser: event is unrecognized, not an exact message'
    preview=base64.b64decode(before['raw'],validate=True)
    assert len(preview)==64 and preview==wire[:64]
    assert before['source_bytes']['SHA256']==sha_bytes(preview)
    fields=field_tree(wire);source=byte_source(before,wire)
    # This is a full expected object. Unknown added/removed keys are rejected.
    result=copy.deepcopy(before)
    result.update(protocol='esp',transport='udp',profile=PROFILE,admission=ADMISSION,entry='ESPHeaderFields',
        status='limited',completeness='headers',expert='',raw=base64.b64encode(wire).decode(),
        source_bytes=source,session=copy.deepcopy(fields),fields=copy.deepcopy(fields),
        summary='ESP · UDP encapsulation · SPI=0xf487bfcf · seq=%d · opaque=88 bytes; SA context unavailable'%fields['Sequence'])
    del result['fields_error']
    result['field_values']=snapshot(before,wire,source,fields)
    return result

def api_contract(before,wire,time_ns):
    expected=event(before,wire);fields=field_tree(wire)
    return {'ID':expected['id'],'FlowID':expected['flow'],'Protocol':'esp','Transport':'udp',
        'Profile':PROFILE,'Admission':ADMISSION,'Rule':'ipsec_esp','Entry':'ESPHeaderFields','Status':'limited',
        'Completeness':'headers','Error':'','ExpertCode':'','Length':len(wire),'TimestampNS':time_ns,
        'Source':before['source'],'Destination':before['destination'],'Direction':before['direction'],
        'Domain':copy.deepcopy(before['domain']),'CarrierLayers':copy.deepcopy(before['carrier_layers']),
        'TransactionID':before['transaction'],'ResponseTo':before['response_to'],'TCPFlowID':0,
        'SourceBytes':copy.deepcopy(expected['source_bytes']),'Raw':expected['raw'],'Summary':expected['summary'],
        'Session':copy.deepcopy(fields),'GetFields':copy.deepcopy(fields),'GetFieldsError':'',
        'Decode':{'fields':copy.deepcopy(fields),'session':copy.deepcopy(fields)},'DecodeError':'',
        'DisplayFieldValues':copy.deepcopy(expected['field_values'])}

PROTOCOL_STATS_DELTA={'Messages':7,'MessageBytes':672,'LimitedBytes':672,'Unknown':-7,'UnclassifiedBytes':-672}
PROTOCOL_STATS_KEYS={'ContextRequired','Flows','ProbeCalls','Messages','Decoded','Deferred','Malformed','Incomplete','Unknown',
    'InputBytes','MessageBytes','UnclassifiedBytes','LimitedBytes','CallbackPanics','BufferedBytes','PeakBufferedBytes'}
def protocol_stats(before):
    assert set(before)==PROTOCOL_STATS_KEYS
    assert all(type(v)is int and v>=0 for v in before.values())
    value=copy.deepcopy(before)
    for key,delta in PROTOCOL_STATS_DELTA.items():value[key]+=delta;assert value[key]>=0
    return value
