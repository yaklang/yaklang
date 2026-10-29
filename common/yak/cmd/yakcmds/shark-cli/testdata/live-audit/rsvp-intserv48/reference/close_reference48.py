"""Close observed reference boundaries and display limitations source48.

This is a second offline evidence view over already bound TShark output. It
never edits the frozen field mapping, source values or primitive comparison.
"""
from pathlib import Path
import argparse,collections,csv,hashlib,json,xml.etree.ElementTree as ET
ROOT=Path(__file__).parent;P=ROOT.parent

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with p.open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,allow_nan=False);f.write('\n')
def walk(e,parents=()):
 for child in e:
  if child.tag=='field':
   yield child,parents;yield from walk(child,parents+(child,))
  else:yield from walk(child,parents)

def close(out):
 assert not out.exists();out.mkdir();plan=ROOT/'plan02';actual=ROOT/'actual01';comparison=ROOT/'comparison02'
 assert sha(plan/'manifest.json')=='07f9652a93da40a5cd768251e82cdc8c50e79462ca124f14004cbed2175ae6c7'
 assert sha(actual/'producer.json')=='214f4bd3adebb7e86805a6adfea1b745d9db4952d234830a1c9fe96027a336ee'
 assert sha(comparison/'summary.json')=='9129debbdf18ba4afb78bbccdd5b634784e2c8ce3efd37e32652fd0116672613'
 rows=read(P/'Intserv-native-CLI-source48-01/plan02/source-rows.json');leaf=read(comparison/'per-source-leaf-comparison.json');owners=read(plan/'source-bounded-owners.json');frames=read(comparison/'physical-reference-observations.json')
 packets={k:ET.parse(actual/k/'reference.pdml').getroot().findall('packet')for k in('official','authored')};siblings=[];local_warnings=[];columns={}
 value_aliases={'rsvp.tspec.token_bucket_rate','rsvp.tspec.token_bucket_size','rsvp.tspec.peak_data_rate','rsvp.flowspec.token_bucket_rate','rsvp.flowspec.token_bucket_size','rsvp.flowspec.peak_data_rate','rsvp.minimum_policed_unit','rsvp.maximum_packet_size','rsvp.adspec.uint','rsvp.adspec.float'}
 for row in rows:
  context=row['capture_key'];number=row['anchor']['number'];base=row['anchor']['PDU_physical_range'][0];raw=Path(row['raw_path']).read_bytes();refs=list(walk(packets[context][number-1]));targets=[o for o in owners if o['context']==context and o['frame']==number]
  for field,parents in refs:
   if field.get('name')!='rsvp.parameter':continue
   at=int(field.get('pos','0'))-base
   for target in targets:
    start,end=target['object_logical_range'];wrapper=[x for x in parents if x.get('name')in('rsvp.tspec','rsvp.flowspec','rsvp.adspec')and int(x.get('pos','-1'))==base+start]
    if wrapper and at>=end and at<len(raw):
     show=int(field.get('show'));assert show==raw[at]
     siblings.append({'context':context,'frame':number,'sourcecase':row['sourcecase'],'proved_object_range':[start,end],'reference_wrapper_range':[int(wrapper[-1].get('pos'))-base,int(wrapper[-1].get('pos'))-base+int(wrapper[-1].get('size'))],'reference_parameter_highlight':[at,at+int(field.get('size'))],'actual_parameter_number_source_byte_range':[at,at+1],'rendered_parameter_number':show,'source_following_bytes_treated_as_owned_root_residual':row['source_case']['fields']["Wire Field Ranges"].get('Unparsed Body'),'field_source_parent_violation':True,'not_claimed_memory_out_of_bounds':True})
  for item in row['composed_source']['semantic_issues']:
   issue=item['issue'];a,b=issue['range'];attached=[]
   for field,parents in refs:
    if field.get('name')!='_ws.expert.message':continue
    if any(parent.get('name')in value_aliases and int(parent.get('pos','-1'))==base+a and int(parent.get('size','0'))==b-a for parent in parents):attached.append(field.get('show'))
   local_warnings.append({'context':context,'frame':number,'sourcecase':row['sourcecase'],'source_issue':issue['code'],'source_primitive_range':[a,b],'reference_expert_messages_attached_to_that_literal':attached,'not_a_claim_that_all_global_diagnostics_are_absent':True})
 for context in('official','authored'):
  with(actual/context/'reference-fields.tsv').open(encoding='utf-8-sig',newline='')as f:exports=list(csv.DictReader(f,delimiter='\t',quotechar='"'))
  columns[context]={'physical_records':len(exports),'raw_column_labels':dict(collections.Counter(r['_ws.col.protocol']for r in exports))}
 assert len(siblings)==1 and siblings[0]['sourcecase']=='message-crosses-object-not-time-sibling'
 assert len(local_warnings)==17 and all(not x['reference_expert_messages_attached_to_that_literal']for x in local_warnings)
 nans=[x for x in leaf if x['source_kind']=='ieee32'and x['source_value']['classification']=='nan'];assert len(nans)==2 and all(x['reference']['show']=='nan'and x['raw_reference_value_matches_its_own_span']for x in nans)
 categories=[
  {'kind':'missing-complete-partial-header-flags','cases':['parameter-header-two-bytes','parameter-header-three-bytes'],'occurrences':2,'meaning':'complete1B flags observable in source but not rendered by selected WS alias; other header prefixes survive'},
  {'kind':'clipped-aggregate-parameter-owner-highlight','cases':['float-prefix-three-bytes','float-prefix-seven-bytes'],'occurrences':2,'meaning':'numeric parameter id matches; WS owner highlight clips to7/11 captured bytes, neither1B primitive nor full declared owner; not a numeric bug'},
  {'kind':'adspec-type-highlight-two-bytes','cases':['parameter-crosses-service'],'occurrences':1,'meaning':'type4 correct but span20..22 differs from complete1B type20..21'},
  {'kind':'adspec-value-highlight-exceeds-captured-parent','cases':['parameter-crosses-service'],'occurrences':1,'meaning':'value3 comes from complete source24..28, but aggregate highlight20..32 exceeds capture/parent28; does not prove value read outside capture'},
  {'kind':'unknown-service-fails-to-resume-known-sibling','cases':['unknown-service-before-known'],'occurrences':6,'meaning':'source service1 and hopcount sibling has six selected primitive/derived aliases absent after unknown99'},
  {'kind':'next-object-read-as-previous-intserv-parameter','cases':['message-crosses-object-not-time-sibling'],'occurrences':1,'meaning':'field ancestry proves TSPEC8..20 owns parameter number byte20 from TIME sibling; safe source profile refuses inner parent overrun'},
  {'kind':'future-version-displays-version0-value-layout','cases':['future-message-version'],'occurrences':5,'meaning':'five TSPEC body fields still rendered despite observed Intserv version1; selected source profile leaves them opaque/context-required'},
  {'kind':'NaN-text-does-not-encode-payload-bits','cases':['b-nan-retain-payload-bits','p-nan-retain-payload-bits'],'occurrences':2,'meaning':'both display nan; distinct source payload bits remain verifiable only in raw values. Display limitation, not numeric conflict'},
  {'kind':'units-not-explicitly-rendered','occurrences':112,'meaning':'source units bytes/s,bytes,microseconds are known; selected PDML shownames omit units. Not a unit agreement or numeric scaling bug'},
  {'kind':'no-literal-owned-value-domain-diagnostic','occurrences':17,'meaning':'source records illegal r/b0,-0,NaN,-Inf,m0/M0, while no expert is attached to those literal owners. Global Unknown session type/procedure warnings can still exist'},
  {'kind':'additional-reference-Guaranteed-service-support','cases':['guaranteed-service-remains-opaque'],'occurrences':1,'meaning':'reference decodes Ctot3; bounded product profile intentionally leaves service2 opaque. Additional capability, not a reference bug'}]
 put(out/'cross-object-owner-evidence.json',siblings);put(out/'literal-owner-semantic-diagnostics.json',local_warnings);put(out/'reference-difference-categories.json',categories)
 summary={'source_mapping_SHA256':sha(plan/'manifest.json'),'reference_producer_SHA256':sha(actual/'producer.json'),'primitive_comparison_SHA256':sha(comparison/'summary.json'),'counts':read(comparison/'summary.json')['counts'],'fields_export_counts':columns,'cross_object_interpretations':len(siblings),'difference_categories':len(categories),'IEEE32_source_occurrences':sum(x['source_kind']=='ieee32'for x in leaf),'all_rendered_float_source_bits_and_numeric_classifications_match':all(x['raw_reference_value_matches_its_own_span']and x['rendered_value_matches']for x in leaf if x['source_kind']=='ieee32'),'source_mapping_unselected_is_not_claimed_WS_absence':True,'mapping_unselected_includes_three_base_ERROR_SPEC_aliases_not_added_post_dissection':True,'authored_controls_wire_scope_not_complete_RSVP_procedure':'No SESSION object is intentionally fabricated; global Unknown session type warnings do not prove numeric domain correctness or incorrectness.','old_mapping01_failure_and_comparator01_erratum_preserved':True,'candidate_Go_SDK_CLI_outputs_not_read':True,'all_external_files_SHA256':{str(p):sha(p)for p in[Path(__file__),plan/'manifest.json',actual/'producer.json',comparison/'summary.json',ROOT/'comparison01-classification-erratum.json',ROOT/'plan01-preparation-failure.json']}|{str(p):sha(p)for p in out.iterdir()if p.is_file()},'no_raw_capture_full_PDML_or_expected_data_for_repository':True}
 put(out/'summary.json',summary)
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'difference_categories':len(categories),'cross_object_interpretations':len(siblings),'IEEE32_occurrences':summary['IEEE32_source_occurrences']}))
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);close(p.parse_args().out)