"""Sequential Raw-bound SSH metadata constructor for old-output preservation.
This validates old field values against their own Raw, not PCAP identification.
It never reads a candidate output or imports the production decoder.
"""
import base64,copy,hashlib,json

def demand(v,m):
    if not v:raise AssertionError(m)
def canonical(x):return json.dumps(x,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def digest(x):return hashlib.sha256(canonical(x)).hexdigest()
def strict(x,y):return canonical(x)==canonical(y)
def legacy(x):
    if isinstance(x,dict):return{k:legacy(v)for k,v in x.items()if k!='Wire Field Ranges'}
    return x
class Reader:
    def __init__(self,raw,start,end):self.raw,self.at,self.end=raw,start,end;self.value={};self.ranges={}
    def take(self,name,n,kind='bytes'):
        demand(0<=n<=self.end-self.at,'literal crosses enclosing SSH owner '+name);a=self.at;self.at+=n;b=self.raw[a:self.at]
        v=int.from_bytes(b,'big')if kind=='uint'else b.decode('utf-8')if kind=='string'else base64.b64encode(b).decode()
        self.value[name]=v;self.ranges[name]=[a,self.at];return b
    def uint(self,name,n=4):return int.from_bytes(self.take(name,n,'uint'),'big')
    def string(self,name,kind='bytes'):
        n=self.uint(name+' Length');return self.take(name,n,kind)
    def mp(self,name):
        b=self.string(name);demand(not b or not(b[0]&128 or b[0]==0 and(len(b)==1 or not b[1]&128)),'noncanonical positive SSH mpint');return b
    def nested(self,name,fn):
        n=self.uint(name+' Length');demand(n<=self.end-self.at,'nested SSH owner extent')
        c=Reader(self.raw,self.at,self.at+n);self.at+=n;fn(c);self.value[name]=c.finish()
    def finish(self):
        demand(self.at==self.end,'unconsumed bytes in old selected SSH layout');self.value['Wire Field Ranges']=self.ranges;return self.value

LISTS=['Kex Algorithms','Host Key Algorithms','Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C','Languages C2S','Languages S2C']
DH=['diffie-hellman-group1-sha1','diffie-hellman-group14-sha1','diffie-hellman-group14-sha256','diffie-hellman-group15-sha512','diffie-hellman-group16-sha512','diffie-hellman-group17-sha512','diffie-hellman-group18-sha512']
PROFILES={**{n:'dh'for n in DH},'diffie-hellman-group-exchange-sha1':'dh-gex','diffie-hellman-group-exchange-sha256':'dh-gex','ecdh-sha2-nistp256':'p256','curve25519-sha256':'curve','curve25519-sha256@libssh.org':'curve'}
def point(c,name):
    def inner(r):
        fmt=r.uint(name+' Format',1);demand(fmt in(2,3,4),'old selected P256 encoding')
        r.take(name+' X',32)
        if fmt==4:r.take(name+' Y',32)
    c.nested(name,inner)
def reply(c,profile):
    def host(r):
        name=r.string('Host Key Format','string').decode('utf-8')
        if name=='ssh-rsa':r.mp('RSA Exponent');r.mp('RSA Modulus')
        elif name in('ssh-ed25519','ssh-ed448'):
            key='Ed25519 Public Key'if name=='ssh-ed25519'else'Ed448 Public Key';b=r.string(key);demand(len(b)==(32 if name=='ssh-ed25519' else 57),'selected public key length')
        else:r.take('Opaque Host Key Data',r.end-r.at)
    c.nested('Host Key',host)
    if profile=='curve':demand(len(c.string('Server Curve25519 Public Key'))==32,'curve public key length')
    elif profile=='p256':point(c,'Server Point')
    else:c.mp('Server Exchange Value')
    def sig(r):
        name=r.string('Signature Format','string').decode('utf-8')
        if name in('ssh-rsa','rsa-sha2-256','rsa-sha2-512'):r.string('RSA Signature Bytes')
        elif name in('ssh-ed25519','ssh-ed448'):
            key='Ed25519 Signature Bytes'if name=='ssh-ed25519'else'Ed448 Signature Bytes';b=r.string(key);demand(len(b)==(64 if name=='ssh-ed25519' else 114),'selected signature length')
        else:r.take('Opaque Signature Data',r.end-r.at)
    c.nested('Signature',sig)

def packet(raw,profile='transport'):
    demand(16<=len(raw)<=35000 and len(raw)%8==0,'old clear packet boundary')
    plen=int.from_bytes(raw[:4],'big');pad=raw[4];demand(plen+4==len(raw)and pad>=4 and pad+6<=len(raw),'SSH framing')
    end=len(raw)-pad;h=Reader(raw,0,5);h.uint('Packet Length');h.uint('Padding Length',1)
    p=Reader(raw,5,end);msg=p.uint('Message Number',1)
    if msg==20:
        p.take('Cookie',16)
        for name in LISTS:p.string(name,'string')
        p.uint('First Kex Packet Follows',1);p.uint('Reserved')
    elif msg==21:pass
    elif profile=='dh-gex'and msg in(30,34):
        for name in(['Preferred Group Bits']if msg==30 else['Minimum Group Bits','Preferred Group Bits','Maximum Group Bits']):p.uint(name)
    elif profile=='dh-gex'and msg==31:p.mp('Group Prime');p.mp('Group Generator')
    elif profile=='dh-gex'and msg==32 or profile=='dh'and msg==30:p.mp('Client Exchange Value')
    elif profile=='dh-gex'and msg==33 or profile in('dh','p256','curve')and msg==31:reply(p,profile)
    elif profile=='p256'and msg==30:point(p,'Client Point')
    elif profile=='curve'and msg==30:demand(len(p.string('Client Curve25519 Public Key'))==32,'curve public key length')
    else:p.take('Opaque Message Data',p.end-p.at)
    h.value['Payload']=p.finish();r=Reader(raw,end,len(raw));r.take('Padding',len(raw)-end)
    h.value['Padding']=r.value['Padding'];h.ranges['Padding']=r.ranges['Padding'];return h.finish(),msg

def map_additions(old,new,path):
    demand(strict(legacy(new),old),'constructed old field values differ at '+str(path))
    out=[]
    for k,v in new.items():
        if k=='Wire Field Ranges':demand(k not in old,'preexisting SSH WR not partof addition contract');out.append({'path':path+[k],'value':copy.deepcopy(v)})
        elif isinstance(v,dict):out+=map_additions(old[k],v,path+[k])
    return out

def native_expected(old):
    demand(old['protocol']=='ssh','SSH-only constructor');new=copy.deepcopy(old);adds=[]
    raw=base64.b64decode(old['raw'],validate=True)if old.get('raw')else b'';f=old.get('fields');s=old.get('session');current={};wire=None;msg=None
    if f is not None:
        demand(isinstance(f,dict),'old SSH fields shape')
        if 'Identification'in f:
            demand(raw.endswith(b'\n')and raw.startswith((b'SSH-2.0-',b'SSH-1.99-')),'complete identification wire')
            literal=raw[:-1];literal=literal[:-1]if literal.endswith(b'\r')else literal
            demand(f['Identification']==literal.decode('utf-8'),'identification literal')
            version='1.99'if raw.startswith(b'SSH-1.99-')else'2.0';demand(f['Version']==version,'banner version literal')
            current={'Identification':[0,len(literal)],'Version':[4,4+len(version)]}
            nf=copy.deepcopy(f);nf['Wire Field Ranges']=current;adds+=map_additions(f,nf,['fields'])
        elif'Text'in f:
            demand(raw.endswith(b'\n'),'complete pre-identification');literal=raw[:-1];literal=literal[:-1]if literal.endswith(b'\r')else literal
            demand(f['Text']==literal.decode('utf-8'),'pre-identification literal');current={'Text':[0,len(literal)]}
            nf=copy.deepcopy(f);nf['Wire Field Ranges']=current;adds+=map_additions(f,nf,['fields'])
        elif 'Packet Length'in f and'Payload'in f:
            profile=PROFILES.get((s or{}).get('Kex Algorithm'),'transport')
            if'Opaque Message Data'in f['Payload']:profile='transport'
            wire,msg=packet(raw,profile);adds+=map_additions(f,wire,['fields'])
        else:raise AssertionError('uncovered old SSH fields shape')
    if s is not None:
        demand(isinstance(s,dict),'old Session owner')
        if msg is None and'Message Number'in s:
            # A refused message may retain its complete transport-bound byte,
            # without gaining successful generic or selected GetFields.
            _,msg=packet(raw,'transport')
        if msg is not None:
            demand(s.get('Message Number')==msg,'current Session message literal');current['Message Number']=[5,6]
        if wire is not None and msg==20:
            p=wire['Payload'];ranges=p['Wire Field Ranges']
            for name in LISTS+['First Kex Packet Follows','Reserved']:
                key='Offered '+name if name in LISTS[2:8]else name
                demand(strict(s[key],p[name]),'current sender offer value '+key);current[key]=copy.deepcopy(ranges[name])
        if'Initial Packet Fields'in s:
            demand(wire is not None,'selected field tree needs complete current wire')
            adds+=map_additions(s['Initial Packet Fields'],wire,['session','Initial Packet Fields'])
        if'Host Key Format'in s:
            demand(wire is not None and'Host Key'in wire['Payload'],'host-key format needs selected reply owner')
            key=wire['Payload']['Host Key'];demand(s['Host Key Format']==key['Host Key Format'],'current reply format value');current['Host Key Format']=key['Wire Field Ranges']['Host Key Format']
        if current:
            demand('Wire Field Ranges'not in s,'old root Session WR must not be overwritten');adds.append({'path':['session','Wire Field Ranges'],'value':copy.deepcopy(current)})
    alias_add=[]
    for i,v in enumerate(old.get('field_values',{}).get('values',[])):
        key={'ssh.message_code':'Message Number','ssh.kex_algorithms':'Kex Algorithms','ssh.server_host_key_algorithms':'Host Key Algorithms'}.get(v.get('name'))
        if key is None or key not in current:continue
        demand(v['origin']=='pdu'and s is not None and strict(v['value'],s[key]),'private current alias value/origin')
        a,b=current[key];demand('pdu_range'not in v,'old alias range cannot be replaced')
        patch={'path':['field_values','values',i,'pdu_range'],'value':{'offset':a,'length':b-a}}
        adds.append(patch);alias_add.append({'index':i,'name':v['name'],'old_value_SHA256':digest(v['value']),'value':patch['value']})
    for a in adds:apply_addition(new,a)
    return new,adds,alias_add

def apply_addition(obj,change):
    p=obj
    for k in change['path'][:-1]:p=p[k]
    k=change['path'][-1];demand(isinstance(p,dict)and k not in p,'only new metadata keys may beadded');p[k]=copy.deepcopy(change['value'])

def compact_key(e,native=False):
    return digest([e['id'],e.get('tcp_flow_id'),e['source_bytes']if native else e['byte_source'],e['field_values']])