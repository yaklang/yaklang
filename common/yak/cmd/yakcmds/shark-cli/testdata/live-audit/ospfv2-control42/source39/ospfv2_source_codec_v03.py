"""Independent bounded OSPFv2 wire facts, RFC2328/5613/5709/3101.

This is an offline source oracle, not a native decoder or a stateful router.
Every scalar records its exact observed PDU/physical span; opaque authentication
data is represented by length/hash only. No MAC/SA/LSDB/neighbor/route is inferred.
"""
import hashlib,ipaddress

def digest(w):return hashlib.sha256(w).hexdigest()
def checksum(w):
    w=w+b'\0'if len(w)%2 else w;s=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,len(w),2))
    while s>>16:s=(s&0xffff)+(s>>16)
    return(~s)&0xffff
def fletcher(w):
    assert len(w)>=20;c0=c1=0
    for b in w[2:]:c0=(c0+b)%255;c1=(c1+c0)%255
    return dict(valid=c0==c1==0 and w[16:18]!=b'\0\0',residue_C0=c0,residue_C1=c1,scope='complete LSA excluding LS Age;no authentication claim')

class Source:
    def __init__(self,w,physical_start=0):self.w=w;self.physical_start=physical_start;self.leaves=[];self.counts={}
    def leaf(self,node,path,name,at,size,kind='uint',limit=None):
        if limit is None:limit=len(self.w)
        assert 0<=at<=at+size<=limit<=len(self.w),(path,name,at,size,limit,len(self.w));raw=self.w[at:at+size]
        value=str(ipaddress.IPv4Address(raw))if kind in('ipv4','id','mask')else dict(bytes=len(raw),SHA256=digest(raw))if kind=='opaque'else int.from_bytes(raw,'big')
        node[name]=value;node.setdefault('Wire Field Ranges',{})[name]=[at,at+size]
        self.leaves.append(dict(path=path+[name],value=value,encoding=kind,PDU_span=[at,at+size],physical_span=[self.physical_start+at,self.physical_start+at+size],wire_SHA256=digest(raw)))
        return value
    def header(self,at,path,full,limit):
        row={}
        for name,size,kind in [('LS Age',2,'uint'),('Options',1,'uint'),('LS Type',1,'uint'),('Link State ID',4,'id'),('Advertising Router',4,'id'),('Sequence',4,'uint'),('Checksum',2,'uint'),('Length',2,'uint')]:
            self.leaf(row,path,name,at,size,kind,limit);at+=size
        row['Checksum Verification']=dict(verified=False,scope='advertised LSA header without body')
        return row
    def lsa_body(self,row,start,end,path):
        typ=row['LS Type'];node={};at=start;mask=None
        def leaf(name,size,kind='uint'):
            nonlocal at
            v=self.leaf(node,path,name,at,size,kind,end);at+=size;return v
        if typ==1:
            leaf('Flags',1);leaf('Reserved',1);n=leaf('Link Count',2);assert n<=(end-at)//12;links=[]
            for i in range(n):
                child={};cp=path+['Links',i]
                for name,size,kind in [('Link ID',4,'id'),('Link Data',4,'ipv4'),('Link Type',1,'uint'),('TOS Count',1,'uint'),('Metric',2,'uint')]:self.leaf(child,cp,name,at,size,kind,end);at+=size
                tos=[];assert child['TOS Count']<=(end-at)//4
                for j in range(child['TOS Count']):
                    entry={}
                    for name,size in [('TOS',1),('Reserved',1),('Metric',2)]:self.leaf(entry,cp+['TOS Metrics',j],name,at,size,'uint',end);at+=size
                    tos.append(entry)
                child['TOS Metrics']=tos;child['Link Data Interpretation']='stub network mask'if child['Link Type']==3 else'raw IPv4 bits;interface-address vs unnumbered-ifIndex not certified';links.append(child)
            node['Links']=links
        elif typ==2:
            mask=leaf('Network Mask',4,'mask');assert(end-at)%4==0;routers=[]
            while at<end:
                child={};self.leaf(child,path+['Attached Routers',len(routers)],'Attached Router',at,4,'id',end);routers.append(child);at+=4
            node['Attached Routers']=routers
        elif typ in(3,4):
            mask=leaf('Network Mask',4,'mask');assert end-at>=4 and(end-at)%4==0;metrics=[]
            while at<end:
                child={};cp=path+['TOS Metrics',len(metrics)];self.leaf(child,cp,'TOS',at,1,'uint',end);self.leaf(child,cp,'Metric',at+1,3,'uint',end);at+=4;metrics.append(child)
            node['TOS Metrics']=metrics;node['Base TOS Field Meaning']='reserved zero in base metric;optional entries carry TOS'
            if typ==4:node['Destination ASBR Router ID']=row['Link State ID'];node['Network Mask Meaning']='not meaningful for ASBR summary;not an advertised network prefix'
        elif typ in(5,7):
            mask=leaf('Network Mask',4,'mask');assert end-at>=12 and(end-at)%12==0;metrics=[]
            while at<end:
                child={};cp=path+['TOS Metrics',len(metrics)];flags=self.leaf(child,cp,'Flags and TOS',at,1,'uint',end);self.leaf(child,cp,'Metric',at+1,3,'uint',end);self.leaf(child,cp,'Forwarding Address',at+4,4,'ipv4',end);self.leaf(child,cp,'External Route Tag',at+8,4,'uint',end)
                child['E Bit']=bool(flags&0x80);child['TOS']=flags&0x7f;child['Metric Type']=2 if flags&0x80 else 1;metrics.append(child);at+=12
            node['TOS Metrics']=metrics
            if typ==7:node['P Bit']=bool(row['Options']&8)
        else:
            if at<end:leaf('Opaque Body',end-at,'opaque')
            node['Unknown Body Semantics']=True
        assert at==end,(typ,start,at,end)
        if mask is not None and typ!=4:
            try:node['Normalized Advertised Prefix']=str(ipaddress.IPv4Network((row['Link State ID'],mask),strict=False))
            except ValueError:node['Normalized Advertised Prefix']=None;node['Prefix Interpretation']='non-contiguous/invalid network mask;wire values retained'
        return node
    def lls(self,start,limit,auth,crypto_sequence):
        node={};self.leaf(node,['LLS'],'Checksum',start,2,'uint',limit);words=self.leaf(node,['LLS'],'Length Words',start+2,2,'uint',limit);size=4*words;assert size>=4 and start+size<=limit
        end=start+size;at=start+4;rows=[];seen=set();violations=[]
        while at<end:
            row={};path=['LLS','TLVs',len(rows)];typ=self.leaf(row,path,'Type',at,2,'uint',end);n=self.leaf(row,path,'Length',at+2,2,'uint',end);value_start=at+4;value_end=value_start+n;pad_end=value_start+((n+3)//4)*4;assert pad_end<=end
            if typ==1:
                if n==4:self.leaf(row,path,'Extended Options and Flags',value_start,4,'uint',end)
                else:self.leaf(row,path,'Opaque Value',value_start,n,'opaque',end);violations.append('EOF TLV length is not4')
            elif typ==2:
                assert n>=4;sequence=self.leaf(row,path,'Sequence',value_start,4,'uint',end);self.leaf(row,path,'Authentication Data',value_start+4,n-4,'opaque',end);row['Sequence Matches OSPF Header']=auth==2 and sequence==crypto_sequence
                if auth!=2:violations.append('CA TLV without OSPF cryptographic authentication')
                if sequence!=crypto_sequence:violations.append('CA sequence differs from OSPF crypto header')
                if pad_end!=end:violations.append('CA TLV is not last')
            else:self.leaf(row,path,'Opaque Value',value_start,n,'opaque',end)
            if typ in(1,2)and typ in seen:violations.append('duplicate singleton LLS TLV '+str(typ))
            seen.add(typ)
            if pad_end>value_end:self.leaf(row,path,'Padding',value_end,pad_end-value_end,'opaque',end)
            rows.append(row);at=pad_end
        node['TLVs']=rows;node['Observed Extent']=[start,end];node['Length Bytes']=size;node['Authentication Verified']=False;node['Violations']=violations
        if auth==2:node['Checksum Verification']=dict(verified=False,scope='omitted:cryptographic LLS authentication;MAC/key/SA unavailable',field_zero=node['Checksum']==0)
        else:node['Checksum Verification']=dict(verified=checksum(self.w[start:end])==0,scope='LLS block only;independent of base OSPF packet',field_zero=node['Checksum']==0)
        return node,end
    def decode(self):
        w=self.w;assert len(w)>=24 and w[0]==2;tree={};header={};tree['Header']=header;at=0
        for name,size,kind in [('Version',1,'uint'),('Type',1,'uint'),('Packet Length',2,'uint'),('Router ID',4,'id'),('Area ID',4,'id'),('Checksum',2,'uint'),('Instance ID',1,'uint'),('Authentication Type',1,'uint')]:self.leaf(header,['Header'],name,at,size,kind);at+=size
        n=header['Packet Length'];typ=header['Type'];auth=header['Authentication Type'];assert 24<=n<=len(w)
        auth_fields={};tree['Authentication']=auth_fields
        if auth==2:
            for name,at,size in [('Reserved',16,2),('Key ID',18,1),('Authentication Data Length',19,1),('Sequence',20,4)]:self.leaf(auth_fields,['Authentication'],name,at,size)
            auth_fields['Algorithm']='not selected without configured SA';auth_fields['Authentication Verified']=False
        else:self.leaf(auth_fields,['Authentication'],'Observed Authentication Field',16,8,'opaque');auth_fields['Authentication Verified']=False
        at=24;Lbit=False;options=None
        if typ==1:
            node={};tree['Hello']=node
            for name,size,kind in [('Network Mask',4,'mask'),('Hello Interval',2,'uint'),('Options',1,'uint'),('Router Priority',1,'uint'),('Router Dead Interval',4,'uint'),('Designated Router',4,'id'),('Backup Designated Router',4,'id')]:self.leaf(node,['Hello'],name,at,size,kind,n);at+=size
            options=node['Options'];assert(n-at)%4==0;neighbors=[]
            while at<n:
                row={};self.leaf(row,['Hello','Neighbors',len(neighbors)],'Neighbor Router ID',at,4,'id',n);neighbors.append(row);at+=4
            node['Neighbors']=neighbors
        elif typ==2:
            node={};tree['Database Description']=node
            for name,size in [('Interface MTU',2),('Options',1),('Flags',1),('DD Sequence',4)]:self.leaf(node,['Database Description'],name,at,size,'uint',n);at+=size
            options=node['Options'];assert(n-at)%20==0;rows=[]
            while at<n:rows.append(self.header(at,['Database Description','LSA Headers',len(rows)],False,n));at+=20
            node['LSA Headers']=rows
        elif typ==3:
            assert(n-at)%12==0;rows=[]
            while at<n:
                row={};path=['Requests',len(rows)]
                for name,size,kind in [('LS Type',4,'uint'),('Link State ID',4,'id'),('Advertising Router',4,'id')]:self.leaf(row,path,name,at,size,kind,n);at+=size
                rows.append(row)
            tree['Requests']=rows
        elif typ==4:
            count=self.leaf(tree,[],'LSA Count',at,4,'uint',n);at+=4;assert count<=(n-at)//20;rows=[]
            for i in range(count):
                row=self.header(at,['LSAs',i],True,n);end=at+row['Length'];assert row['Length']>=20 and end<=n;row['Body']=self.lsa_body(row,at+20,end,['LSAs',i,'Body']);row['Checksum Verification']=fletcher(w[at:end]);rows.append(row);at=end
            tree['LSAs']=rows
        elif typ==5:
            assert(n-at)%20==0;rows=[]
            while at<n:rows.append(self.header(at,['LSA Headers',len(rows)],False,n));at+=20
            tree['LSA Headers']=rows
        else:
            if at<n:self.leaf(tree,[],'Opaque Body',at,n-at,'opaque',n)
            at=n;tree['Unknown Message Type']=True
        assert at==n
        if auth in(0,1):tree['Packet Checksum Verification']=dict(verified=checksum(w[:16]+w[24:n])==0,scope='OSPF packet length excluding authentication8 bytes;not authentication validation')
        elif auth==2:tree['Packet Checksum Verification']=dict(verified=False,scope='omitted:cryptographic authentication;MAC/key/SA unavailable',field_zero=header['Checksum']==0)
        else:tree['Packet Checksum Verification']=dict(verified=False,scope='unselected authentication type;no checksum applicability verdict')
        tail=n
        if auth==2:
            size=auth_fields['Authentication Data Length'];self.leaf(auth_fields,['Authentication'],'Authentication Digest',tail,size,'opaque');tail+=size
        if options is not None:Lbit=bool(options&0x10)
        tree['L Bit']=Lbit
        if Lbit:tree['LLS'],tail=self.lls(tail,len(w),auth,auth_fields.get('Sequence'))
        if tail<len(w):self.leaf(tree,[],'Unexplained Tail',tail,len(w)-tail,'opaque')
        tree['Base Body Extent']=[24,n];tree['Observed PDU Extent']=[0,len(w)];tree['Authentication Verified']=False
        return dict(fields=tree,source_leaves=self.leaves,source_observed_bytes=len(w),source_OSPF_packet_length=n,no_LSDB_neighbor_SA_replay_or_installed_route_inferred=True)

def decode(w,physical_start=0):return Source(w,physical_start).decode()
