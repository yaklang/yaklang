"""Close source50 reference facts; no reference or SDK producer is invoked."""
import hashlib,json
from pathlib import Path
ROOT=Path(__file__).resolve().parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,v):
    with p.open("x",encoding="utf-8")as f:json.dump(v,f,indent=2,sort_keys=True);f.write("\n")
out=ROOT/"closure01";out.mkdir(exist_ok=False)
plan=ROOT/"plan01";actual=ROOT/"actual01";before=ROOT/"comparison01";after=ROOT/"comparison02"
m=read(plan/"manifest.json");p=read(actual/"producer.json")
assert p["all_processes_closed"]and p["plan_SHA256"]==sha(plan/"manifest.json")
for path,h in m["inputs"].items():assert sha(path)==h
for name,h in m["files"].items():assert sha(plan/name)==h
for r in p["runs"]:assert r["exit_code"]==0 and sha(r["output"])==r["output_SHA256"]and sha(r["stderr"])==r["stderr_SHA256"]
counts=read(after/"summary.json")["contexts"]
rep=[]
for mode in("natural","explicit-ssh-reference"):
    old=read(before/(mode+"-packet-observations.json"));new=read(after/(mode+"-packet-observations.json"))
    assert (before/(mode+"-field-results.json")).read_bytes()==(after/(mode+"-field-results.json")).read_bytes()
    for x,y in zip(old,new):
        if x!=y:
            a=x.copy();b=y.copy();assert a.pop("direction_matches_SYN")is True and b.pop("direction_matches_SYN")is None
            assert a==b and not x["reference_directions"]
            rep.append({"mode":mode,"frame":x["frame"],"reason":"missing direction alias is unknown, not a vacuous all([]) success"})
    rows=read(after/(mode+"-field-results.json"));missing=[r for r in rows if r["result"]=="not-matched"]
    assert missing==[{"frame":47,"alias":"ssh.padding_string","source_span":[78,86],"mapping_kind":"primitive","result":"not-matched"}]
    unknown=next(r for r in new if r["frame"]==47)
    assert unknown["source_assessment"]=="unselected-private-opaque"and unknown["reference_experts"]==["Malformed Packet (Exception occurred)"]
    assert unknown["reference_method_names"]==["private-role@example.test"]
    bad=[r for r in new if r["source_assessment"]=="known-role-contradiction"]
    assert [r["frame"]for r in bad]==[24,36,89,103]and all(not r["reference_experts"]for r in bad)
    assert [r["frame"]for r in bad if r["reference_hostkey_or_signature_aliases"]]==[36,103]
put(out/"erratum.json",{"old_comparison_SHA256":sha(before/"summary.json"),"new_comparison_SHA256":sha(after/"summary.json"),
   "changes":rep,"field_result_arrays_byte_identical":True,"source_plan_and_actual_unchanged":True,"old_comparison_preserved":True})
sources=[ROOT/"ssh_role_reference50.py",ROOT/"ssh_role_reference50_v02.py",Path(__file__)]
src=ROOT.parent/"SSH-role-source50-01"
sources+=[src/"ssh_role_source50.py",src/"ssh_role_source50_v02.py"]
put(out/"summary.json",{"result":"CLOSED_REFERENCE_WITH_PRESERVED_ROLE_DIFFERENCES","source50_manifest_SHA256":sha(src/"plan02/manifest.json"),
    "reference_plan_SHA256":sha(plan/"manifest.json"),"producer_SHA256":sha(actual/"producer.json"),"comparison02_SHA256":sha(after/"summary.json"),
    "physical_frames":115,"source_payloads":64,"source_TCP_carriers":9,"real_capture_count":0,
    "contexts":counts,"wrong_role_without_reference_role_expert":[24,36,89,103],"wrong_role_with_hostkey_fields":[36,103],
    "unmapped_private_method_padding":{"frame":47,"alias":"ssh.padding_string","source_physical_span":[78,86],
       "reference":"unknown private method is passed to default DH decoder and gets generic Malformed; padding isn't rendered",
       "source":"complete captured binary packet/enclosing carrier; unselected private method body does not justify DH semantic failure"},
    "natural_role_labels_wrong_for_SYN_positive":10,"explicit_adapter_role_labels_match_known_SYN":True,
    "noSYN_reference_direction_is_not_role_proof":True,"protected_sentinels_without_clear_fields":[11,13],
    "source50_does_not_include_tuple_reuse":"separate source49 case06->07 remains retained source proof",
    "erratum_SHA256":sha(out/"erratum.json"),"all_TShark_processes_closed":True,"no_Go_or_SDK_process_started":True,
    "no_candidate_SDK_payload_read":True,"no_more_TShark_until_parent_unfreezes":True,
    "reference_implementation":{"external_source":str(ROOT.parent/"SSH-Wireshark-source49-01/primary-reference01/packet-ssh-v4.6.9.c"),
      "SHA256":"74d36757fec72106de832989afae8d72ae9bd928f379ff6467a339bedf8b0fe2",
      "URL":"https://raw.githubusercontent.com/wireshark/wireshark/v4.6.9/epan/dissectors/packet-ssh.c"},
    "source_only_archive_candidates":[{"path":str(f),"SHA256":sha(f),"bytes":f.stat().st_size}for f in sources],
    "exclude_from_archive":["PCAP","raw inputs","source facts/wholeplans/expected JSON","PDML/fields outputs","downloaded C/RFC","logs","keys","PNG"]})
print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"erratum_SHA256":sha(out/"erratum.json")}))

