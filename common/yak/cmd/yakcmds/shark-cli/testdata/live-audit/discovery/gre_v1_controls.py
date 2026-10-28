"""Author small, explicit GRE-v1/PPTP admission and session-identity controls.

These packets are offline authored controls, not live traffic or copies of the
public corpus. They distinguish RFC 2784 generic GRE-v0 from RFC 2637 enhanced
GRE-v1 and isolate payload-length/Call-ID ownership without a PPTP control call.
The latter is an observed header/data-path test, not proof of a negotiated call.
"""
import argparse,hashlib,ipaddress,json,struct
from pathlib import Path


def sha(w):return hashlib.sha256(w).hexdigest()
def checksum(w):
    if len(w)%2:w+=b'\0'
    n=sum(struct.unpack('>'+str(len(w)//2)+'H',w))
    while n>>16:n=(n&65535)+(n>>16)
    return(~n)&65535
def ipv4(payload,proto,source='192.0.2.1',destination='192.0.2.2'):
    a=ipaddress.ip_address(source).packed;b=ipaddress.ip_address(destination).packed
    header=struct.pack('>BBHHHBBH4s4s',0x45,0,20+len(payload),42,0,64,proto,0,a,b)
    return header[:10]+struct.pack('>H',checksum(header))+header[12:]+payload
def udp(payload):
    a=ipaddress.ip_address('192.0.2.1').packed;b=ipaddress.ip_address('192.0.2.2').packed
    header=struct.pack('>HHHH',53000,53,8+len(payload),0);value=checksum(a+b+b'\0\x11'+struct.pack('>H',len(header)+len(payload))+header+payload)or 65535
    return header[:6]+struct.pack('>H',value)+payload
def tcp(payload,sequence,flags):
    a=ipaddress.ip_address('192.0.2.1').packed;b=ipaddress.ip_address('192.0.2.2').packed
    header=struct.pack('>HHIIBBHHH',50000,80,sequence,0,0x50,flags,65535,0,0);value=checksum(a+b+b'\0\x06'+struct.pack('>H',len(header)+len(payload))+header+payload)
    return header[:16]+struct.pack('>H',value)+header[18:]+payload
def ethernet(ip):return bytes.fromhex('0200000000020200000000010800')+ip
def generic(inner,version=0,key=42):return struct.pack('>HHI',0x2000|version,0x0800,key)+inner
def pptp(inner,flags=0x3001,call=42,sequence=0,ack=0,length=None):
    payload=b'\x00\x21'+inner;size=len(payload)if length is None else length;header=struct.pack('>HH',flags,0x880b)
    if flags&0xc000:header+=b'\0'*4
    if flags&0x2000:header+=struct.pack('>HH',size,call)
    if flags&0x1000:header+=struct.pack('>I',sequence)
    if flags&0x4000:header+=b'\0'*4
    if flags&0x80:header+=struct.pack('>I',ack)
    return header+payload
def wrap(gre):return ethernet(ipv4(gre,47,'198.51.100.1','198.51.100.2'))
def pcap(path,wires):
    with path.open('xb')as out:
        out.write(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
        for n,w in enumerate(wires):out.write(struct.pack('<IIII',1700000000,n*1000,len(w),len(w))+w)


def run(out):
    assert not any((v/'.git').exists()for v in[out.resolve(),*out.resolve().parents]);out.mkdir(parents=True,exist_ok=False)
    DNS=bytes.fromhex('123401000001000000000000076578616d706c6503636f6d0000010001');ip=ipv4(udp(DNS),17)
    request=b'GET /gre-v1 HTTP/1.1\r\nHost: example.test\r\n\r\n';chunks=[b'',request[:10],request[10:]]
    cases={
        'GRE-v0-DNS-selected':([wrap(generic(ip))],'selected generic GRE-v0 IPv4 control'),
        'GRE-v1-IPv4-no-PPTP':([wrap(generic(ip,1))],'version one with generic IPv4 EtherType and no PPTP sequence layout'),
        'GRE-v1-IPv4-no-key':([wrap(struct.pack('>HH',1,0x0800)+ip)],'version one with no key and generic IPv4 type'),
        'PPTP-v1-DNS-header-control':([wrap(pptp(ip))],'complete enhanced-GRE header and PPP/IP data, without captured PPTP call control'),
        'PPTP-v1-zero-declared-payload':([wrap(pptp(ip,length=0))],'declared zero payload followed by actual PPP/IP bytes'),
        'PPTP-v1-short-declared-payload':([wrap(pptp(ip,length=2))],'only PPP protocol bytes declared, trailing IP bytes outside GRE payload'),
        'PPTP-v1-long-declared-payload':([wrap(pptp(ip,length=2+len(ip)+10))],'GRE payload length exceeds a complete outer IP envelope by ten bytes'),
        'PPTP-v1-data-without-S':([wrap(pptp(ip,flags=0x2001))],'payload present without required sequence field'),
        'PPTP-v1-ACK-only-with-data':([wrap(pptp(ip,flags=0x2081,length=0,ack=7))],'ACK-only header has out-of-envelope PPP/IP trailing bytes'),
        'PPTP-v1-reserved-strict-route':([wrap(pptp(ip,flags=0x3801))],'reserved strict source route bit set'),
        'PPTP-v1-reserved-recur':([wrap(pptp(ip,flags=0x3101))],'reserved recursion bit set'),
        'PPTP-v1-reserved-flags':([wrap(pptp(ip,flags=0x3009))],'reserved enhanced GRE flags bit set'),
        'PPTP-v1-forbidden-checksum':([wrap(pptp(ip,flags=0xb001))],'checksum present flag forbidden by PPTP header grammar'),
    }
    for version in[0,1]:
        wires=[];sequence=1000
        for n,body in enumerate(chunks):
            segment=ipv4(tcp(body,sequence,2 if n==0 else 0x18),6);sequence+=len(body)+(n==0)
            wires.append(wrap(generic(segment)if version==0 else pptp(segment,sequence=n)))
        cases['GRE-v0-TCP-constant-key'if version==0 else'PPTP-v1-TCP-variable-length']=wires,'same inner TCP tuple with SYN and two different-length request contributors'
    ledger=[]
    for name,(wires,scope)in cases.items():
        dest=out/name;dest.mkdir();capture=dest/'observation.pcap';pcap(capture,wires)
        ledger.append(dict(name=name,scope=scope,capture=str(capture.resolve()),capture_SHA256=sha(capture.read_bytes()),frames=[dict(frame=n+1,wire_SHA256=sha(w),captured_length=len(w),GRE_offset=34,GRE_flags=w[34:36].hex(),GRE_protocol_type=w[36:38].hex(),GRE_key_literal=int.from_bytes(w[38:42],'big')if w[34]&0x20 else None)for n,w in enumerate(wires)]))
    manifest=dict(scope=__doc__,generator_SHA256=sha(Path(__file__).read_bytes()),references=['https://www.rfc-editor.org/rfc/rfc2784.html#section-2.3.1','https://www.rfc-editor.org/rfc/rfc2637.html#section-4.1','https://www.rfc-editor.org/rfc/rfc2637.html#section-3.2.2'],cases=ledger)
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(cases=len(ledger),bytes=sum(Path(v['capture']).stat().st_size for v in ledger))),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);run(ap.parse_args().out)
