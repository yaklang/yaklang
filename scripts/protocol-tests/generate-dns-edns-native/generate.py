"""Capture real CoreDNS DNS UDP/TCP EDNS and Additional variants.
Requires pinned dnspython 2.7.0; no Yaklang parser is used for production/oracles.
"""
import argparse, hashlib, json, os, pathlib, signal, subprocess, time
import dns.message,dns.query,dns.rcode,dns.flags,dns.rdatatype,dns.edns
IMAGE='coredns/coredns@sha256:40384aa1f5ea6bfdc77997d243aec73da05f27aed0c5e9d65bfa98933c519d97'
def run(cmd,**kw):return subprocess.run(cmd,check=True,**kw)
def canonical(m):
 return {'id':m.id,'flags':m.flags,'rcode':m.rcode(),'edns':m.edns,'edns_flags':m.ednsflags,
         'question':[{'name':q.name.to_text(),'type':q.rdtype,'class':q.rdclass} for q in m.question],
         **{key:[{'name':r.name.to_text(),'type':r.rdtype,'class':r.rdclass,'ttl':r.ttl,'data':sorted(x.to_text() for x in r)} for r in getattr(m,key)] for key in ['answer','authority','additional']}}
def main():
 ap=argparse.ArgumentParser();ap.add_argument('output',type=pathlib.Path);out=ap.parse_args().output.resolve();out.mkdir(parents=True,exist_ok=True)
 if any(out.iterdir()):ap.error('output must be empty')
 if dns.__version__!='2.7.0':ap.error('dnspython must be 2.7.0')
 (out/'Corefile').write_text('native.test:19555 {\n file /fixtures/db.native.test native.test\n log\n errors\n}\n')
 (out/'db.native.test').write_text('$ORIGIN native.test.\n$TTL 300\n@ IN SOA ns.native.test. hostmaster.native.test. (2026093002 3600 600 86400 300)\n@ IN NS ns.native.test.\n@ IN MX 10 mail.native.test.\nns IN A 192.0.2.53\nmail IN A 192.0.2.25\n')
 name=f'pr5013-edns-{os.getpid()}';container=None;capture=None
 try:
  container=run(['docker','run','-d','--name',name,'-p','127.0.0.1:19555:19555/udp','-p','127.0.0.1:19555:19555/tcp','-v',f'{out}:/fixtures:ro',IMAGE,'-conf','/fixtures/Corefile'],capture_output=True,text=True).stdout.strip();time.sleep(1)
  with (out/'capture.log').open('wb') as log:
   capture=subprocess.Popen(['/Applications/Wireshark.app/Contents/MacOS/dumpcap','-i','lo0','-f','port 19555','-w',str(out/'native.pcapng')],stdout=log,stderr=log);time.sleep(.6)
   oracles=[]
   for transport in ['udp','tcp']:
    for typ,version in [('NS',0),('MX',0),('A',1)]:
     q=dns.message.make_query('native.test',typ,use_edns=version,want_dnssec=True,payload=1232)
     q.id=100+len(oracles)
     reply=getattr(dns.query,transport)(q,'127.0.0.1',port=19555,timeout=5)
     oracles.append({'transport':transport,'query':canonical(q),'response':canonical(reply)})
   time.sleep(.3);capture.send_signal(signal.SIGINT);capture.wait(timeout=5);capture=None
  (out/'client-oracle.json').write_text(json.dumps(oracles,indent=2)+'\n')
  (out/'server.log').write_text(run(['docker','logs',name],capture_output=True,text=True).stdout)
  cmd=['/Applications/Wireshark.app/Contents/MacOS/tshark','-n','-r',str(out/'native.pcapng'),'-d','udp.port==19555,dns','-d','tcp.port==19555,dns','-Y','dns','-T','fields']
  for f in ['frame.number','dns.id','dns.flags.response','dns.flags.rcode','dns.resp.ext_rcode','dns.resp.z.do','dns.qry.name','dns.qry.type','dns.count.add_rr','dns.a','dns.mx.mail_exchange']:cmd+=['-e',f]
  (out/'tshark.tsv').write_text(run(cmd,capture_output=True,text=True).stdout)
  (out/'versions.json').write_text(json.dumps({'image':IMAGE,'client':'dnspython '+dns.__version__,'sources':['https://github.com/coredns/coredns/tree/v1.12.0','https://github.com/rthalley/dnspython/tree/v2.7.0'],'representation':'native_capture','wire_mutation':'none'},indent=2)+'\n')
 finally:
  if capture:capture.send_signal(signal.SIGINT);capture.wait(timeout=5)
  if container:run(['docker','rm','-f',name],capture_output=True)
 files=[{'file':p.name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted(out.iterdir()) if p.is_file()]
 (out/'manifest.json').write_text(json.dumps({'evidence_kind':'independent-native-capture','fixture_license':'CC0-1.0','generator_sha256':hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),'files':files},indent=2)+'\n')
if __name__=='__main__':main()
