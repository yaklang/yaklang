#!/usr/bin/env python3
"""Capture untouched native packets. Requires Docker, tcpdump and tshark.
Run with a venv containing exactly stomp.py==8.2.0. No Yaklang decoder is
used to generate expected results. Existing output files are never replaced.
"""
import argparse,datetime,hashlib,json,os,pathlib,signal,subprocess,sys,time,uuid
IMAGE='rabbitmq@sha256:d5172fdc63a9d0e8b31c57b4b473f73c19f47b75ef0cf2a49af06a86f8c0541d'
HERE=pathlib.Path(__file__).resolve().parent

def run(*args,**kw):return subprocess.run(args,check=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,**kw).stdout

def main():
 p=argparse.ArgumentParser();p.add_argument('--out',required=True);p.add_argument('--port',type=int,default=46164);p.add_argument('--interface',default='lo0');a=p.parse_args()
 out=pathlib.Path(a.out);out.mkdir(parents=True,exist_ok=True)
 if list(out.iterdir()):raise SystemExit('Refusing to overwrite existing capture/evidence files')
 import stomp
 assert stomp.__version__=='8.2.0'
 name='pr5013-stomp-'+uuid.uuid4().hex[:8]
 started=datetime.datetime.now(datetime.timezone.utc).isoformat()
 run('docker','pull',IMAGE)
 run('docker','run','-d','--name',name,'--hostname','stomp-native','-p',f'127.0.0.1:{a.port}:61613','-e','RABBITMQ_DEFAULT_USER=fixture','-e','RABBITMQ_DEFAULT_PASS=fixture',IMAGE)
 try:
  for attempt in range(30):
   check=subprocess.run(['docker','exec','--user','rabbitmq',name,'rabbitmq-diagnostics','-q','ping'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
   if check.returncode==0:break
   time.sleep(.5)
  else:raise RuntimeError('broker did not start')
  run('docker','exec','--user','rabbitmq',name,'rabbitmq-plugins','enable','rabbitmq_stomp')
  (out/'broker-version.txt').write_bytes(run('docker','exec','--user','rabbitmq',name,'rabbitmq-diagnostics','-q','server_version'))
  (out/'broker-listeners.txt').write_bytes(run('docker','exec','--user','rabbitmq',name,'rabbitmq-diagnostics','-q','listeners'))
  (out/'broker-image.json').write_bytes(run('docker','image','inspect',IMAGE,'--format','{{json .}}'))
  (out/'tshark-version.txt').write_bytes(run('tshark','--version'))
  (out/'tcpdump-version.txt').write_bytes(run('tcpdump','--version'))
  for version in ['1.0','1.1','1.2']:
   stem='rabbitmq-4.1.0-stomp-'+version
   with (out/(stem+'-capture.log')).open('wb') as log:
    cap=subprocess.Popen(['tcpdump','-i',a.interface,'-s','0','-U','-w',str(out/(stem+'.pcap')),'tcp port '+str(a.port)],stdout=log,stderr=log)
    try:
     time.sleep(.5)
     if cap.poll() is not None:raise RuntimeError('tcpdump could not start')
     run(sys.executable,str(HERE/'client.py'),'--port',str(a.port),'--version',version,'--out',str(out/(stem+'-client-oracle.json')))
     time.sleep(.3)
    finally:cap.send_signal(signal.SIGINT);cap.wait(timeout=10)
   fields=['frame.number','frame.time_epoch','frame.len','ip.src','tcp.srcport','ip.dst','tcp.dstport','tcp.stream','tcp.seq_raw','tcp.ack_raw','tcp.flags','tcp.len','tcp.payload']
   command=['tshark','-n','-r',str(out/(stem+'.pcap')),'-T','fields','-E','header=y','-E','separator=/t']
   for field in fields:command+=['-e',field]
   (out/(stem+'-tcp-oracle.tsv')).write_bytes(run(*command))
   (out/(stem+'-broker-queues.txt')).write_bytes(run('docker','exec','--user','rabbitmq',name,'rabbitmqctl','-q','list_queues','name','messages_ready','messages_unacknowledged'))
  (out/'broker.log').write_bytes(run('docker','logs',name))
  files=[]
  for path in sorted(out.iterdir()):
   raw=path.read_bytes();files.append(dict(file=path.name,sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw)))
  manifest=dict(kind='native',started_utc=started,broker={'name':'RabbitMQ','version':'4.1.0','image':IMAGE,'license':'MPL-2.0','source':'https://github.com/rabbitmq/rabbitmq-server/tree/v4.1.0'},client={'name':'stomp.py','version':'8.2.0','license':'Apache-2.0','source':'https://github.com/jasonrbriggs/stomp.py/tree/v8.2.0'},capture={'interface':a.interface,'port':a.port,'original_packets_unmodified':True,'credentials':'fixture/fixture are disposable local lab credentials','license':'CC0-1.0'},oracle={'application':'stomp.py callbacks and asserted body/header round trips, no aborted delivery, receipt/ACK/NACK outcomes','server':'broker version, listeners, post-session queue state, unmodified broker log','tshark':'4.4.8 has no STOMP dissector; only listed TCP/frame fields are applicable'},variants=['1.0 client acknowledgements','1.1 client-individual acknowledgements and NACK','1.2 embedded NUL body, client-individual ACK and NACK'],files=files)
  manifest['client']['source_commit']='abb4d8244d6640f6a7cf260dc75c106aaa078841'
  manifest['client']['dependencies']={'docopt':'0.6.2','websocket-client':'1.9.2'}
  manifest['references']=['https://stomp.github.io/stomp-specification-1.0.html','https://stomp.github.io/stomp-specification-1.1.html','https://stomp.github.io/stomp-specification-1.2.html','https://www.rabbitmq.com/docs/stomp']
  (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
 finally:run('docker','rm','-f','-v',name)

if __name__=='__main__':main()
