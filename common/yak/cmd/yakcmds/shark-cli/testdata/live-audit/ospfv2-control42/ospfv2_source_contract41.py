"""Pure OSPFv2 public observation contract from independent original wire facts.

This constructor selects the556 complete Ethernet/IPv4 source observations.
It reads no candidate decoder/output. Public metadata/alias names are an
explicit contract; every literal value and span comes from the source grid.
"""
import base64,copy,hashlib
from pathlib import Path

RENAME={'TOS Count':'TOS/MT-ID Count','TOS Metrics':'TOS/MT-ID Metrics','TOS':'TOS/MT-ID Selector','Flags and TOS':'Flags and TOS/MT-ID'}
NAMES={1:'Hello',2:'Database Description',3:'Link State Request',4:'Link State Update',5:'Link State Acknowledgment'}
SELECTOR_SCOPE='observed legacy TOS/MT-ID selector; topology semantics not selected'

def source_wire(fact):
    raw=Path(fact['original_frame']).read_bytes()
    assert hashlib.sha256(raw).hexdigest()==fact['original_frame_SHA256']
    assert fact['domain']['link']==1 and [h['kind']for h in fact['carrier']['carrier']]==['Ethernet','IPv4']
    assert fact['carrier']['parent_complete'] and fact['carrier']['IPv4_flags_fragment']==0
    first,last=fact['PDU_range'];wire=raw[first:last]
    assert hashlib.sha256(wire).hexdigest()==fact['PDU_SHA256']
    return raw,wire

def translated(node,wire,path=()):
    if isinstance(node,list):return [translated(c,wire,path+(i,))for i,c in enumerate(node)]or None
    if not isinstance(node,dict):return copy.deepcopy(node)
    result={}
    for key,value in node.items():
        name='TLV Type'if key=='Type'and path[:2]==('LLS','TLVs')else RENAME.get(key,key)
        if key=='Wire Field Ranges':
            result[key]={('TLV Type'if k=='Type'and path[:2]==('LLS','TLVs')else RENAME.get(k,k)):copy.deepcopy(v)for k,v in value.items()};continue
        if isinstance(value,dict)and set(value)=={'bytes','SHA256'}:
            a,b=node['Wire Field Ranges'][key];raw=wire[a:b]
            assert len(raw)==value['bytes'] and hashlib.sha256(raw).hexdigest()==value['SHA256']
            result[name]=base64.b64encode(raw).decode();continue
        result[name]=translated(value,wire,path+(key,))
    return result

def fields(fact):
    _,wire=source_wire(fact);source=fact['source']['fields'];m=translated(source,wire)
    h=m['Header'];auth=h['Authentication Type'];typ=h['Type'];n=h['Packet Length']
    assert typ in NAMES and auth in(0,2) and n>=24
    for key in ['Base Body Extent','Observed PDU Extent','Packet Checksum Verification','L Bit']:
        m.pop(key,None)
    m['Packet Name']=NAMES[typ]
    m['Observation Scope']='bounded wire advertisements only; no SA, replay protection, verified neighbor, LSDB or installed route'
    m['Checksum Verified']=source['Packet Checksum Verification']['verified']
    m['Checksum Scope']='declared OSPFv2 packet excluding eight authentication bytes; no authentication verdict'if auth==0 else'omitted: cryptographic authentication; MAC, key and SA unavailable'
    m['Required Captured Bytes']=len(wire)
    if typ in(1,2):m['L Bit']=bool(source['L Bit'])
    m.setdefault('Wire Field Ranges',{})
    def lsas(rows,full):
        for row in rows or[]:
            proof=row.pop('Checksum Verification')
            row['Checksum Verified']=proof.get('verified',proof.get('valid',False))
            row['Checksum Scope']='complete LSA excluding LS Age; no authentication verdict'if full else'unverified: advertised LSA header without body'
            if not full:continue
            body=row['Body'];t=row['LS Type']
            body.pop('Normalized Advertised Prefix',None);body.pop('Prefix Interpretation',None)
            body.setdefault('Wire Field Ranges',{})
            if t==1:
                for link in body['Links']or[]:
                    link.pop('Link Data Interpretation',None)
                    link['Selector Scope']=SELECTOR_SCOPE
                    link['Link Data Meaning']='stub network mask bytes; not installed routing state'if link['Link Type']==3 else'raw interface address or unnumbered ifIndex; interpretation not certified'
            elif t in(3,4):
                body.pop('Base TOS Field Meaning',None);body.pop('Destination ASBR Router ID',None)
                body['Selector Scope']='first selector is reserved in base RFC2328; further selectors may represent TOS or MT-ID; topology semantics not selected'
                if t==4:body['Network Mask Meaning']='not meaningful for ASBR summary; no network prefix inferred'
            elif t in(5,7):
                body['Selector Scope']=SELECTOR_SCOPE
                for metric in body['TOS/MT-ID Metrics']or[]:
                    metric.pop('E Bit',None);metric['External Metric Type']=metric.pop('Metric Type')
            assert t in(1,2,3,4,5,7),'contract deliberately excludes opaque LSA semantics'
    if typ==2:lsas(m['Database Description']['LSA Headers'],False)
    if typ==4:lsas(m['LSAs'],True)
    if typ==5:lsas(m['LSA Headers'],False)
    if 'LLS'in m:
        lls=m['LLS'];proof=lls.pop('Checksum Verification')
        for key in ['Observed Extent','Length Bytes','Violations']:lls.pop(key,None)
        lls['Checksum Verified']=proof['verified']
        lls['Checksum Scope']='LLS block only; independent of OSPF packet checksum'if auth==0 else'omitted: cryptographic LLS authentication; MAC, key and SA unavailable'
        for tlv in lls['TLVs']or[]:
            if tlv['TLV Type']==2:tlv['Authentication Data Length Matches Header']=len(base64.b64decode(tlv['Authentication Data']))==m['Authentication']['Authentication Data Length']
    return m

