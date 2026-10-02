#!/usr/bin/env python3
"""CC0-1.0: capture real open62541 socket sessions; never construct packets."""
import argparse, json, pathlib, signal, subprocess, time
p=argparse.ArgumentParser(); p.add_argument('--only',default=''); p.add_argument('driver'); p.add_argument('output'); p.add_argument('--interface',default='lo0'); p.add_argument('--tcpdump',default='/usr/sbin/tcpdump'); p.add_argument('--openssl',default='openssl'); a=p.parse_args()
out=pathlib.Path(a.output).resolve(); out.mkdir(parents=True,exist_ok=True)
keys=out/'keys'; keys.mkdir(exist_ok=True)
for kind in ('rsa','ecc'):
 for role in ('server','client'):
  key=keys/f'{kind}-{role}-key.pem'; cert=keys/f'{kind}-{role}.pem'
  gen=['-newkey','rsa:2048'] if kind=='rsa' else ['-newkey','ec','-pkeyopt','ec_paramgen_curve:prime256v1']
  subprocess.run([a.openssl,'req','-x509','-new','-nodes',*gen,'-keyout',str(key),'-out',str(cert),'-days','30','-subj',f'/CN=PR5013-local-{kind}-{role}','-addext','subjectAltName=URI:urn:open62541.unconfigured.application,DNS:localhost,IP:127.0.0.1','-addext','keyUsage=digitalSignature,keyEncipherment,dataEncipherment,keyCertSign','-addext','extendedKeyUsage=serverAuth,clientAuth'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  subprocess.run([a.openssl,'x509','-in',str(cert),'-outform','DER','-out',str(keys/f'{kind}-{role}.der')],check=True)
  subprocess.run([a.openssl,'pkey','-in',str(key),'-outform','DER','-out',str(keys/f'{kind}-{role}-key.der')],check=True)
results=[]
for name,policy,mode,reverse in [('none','None',1,0),('reverse-none','None',1,1),('rsa-sign','Basic256Sha256',2,0),('rsa-encrypt','Basic256Sha256',3,0),('ecc-sign','ECC_nistP256',2,0),('ecc-encrypt','ECC_nistP256',3,0),('reverse-ecc-encrypt','ECC_nistP256',3,1)]:
 if a.only and name != a.only: continue
 logs={role:open(out/f'{name}-{role}.log','w') for role in ('server','client','capture')}
 bpf = f'tcp port {14843 if policy.startswith("ECC") else 14841}' if reverse else 'tcp port 14840'
 cap=subprocess.Popen([a.tcpdump,'-i',a.interface,'-U','-s','0','-w',str(out/f'open62541-{name}.pcap'),bpf],stdout=logs['capture'],stderr=subprocess.STDOUT)
 server=client=None
 try:
  time.sleep(.4)
  def start(role): return subprocess.Popen([a.driver,role,policy,str(mode),str(reverse),str(keys),str(14843 if policy.startswith("ECC") else 14841)],stdout=logs[role],stderr=subprocess.STDOUT)
  if reverse:
   client=start('client'); time.sleep(1); server=start('server')
  else:
   server=start('server'); time.sleep(1); client=start('client')
  code=client.wait(timeout=30)
  time.sleep(.25)
 finally:
  for proc in (client,server):
   if proc and proc.poll() is None: proc.send_signal(signal.SIGTERM); proc.wait(timeout=5)
  time.sleep(.25); cap.send_signal(signal.SIGINT); cap.wait(timeout=5)
  for log in logs.values(): log.close()
 result=dict(name=name,policy=policy,mode=mode,reverse=bool(reverse),exit_code=code)
 results.append(result); print(json.dumps(result),flush=True)
(out/'endpoint-results.json').write_text(json.dumps(results,indent=2)+'\n')
if any(r['exit_code'] for r in results): raise SystemExit(1)
