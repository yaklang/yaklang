"""Independent complete MLD observation expectation from its bounded wire.

The existing source scalar proof and RFC2710/9777 are the byte authority. This
module adds the root's pre-runtime semantic tree contract and metadata spans;
it does not read a candidate decoder, event or newly replayed export.
"""
import base64
import ipaddress

RECORD_NAMES={1:'MODE_IS_INCLUDE',2:'MODE_IS_EXCLUDE',3:'CHANGE_TO_INCLUDE_MODE',4:'CHANGE_TO_EXCLUDE_MODE',5:'ALLOW_NEW_SOURCES',6:'BLOCK_OLD_SOURCES'}
SCOPE='captured wire fields; checksum, IP envelope and listener state unverified'


def addr(w):
    a=ipaddress.IPv6Address(w)
    return '::ffff:'+str(a.ipv4_mapped)if a.ipv4_mapped is not None else str(a)


def decode(w,declared,max_elements=4096):
    assert type(w)is bytes and len(w)>=4 and w[0]in(130,131,132,143)
    assert type(declared)is int and (declared<0 or declared>=len(w))
    m={'Observation Scope':SCOPE};fields={'Type':w[0],'Code':w[1],'Checksum':int.from_bytes(w[2:4],'big'),'MLD':m};values=[]
    r=dict(fields=fields,values=values,version=0,status='decoded',expert='',summary='')
    def leaf(target,key,alias,at,size,path,kind='unsigned',value=None):
        assert at>=0 and size>0 and at+size<=len(w)
        word=w[at:at+size]
        if value is None:value=addr(word)if kind=='address'else base64.b64encode(word).decode()if kind=='bytes'else int.from_bytes(word,'big')
        target[key]=value
        values.append(dict(name=alias,value=word.hex()if kind=='bytes'else value,offset=at,length=size,path=path+[key]))
    def put(key,alias,at,size,kind='unsigned',value=None):leaf(m,key,alias,at,size,['MLD'],kind,value)
    def ranges():
        for f in values:
            node=fields
            for k in f['path'][:-1]:node=node[int(k)]if isinstance(node,list)else node[k]
            key=f['path'][-1]
            if isinstance(node,list):
                node=fields
                for k in f['path'][:-2]:node=node[int(k)]if isinstance(node,list)else node[k]
                key=f['path'][-2]+'['+key+']'
            node.setdefault('Wire Field Ranges',{})[key]=[f['offset'],f['offset']+f['length']]
        return r
    def fail(status,code,reason,required=0):
        r.update(status=status,expert=code,summary=r['summary']+'; '+reason)
        m.update(Status=status,Error=reason);m['Available ICMPv6 Bytes']=len(w)
        if required>len(w):m['Minimum Bytes Needed for Next Field']=required
        return ranges()
    def finish(at):
        if at<len(w):put('Additional Data (uninterpreted)','',at,len(w)-at,'bytes')
        if declared>len(w):return fail('incomplete','MLDEnvelopeTruncated','declared enclosing message tail unavailable',declared)
        if declared<0:return fail('limited','MLDLengthUnconfirmed','declared enclosing message length unavailable')
        return ranges()
    leaf(fields,'Type','',0,1,[]);leaf(fields,'Code','',1,1,[]);leaf(fields,'Checksum','icmpv6.checksum',2,2,[])
    used=0
    if w[0]==143:
        r.update(version=2,summary='MLDv2 Multicast Listener Report');m.update(Version=2,Message='Multicast Listener Report')
        if len(w)>=6:put('Report Flags (raw)','',4,2)
        if len(w)<8:return fail('incomplete','MLDTruncated','report fixed header unavailable',8)
        put('Address Record Count','icmpv6.mldr.nb_mcast_records',6,2);records=[];m['Address Records']=records;at=8
        for i in range(m['Address Record Count']):
            if at>=len(w):return fail('incomplete','MLDTruncated','address record fixed header unavailable',at+20)
            if used==max_elements:return fail('limited','MLDCollectionBudget','address record collection budget exceeded')
            used+=1;rec={'Index':i+1};path=['MLD','Address Records',str(i)];records.append(rec)
            leaf(rec,'Record Type','icmpv6.mldr.mar.record_type',at,1,path)
            rec['Record Type Name']=RECORD_NAMES.get(rec['Record Type'],'unrecognized; record bytes observed only')
            values.append(dict(name='',value=rec['Record Type Name'],offset=at,length=1,path=path+['Record Type Name']))
            for key,alias,off,size,kind in [('Auxiliary Data Words','icmpv6.mldr.mar.aux_data_len',1,1,'unsigned'),('Source Count','icmpv6.mldr.mar.nb_sources',2,2,'unsigned'),('Multicast Address','icmpv6.mldr.mar.multicast_address',4,16,'address')]:
                if at+off+size<=len(w):leaf(rec,key,alias,at+off,size,path,kind)
            if at+20>len(w):return fail('incomplete','MLDTruncated','address record fixed header unavailable',at+20)
            ns=rec['Source Count'];aux=4*rec['Auxiliary Data Words'];rec['Declared Record Bytes']=20+16*ns+aux;at+=20;rec['Sources']=[]
            for j in range(ns):
                if at+16>len(w):return fail('incomplete','MLDTruncated','record source address unavailable',at+16)
                if used==max_elements:return fail('limited','MLDCollectionBudget','source address collection budget exceeded')
                used+=1;value=addr(w[at:at+16]);rec['Sources'].append(value)
                values.append(dict(name='icmpv6.mldr.mar.source_address',value=value,offset=at,length=16,path=path+['Sources',str(j)]));at+=16
            if at+aux>len(w):return fail('incomplete','MLDTruncated','record auxiliary data unavailable',at+aux)
            if aux:leaf(rec,'Auxiliary Data (uninterpreted)','icmpv6.mldr.mar.auxiliary_data',at,aux,path,'bytes')
            at+=aux
        return finish(at)
    message={130:'Multicast Listener Query',131:'Multicast Listener Report',132:'Multicast Listener Done'}[w[0]]
    m['Message']=message;r['summary']='MLD Multicast Listener Query';invalid=False
    if w[0]in(131,132):
        r['version']=1;r['summary']='MLDv1 '+message
    else:
        # Captured bytes can contain enclosing padding when extent is unknown.
        # A query version requires the actual declared enclosing length.
        n=declared
        if n==24:r['version']=1
        elif n>=28:r['version']=2
        elif n>=24:invalid=True
    if r['version']:
        m['Version']=r['version']
        if w[0]==130:r['summary']='MLDv%d Multicast Listener Query'%r['version']
    if len(w)>=6:
        if w[0]!=130:put('Unused Delay Word','',4,2)
        else:
            code=int.from_bytes(w[4:6],'big');put('Response Code/Delay Word','icmpv6.mld.maximum_response_code.raw',4,2)
            if r['version']==1:put('Maximum Response Delay (ms)','icmpv6.mld.maximum_response_delay',4,2,value=code)
            elif r['version']==2:put('Maximum Response Delay (ms)','icmpv6.mld.maximum_response_code',4,2,value=code if code<32768 else ((code&4095)+4096)*2**(((code>>12)&7)+3))
    if len(w)>=8:put('Reserved','',6,2)
    if len(w)>=24:put('Multicast Address','icmpv6.mld.multicast_address',8,16,'address')
    if len(w)<24:return fail('incomplete','MLDTruncated','query/report/done fixed header unavailable',24)
    if invalid:return fail('malformed','MLDQueryLength','query length selects neither v1 (24) nor v2 (at least 28)')
    if not r['version']:return fail('limited','MLDLengthUnconfirmed','query version unavailable without a declared complete extent')
    if r['version']==1:return finish(24)
    if len(w)>=25:
        for key,alias,value in [('Flags','icmpv6.mld.flag',w[24]),('Suppress Router-Side Processing','icmpv6.mld.flag.s',bool(w[24]&8)),('Querier Robustness Variable','icmpv6.mld.flag.qrv',w[24]&7),('Extension Flags (raw)','icmpv6.mld.flag.reserved',w[24]>>4)]:put(key,alias,24,1,value=value)
    if len(w)>=26:
        code=w[25];put('Query Interval Code','icmpv6.mld.qqi.raw',25,1)
        put('Query Interval (s)','icmpv6.mld.qqi',25,1,value=code if code<128 else ((code&15)+16)*2**(((code>>4)&7)+3))
    if len(w)<28:return fail('incomplete','MLDTruncated','v2 query fixed header unavailable',28)
    put('Source Count','icmpv6.mld.nb_sources',26,2);m['Sources']=[];at=28
    for i in range(m['Source Count']):
        if at+16>len(w):return fail('incomplete','MLDTruncated','query source address unavailable',at+16)
        if used==max_elements:return fail('limited','MLDCollectionBudget','source address collection budget exceeded')
        used+=1;value=addr(w[at:at+16]);m['Sources'].append(value)
        values.append(dict(name='icmpv6.mld.source_address',value=value,offset=at,length=16,path=['MLD','Sources',str(i)]));at+=16
    return finish(at)
