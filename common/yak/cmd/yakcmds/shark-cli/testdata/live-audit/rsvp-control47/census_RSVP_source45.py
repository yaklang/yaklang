"""Independent original-wire RSVP IP46 inventory, without candidate Go output.

Only common header, exact length, checksum arithmetic and object envelopes are
observations here. Zero checksum is explicitly omitted, not verified. Bundle
bodies remain unselected; no RSVP session, reservation or authentication claim.
"""
from pathlib import Path
import argparse,collections,hashlib,importlib.util,json,shutil,sys
def sha(p):
 with Path(p).open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
 with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def inspect(w):
 out={'captured_bytes':len(w),'raw_SHA256':dg(w),'scope':'common header/object envelopes; procedure/session/authentication unavailable'}
 if len(w)<8:return dict(out,refusal='common-header-short')
 length=int.from_bytes(w[6:8],'big');checksum=int.from_bytes(w[2:4],'big')
 out.update(version=w[0]>>4,flags=w[0]&15,type=w[1],checksum=checksum,send_TTL=w[4],reserved=w[5],length=length,header_spans={'Version and Flags':[0,1],'Message Type':[1,2],'Checksum':[2,4],'Send TTL':[4,5],'Reserved':[5,6],'Message Length':[6,8]})
 if w[0]>>4!=1:return dict(out,refusal='unselected-version')
 if length<8 or length>len(w):return dict(out,refusal='declared-message-outside-captured-parent')
 out['message_span']=[0,length];out['unselected_tail_bytes']=len(w)-length
 total=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,length-1,2))
 if length&1:total+=w[length-1]<<8
 while total>>16:total=(total&65535)+(total>>16)
 out['checksum_arithmetic']='omitted'if checksum==0 else'valid'if total==65535 else'invalid'
 if w[1]==12:return dict(out,unselected_bundle_body_span=[8,length])
 objects=[];at=8
 while at<length:
  if len(objects)>=4096:return dict(out,objects=objects,refusal='source-object-count-limit')
  if at+4>length:return dict(out,objects=objects,refusal='object-header-outside-parent')
  n=int.from_bytes(w[at:at+2],'big');o={'length':n,'class':w[at+2],'C-Type':w[at+3],'header_span':[at,at+4]};objects.append(o)
  if n<4 or n%4 or at+n>length:return dict(out,objects=objects,refusal='object-extent-or-alignment')
  o.update(value_span=[at+4,at+n],value_SHA256=dg(w[at+4:at+n]));at+=n
 return dict(out,objects=objects,object_envelopes_complete=True)
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False)
 root=Path(__file__).parent;base=root/'OSPFv3-independent-source37-a82-01';inventory=base/'inputs01/inputs.json';doc=read(inventory)
 assert shutil.disk_usage(root).free>20*2**30
 sys.path.insert(0,str(base));from scan_source37 import Census
 rp=Path(doc['capture_reader']);assert sha(rp)==doc['capture_reader_SHA256'];spec=importlib.util.spec_from_file_location('independent_RSVP_capture_reader',rp);reader=importlib.util.module_from_spec(spec);spec.loader.exec_module(reader)
 inputs={str(p):sha(p)for p in[Path(__file__),inventory,rp,base/'scan_source37.py',base/'ospf_source_codec.py']}
 summaries=[];samples=set()
 class V(Census):
  def __init__(self,*args):super().__init__(*args);self.rsvp=[]
  def upper(self,proto,at,end,path,depth):
   if proto==46:self.rsvp.append((at,end,path));return
   return super().upper(proto,at,end,path,depth)
 for item in doc['inputs']:
  capture=Path(item['capture']);assert sha(capture)==item['capture_SHA256'];inputs[str(capture)]=item['capture_SHA256'];counts=collections.Counter();rows=[];frames=0;types=collections.Counter();classes=collections.Counter()
  for frame,record in enumerate(reader.capture_records(capture),1):
   frames+=1;v=V(record['wire'],record,frame,counts)
   try:v.run()
   except(AssertionError,IndexError,ValueError):counts['unselected_carrier_refusals']+=1
   for start,end,path in v.rsvp:
    w=record['wire'][start:end];parents=[x for x in path if x['kind']in('IPv4','IPv6')];complete=bool(parents)and all(x['declared_end']==x['available_end']for x in parents)
    observation=inspect(w);reason=observation.get('refusal','bounded-common-header');counts['IP46_observations']+=1;counts[reason]+=1
    if not complete:counts['incomplete_IP_parent']+=1
    if 'type'in observation:types[str(observation['type'])]+=1
    for o in observation.get('objects',[]):classes[str(o['class'])]+=1
    row=dict(input=item['input'],capture_SHA256=item['capture_SHA256'],frame=frame,domain={k:record[k]for k in('section','interface','link')},time_ns=record['time_ns'],captured_length=record['captured_length'],original_length=record['original_length'],original_frame_SHA256=dg(record['wire']),carrier=path,IP_parent_complete=complete,physical_PDU_span=[start,end],observation=observation);rows.append(row)
    feature=(observation.get('type'),reason,observation.get('checksum_arithmetic'))
    if complete and feature not in samples and len(samples)<16:
     samples.add(feature);p=a.out/f'input{item["input"]:02d}-frame{frame}.frame';p.write_bytes(record['wire']);row['representative_frame']=str(p)
  assert frames==item['physical_frames']and sha(capture)==item['capture_SHA256']
  target=a.out/f'input-{item["input"]:02d}.json';write(target,rows);s=dict(input=item['input'],physical_frames=frames,observations=len(rows),counts=dict(counts),message_types=dict(types),object_classes=dict(classes),facts_SHA256=sha(target));summaries.append(s);print(json.dumps({k:s[k]for k in('input','observations','message_types')}),flush=True)
 assert inputs=={p:sha(p)for p in inputs}
 write(a.out/'summary.json',dict(scope=__doc__,inputs=inputs,inputs_unchanged=True,input_count=37,captures=summaries,representatives=len(samples),candidate_native_CLI_or_Go_read=False,primary_sources=['https://www.rfc-editor.org/rfc/rfc2205','https://www.rfc-editor.org/rfc/rfc2961','https://www.rfc-editor.org/rfc/rfc2747']))
 print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),observations=sum(s['observations']for s in summaries))),flush=True)
if __name__=='__main__':main()