def nested_rows(root):
    rows=[]
    def walk(v):
        if isinstance(v,dict):
            if 'Wire Field Ranges'in v:rows.append(v)
            for k,c in v.items():
                if k!='Wire Field Ranges':walk(c)
        elif isinstance(v,list):
            for c in v:walk(c)
    walk(root)
    return sorted(rows,key=lambda r:min((s[0]for s in r['Wire Field Ranges'].values()),default=2**64-1))

# Only the public projection schema is declared here. Values and literal ranges
# are looked up in source-constructed nodes, never in any native publication.
WIRE_SPECS=[
 ('ospf.version','Version','header'),('ospf.msg','Type','header'),('ospf.packet_length','Packet Length','header'),('ospf.checksum','Checksum','header'),('ospf.srcrouter','Router ID','header'),('ospf.area_id','Area ID','header'),
 ('ospfv2.instance_id','Instance ID','header'),('ospfv2.auth.type','Authentication Type','header'),('ospfv2.auth.key_id','Key ID','auth'),('ospfv2.auth.data_length','Authentication Data Length','auth'),('ospfv2.auth.sequence','Sequence','auth'),
 ('ospfv2.options','Options','all'),('ospfv2.router_priority','Router Priority','all'),('ospfv2.hello_interval','Hello Interval','all'),('ospfv2.dead_interval','Router Dead Interval','all'),('ospfv2.interface_mtu','Interface MTU','all'),('ospfv2.dd_sequence','DD Sequence','all'),('ospfv2.flags','Flags','all'),('ospfv2.lsa_count','LSA Count','root'),
 ('ospfv2.lsa.age','LS Age','lsa'),('ospfv2.lsa.type','LS Type','lsa'),('ospfv2.lsa.sequence','Sequence','lsa'),('ospfv2.lsa.checksum','Checksum','lsa'),('ospfv2.lsa.length','Length','lsa'),('ospfv2.request.ls_type','LS Type','requests'),
 ('ospfv2.link_count','Link Count','all'),('ospfv2.link.type','Link Type','all'),('ospfv2.link.selector_count','TOS/MT-ID Count','all'),('ospfv2.selector','TOS/MT-ID Selector','all'),('ospfv2.metric','Metric','all'),('ospfv2.external.flags','Flags and TOS/MT-ID','all'),('ospfv2.external.tag','External Route Tag','all'),
 ('ospfv2.lls.checksum','Checksum','lls'),('ospfv2.lls.length_words','Length Words','lls'),('ospfv2.lls.tlv.type','TLV Type','lls-tlv'),('ospfv2.lls.tlv.length','Length','lls-tlv'),('ospfv2.lls.extended_options','Extended Options and Flags','lls-tlv'),('ospfv2.lls.auth.sequence','Sequence','lls-tlv'),
 ('ospfv2.network_mask','Network Mask','all'),('ospfv2.designated_router','Designated Router','all'),('ospfv2.backup_designated_router','Backup Designated Router','all'),('ospfv2.neighbor','Neighbor Router ID','all'),('ospfv2.lsa.id','Link State ID','lsa'),('ospfv2.lsa.advertising_router','Advertising Router','lsa'),('ospfv2.request.id','Link State ID','requests'),('ospfv2.request.advertising_router','Advertising Router','requests'),('ospfv2.link.id','Link ID','all'),('ospfv2.link.data','Link Data','all'),('ospfv2.attached_router','Attached Router','all'),('ospfv2.forwarding_address','Forwarding Address','all')]
DERIVED_SPECS=[('ospfv2.checksum_verified','Checksum Verified','root'),('ospfv2.lsa.checksum_verified','Checksum Verified','lsa'),('ospfv2.lls.checksum_verified','Checksum Verified','lls'),('ospfv2.lls.auth.sequence_matches_header','Sequence Matches OSPF Header','lls-tlv'),('ospfv2.lls.auth.length_matches_header','Authentication Data Length Matches Header','lls-tlv'),('ospfv2.external.metric_type','External Metric Type','all')]

