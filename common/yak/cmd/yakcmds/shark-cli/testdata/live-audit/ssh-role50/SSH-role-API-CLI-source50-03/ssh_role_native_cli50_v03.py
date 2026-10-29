#!/usr/bin/env python3
"""Metadata-only correction: nonnil SSH Session always determines SDK Summary."""
import argparse,copy,hashlib,importlib.util,json,subprocess
from pathlib import Path
HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
V2ROOT=PHASE/"SSH-role-API-CLI-source50-02"
V2PLAN=V2ROOT/"plan02"
V1PLAN=PHASE/"SSH-role-API-CLI-source50-01"/"plan01"
REPO=Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,indent=2,sort_keys=True);f.write("\n")
def demand(x,m):
    if not x:raise AssertionError(m)
def module(path):
    s=importlib.util.spec_from_file_location("source50_preview_checker",path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def previous():return module(V2ROOT/"ssh_role_native_cli50_v02.py")
def prepare(out):
    out.mkdir(exist_ok=False);v2=previous();v2.load(V2PLAN)
    assert sha(V2PLAN/"manifest.json")=="846909d9231ca1ef690bd45a7efda68a17bb1fd1c733b39e6bbac086088ac4b6"
    path="common/pcapx/pcaputil/bin_parser_session.go"
    regions=[];source=[]
    marker="\n\tif e.Session != nil {\n\t\t// Carried HTTP/2 and MySQL messages"
    for commit in("cc9d84079d455a64b57365e143b9e5ee641a7997","26f01f4baa5fd39b88deadf901257a70bc5c1dfe"):
        raw=subprocess.check_output(["git","show",commit+":"+path],cwd=REPO)
        text=raw.decode();at=text.index(marker);end=text.index("\n\treturn err\n}",at)
        region=text[at:end].encode()
        assert b'e.Summary = fmt.Sprintf("SSH %v", e.Session["Packet Name"])'in region
        regions.append(region)
        source.append({"commit":commit,"path":path,"file_SHA256":dg(raw),"shared_non_nil_Session_region_SHA256":dg(region),
            "outer_guard":"if e.Session != nil","SSH_summary_formula":'fmt.Sprintf("SSH %v", e.Session["Packet Name"])',
            "err_nil_is_not_an_outer_condition":True})
    assert regions[0]==regions[1],"preexisting Summary contract changed"
    put(out/"immutable-Summary-schema-source.json",source)
    for name in("source-rows.json","physical-rows.json","close-source-rows.json","alias-schema.json","opaque-preview-policy.json",
                "immutable-preview-policy-source.json","unchanged-field-contract-bridge.json"):
        (out/name).write_bytes((V2PLAN/name).read_bytes())
    rows=read(V2PLAN/"source-rows.json");changes=[]
    for r in rows:
        if r["expected_status"]!="malformed":continue
        assert r["source_role_assessment"]=="known-role-contradiction"and r["expected_summary"]=="ssh"
        changes.append({"frame":r["frame"],"sourcecase":r["sourcecase"],"old_expected_summary":r["expected_summary"],
            "new_expected_summary":"SSH "+r["session"]["Packet Name"],"derivation":"preexisting unconditional nonnil Session formula plus already frozen Packet Name",
            "Session_GetFields_alias_Error_role_status_range_or_CLI_changes":0})
    assert [r["frame"]for r in changes]==[24,36,89,103]
    put(out/"Summary-metadata-supplement.json",changes)
    put(out/"unchanged-source64-byte-bridge.json",{
        "original_source_rows_SHA256":sha(V1PLAN/"source-rows.json"),"v02_source_rows_SHA256":sha(V2PLAN/"source-rows.json"),
        "v03_source_rows_SHA256":sha(out/"source-rows.json"),
        "all64_original_source_row_file_bytes_equal":(V1PLAN/"source-rows.json").read_bytes()==(out/"source-rows.json").read_bytes(),
        "v02_preview_policy_bytes_equal":(V2PLAN/"opaque-preview-policy.json").read_bytes()==(out/"opaque-preview-policy.json").read_bytes(),
        "only_runtime_projection_delta_from_v02":"4 expected_summary metadata values; frozen Session determines them",
        "product_parser_or_fields_bug_claimed":False,"old_failure_outputs_and_plans_preserved":True})
    scope=read(V2PLAN/"scope.json")
    scope["additional_metadata_correction"]={"kind":"static outer-condition misread in checker, not product regression","affected_frames":[24,36,89,103],
        "formula":"SSH + space + frozen Session.Packet Name","all_codec_fields_CLI_preview_and_other60_summaries_unchanged":True,
        "candidate_values_used":"none; parent discovered Summary mismatch but expected strings come from original frozen Packet Name and immutable source"}
    put(out/"scope.json",scope)
    command=read(V2PLAN/"runtime-command-plan.json");command["verify"][0]=str(Path(__file__).resolve());command["verify"][3]=str(out)
    put(out/"runtime-command-plan.json",command)
    paths=[Path(__file__),V2ROOT/"ssh_role_native_cli50_v02.py",V2PLAN/"manifest.json",V2PLAN/"source-rows.json",V2PLAN/"opaque-preview-policy.json",V1PLAN/"source-rows.json"]
    paths+=list(p for p in out.iterdir()if p.is_file())
    put(out/"manifest.json",{"schema":"source50-SDK-Summary-metadata-supplement-v3","inputs":{str(p.resolve()):sha(p)for p in paths},
        "original64_source_rows_unchanged":True,"v02_preview_policy_unchanged":True,"only4_expected_summary_metadata_changes":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),
        "metadata_supplement_SHA256":sha(out/"Summary-metadata-supplement.json"),"original64_rows_SHA256":sha(out/"source-rows.json")}))
