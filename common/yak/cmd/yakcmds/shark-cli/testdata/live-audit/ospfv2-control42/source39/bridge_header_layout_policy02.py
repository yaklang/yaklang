"""Prove RFC6549 header-only source-model correction; no native values read."""
import argparse,copy,hashlib,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args();assert not a.out.exists()
 paths=[a.root/'source-grid01/summary.json',a.root/'source-grid02/summary.json',a.root/'header-layout-policy-correction02.json',Path(__file__)]
 binding={str(p):sha(p)for p in paths};checks=0;counts={};fail=[]
 for number in (35,37):
  oldp=a.root/('source-grid01/input-%02d-source-grid.json'%number);newp=a.root/('source-grid02/input-%02d-source-grid.json'%number)
  binding.update({str(p):sha(p)for p in (oldp,newp)});old=read(oldp);new=read(newp);assert len(old)==len(new)==278
  for prior,corrected in zip(old,new):
   assert prior['frame']==corrected['frame'];rawp=Path(prior['original_frame']);binding[str(rawp)]=sha(rawp);assert binding[str(rawp)]==prior['original_frame_SHA256'];wire=rawp.read_bytes()[prior['PDU_range'][0]:prior['PDU_range'][1]]
   row=copy.deepcopy(corrected);header=row['source']['fields']['Header'];assert header['Instance ID']==wire[14]==0;assert header['Authentication Type']==wire[15]
   assert header['Wire Field Ranges']['Instance ID']==[14,15];assert header['Wire Field Ranges']['Authentication Type']==[15,16]
   del header['Instance ID'];del header['Wire Field Ranges']['Instance ID'];header['Authentication Type']=int.from_bytes(wire[14:16],'big');header['Wire Field Ranges']['Authentication Type']=[14,16]
   leaves=[]
   for leaf in row['source']['source_leaves']:
    if leaf['path']==['Header','Instance ID']:continue
    if leaf['path']==['Header','Authentication Type']:
     leaf['value']=int.from_bytes(wire[14:16],'big');leaf['PDU_span']=[14,16];base=prior['PDU_range'][0];leaf['physical_span']=[base+14,base+16];leaf['wire_SHA256']=hashlib.sha256(wire[14:16]).hexdigest()
    leaves.append(leaf)
   row['source']['source_leaves']=leaves
   if row!=prior:fail.append({'input':number,'frame':prior['frame'],'reason':'whole old source row differs beyond declared header layout split'})
   checks+=1
  counts[str(number)]={'source_rows':len(old),'old_leaves':sum(len(x['source']['source_leaves'])for x in old),'new_leaves':sum(len(x['source']['source_leaves'])for x in new)}
 assert binding=={p:sha(p)for p in binding}
 report={'scope':__doc__,'source_rows':checks,'counts':counts,'header_only_layout_split':{'primary_source':'https://datatracker.ietf.org/doc/html/rfc6549#section-2','old_legacy_AuType':[14,16],'new_InstanceID':[14,15],'new_AuType':[15,16],'all_observed_InstanceID_zero':True},'all_other_full_source_row_values_ranges_checksums_authentication_LLS_and_bytes_literal_equal':not fail,'failure_count':len(fail),'failures':fail,'old_grid_retained':True,'source_model_correction_not_product_bug_or_candidate_runtime':True,'no_native_output_read':True,'inputs':binding,'inputs_unchanged':True}
 with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
 print(json.dumps({'certificate':str(a.out),'SHA256':sha(a.out),'rows':checks,'failures':len(fail),'counts':counts}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
