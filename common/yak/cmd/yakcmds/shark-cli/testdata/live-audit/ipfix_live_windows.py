"""Capture newly generated owned IPFIX socket traffic on Windows Npcap Loopback.
This is an authored exporter/collector experiment, not production router traffic.
Only127.0.0.1:4739 is captured; no remote destinations or existing services used.
"""
import argparse,hashlib,json,socket,struct,subprocess,threading,time
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['dumpcap','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
assert not (a.out/'capture.pcapng').exists(),'use a fresh output directory'
def st(i,b):return struct.pack('!HH',i,len(b)+4)+b
def msg(seq,*sets):return struct.pack('!HHIII',10,16+sum(map(len,sets)),int(time.time()),seq,19)+b''.join(sets)
template=st(2,bytes.fromhex('01000003000800040002000400980008'))
def data(n):return st(256,bytes([192,0,2,10+n])+struct.pack('!IQ',17+n,int(time.time()*1000)))
udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);tcp=socket.socket();udp.bind(('127.0.0.1',4739));tcp.bind(('127.0.0.1',4739));tcp.listen(4);tcp.settimeout(.2);udp.settimeout(.2)
proxy=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);proxy.bind(('127.0.0.1',0));proxy.settimeout(.2)
forward=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);forward.bind(('127.0.0.1',0))
stop=threading.Event();errors=[];received_udp=[];received_tcp=[];sent_tcp=[];stats={'dropped':0,'delayed':0,'forwarded':0};threads=[]
def guarded(fn):
 def wrapped():
  try:fn()
  except Exception as e:errors.append(repr(e))
 t=threading.Thread(target=wrapped,daemon=True);t.start();threads.append(t)
def collect_udp():
 while not stop.is_set():
  try:b,_=udp.recvfrom(65535);received_udp.append(hashlib.sha256(b).hexdigest())
  except socket.timeout:pass
def collect_tcp():
 while not stop.is_set():
  try:c,_=tcp.accept()
  except socket.timeout:continue
  with c:
   c.settimeout(2);w=b''
   while True:
    b=c.recv(65535)
    if not b:break
    w+=b
   records=[];at=0
   while at<len(w):
    assert len(w)-at>=16;n=int.from_bytes(w[at+2:at+4],'big');assert n>=16 and at+n<=len(w);records.append(hashlib.sha256(w[at:at+n]).hexdigest());at+=n
   received_tcp.append(records)
def relay():
 held=None
 while not stop.is_set():
  try:b,_=proxy.recvfrom(65535)
  except socket.timeout:continue
  seq=int.from_bytes(b[8:12],'big');is_data=b[16:18]==b'\x01\x00'
  if is_data and seq==1:stats['dropped']+=1;continue
  if is_data and seq==3:held=b;stats['delayed']+=1;continue
  forward.sendto(b,('127.0.0.1',4739));stats['forwarded']+=1
  if is_data and seq==4 and held:forward.sendto(held,('127.0.0.1',4739));stats['forwarded']+=1;held=None
for fn in [collect_udp,collect_tcp,relay]:guarded(fn)
log=(a.out/'dumpcap.log').open('wb')
p=subprocess.Popen([str(a.dumpcap),'-i',r'\Device\NPF_Loopback','-f','host 127.0.0.1 and port 4739','-a','duration:8','-w',str(a.out/'capture.pcapng')],stdout=log,stderr=log,creationflags=subprocess.CREATE_NO_WINDOW)
try:
 time.sleep(1);assert p.poll() is None,'capture failed to start'
 sender=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
 for w in [msg(0,template),msg(0,data(0)),msg(1,data(1)),msg(2,data(2)),msg(2,template),msg(2,data(2)),msg(3,data(3)),msg(4,data(4)),msg(5,template),msg(5,data(5))]:sender.sendto(w,proxy.getsockname());time.sleep(.025)
 sender.close()
 for generation in range(3):
  messages=[msg(10,template),msg(10,data(6)),msg(11,data(7))] if generation!=1 else [msg(10,data(8))]
  sent_tcp.append([hashlib.sha256(w).hexdigest() for w in messages]);wire=b''.join(messages)
  with socket.create_connection(('127.0.0.1',4739),timeout=2) as c:
   for b in [wire[:1],wire[1:3],wire[3:37],wire[37:]]:c.sendall(b);time.sleep(.025)
   c.shutdown(socket.SHUT_WR)
  time.sleep(.1)
 assert p.wait(timeout=12)==0,'dumpcap failed';time.sleep(.2)
finally:
 stop.set()
 for t in threads:t.join(timeout=3)
 for s in [udp,tcp,proxy,forward]:s.close()
 if p.poll() is None:p.kill();p.wait()
 log.close()
assert not errors,errors
assert sent_tcp==received_tcp,('TCP collector bytes differ',sent_tcp,received_tcp)
assert len(received_udp)==stats['forwarded']==9 and stats['dropped']==1 and stats['delayed']==1,stats
result={'scope':__doc__,'proxy':stats,'UDP_received_message_hashes':received_udp,'TCP_generations':len(sent_tcp),'TCP_received_message_hashes':received_tcp,'capture_sha256':hashlib.sha256((a.out/'capture.pcapng').read_bytes()).hexdigest(),'capture_bytes':(a.out/'capture.pcapng').stat().st_size,'dumpcap_statistics_log':'dumpcap.log','errors':errors}
(a.out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8');print(json.dumps(result))
