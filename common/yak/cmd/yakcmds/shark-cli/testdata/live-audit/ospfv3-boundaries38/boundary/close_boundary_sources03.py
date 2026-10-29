"""Freeze authored20 source/range/reference/checker metadata before candidate.

Only paths, hashes and counts are written; original capture bytes, full source
case tables, field grids and PDML remain external references, not an archive.
"""
import argparse,ast,datetime,hashlib,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    root=a.root;phase=root.parent;artifacts=[]
    def bind(p,role):
        p=Path(p);data=p.read_bytes()
        if p.suffix=='.py':ast.parse(data)
        row=dict(path=str(p),SHA256=hashlib.sha256(data).hexdigest(),bytes=len(data),role=role);artifacts.append(row);return row
    roles={
        'prepare_controls.py':'authored original17 control source;not live traffic',
        'prepare_controls02.py':'add independent mixed budget+bad packet checksum source',
        'prepare_controls03.py':'add nonzero short-LSA-header prefixes12/18 source',
        'run_reference02.py':'closed18 TShark reference producer',
        'verify_reference_checksum_owners.py':'header-vs-LLS checksum owner and official-source proof',
        'run_prefix_reference03.py':'two new authored-prefix TShark reference producer',
        'close_source_revisions01.py':'literal original capture/frame/source-facts revision bridge',
        'build_boundary_field_grid.py':'independent complete captured wire leaf/range and pure alias constructor',
        'verify_boundary_fields01.py':'closed native Session/private aliases and actual CLI progression verifier',
        'selftest_boundary_verifier01.py':'source-only ephemeral checker positive/negative controls;not native correctness',
        'verify_direct_fields01.py':'three caller-context/budget direct fields verifier;no native/typed claim',
        'run_native_CLI_source03.py':'prepared ordinary17 source03 runner;not executed by child',
        'close_boundary_sources03.py':'this counts/hash source closure'}
    for name,role in roles.items():bind(root/name,role)
    bind(phase/'OSPFv3-independent-source37-a82-01/ospf_source_codec.py','immutable independent RFC source codec;not candidate code')
    bind(phase/'OSPFv3-field-plan-before-native-02/build_source_field_plan02.py','old pure alias mapper;source values supplied by authored bytes')
    bind(phase/'OSPFv3-integrity-name-contract38.json','root-supplied pure metadata names/strings;no candidate values')
    bind(phase/'OSPFv3-field-contract36-v02.json','immutable declared alias schema;32-bit aliases have no exact_value')
    bind(phase/'ospfv3_direct_boundary_probe.go','root-owned read-only direct helper source;not built or copied by child')
    bind(phase/'run_OSPF_boundary_replay.py','old685 source02 controller;kept distinct from prepared source03 runner')
    plans=[]
    for n in(1,2,3):
        d=root/('frozen-source%02d'%n);p=bind(d/'plan.json','frozen authored source plan reference;not included as payload')
        plan=read(d/'plan.json');assert sha(d/'cases.json')==plan['cases_SHA256'];bind(d/'cases.json','external original case metadata;never copied to a repository')
        for c in read(d/'cases.json'):
            for k in('capture','original_frame','source_facts'):assert sha(c[k])==c[k+'_SHA256']
        plans.append(dict(revision=n,cases=len(read(d/'cases.json')),plan=p))
    fp=read(root/'field-source-plan01/plan.json');bind(root/'field-source-plan01/plan.json','frozen source field plan reference')
    bind(fp['data_file'],'frozen external source leaf/range/typed grid reference;not archived')
    assert sha(fp['data_file'])==fp['data_SHA256'];assert fp['source_scalar_leaves']==343 and fp['typed_occurrences']==313
    proof_names=['source-revision-byte-bridge01.json','verifier-selftest01.json','reference02/summary.json','reference-prefix03/summary.json','reference-checksum-owner-proof01.json']
    for name in proof_names:bind(root/name,'closed source/reference/checker proof;no candidate correctness claim')
    bind(root/'reference-source/packet-ospf-v4.6.9.c','official exact Wireshark binary-tag source for checksum-owner analysis')
    bind(root/'reference-source/receipt02.json','official-source retrieval/tag-vs-binary commit equality proof')
    old=phase/'OSPF-boundary-source-fields-baseline685-01/summary.json';bind(old,'old685 source-grid negative baseline:15/15 fail;not a new producer')
    test=read(root/'verifier-selftest01.json');assert test['verifier_SHA256']==sha(root/'verify_boundary_fields01.py')and test['all_negative_mutations_rejected']is True
    ref=read(root/'reference02/summary.json');pre=read(root/'reference-prefix03/summary.json')
    assert ref['checks']==86 and ref['differences']==0 and pre['source_prefix_scalar_checks']==10 and pre['prefix_scalar_reference_omissions']==10
    rows=read(fp['data_file']);groups=[]
    for direct in(False,True):
        group=[r for r in rows if r['direct_codec_invocation_required']==direct]
        groups.append(dict(direct_only=direct,cases=len(group),source_wire_leaves=sum(len(r['source_fields']['source_leaves'])for r in group),source_typed_occurrences=sum(sum(len(x)for x in r['expected_private_typed_aliases'].values())for r in group),actual_typed_probe_available=not direct))
    summary=dict(scope=__doc__,created_UTC=datetime.datetime.now(datetime.timezone.utc).isoformat(),authored_not_real_traffic=True,source_revisions=plans,
        cases=20,ordinary_capture_cases=17,direct_context_or_budget_cases=3,source_wire_leaves=343,source_typed_occurrences=313,groups=groups,
        TShark_inputs=20,reference_raw_value_range_checks=94,reference_raw_differences=0,reference_captured_prefix_observations_omitted=10,
        normative_Wireshark_checksum_disagreements=2,disagreement_owner='OSPF checksum physical66..68 / PDU12..14,not LLS checksum90..92',
        reference_disagreements=['LLS-only bad block changes Wireshark OSPF-header sum verdict despite identical OSPFn bytes','AT+LLS header checksum should be omitted even when nonzero'],
        pure_checker_positive_witnesses=20,pure_checker_negative_mutations_rejected=18,checker_witnesses_are_not_native_correctness=True,
        old685_runtime_cases=15,old685_new_policy_failures=15,new_candidate_runtime_read=False,new_candidate_producer_executed_by_child=False,
        pending=['parent must execute source03 ordinary17 on closed binaries','parent direct probe must execute3 explicit caller contexts;no direct typed snapshots exposed','runtime fields certificate does not claim unrelated full event JSON equality'],
        no_repository_write=True,no_Go_execution_by_child=True,artifacts=artifacts)
    before={r['path']:r['SHA256']for r in artifacts};after={p:sha(p)for p in before};assert before==after
    write(a.out/'input-hashes-before.json',before);write(a.out/'input-hashes-after.json',after)
    summary['inputs_before_SHA256']=sha(a.out/'input-hashes-before.json');summary['inputs_after_SHA256']=sha(a.out/'input-hashes-after.json')
    write(a.out/'manifest.json',summary)
    print(json.dumps(dict(manifest=str(a.out/'manifest.json'),SHA256=sha(a.out/'manifest.json'),sources=len(roles),cases=20,source_leaves=343,typed_occurrences=313)),flush=True)
if __name__=='__main__':main()
