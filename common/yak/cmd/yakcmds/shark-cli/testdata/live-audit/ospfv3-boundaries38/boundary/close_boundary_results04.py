"""Append closed d677 source-field proof without relabeling prior manifests.

Counts/hashes only. No complete runtime events, packet data, key material or
expected case/field grids are copied. The ordinary proof is field/range/API and
progression coverage, not equality of every unrelated JSON event property.
"""
import argparse,ast,hashlib,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    root=a.root;phase=root.parent;bound={};artifacts=[]
    def bind(p,expected,role):
        p=Path(p);assert sha(p)==expected;bound[str(p)]=expected
        if p.suffix=='.py':ast.parse(p.read_bytes())
        artifacts.append(dict(path=str(p),SHA256=expected,bytes=p.stat().st_size,role=role))
        return read(p)if p.suffix=='.json'else None
    bind(root/'source-closure03-01/manifest.json','b78d384ec4aa4b64367ff34f1abb2a8ab63ff0510a5bc7543d6729edfab34837','historical before-candidate source closure;not rewritten')
    gp=bind(root/'field-source-plan01/plan.json','8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d','unchanged field source plan')
    bind(gp['data_file'],'c5133b8f3b4d57a4456f47aab58eb82ad9fac99ad52e61702df6dd33fa7c56e0','external unchanged source grid reference;not included as payload')
    bind(root/'verify_boundary_fields01.py','a4604d3ef75563bd619bcb8afaf11ad82917e484e94d5bc84249f2dacc0b3804','old checker retained;wrong snapshot metadata assertion')
    bind(root/'verify_boundary_fields02.py','1eedc0cd7367b2ba2556de9d7e30ea51a3f4e6c850bff3e044c538b1e186d5f3','checker with immutable public snapshot policy correction only')
    bind(root/'verify_direct_fields01.py','1a0b6ed40459384baf3574a0aee2c3412e3a7ffd848a186888c4032334d437fc','old flat execution-receipt reader retained')
    bind(root/'verify_direct_fields02.py','3b909196890535c2704ce17a195ad4b6c273f4762558e75b0ba23075dbaebae9','same direct protocol checks;structured producer binding and explicit errors.Is flag')
    policy=bind(root/'metadata-policy-correction02/policy-bridge.json','979c8b4b32dd3c82d89a192d03482ca6dc1940a650d36820a8ae0c465abbb74e','a82/685/d677 API policy source correction bridge;not a product bug')
    bind(root/'direct-metadata-adapter02/receipt-adapter-bridge.json','7a5f2e8eaa0ba53e6c4db6fc5954a348e376792cde75844fefec4eef3b6ad22f','receipt shape correction;protocol source values unchanged')
    for name in('correct_snapshot_metadata_policy02.py','prepare_direct_metadata_adapter02.py','close_boundary_results04.py'):bind(root/name,sha(root/name),'reproducible external metadata correction/closure source')
    bind(root/'verifier-selftest02.json','e3283ac1ad4a677d430fb178317bdb8ad9c99661fdd6c6a1154966ed1ce75288','pure checker20positive/18negative;not native correctness')
    actual=bind(phase/'OSPF-boundary-after-d677-source03-01/summary.json','0946aab8b69555627a88335b62480dbc8955f4c27b63d1757cfe38eb7c612742','closed source03 ordinary17 existing-binary producer')
    failed=bind(phase/'OSPF-boundary-fields-d677-source03-01/summary.json','fc96147056c65bd93785068a2714a874749c41b3967cc606fbccc3a8bdf76d7b','preserved checker01 snapshot-policy failure17/17')
    native=bind(phase/'OSPF-boundary-fields-d677-source03-02/summary.json','7d3ffaa854ca318e4b4b98495ee8b8e8f47d9368b6bcedb1b9c1c7adfdb96aae','closed source fields/private typed/API/physical-vs-logical CLI proof')
    direct=bind(phase/'OSPF-direct-source-fields-d677-01/summary.json','4ee28484e91e1e47ada32d37250fe132924f91cd06c5fc13fe1fd2da4c4572ef','closed three explicit direct source/budget/context/integrity checks')
    bind(phase/'OSPF-direct-d677-source03-01/producer.json','3d93dba7c8c9babc31967abdc1a95eb189ee570e247885fa9a072993f647c1f8','closed direct existing-binary execution;no native ingress or typed export')
    bind(phase/'ospfv3_direct_boundary_probe38.go','76060b1a7acc8df79205dda87045fced03ae50d1877771c2cd7751fe631fa9bd','root-owned Go helper source with explicit errors.Is integrity;not copied or built by child')
    assert native['passed_cases']==17 and native['failed_cases']==0 and native['source_leaves']==283 and native['private_typed_occurrences']==259
    assert direct['passed_cases']==3 and direct['failed_cases']==0 and direct['source_leaves']==60
    assert all(r['Go_errors_Is_integrity_checked']for r in direct['results'])
    assert failed['failed_cases']==17 and policy['wire_or_typed_oracle_modified']is False
    counts=dict(source_commit='d6772965c669e288705f7e0d0e25c62972226832',authored_controls=20,ordinary_captures=17,direct_explicit_caller_contexts=3,
        ordinary_source_leaves_verified=283,ordinary_private_typed_occurrences_verified=259,direct_source_leaves_verified=60,total_source_leaves_verified=343,
        direct_typed_occurrences_not_exported_or_verified=54,Go_errors_Is_integrity_verified_cases=3,unplanned_source_field_differences=0,
        reference_inputs=20,raw_reference_source_checks=94,raw_reference_differences=0,reference_checksum_applicability_disagreements_preserved=2,reference_complete_captured_prefix_scalars_omitted=10,
        source_grid_changed=False,old_checker01_failures=17,old_checker_failure_reason='public snapshot field-coverage completeness was incorrectly equated to event outcome;not a parser defect',
        failed_artifacts_preserved=True,no_new_expected_values_from_candidate=True,whole_event_JSON_equality_claim=False,TOP30_complete_claim=False)
    after={p:sha(p)for p in bound};assert bound==after
    with(a.out/'manifest.json').open('x',encoding='utf8')as f:json.dump(dict(scope=__doc__,counts=counts,artifacts=artifacts,inputs_unchanged=True),f,indent=2);f.write('\n')
    print(json.dumps(dict(manifest=str(a.out/'manifest.json'),SHA256=sha(a.out/'manifest.json'),ordinary=17,direct=3,source_leaves=343,private_typed=259)),flush=True)
if __name__=='__main__':main()