def aliases(m):
    result=[]
    def scoped(scope):
        if scope=='header':return [m['Header']]
        if scope=='auth':return [m['Authentication']]
        if scope=='root':return [m]
        if scope=='requests':return m.get('Requests')or[]
        if scope=='lls':return [m['LLS']]if 'LLS'in m else[]
        if scope=='lls-tlv':return m.get('LLS',{}).get('TLVs')or[]
        rows=nested_rows(m)
        return [r for r in rows if 'LS Age'in r]if scope=='lsa'else rows
    for derived,specs in [(False,WIRE_SPECS),(True,DERIVED_SPECS)]:
        for name,key,scope in specs:
            for row in scoped(scope):
                if key not in row:continue
                item=dict(name=name,value=copy.deepcopy(row[key]),origin='derived'if derived else'pdu')
                if not derived:
                    if key not in row.get('Wire Field Ranges',{}):continue
                    a,b=row['Wire Field Ranges'][key];assert 0<=a<b
                    item['pdu_range']=dict(offset=a,length=b-a)
                result.append(item)
    return sorted(result,key=lambda r:r['name'])

def event(identifier,fact):
    assert type(identifier)is int and identifier>0
    _,wire=source_wire(fact);m=fields(fact)
    assert fact['source']['fields']['Packet Checksum Verification']['verified']or m['Header']['Authentication Type']==2
    d=fact['domain'];domain=dict(Section=d['section'],Interface=d['interface'],Encapsulation='')
    frame=fact['frame'];ref=dict(Number=frame,Domain=copy.deepcopy(domain))
    bs=dict(Kind='captured',SHA256=fact['PDU_SHA256'],ParentPDU=0,ParentPDUs=None,PacketRefs=[ref],CompletedIn=copy.deepcopy(ref))
    source=fact['carrier']['source'];destination=fact['carrier']['destination']
    metadata={'destination':[destination],'flow.id':[0],'ip.addr':[source,destination],'pdu.completed_in':[frame],'pdu.id':[identifier],'pdu.length':[len(wire)],'protocol':['ospf'],'source':[source],'status':['decoded']}
    values=[]
    for name,vs in sorted(metadata.items()):
        for value in vs:
            item=dict(name=name,value=value,origin='event')
            if name in('flow.id','pdu.id','pdu.length','pdu.completed_in'):item['exact_value']=str(value)
            values.append(item)
    # SDK reserves event facts first, then all protocol/derived aliases sorted
    # by public name. Equal aliases preserve source occurrence order.
    values+=aliases(m)
    snapshot=dict(schema_version=1,pdu=identifier,source=copy.deepcopy(bs),values=values,completeness='message')
    return dict(id=identifier,flow=0,protocol='ospf',status='decoded',profile='ospfv2-bounded-control',completeness='message',summary='OSPFv2 '+m['Packet Name'],expert='',source_bytes=bs,fields=m,session=copy.deepcopy(m),field_values=snapshot,raw=base64.b64encode(wire).decode(),entry='OSPFv2ControlFields',admission='observed-IPv4-protocol-and-complete-control-grammar',source=source,destination=destination,direction=0,transport='network',domain=domain,carrier_layers=None,transaction=0,response_to=0)

def field_map(snapshot):
    result={}
    for item in snapshot['values']:result.setdefault(item['name'],[]).append(copy.deepcopy(item['value']))
    return result

def cli_row(before,identifier,fact):
    assert before['number']==fact['frame'] and set(before)=={'capture_domain','pdu_scope','protocol_scope','layers','number','time','source','destination','protocol','length','captured_length','info'}
    assert before['protocol']=='OSPF'and before['layers']==['Ethernet','IPv4','OSPF']and before['info']==''
    e=event(identifier,fact);s=e['field_values'];m=e['fields'];h=m['Header']
    result=copy.deepcopy(before)
    result.update(protocol='OSPFv2',protocol_stack=['Ethernet','IPv4','OSPFv2'],visibility='message',protocol_status='decoded',protocol_evidence='declared IPv4 protocol/version and bounded OSPFv2 wire observations',protocol_scope='observed control message; advertisements only',info='OSPFv2 '+m['Packet Name']+' · router='+h['Router ID']+' area='+h['Area ID']+' instance='+str(h['Instance ID']))
    result['pdus']=[dict(id=identifier,protocol='ospf',status='decoded',completeness='message',response_to=0,byte_source=copy.deepcopy(e['source_bytes']),fields=field_map(s),field_values=copy.deepcopy(s))]
    assert result['capture_domain']==e['domain']and result['source']==e['source']and result['destination']==e['destination']
    return result