def load(plan):
    m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():demand(sha(p)==h,"frozen metadata supplement input "+p)
    demand((plan/"source-rows.json").read_bytes()==(V1PLAN/"source-rows.json").read_bytes(),"64 original source rows cannot change")
    v2=previous();previous_load=v2.load
    patches={r["frame"]:r for r in read(plan/"Summary-metadata-supplement.json")}
    def corrected_load(_):
        base=previous_load(V2PLAN);preview_load=base.load
        def summary_projection(ignored):
            old,eof,api,rows,physical,closes=preview_load(V2PLAN)
            rows=copy.deepcopy(rows)
            for r in rows:
                patch=patches.get(r["frame"])
                if patch is None:continue
                before=copy.deepcopy(r)
                demand(r["expected_summary"]==patch["old_expected_summary"],"old metadata unexpectedly changed")
                demand(patch["new_expected_summary"]=="SSH "+r["session"]["Packet Name"],"independent frozen-name formula")
                r["expected_summary"]=patch["new_expected_summary"]
                restored=copy.deepcopy(r);restored["expected_summary"]=before["expected_summary"]
                demand(restored==before,"metadata patch cannot change fields or CLI")
            return old,eof,api,rows,physical,closes
        base.load=summary_projection
        return base
    v2.load=corrected_load
    return v2
def selftest(plan,out):
    out.mkdir(exist_ok=False);v2=load(plan);base=v2.load(plan);events,CLI=base.synthetic(plan)
    result=base.verify_all(plan,events,CLI);neg=[]
    find=lambda x,n:next(e for e in x if e.get("raw")and e["source_bytes"]["PacketRefs"][0]["Number"]==n)
    def bad(name,fn):
        x,y=copy.deepcopy(events),copy.deepcopy(CLI);fn(x,y)
        try:base.verify_all(plan,x,y)
        except(AssertionError,KeyError,ValueError,TypeError):neg.append(name)
        else:raise AssertionError("bad Summary metadata accepted "+name)
    bad("old-default-summary-not-source-formula",lambda x,y:find(x,24).update(summary="ssh"))
    bad("standard-reply-caption-not-diagnostic-name",lambda x,y:find(x,36).update(summary="SSH KEX_ECDH_REPLY"))
    bad("Session-name-cannot-be-changed-to-fit-summary",lambda x,y:find(x,89)["session"].update({"Packet Name":"KEX_DH_GEX_GROUP"}))
    bad("ordinary-summary-still-exact",lambda x,y:find(x,61).update(summary="ssh"))
    bad("CLI-diagnostic-may-not-be-relaxed",lambda x,y:y[35].update(info="ssh [connection #3: SSH]"))
    bad("preview64-policy-remains-active",lambda x,y:find(x,13)["source_bytes"].update(SHA256=read(V1PLAN/"source-rows.json")[0]["raw_SHA256"]))
    result.update(SDK_opaque_retained_bytes=112,source_opaque_consumed_bytes=240,unretained_source_tail_bytes=128,SDK_full_tail_coverage_claimed=False)
    put(out/"summary.json",{"passed":True,"plan_SHA256":sha(plan/"manifest.json"),"checker_SHA256":sha(__file__),
        "counts":result,"negative_controls":neg,"source64_rows_unchanged":True,"v02_preview_policy_unchanged":True,"candidate_payload_read":False})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"negative_controls":len(neg),"counts":result}))
def main():
    p=argparse.ArgumentParser();s=p.add_subparsers(dest="cmd",required=True)
    q=s.add_parser("prepare");q.add_argument("--out",type=Path,required=True)
    q=s.add_parser("selftest");q.add_argument("--plan",type=Path,required=True);q.add_argument("--out",type=Path,required=True)
    q=s.add_parser("verify")
    for n in("plan","events","CLI","descriptors","out"):q.add_argument("--"+n,type=Path,required=True)
    q.add_argument("--events-producer",type=Path);q.add_argument("--CLI-producer",type=Path)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.out)
    elif a.cmd=="selftest":selftest(a.plan,a.out)
    else:
        v2=load(a.plan)
        v2.verify(a.plan,a.events,a.CLI,a.descriptors,a.out,a.events_producer,a.CLI_producer)
if __name__=="__main__":main()

