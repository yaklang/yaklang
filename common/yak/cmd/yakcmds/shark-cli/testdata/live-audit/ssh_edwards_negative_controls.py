"""Challenge Edwards-only whole-export invariants using real certified rows.

Use a declared subset, preserving all independent wire/tree witnesses. The
outer export SHA is deliberately rebound for mutations to reach deeper checks;
the independent wire/tree hashes are not recomputed to bless altered content.
No immutable capture, answer or full reference export is rewritten.
"""
import argparse,base64,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','field-certificate','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args();root=a.out
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
root.mkdir(exist_ok=False,parents=True)
def load(path):return[json.loads(line)for line in path.open(encoding='utf-8-sig')]
old,new=load(a.before),load(a.after);assert len(old)==len(new)
proof=json.loads(a.field_certificate.read_text());assert proof['events_sha256']==hashlib.sha256(a.after.read_bytes()).hexdigest()
selected=[i for i,(x,y)in enumerate(zip(old,new))if x!=y];assert selected,'real Edwards field expansions required'
indices=selected[:2];indices+=next(([i]for i,e in enumerate(new)if e['protocol']!='ssh'),[])
assert len(indices)>=2,'multiple real rows for order/duplicate controls'
before=[old[i]for i in indices];after=[new[i]for i in indices]
base_proof=copy.deepcopy(proof);base_proof['compared_PDUs']=[v for v in proof['compared_PDUs']if v['PDU']in{e['id']for e in after}]
def dump(path,rows):path.write_text(''.join(json.dumps(e,separators=(',',':'))+'\n'for e in rows))
def run(name,mutate,expected):
 folder=root/name;folder.mkdir();b,n,cert=copy.deepcopy(before),copy.deepcopy(after),copy.deepcopy(base_proof);mutate(b,n,cert)
 dump(folder/'before.jsonl',b);dump(folder/'after.jsonl',n);cert['events_sha256']=hashlib.sha256((folder/'after.jsonl').read_bytes()).hexdigest();(folder/'fields.json').write_text(json.dumps(cert))
 r=subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('ssh_edwards_export_invariants.py')),'--before',str(folder/'before.jsonl'),'--after',str(folder/'after.jsonl'),'--field-certificate',str(folder/'fields.json'),'--out',str(folder/'invariants.json')],capture_output=True,timeout=30);(folder/'audit.log').write_bytes(r.stdout+r.stderr)
 assert (r.returncode==0)==expected,(name,r.returncode,(r.stdout+r.stderr).decode('utf8','replace')[-800:])
def tree(e,container):return e['fields']['Payload'][container]
def both(e,container,key,value):
 for target in [e['fields'],e['session']['Initial Packet Fields']]:target['Payload'][container][key]=value
key_name=next(k for k in tree(after[0],'Host Key')if k.endswith(' Public Key'))
sig_name=next(k for k in tree(after[0],'Signature')if k.endswith(' Signature Bytes'))
controls={
 'typed-key-octets':lambda b,n,c:both(n[0],'Host Key',key_name,base64.b64encode(bytes([1])*32).decode()),
 'typed-key-length':lambda b,n,c:both(n[0],'Host Key',key_name+' Length',31),
 'typed-signature-octets':lambda b,n,c:both(n[0],'Signature',sig_name,base64.b64encode(bytes([2])*64).decode()),
 'typed-signature-length':lambda b,n,c:both(n[0],'Signature',sig_name+' Length',63),
 'format-is-not-certificate':lambda b,n,c:both(n[0],'Host Key','Host Key Format','ssh-ed25519-cert-v01@openssh.com'),
 'signature-verification-is-not-parsing':lambda b,n,c:n[0]['session'].__setitem__('Signature Verified',True),
 'peer-identity-is-not-parsing':lambda b,n,c:n[0]['session'].__setitem__('Peer Identity Verified',True),
 'raw-message':lambda b,n,c:n[0].__setitem__('raw',base64.b64encode(b'changed original message').decode()),
 'connection-id':lambda b,n,c:n[0].__setitem__('flow',n[0]['flow']+1),
 'direction':lambda b,n,c:n[0].__setitem__('direction',1-n[0]['direction']),
 'source-domain':lambda b,n,c:n[0]['domain'].__setitem__('Section',n[0]['domain']['Section']+1),
 'physical-contributors':lambda b,n,c:n[0]['source_bytes']['PacketRefs'][0].__setitem__('Number',999999),
 'missing-independent-witness':lambda b,n,c:c.__setitem__('compared_PDUs',[]),
 'wire-witness-hash':lambda b,n,c:c['compared_PDUs'][0].__setitem__('raw_sha256','0'*64),
 'field-witness-hash':lambda b,n,c:c['compared_PDUs'][0].__setitem__('field_tree_sha256','0'*64),
 'drop-row':lambda b,n,c:n.pop(),
 'duplicate-row':lambda b,n,c:n.append(copy.deepcopy(n[-1])),
 'row-order':lambda b,n,c:n.reverse(),
}
run('positive',lambda b,n,c:None,True)
for name,mutate in controls.items():run(name,mutate,False)
result={'scope':__doc__,'positive_subsets':1,'subset_records':len(indices),'negative_controls_rejected':list(controls),'before_sha256':hashlib.sha256(a.before.read_bytes()).hexdigest(),'after_sha256':hashlib.sha256(a.after.read_bytes()).hexdigest(),'independent_field_certificate_sha256':hashlib.sha256(a.field_certificate.read_bytes()).hexdigest(),'independent_witnesses_not_recomputed_to_accept_mutations':True};(root/'summary.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
