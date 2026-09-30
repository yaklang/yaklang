#!/usr/bin/env python3
"""CC0-1.0. Archive original native captures plus independent endpoint/TShark oracles."""
import collections, hashlib, json, pathlib, platform, re, shutil, subprocess, sys
work=pathlib.Path(sys.argv[1]); out=pathlib.Path(sys.argv[2]); out.mkdir(parents=True,exist_ok=True)
tshark=shutil.which('tshark') or '/Applications/Wireshark.app/Contents/MacOS/tshark'
version=subprocess.check_output([tshark,'--version'],text=True).splitlines()[0]
manifest={'evidence_kind':'independent-native-capture','fixture_license':'CC0-1.0','packet_mutation':'none; original tcpdump files copied byte-for-byte','implementations':[
 {'name':'open62541','version':'v1.5.5','commit':'3bdeed5dfe8309cecabf36e04afe956b7f72ebd8','source':'https://github.com/open62541/open62541/tree/3bdeed5dfe8309cecabf36e04afe956b7f72ebd8','license':'MPL-2.0 (library), CC0-1.0 (examples); no library binary redistributed','crypto_backend':subprocess.check_output(['openssl','version'],text=True).strip(), 'host':platform.platform()},
 {'name':'libiec61850','version':'v1.6.0','commit':'519b0208cc79d1af09d5ca40fb9ad1fd93822e93','source':'https://github.com/mz-automation/libiec61850/tree/519b0208cc79d1af09d5ca40fb9ad1fd93822e93','license':'GPL-3.0; no library code or binary redistributed','isolation':'Docker --network none; two veth interfaces in the same private namespace'}],
 'key_provenance':'Fresh local OpenSSL self-signed RSA-2048 and P-256 application certificates. Public DER certificates are included; generated private keys remain outside the repository. No imported credentials, key logs or decryption claims.',
 'reverse_ecc_configuration':'Unmodified v1.5.5 reverse-listener API omits copying config.endpoint. The driver initializes its configured endpoint through a refused localhost connection before the server starts, then uses the public reverse listener. The reverse capture filter only records the reverse listening port, preserving the complete successful TCP connection.',
 'boundaries':['Protected OPC UA OPN/MSG/CLO are envelope-only without signature verification or decryption. Endpoint success proves real native negotiation, not plaintext recovery by Yaklang.','This open62541 revision requires >=8192 negotiated buffers in src/ua_securechannel.c:280; 1024 ECC remains covered by separate synthetic parser tests, not these native recordings.','GOOSE Reserved1 includes an S-bit in Wireshark; no blanket zero-field restriction is inferred. The pinned publisher emits zero Reserved1/2 even for PDU simulation=true.','Regeneration uses fresh keys/nonces/timestamps; hashes identify these originals rather than promising byte-identical regeneration.'],
 'tshark_version':version,'captures':[],'certificates':[]}
def digest(b): return hashlib.sha256(b).hexdigest()
def fields(path,proto,args,columns):
 cmd=[tshark,'-r',str(path),*args,'-Y',proto,'-T','fields','-E','separator=\t']
 for column in columns: cmd+=['-e',column]
 text=subprocess.check_output(cmd,text=True)
 return [dict(zip(columns,line.split('\t'))) for line in text.splitlines()]
configs=json.loads((work/'opcua-archive/endpoint-results.json').read_text())
for config in configs:
 assert config['exit_code']==0,config
 name=config['name']; src=work/'opcua-archive'/f'open62541-{name}.pcap'
 capture_log=(src.parent/f'{name}-capture.log').read_text(); assert '0 packets dropped by kernel' in capture_log
 client_log=(src.parent/f'{name}-client.log').read_text(); oracle=[line for line in client_log.splitlines() if line.startswith('ORACLE read ')]; assert len(oracle)==1
 params=['-d','tcp.port==14840,opcua','-d','tcp.port==14841,opcua','-d','tcp.port==14843,opcua']
 rows=fields(src,'opcua',params,['frame.number','tcp.stream','tcp.srcport','tcp.dstport','opcua.transport.type','opcua.transport.endpoint','opcua.transport.suri','opcua.security.spu','_ws.expert.message'])
 malformed=fields(src,'_ws.malformed',params,['frame.number']); assert not malformed
 types=collections.Counter(t for row in rows for t in row['opcua.transport.type'].split(',') if t)
 b=src.read_bytes();shutil.copyfile(src,out/src.name)
 manifest['captures'].append(dict(file=src.name,protocol='opcua',sha256=digest(b),size_bytes=len(b),**config,packet_count=int(re.search(r'(\d+) packets captured',capture_log).group(1)),message_count=sum(types.values()),message_types=dict(types),endpoint_oracle=oracle[0],endpoint_exit_code=0,kernel_drops=0,tshark_rows=rows))
for variant in range(3):
 src=work/'goose-captures'/f'libiec61850-{variant}.pcap'; b=src.read_bytes()
 oracle=(src.parent/f'goose-{variant}-subscriber.log').read_text().splitlines(); assert len(oracle)==3 and all('ORACLE valid=1 ' in l for l in oracle)
 rows=fields(src,'goose',[],['frame.number','vlan.id','goose.reserve1','goose.reserve2','goose.stNum','goose.sqNum','goose.simulation','goose.ndsCom','_ws.expert.message']);assert len(rows)==3
 assert not fields(src,'_ws.malformed',[],['frame.number'])
 capture_log=(src.parent/f'goose-{variant}-capture.log').read_text(); assert '0 packets dropped by kernel' in capture_log
 shutil.copyfile(src,out/src.name)
 manifest['captures'].append(dict(file=src.name,protocol='goose',sha256=digest(b),size_bytes=len(b),variant=variant,packet_count=3,message_count=3,endpoint_oracle=oracle,endpoint_exit_code=0,kernel_drops=0,tshark_rows=rows))
for src in sorted((work/'opcua-archive/keys').glob('*.der')):
 if '-key' in src.name: continue
 dest=out/'certificates'/src.name;dest.parent.mkdir(exist_ok=True);shutil.copyfile(src,dest)
 manifest['certificates'].append(dict(file=str(dest.relative_to(out)),sha256=digest(src.read_bytes())))
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
print(json.dumps([(c['file'],c['message_count']) for c in manifest['captures']]))
