"""Independent RFC1112/2236/9776 IGMP membership byte oracle.

Only a caller-proven IGMP input envelope is accepted. No packet/application
decoder, candidate event, field label or Raw search supplies source coordinates.
"""
import base64
import ipaddress

KNOWN={0x11,0x12,0x16,0x17,0x22}
NAMES={1:'MODE_IS_INCLUDE',2:'MODE_IS_EXCLUDE',3:'CHANGE_TO_INCLUDE_MODE',4:'CHANGE_TO_EXCLUDE_MODE',5:'ALLOW_NEW_SOURCES',6:'BLOCK_OLD_SOURCES'}
SCOPE='captured membership wire fields; checksum, IPv4 envelope and membership state unverified'


def floating(code):
    return code if code<128 else ((code&15)+16)*(2**(((code//16)&7)+3))


def decode(w,declared,max_elements=4096):
    assert type(w)is bytes and type(declared)is int and type(max_elements)is int and max_elements>0
    m={'Observation Scope':SCOPE};fields={'IGMP':m};values=[]
    result={'fields':fields,'values':values,'version':0,'summary':'IPv4 protocol2; IGMP membership header unavailable','status':'decoded','expert':''}
    def leaf(target,key,alias,at,length,path,kind='unsigned',value=None):
        assert at>=0 and length>0 and at+length<=len(w)
        word=w[at:at+length]
        if value is None:
            value=int.from_bytes(word,'big')if kind=='unsigned'else str(ipaddress.IPv4Address(word))if kind=='address'else base64.b64encode(word).decode()
        target[key]=value
        values.append({'name':alias,'value':word.hex()if kind=='bytes'else value,'offset':at,'length':length,'path':path+[key]})
    def put(key,alias,at,length,kind='unsigned',value=None):leaf(m,key,alias,at,length,['IGMP'],kind,value)
    def fail(status,code,reason,required=0):
        result.update(status=status,expert=code,summary=result['summary']+'; '+reason)
        m.update(Status=status,Error=reason);m['Available IGMP Bytes']=len(w)
        if required>len(w):m['Minimum Bytes Needed for Next Field']=required
        return finish_ranges()
    def finish_ranges():
        for v in values:
            node=fields
            for part in v['path'][:-1]:node=node[int(part)]if isinstance(node,list)else node[part]
            key=v['path'][-1]
            if isinstance(node,list):
                # Array string leaves belong to their owning source-list map.
                node=fields
                for part in v['path'][:-2]:node=node[int(part)]if isinstance(node,list)else node[part]
                key=v['path'][-2]+'['+key+']'
            node.setdefault('Wire Field Ranges',{})[key]=[v['offset'],v['offset']+v['length']]
        return result
    def finish(at):
        if at<len(w):put('Additional Data (uninterpreted)','',at,len(w)-at,'bytes')
        if declared>len(w):return fail('incomplete','IGMPEnvelopeTruncated','declared enclosing message tail unavailable',declared)
        if declared<0:return fail('limited','IGMPLengthUnconfirmed','declared enclosing message extent unavailable')
        return finish_ranges()
    if not w:return fail('incomplete','IGMPTruncated','type octet unavailable',1)
    typ=w[0];leaf(fields,'Type','igmp.type',0,1,[])
    if typ not in KNOWN:return fail('limited','IGMPTypeUnselected','type outside selected membership profile')
    if declared>=0 and len(w)>declared:return fail('malformed','IGMPEnvelopeLength','captured bytes exceed enclosing declared extent')
    if len(w)>=4:leaf(fields,'Checksum','igmp.checksum',2,2,[])
    used=0
    if typ==0x22:
        result.update(version=3,summary='IGMPv3 Membership Report');m.update(Version=3,Message='Membership Report')
        for key,at,size in [('Reserved Octet',1,1),('Report Flags (raw)',4,2)]:
            if at+size<=len(w):put(key,'',at,size)
        if len(w)<8:return fail('incomplete','IGMPTruncated','v3 report fixed header unavailable',8)
        put('Group Record Count','igmp.num_grp_recs',6,2);records=[];m['Group Records']=records;at=8
        for i in range(m['Group Record Count']):
            if at>=len(w):return fail('incomplete','IGMPTruncated','group record fixed header unavailable',at+8)
            if used==max_elements:return fail('limited','IGMPCollectionBudget','group record collection budget exceeded')
            used+=1;rec={'Index':i+1};path=['IGMP','Group Records',str(i)];records.append(rec)
            for key,alias,off,size,kind in [('Record Type','igmp.record_type',0,1,'unsigned'),('Auxiliary Data Words','igmp.aux_data_len',1,1,'unsigned'),('Source Count','igmp.num_src',2,2,'unsigned'),('Multicast Address','igmp.maddr',4,4,'address')]:
                if at+off+size<=len(w):leaf(rec,key,alias,at+off,size,path,kind)
            rec['Record Type Name']=NAMES.get(rec['Record Type'],'unrecognized; wire record only')
            if at+8>len(w):return fail('incomplete','IGMPTruncated','group record fixed header unavailable',at+8)
            ns,aux=rec['Source Count'],4*rec['Auxiliary Data Words'];rec['Declared Record Bytes']=8+4*ns+aux;at+=8;rec['Sources']=[]
            for j in range(ns):
                if at+4>len(w):return fail('incomplete','IGMPTruncated','record source address unavailable',at+4)
                if used==max_elements:return fail('limited','IGMPCollectionBudget','source address collection budget exceeded')
                used+=1;value=str(ipaddress.IPv4Address(w[at:at+4]));rec['Sources'].append(value)
                values.append(dict(name='igmp.saddr',value=value,offset=at,length=4,path=path+['Sources',str(j)]));at+=4
            if at+aux>len(w):return fail('incomplete','IGMPTruncated','record auxiliary data unavailable',at+aux)
            if aux:leaf(rec,'Auxiliary Data (uninterpreted)','igmp.aux_data',at,aux,path,'bytes')
            at+=aux
        return finish(at)
    typinfo={0x12:(1,'Membership Report'),0x16:(2,'Membership Report'),0x17:(2,'Leave Group')}
    version,message=typinfo.get(typ,(0,'Membership Query'));invalid=False
    if typ==0x11 and len(w)>=2:
        # Unknown IPv4 extent may include physical padding; captured length
        # cannot select a query version or authorize extension fields.
        n=declared
        if n==8:version=1 if w[1]==0 else 2
        elif n>=12:version=3
        elif n>=8:invalid=True
        put('Max Response Code (raw)','igmp.max_resp',1,1)
        if version:put('Maximum Response Time (ms)','igmp.max_resp.decoded_ms',1,1,value=10000 if version==1 else 100*(floating(w[1])if version==3 else w[1]))
    elif typ!=0x11 and len(w)>=2:put('Reserved Response Octet','',1,1)
    result['version']=version;m['Message']=message
    if version:m['Version']=version
    result['summary']=('IGMPv%d '%version if version else'IGMP ')+message
    if len(w)>=8:put('Group Address','igmp.maddr',4,4,'address')
    if len(w)<8:return fail('incomplete','IGMPTruncated','query/report fixed header unavailable',8)
    if invalid:return fail('malformed','IGMPQueryLength','query selects neither 8-byte v1/v2 nor at-least-12-byte v3 layout')
    if typ==0x11 and not version:return fail('limited','IGMPLengthUnconfirmed','query version unavailable without declared enclosing length')
    if typ!=0x11 or version!=3:return finish(8)
    if len(w)>=9:
        for key,alias,value in [('Query Flags','igmp.query.flags',w[8]),('Suppress Router-Side Processing','igmp.s',bool(w[8]&8)),('Querier Robustness Variable','igmp.qrv',w[8]&7),('Extension Flags (raw)','igmp.query.reserved',w[8]>>4)]:put(key,alias,8,1,value=value)
    if len(w)>=10:
        put('Query Interval Code (raw)','igmp.qqic',9,1);put('Query Interval (seconds)','igmp.qqic.decoded_seconds',9,1,value=floating(w[9]))
    if len(w)<12:return fail('incomplete','IGMPTruncated','v3 query fixed header unavailable',12)
    put('Source Count','igmp.num_src',10,2);m['Sources']=[];at=12
    for i in range(m['Source Count']):
        if at+4>len(w):return fail('incomplete','IGMPTruncated','query source address unavailable',at+4)
        if used==max_elements:return fail('limited','IGMPCollectionBudget','source address collection budget exceeded')
        used+=1;value=str(ipaddress.IPv4Address(w[at:at+4]));m['Sources'].append(value)
        values.append(dict(name='igmp.saddr',value=value,offset=at,length=4,path=['IGMP','Sources',str(i)]));at+=4
    return finish(at)
