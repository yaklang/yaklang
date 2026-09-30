"""CC0-1.0. Native open62541 renewal/chunk capture; no packet construction.
Pass a pristine open62541 v1.5.5 source with its built static library.
"""
import argparse,hashlib,json,pathlib,signal,subprocess,tempfile,time
COMMIT='3bdeed5dfe8309cecabf36e04afe956b7f72ebd8'
def run(cmd,**kw):return subprocess.run(cmd,check=True,**kw)
def main():
 p=argparse.ArgumentParser();p.add_argument('source',type=pathlib.Path);p.add_argument('output',type=pathlib.Path);a=p.parse_args();src=a.source.resolve();out=a.output.resolve();out.mkdir(parents=True,exist_ok=True)
 if any(out.iterdir()):p.error('output must be empty')
 assert run(['git','-C',str(src),'rev-parse','HEAD'],capture_output=True,text=True).stdout.strip()==COMMIT
 assert not run(['git','-C',str(src),'status','--porcelain'],capture_output=True,text=True).stdout.strip()
 script=pathlib.Path(__file__).resolve().parent;cap=server=client=None
 with tempfile.TemporaryDirectory(prefix='opcua-lifecycle-') as work:
  binary=str(pathlib.Path(work)/'endpoint')
  flags=run(['pkg-config','--libs','openssl'],capture_output=True,text=True).stdout.split()
  run(['cc',str(script/'endpoint.c'),*['-I'+str(src/d) for d in ['include','plugins/include','build/src_generated']],str(src/'build/bin/libopen62541.a'),*flags,'-lpthread','-lm','-o',binary])
  handles=[(out/(role+'.log')).open('wb') for role in ['capture','server','client']]
  try:
   cap=subprocess.Popen(['/usr/sbin/tcpdump','-i','lo0','-U','-s','0','-w',str(out/'native.pcap'),'tcp port 14846'],stdout=handles[0],stderr=handles[0]);time.sleep(.6)
   server=subprocess.Popen([binary,'server'],stdout=handles[1],stderr=handles[1])
   deadline=time.monotonic()+10
   while 'Creating listen socket' not in (out/'server.log').read_text():
    if server.poll() is not None or time.monotonic()>deadline:raise RuntimeError('native server did not start')
    time.sleep(.1)
   time.sleep(.2)
   client=subprocess.Popen([binary,'client'],stdout=handles[2],stderr=handles[2]);assert client.wait(timeout=30)==0;time.sleep(.5)
  finally:
   for proc in [client,server,cap]:
    if proc and proc.poll() is None:proc.send_signal(signal.SIGINT);proc.wait(timeout=5)
   for h in handles:h.close()
 assert '0 packets dropped by kernel' in (out/'capture.log').read_text()
 oracles=[x for x in (out/'client.log').read_text().splitlines() if x.startswith('ORACLE ')]
 assert oracles==['ORACLE round=1 results=2048 all=Running','ORACLE round=2 results=2048 all=Running']
 tshark='/Applications/Wireshark.app/Contents/MacOS/tshark'
 fields=['frame.number','opcua.transport.type','opcua.transport.chunk','opcua.ChannelId','opcua.security.tokenid','opcua.security.seq','opcua.security.rqid','opcua.servicenodeid.numeric']
 cmd=[tshark,'-n','-r',str(out/'native.pcap'),'-d','tcp.port==14846,opcua','-Y','opcua','-T','fields']
 for field in fields:cmd+=['-e',field]
 (out/'tshark.tsv').write_text(run(cmd,capture_output=True,text=True).stdout)
 (out/'endpoint-oracle.json').write_text(json.dumps({'producer':'open62541 v1.5.5','commit':COMMIT,'source':'https://github.com/open62541/open62541/tree/'+COMMIT,'license':'MPL-2.0; no library code or binary redistributed','security_policy':'None','negotiated_buffer':8192,'requested_channel_lifetime_ms':2000,'oracles':oracles,'wire_mutation':'none'},indent=2)+'\n')
 files=[{'file':f.name,'sha256':hashlib.sha256(f.read_bytes()).hexdigest()} for f in sorted(out.iterdir()) if f.is_file()]
 generators={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in script.iterdir() if f.is_file()}
 (out/'manifest.json').write_text(json.dumps({'evidence_kind':'independent-native-capture','fixture_license':'CC0-1.0','files':files,'generators':generators},indent=2)+'\n')
if __name__=='__main__':main()
