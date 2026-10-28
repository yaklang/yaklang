"""Reject source/projection/refusal forgeries using four original Sharon witnesses.

No capture bytes are bundled. Input is the full original aa4fcc81... PCAPNG,
its current SDK/CLI exports, the 31ba baseline SDK and raw reference export.
Frame identities are literal original source coordinates, never rewritten labels.
"""
import argparse,json,sys,pathlib,copy,base64
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]))
import gre_parent_source_audit as a


def audit(capture,events,cli,baseline,oracle,legacy_marker=False):
 assert a.sha(capture)=='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'
 selected={315252,289804,39621,23084};wire={n:(s,i,l,w)for n,s,i,l,w in a.records(capture)if n in selected};native={};projections={};old={}
 for e in a.rows(events):
  rs=e['source_bytes'].get('PacketRefs')or[]
  if len(rs)==1 and rs[0]['Number']in selected:native[rs[0]['Number']]=e
 for row in a.rows(cli):
  if row.get('record_type')!='protocol_publication'and row['number']in selected:projections[row['number']]=row
 for e in a.rows(baseline):
  rs=e['source_bytes'].get('PacketRefs')or[]
  if e['protocol']!='ip'and len(rs)==1 and rs[0]['Number']==39621:old[39621]=e
 claim={n:a.classify(w,l,n==23084)for n,(s,i,l,w)in wire.items()}
 checks=[]
 def reject(name,fn):
  try:fn()
  except AssertionError:checks.append(dict(name=name,rejected=True));return
  raise AssertionError(('negative control accepted',name))
 def event_negative(name,n,mutate):
  e=copy.deepcopy(native[n]);mutate(e);s,i,l,w=wire[n];reject(name,lambda:a.refusal(e,n,s,i,claim[n]))
 def projection_negative(name,n,mutate):
  row=copy.deepcopy(projections[n]);mutate(row);reject(name,lambda:a.projection(row,native[n],legacy_marker))
 def removed_negative(name,mutate):
  e=copy.deepcopy(old[39621]);mutate(e);reject(name,lambda:a.removed_udp(e,39621,claim[39621],native[39621]['domain']))
 for n,e in native.items():
  s,i,l,w=wire[n];a.refusal(e,n,s,i,claim[n]);a.projection(projections[n],e,legacy_marker)
 a.removed_udp(old[39621],39621,claim[39621],native[39621]['domain'])
 event_negative('wrong literal GRE version',315252,lambda e:e['session'].__setitem__('Observed GRE Version',6))
 event_negative('numerically equal noninteger GRE version',315252,lambda e:e['session'].__setitem__('Observed GRE Version',7.0))
 event_negative('lost observed nested source domain',289804,lambda e:(e['domain'].__setitem__('Encapsulation',e['domain']['Encapsulation'].split('/gre:')[0]),e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Encapsulation','')))
 event_negative('wrong physical frame reference',315252,lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',315253))
 event_negative('boolean physical section',315252,lambda e:e['domain'].__setitem__('Section',False))
 event_negative('claimed child IP behind refused parent',315252,lambda e:e['session'].__setitem__('Expected IP Version',4))
 event_negative('claimed GRE identity behind unselected Geneve',39621,lambda e:e['session'].update({'Unselected Carrier':'GRE','Observed GRE Version':7}))
 event_negative('child transport connection ownership',315252,lambda e:e.__setitem__('flow',1))
 event_negative('exact native fields for limited parent',315252,lambda e:e.__setitem__('fields',[{'name':'IP','value':4}]))
 event_negative('boolean source parent PDU',315252,lambda e:e['source_bytes'].__setitem__('ParentPDU',False))
 event_negative('floating source interface',315252,lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',226.0))
 event_negative('floating source frame number',315252,lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',315252.0))
 event_negative('noninteger SDK projection PDU',315252,lambda e:e['field_values'].__setitem__('pdu',float(e['id'])))
 event_negative('wrong physical source kind',315252,lambda e:e['source_bytes'].__setitem__('Kind','reassembled'))
 s,i,l,w=wire[315252];mutated=bytearray(w);mutated[35]=(mutated[35]&248)|6;changed=a.classify(bytes(mutated),l);reject('actual wire GRE version changed independently',lambda:a.refusal(native[315252],315252,s,i,changed))
 extra=copy.deepcopy(native[315252]);extra['id']+=1000000;extra['protocol']='tls';reject('second application contribution from refused source',lambda:a.exclusive([native[315252],extra],{315252:native[315252]}))
 projection_negative('UI claims child TLS',315252,lambda row:row.__setitem__('protocol','TLSv1.3'))
 projection_negative('UI speculative inner stack',315252,lambda row:row['protocol_stack'].append('TCP'))
 projection_negative('UI foreign PDU source identity',315252,lambda row:row['pdus'][0].__setitem__('id',native[315252]['id']+1))
 projection_negative('UI drops exact refusal reason',315252,lambda row:row.__setitem__('info','[incomplete/undecoded]'))
 removed_negative('removed UDP replaced with a recognized protocol',lambda e:e.__setitem__('protocol','tls'))
 removed_negative('removed UDP preview byte changed',lambda e:e.__setitem__('raw',base64.b64encode(b'X'+base64.b64decode(e['raw'])[1:]).decode()))
 removed_negative('removed UDP preview digest falsely covers full body',lambda e:e['source_bytes'].__setitem__('SHA256',claim[39621]['UDP']['payload_SHA256']))
 removed_negative('removed UDP socket endpoint changed',lambda e:e.__setitem__('destination','4.3.2.1:443'))
 removed_negative('removed UDP contributes through wrong capture interface',lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',226))
 # Independent raw descriptor controls use an actual original reference field.
 reference=json.loads(oracle.read_text('utf8'));layers=next(v['_source']['layers']for v in reference if a.number(v['_source']['layers']['frame']['frame.number'])==315252);offset=claim[315252]['trace'][0]['offset'];flags=claim[315252]['trace'][0]['flags'];a.raw_scalar(layers,'gre.flags_and_version',offset,2,w,flags)
 def reference_negative(name,mutate):
  tree=copy.deepcopy(layers);mutate(tree['gre']);reject(name,lambda:a.raw_scalar(tree,'gre.flags_and_version',offset,2,w,flags))
 reference_negative('raw descriptor rebound to another byte offset',lambda g:g['gre.flags_and_version_raw'].__setitem__(1,offset+1))
 reference_negative('raw descriptor uses transformed source bytes',lambda g:g['gre.flags_and_version_raw'].__setitem__(5,1))
 reference_negative('raw descriptor forged byte mask',lambda g:g['gre.flags_and_version_raw'].__setitem__(3,7))
 reference_negative('raw field scalar differs from exact captured bytes',lambda g:g.__setitem__('gre.flags_and_version',hex(flags+1)))
 reference_negative('boolean raw descriptor source index',lambda g:g['gre.flags_and_version_raw'].__setitem__(5,False))
 return dict(scope=__doc__,auditor_SHA256=a.sha(pathlib.Path(a.__file__)),driver_SHA256=a.sha(pathlib.Path(__file__)),capture_SHA256=a.sha(capture),SDK_SHA256=a.sha(events),CLI_SHA256=a.sha(cli),baseline_SDK_SHA256=a.sha(baseline),reference_SHA256=a.sha(oracle),legacy_generic_markers_allowed=legacy_marker,source_positive_frames=len(selected),negative_controls=len(checks),all_rejected=all(x['rejected']for x in checks),checks=checks)


if __name__=='__main__':
 ap=argparse.ArgumentParser(description=__doc__)
 for n in['capture','events','cli','baseline','oracle','out']:ap.add_argument('--'+n,type=pathlib.Path,required=True)
 ap.add_argument('--allow-legacy-generic-marker',action='store_true');args=ap.parse_args()
 for p in[args.capture,args.events,args.cli,args.baseline,args.oracle,args.out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
 proof=audit(args.capture,args.events,args.cli,args.baseline,args.oracle,args.allow_legacy_generic_marker)
 with args.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
 print(json.dumps({k:v for k,v in proof.items()if k not in['scope','checks']}),flush=True)
