#!/usr/bin/env python3
"""Close reference observations and checker erratum, without product execution."""
import argparse
import collections
import csv
import hashlib
import json
from pathlib import Path
import xml.etree.ElementTree as ET
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2,ensure_ascii=False);f.write("\n")
def rows(p):return list(csv.DictReader(Path(p).open(encoding="utf-8"),delimiter="\t"))
def main():
    ap=argparse.ArgumentParser();ap.add_argument("--root",type=Path,required=True);ap.add_argument("--out",type=Path,required=True);a=ap.parse_args()
    root=a.root.resolve();a.out.mkdir(exist_ok=False);plan=root/"plan01";actual=root/"actual01";cmp1=root/"comparison01";cmp2=root/"comparison02"
    p=read(plan/"manifest.json");prod=read(actual/"producer.json");s1=read(cmp1/"summary.json");s2=read(cmp2/"summary.json")
    assert sha(plan/"manifest.json")=="e8ce89c0dd9951191ce317926cc7039d15804e203e039ec208d2c1b8b2e1ccfe"
    assert sha(actual/"producer.json")=="f8262508691566e1d2da7db304f3564cd18cd8e21fee253ec57229dc38163dfe"
    assert sha(cmp1/"summary.json")=="fc2973996bb061b2371c40cd61f8e05d4531d24138940d1ee9537f4268ef9204"
    assert sha(cmp2/"summary.json")=="62d141d4af92db409ae416431c5a663a026ffd0f06a8eb02657fd15e269a7c3a"
    assert prod["all_processes_closed"] and prod["source_mapping_frozen_before_packet_execution"]
    for path,d in p["inputs"].items():assert sha(path)==d
    for path,d in p["files"].items():assert sha(plan/path)==d
    for r in prod["runs"]:assert r["exit_code"]==0 and sha(r["output"])==r["output_SHA256"]and sha(r["stderr"])==r["stderr_SHA256"]
    # Inventory explicitly advertised no host-key fingerprint field; do not
    # manufacture an alias to claim a reference fingerprint comparison.
    inv=(root/"metadata01/fields.txt").read_text(encoding="utf-8").splitlines()
    fingerprints=[line for line in inv if line.startswith("F\t")and "\tssh."in line and ("fingerprint"in line.lower()or"sha256"in line.lower())]
    assert not fingerprints
    wire_corrections=[]
    for mode in("natural","explicit-ssh-reference"):
        r1=read(cmp1/(mode+"-field-results.json"));r2=read(cmp2/(mode+"-field-results.json"))
        assert len(r1)==len(r2)==866
        for x,y in zip(r1,r2):
            if x["result"]!=y["result"]:
                assert x["frame"]==y["frame"] and x["path"]==y["path"]and x["alias"]==y["alias"]and x["reference_expected_span"]==y["reference_expected_span"]
                assert (x["result"],y["result"])in(
                    ("literal-value-difference","zero-width-literal-matched"),
                    ("reference-alias-present-without-source-leaf","identification-content-match-CRLF-excluded"))
                wire_corrections.append({"mode":mode,"frame":x["frame"],"path":x["path"],"old":x["result"],"new":y["result"]})
        assert sum(r["result"]=="zero-width-literal-matched"for r in r2)==48
        assert sum(r["result"]=="identification-content-match-CRLF-excluded"for r in r2)==24
        assert not any(r["result"]=="literal-value-difference"for r in r2)
    put(a.out/"checker-01-to-02-erratum.json",{
        "kind":"checker/PDML representation correction, no product or source fact change",
        "old_summary_SHA256":sha(cmp1/"summary.json"),"new_summary_SHA256":sha(cmp2/"summary.json"),
        "source_mapping_manifest_SHA256_unchanged":sha(plan/"manifest.json"),
        "actual_producer_SHA256_unchanged":sha(actual/"producer.json"),
        "changes":wire_corrections,
        "explanation":[
            "A PDML empty FT_STRING has show='' and size0 with no value attribute. This proves the empty rendered value, not a mismatch or selectable range.",
            "ssh.protocol intentionally selects the29-byte identification content, while the source binary line is31bytes including CRLF. Exact source prefix matches; terminators stay outside this reference field.",
            "Generic subtree method names are carried in show, not showname. v02 reads both display metadata attributes.",
            "v02 adds independent SYN-role and unknown-method-caption observations; no expected packet value/span changed.",
            "An ad hoc inspection first used _ws.col.Protocol as a TSV header; this pinned tool emits lowercase _ws.col.protocol. No producer or checker plan was changed for this inspection typo."],
        "old_files_preserved":True})
    natural=rows(actual/"natural/reference-fields.tsv");forced=rows(actual/"explicit-ssh-reference/reference-fields.tsv")
    assert len(natural)==len(forced)==148
    fielddiff=[]
    for x,y in zip(natural,forced):
        assert x.keys()==y.keys()
        changed={k:{"natural":x[k],"explicit":y[k]}for k in x if x[k]!=y[k]}
        if changed:
            assert set(changed)=={"ssh.direction"}
            fielddiff.append({"frame":int(x["frame.number"]),"differences":changed})
    assert [r["frame"]for r in fielddiff]==list(range(92,98))
    trees=[ET.parse(actual/mode/"reference.pdml").getroot()for mode in("natural","explicit-ssh-reference")]
    domdiff=[];generation_metadata_diff=[]
    for i,(x,y)in enumerate(zip(trees[0].iter(),trees[1].iter())):
        assert x.tag==y.tag and len(x)==len(y)
        if x.attrib!=y.attrib:
            if x.tag=="pdml":
                diff={k:{"natural":x.get(k),"explicit":y.get(k)}for k in x.attrib.keys()|y.attrib.keys()if x.get(k)!=y.get(k)}
                assert set(diff)=={"time"}
                generation_metadata_diff.append(diff)
                continue
            assert x.tag=="field"and x.get("name")==y.get("name")=="ssh.direction"
            diff={k:{"natural":x.get(k),"explicit":y.get(k)}for k in x.attrib.keys()|y.attrib.keys()if x.get(k)!=y.get(k)}
            assert set(diff)=={"show","showname"}
            domdiff.append({"element_index":i,"field":"ssh.direction","differences":diff})
    assert len(domdiff)==6
    put(a.out/"natural-to-decode-as-differences.json",{"byte_identical":False,"TSV_changed_rows":fielddiff,
        "PDML_changed_attributes":domdiff,"PDML_export_generation_time_difference":generation_metadata_diff,
        "all_other_PDML_packet_elements_attributes_and_selected_TSV_values_equal":True,
        "natural_role_error_sourcecase":"08-libssh-correct-guess-server-first",
        "source_server_first_banner":92,"source_client_SYN":89,
        "source_role_bound_to_SYN_and_packet_direction":True,
        "decode_as_is_reference_adapter_not_content_identification":True})
    packets=read(cmp2/"natural-packet-results.json")
    lookup={r["frame"]:r for r in packets}
    assert lookup[82]["SSH_message_codes"]==["21"]and lookup[84]["SSH_message_codes"]==["21"]
    assert not lookup[83]["SSH_message_codes"]and not lookup[85]["SSH_message_codes"]
    watches=read(cmp2/"natural-semantic-observations.json")
    bad_methods=sorted(w["frame"]for w in watches if w["category"]=="reference-method-fields-on-unselected-source")
    assert bad_methods==[8,9,20,21,32,33,56,68,143]
    bad_caption=sorted(w["frame"]for w in watches if w["category"]=="reference-standard-method-caption-on-unselected-source")
    malformed=sorted(w["frame"]for w in watches if w["category"]=="reference-malformed-on-unselected-private-method")
    assert malformed==[45,57,69]
    methods=read(cmp2/"natural-method-displays.json")
    assert sum(bool(r["reference_method_names"])for r in methods)==50
    assert all(not r["reference_method_names"]or r["reference_method_names"]==[r["source_selected_literal"]]for r in methods)
    missing=[{"frame":r["frame"],"alias":r["alias"],"path":r["path"]}for r in read(cmp2/"natural-field-results.json")if r["result"]=="source-alias-not-rendered"]
    assert missing==[{"frame":n,"alias":"ssh.padding_string","path":["Padding"]}for n in(45,57,69)]
    reference_primary=read(root/"primary-reference01/manifest.json")
    assert sha(reference_primary["path"])==reference_primary["SHA256"]
    # Line windows are citations into a hash-pinned external source, not packet
    # values and never copied into repository source bundles.
    evidence={
        "official_reference_source":reference_primary,
        "source_lines":[
            {"lines":[1058,1059],"claim":"New reference flow defaults the method-specific handler to DH."},
            {"lines":[1079,1092],"claim":"Initial reference server role uses registered/matched port or first dissector arrival, rather than original SYN role."},
            {"lines":[1665,1668],"claim":"Reference retains the negotiated literal algorithm in Key Exchange display metadata."},
            {"lines":[2555,2561],"claim":"Reference negotiation explicitly does not apply first_kex_packet_follows."}],
        "wire_source_plan_SHA256":sha(plan/"manifest.json"),
        "reference_method_fields_on_unselected_frames":bad_methods,
        "reference_standard_method_caption_on_unselected_frames":bad_caption,
        "reference_malformed_on_private_body_frames":malformed,
        "missing_padding_after_reference_exception":missing,
        "wrong_guess":{"frame":143,"source":"ignored guessed DH value, selected Curve25519",
                       "reference":"renders ECDH q_c from the1byte integer payload","following_correct_curve_frames":[144,145]},
        "newkeys":{"server":82,"server_protected_sentinel":83,"client":84,"client_protected_sentinel":85,
                   "both_direction_plaintext_stops_observed":True,
                   "sentinel83_reference_exception":"not a valid ciphertext control; no AEAD or real SSH encrypted-flow failure claimed"},
        "tuple_reuse":{"previous_private_sourcecase":6,"new_curve_sourcecase":7,"source_SYN_frames":[61,73],"reference_streams":[5,6],
                       "all12_source_generations_have_distinct_reference_streams":True},
        "native_or_product_output_read":False,
    }
    put(a.out/"reference-difference-evidence.json",evidence)
    scripts=[root/n for n in("prepare_ssh_reference49.py","run_ssh_reference49.py","compare_ssh_reference49.py","compare_ssh_reference49_v02.py","close_ssh_reference49.py")]
    put(a.out/"summary.json",{
        "result":"SOURCE_REFERENCE_CLOSED_WITH_PRESERVED_DIFFERENCES",
        "tool_SHA256":"662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b",
        "plan_SHA256":sha(plan/"manifest.json"),"producer_SHA256":sha(actual/"producer.json"),
        "comparison02_SHA256":sha(cmp2/"summary.json"),"physical_frames":148,"authored_connections":12,"real_capture_count":0,
        "source_control_groups":{"unknown_method":6,"known_method_or_state":6},"SDK_mutation_negatives":16,
        "per_reference_counts":s2["contexts"]["natural"]["counts"],
        "unique_reference_aggregate_owners":s2["contexts"]["natural"]["unique_aggregate_owners_matched"],
        "differences_overlap_not_additive":True,"literal_value_conflicts_after_representation_correction":0,
        "reference_method_names_literal_matches":50,"unmapped_source_leaves":13,"zero_width_strings_not_highlight_proof":48,
        "semantic_differences":{"unselected_method_fields":bad_methods,"unselected_standard_captions":bad_caption,
                                 "private_body_reference_exception":malformed,"natural_server_first_wrong_direction":list(range(92,98))},
        "all_TShark_processes_closed":True,"SDK_baseline_or_candidate_read":False,
        "erratum_SHA256":sha(a.out/"checker-01-to-02-erratum.json"),
        "reference_evidence_SHA256":sha(a.out/"reference-difference-evidence.json"),
        "natural_decode_as_delta_SHA256":sha(a.out/"natural-to-decode-as-differences.json"),
        "source_only_archive_candidates":[{"path":str(x),"SHA256":sha(x),"bytes":x.stat().st_size}for x in scripts],
        "archive_exclude":["allPCAP/raw/case/plan/expected/PDML/fields exports","downloaded official C/RFC","keys","logs","images"],
        "scope_limit":"This is authored wire and public-reference behavior, not a live SSH/cipher/key exchange/host authentication/SFTP correctness proof.",
    })
    print(json.dumps({"summary":str(a.out/"summary.json"),"SHA256":sha(a.out/"summary.json"),
                      "reference_evidence_SHA256":sha(a.out/"reference-difference-evidence.json"),
                      "erratum_SHA256":sha(a.out/"checker-01-to-02-erratum.json")}))
if __name__=="__main__":main()

