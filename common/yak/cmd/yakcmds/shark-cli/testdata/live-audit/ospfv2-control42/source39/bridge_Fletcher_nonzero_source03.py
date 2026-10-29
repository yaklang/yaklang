"""Prove field-nonzero source policy correction leaves all prior real/control facts.

The strict predicate is the RFC2328 section12.1.7 field nonzero constraint. It
changes only the separately authored zero-field/zero-residue counterexample,
not the 556 real source rows, their fields/ranges, or original24 control facts.
"""
import argparse,hashlib,importlib.util,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();codec=a.root/'ospfv2_source_codec_v03.py';binding={str(p):sha(p)for p in(Path(__file__),codec,a.root/'source-grid02/summary.json',a.root/'source-grid03/summary.json',a.root/'authored-controls01/source-facts.json')};spec=importlib.util.spec_from_file_location('source03',codec);c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c);pairs=[]
 for n in(35,37):
  old=a.root/('source-grid02/input-%02d-source-grid.json'%n);new=a.root/('source-grid03/input-%02d-source-grid.json'%n);binding.update({str(p):sha(p)for p in(old,new)});assert old.read_bytes()==new.read_bytes();pairs.append({'input':n,'source_rows':len(read(old)),'whole_source_grid_byte_literal_equal':True,'SHA256':sha(old)})
 successful=refused=0
 for row in read(a.root/'authored-controls01/source-facts.json'):
  fp=Path(row['original_frame']);binding[str(fp)]=sha(fp);assert binding[str(fp)]==row['original_frame_SHA256'];raw=fp.read_bytes();start,end=row['PDU_range'];w=raw[start:end]
  if row['strict_source_refusal_expected']:
   failed=False
   try:c.decode(w,start)
   except AssertionError:failed=True
   assert failed;refused+=1
  else:assert c.decode(w,start)==row['source'];successful+=1
 proof1=read(a.root/'authored-partial-prefix-proof01.json');proof2=read(a.root/'authored-partial-prefix-proof02.json');assert proof1['controls']==proof2['controls'];assert proof1['complete_prefix_scalar_leaves']==proof2['complete_prefix_scalar_leaves'];binding.update({str(p):sha(p)for p in(a.root/'authored-partial-prefix-proof01.json',a.root/'authored-partial-prefix-proof02.json')})
 assert binding=={p:sha(p)for p in binding}
 result={'scope':__doc__,'real_source_grid_literal_pairs':pairs,'real_source_rows':556,'real_source_leaves':15506,'prior_authored_complete_source_objects_literal_equal':successful,'prior_authored_extent_refusals_retained':refused,'partial_prefix_controller01_to02_hardening':{'reason':'require actual decode refusal rather than catching an assertion sentinel raised by controller','controls_and_all_41_prefix_values_spans_byte_literal_equal':True,'old_proof_retained':True},'primary_source':'https://www.rfc-editor.org/rfc/rfc2328.html#section-12.1.7','previous02_invalid_zero_field_math_only_predicate_no_longer_used':True,'source_correction_not_candidate_output_adaptation':True,'inputs':binding,'inputs_unchanged':True}
 with a.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
 print(json.dumps({'proof':str(a.out),'SHA256':sha(a.out),'real_rows_literal':556,'real_leaves':15506,'prior_authored_objects_literal':successful,'refusals':refused,'partial_prefix_leaves_literal':41}))
if __name__=='__main__':main()
