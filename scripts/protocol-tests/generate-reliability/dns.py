"""Controlled RFC 1035/4795 UDP peers, not a production DNS/LLMNR daemon.
Capture lo0 UDP ports 19553 or 5355. No external DNS traffic is sent.
"""
import socket
import struct

def query(labels):
    return struct.pack('!6H', 0x1234, 0, 1, 0, 0, 0) + b''.join(bytes([len(x)])+x for x in labels) + b'\0\0\1\0\1'
def response(q):
    return q[:2] + b'\x80\0' + q[4:]

def dns():
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as server, socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as client:
        server.bind(('127.0.0.1',19553));client.bind(('127.0.0.1',0));client.settimeout(2)
        for labels in ([b'a.b'],[b'a',b'b'],[b'EXAMPLE'],[b'example']):
            q=query(labels);client.sendto(q,server.getsockname());got,addr=server.recvfrom(4096)
            server.sendto(response(got),addr);assert client.recvfrom(4096)[0]==response(q)
            print('DNS',labels,flush=True)

def llmnr():
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as listener, socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as client:
        listener.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);listener.bind(('',5355));listener.settimeout(2)
        listener.setsockopt(socket.IPPROTO_IP,socket.IP_ADD_MEMBERSHIP,socket.inet_aton('224.0.0.252')+socket.inet_aton('127.0.0.1'))
        client.bind(('127.0.0.1',0));client.settimeout(2);client.setsockopt(socket.IPPROTO_IP,socket.IP_MULTICAST_IF,socket.inet_aton('127.0.0.1'))
        client.setsockopt(socket.IPPROTO_IP,socket.IP_MULTICAST_TTL,1)
        q=query([b'local-test']);client.sendto(q,('224.0.0.252',5355));got,addr=listener.recvfrom(4096);assert got==q
        listener.sendto(response(got),addr);assert client.recvfrom(4096)[0]==response(q)
        print('LLMNR multicast query / unicast response',flush=True)

dns();llmnr()
