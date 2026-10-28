"""Whole pre-runtime MLD events-observer expectation, with explicit mapped ID."""
import base64
import copy
import hashlib

from mld_expected import decode


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b


def enclosing(source,record):
    location=source['source_profile'];w=record['wire']
    network=next(h for h in reversed(location['headers'])if h['kind']in('IPv4','IPv6'))
    start=network['offset'];own=location['offset']
    if network['kind']=='IPv4':
        assert start+20<=len(w)and w[start]>>4==4
        total=int.from_bytes(w[start+2:start+4],'big')
        assert total==network['declared_length']
        return total-(own-start)if total>=own-start else -1
    assert start+40<=len(w)and w[start]>>4==6
    total=int.from_bytes(w[start+4:start+6],'big')
    assert total==network['declared_payload']
    return 40+total-(own-start)if total>0 and 40+total>=own-start else -1


def event(identifier,record,source,locate):
    assert type(identifier)is int and identifier>0
    assert hashlib.sha256(record['wire']).hexdigest()==source['wire_SHA256']
    meta={k:v for k,v in record.items()if k!='wire'}
    meta.setdefault('frame',source['frame'])
    assert same(meta,source['record'])
    location=locate(record)
    assert same(location,source['source_profile'])and location['protocol']=='icmpv6'
    raw=record['wire'][location['raw_start']:location['raw_end']]
    old=source['old_event']
    assert old['protocol']=='icmpv6'and old['transport']=='network'and old['flow']==0
    assert raw==base64.b64decode(old['raw'])and hashlib.sha256(raw).hexdigest()==old['source_bytes']['SHA256']
    assert old['domain']['Section']==record['section']and old['domain']['Interface']==record['interface']
    assert old['source_bytes']['PacketRefs']==[dict(Number=source['frame'],Domain=old['domain'])]
    at,end=location['offset'],location['end'];own=record['wire'][at:end]
    assert own[:2]==bytes([old['session']['Type'],old['session']['Code']])
    declared=enclosing(source,record);result=decode(own,declared);base=at-location['raw_start']
    fields=copy.deepcopy(result['fields'])
    def shift(node):
        if isinstance(node,dict):
            for name,value in node.items():
                if name=='Wire Field Ranges':
                    for v in value.values():v[0]+=base;v[1]+=base
                else:shift(value)
        elif isinstance(node,list):
            for item in node:shift(item)
    shift(fields)
    # No existing context is removed by constructing the new wire subtree.
    extra={k:v for k,v in old['session'].items()if k not in('Type','Code')}
    for name,value in extra.items():
        assert name not in fields,'an old context key collides with new wire contract'
        fields[name]=copy.deepcopy(value)
    expected=copy.deepcopy(old)
    status=result['status'];complete='message'if status=='decoded'else status
    expected.update(id=identifier,profile='icmpv6-mld',status=status,completeness=complete,
                    summary=result['summary'],expert=result['expert'],fields=copy.deepcopy(fields),session=copy.deepcopy(fields))
    expected.pop('fields_error',None)
    if result['expert']:expected['error']=result['expert']
    else:expected.pop('error',None)
    byte_source=copy.deepcopy(old['source_bytes'])
    byte_source.pop('CompletedIn',None)
    if status=='decoded':byte_source['CompletedIn']=copy.deepcopy(max(byte_source['PacketRefs'],key=lambda x:x['Number']))
    expected['source_bytes']=byte_source
    old_snapshot=old['field_values'];snapshot=copy.deepcopy(old_snapshot)
    snapshot.update(pdu=identifier,source=copy.deepcopy(byte_source),completeness=complete)
    snapshot.pop('incomplete',None)
    if status=='limited':snapshot['incomplete']=True
    # Existing event/base aliases retain every literal scalar and range. Only
    # the specific mapped publisher ID, status and completion fact can change.
    before=[]
    for v in old_snapshot['values']:
        v=copy.deepcopy(v)
        assert not v['name'].startswith(('icmpv6.mld.','icmpv6.mldr.'))and v['name']!='icmpv6.checksum'
        if v['name']=='pdu.id':v.update(value=identifier,exact_value=str(identifier))
        if v['name']=='status':v['value']=status
        if v['name']=='pdu.completed_in'and status!='decoded':continue
        if v['name']in('icmpv6.type','icmpv6.code'):
            offset=0 if v['name']=='icmpv6.type'else 1
            assert type(v['value'])is int and v['value']==own[offset]and v['pdu_range']==dict(offset=base+offset,length=1)
        before.append(v)
    # The registry reserves name-sorted event facts, then name-sorted wire
    # bindings; order among repeated values from one binding remains ordinal.
    added=[dict(name=f['name'],value=f['value'],origin='pdu',pdu_range=dict(offset=base+f['offset'],length=f['length']))for f in result['values']if f['name']]
    events=[v for v in before if v['origin']=='event']
    wire=[v for v in before if v['origin']!='event']+added
    snapshot['values']=sorted(events,key=lambda x:x['name'])+sorted(wire,key=lambda x:x['name'])
    expected['field_values']=snapshot
    # All keys not listed above, including Raw/domain/physical refs/flow and
    # carrier_layers, are preserved literally by construction, not waived.
    return expected,result,declared
