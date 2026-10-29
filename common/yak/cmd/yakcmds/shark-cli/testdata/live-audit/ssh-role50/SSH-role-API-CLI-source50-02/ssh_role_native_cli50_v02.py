#!/usr/bin/env python3
"""Source-derived legacy opaque preview policy; original64 field rows immutable."""
import argparse,base64,copy,hashlib,importlib.util,json,subprocess,re
from pathlib import Path
HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
V1=PHASE/"SSH-role-API-CLI-source50-01"
OLDPLAN=V1/"plan01"
SOURCE=PHASE/"SSH-role-source50-01"/"plan02"
REPO=Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,indent=2,sort_keys=True);f.write("\n")
def demand(x,why):
    if not x:raise AssertionError(why)
def module(path,name):
    s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def original():return module(V1/"ssh_role_native_cli50.py","unchanged_role_field_checker50")
def prepare(out):
    out.mkdir(exist_ok=False);base=original();base.load(OLDPLAN)
    assert sha(OLDPLAN/"manifest.json")=="a804afaa225ebcc1f46a799416f229cb65378909669da4ab9081024aaf3ef979"
    policyfiles=[
       "common/pcapx/pcaputil/bin_parser.go","common/pcapx/pcaputil/protocol_evidence.go",
       "common/pcapx/pcaputil/protocol_field_registry.go","common/pcapx/pcaputil/protocol_session.go"]
    source_bind=[]
    texts={}
    for path in policyfiles:
        old=subprocess.check_output(["git","show","cc9d84079d455a64b57365e143b9e5ee641a7997:"+path],cwd=REPO)
        new=subprocess.check_output(["git","show","26f01f4baa5fd39b88deadf901257a70bc5c1dfe:"+path],cwd=REPO)
        assert old==new,"preview policy unexpectedly changed "+path
        source_bind.append({"file":path,"old_source":"cc9d84079d455a64b57365e143b9e5ee641a7997",
                           "new_source":"26f01f4baa5fd39b88deadf901257a70bc5c1dfe","git_blob_bytes_SHA256":dg(old),"literal_bytes_equal":True})
        texts[path]=old.decode()
    bp=texts[policyfiles[0]];pe=texts[policyfiles[1]];registry=texts[policyfiles[2]];budget=texts[policyfiles[3]]
    assert "config.ProbeBytes = 64"in bp and re.search(r"ProbeBytes:\s+64",budget)
    assert "preview := wire[:min(len(wire), f.a.config.ProbeBytes)]"in bp
    assert "e.Length = len(wire)"in bp and "d.offset += uint64(len(wire))"in bp
    assert "f.a.contextRequired.Add(1)"in bp and "f.a.unclassified.Add(uint64(len(wire)))"in bp
    assert "sum := sha256.Sum256(e.Raw)"in pe and 'e.Status == "decoded" || e.Status == "deferred"'in pe
    assert 'meta("pdu.length"'in registry and "return e.Length"in registry
    put(out/"immutable-preview-policy-source.json",source_bind)
    for name in("source-rows.json","physical-rows.json","close-source-rows.json","alias-schema.json"):
        (out/name).write_bytes((OLDPLAN/name).read_bytes())
    rows=read(OLDPLAN/"source-rows.json");opaque=[];(out/"derived-preview").mkdir()
    for r in rows:
        if r["kind"]!="protected-sentinel":continue
        assert r["expected_status"]=="context-required"and r["expert"]=="Encrypted"and r["session"]is None and r["fields"]is None
        w=Path(r["raw_path"]).read_bytes();n=min(len(w),64);preview=w[:n];tail=w[n:]
        entry={"frame":r["frame"],"sourcecase":r["sourcecase"],"source_path":r["raw_path"],"source_SHA256":dg(w),"source_bytes":len(w),
           "policy":"complete observed NEWKEYS -> encrypted opaque stop -> default ProbeBytes64 retained prefix",
           "preview_bytes":n,"preview_SHA256":dg(preview),"unretained_source_bytes":len(tail),"unretained_source_SHA256":dg(tail),
           "logical_consumed_byte_count":len(w),"snapshot_pdu_length":len(w),"SDK_SourceBytes_SHA256_covers":"retained preview bytes only",
           "SDK_completed_in":"absent for context-required observation","source_tail_physical_span":[54+n,54+len(w)],
           "source_physical_frame_SHA256":r["anchor"]["physical_SHA256"],"full_input_capture_SHA256":sha(Path(read(SOURCE/"manifest.json")["capture"]["path"])),
           "opaque_chunk_is_not_proven_SSH_PDU_boundary":True}
        if tail:
            p=out/"derived-preview"/("frame"+str(r["frame"])+"-opaque-prefix.bin");p.write_bytes(preview);entry["derived_preview_path"]=str(p.resolve())
        else:entry["derived_preview_path"]=r["raw_path"]
        opaque.append(entry)
    assert [r["frame"]for r in opaque if r["unretained_source_bytes"]]==[13]
    assert next(r for r in opaque if r["frame"]==13)["source_bytes"]==192
    put(out/"opaque-preview-policy.json",opaque)
    put(out/"unchanged-field-contract-bridge.json",{
        "old_manifest_SHA256":sha(OLDPLAN/"manifest.json"),"new_source_rows_SHA256":sha(out/"source-rows.json"),
        "old_source_rows_SHA256":sha(OLDPLAN/"source-rows.json"),
        "all64_source_row_file_bytes_equal":(out/"source-rows.json").read_bytes()==(OLDPLAN/"source-rows.json").read_bytes(),
        "Session_GetFields_API_literal_alias_status_role_or_range_changes":0,
        "per_runtime_projection_exception":{"frame":13,"changed":["expected retained Raw input path","SourceBytes Raw SHA256"],
            "unchanged":["source192bytes","logical consumed Length192","all fields/Session values","all private SSH aliases",
                         "source physical frame","role/NEWKEYS interpretation","all other63 entire row expectations"]}})
    put(out/"scope.json",{
        "purpose":"checker01 incorrectly required opaque stop Raw to retain all consumed bytes; known default64 preview is preexisting source policy",
        "SDK_expected_retained_raw":"frame13 first64 of source192; not a full encrypted SSH PDU",
        "physical_input_tail":"remaining128 only source PCAP/full capture producer hash and physical frame metadata can prove; SDK Raw/snapshot does not carry them",
        "all_original64_field_rows_unchanged":True,"all_other63_runtime_row_expectations_unchanged":True,
        "retained_opaque_bytes":sum(r["preview_bytes"]for r in opaque),
        "consumed_opaque_bytes":sum(r["source_bytes"]for r in opaque),"unretained_opaque_source_bytes":sum(r["unretained_source_bytes"]for r in opaque),
        "SourceBytes_ref_scope":"constructed over retained preview before Length is expanded; this single-frame source still has the same sole frame ref",
        "accounting_code_only":{"frame13":{"contextRequired_increment":1,"unclassified_increment":192,"direction_offset_advance":192},
            "two_opaque_events":{"contextRequired_increments":2,"unclassified_increment":240},"runtime_stats_API_not_executed":True},
        "no_deciphered_record_boundary_or_crypto_claim":True,
        "failed_plan01_and_actual_checker_failures_preserved":True,"not_a_product_parsing_failure":True,
        "candidate_values_read_for_this_correction":"only parent-provided Raw SHA/length discovery and producer metadata, no Session/GetFields payload oracle"})
    cmd=read(OLDPLAN/"runtime-command-plan.json");cmd["verify"][0]=str(Path(__file__).resolve());cmd["verify"][3]=str(out)
    put(out/"runtime-command-plan.json",cmd)
    paths=[Path(__file__),V1/"ssh_role_native_cli50.py",V1/"ssh_role_API50.py",OLDPLAN/"manifest.json",OLDPLAN/"source-rows.json",
         SOURCE/"manifest.json",Path(read(SOURCE/"manifest.json")["capture"]["path"])]
    paths+=list(p for p in out.rglob("*")if p.is_file())
    put(out/"manifest.json",{"schema":"source50-API-CLI-preview-supplement-v2","inputs":{str(p.resolve()):sha(p)for p in paths},
        "original_plan":str(OLDPLAN),"source_rows_bytes_unchanged":True,"preview_policy_source_before_and_after_equal":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),"preview_policy_SHA256":sha(out/"opaque-preview-policy.json"),
                      "source_rows_SHA256":sha(out/"source-rows.json")}))
def load(plan):
    m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():demand(sha(p)==h,"frozen preview supplement input "+p)
    demand((plan/"source-rows.json").read_bytes()==(OLDPLAN/"source-rows.json").read_bytes(),"original64 field rows changed")
    base=original();original_load=base.load
    policy={p["frame"]:p for p in read(plan/"opaque-preview-policy.json")}
    def project(_):
        old,eof,api,rows,physical,closes=original_load(OLDPLAN);rows=copy.deepcopy(rows)
        for r in rows:
            p=policy.get(r["frame"])
            if p is None:continue
            full=Path(r["raw_path"]).read_bytes();retained=Path(p["derived_preview_path"]).read_bytes()
            demand(dg(full)==p["source_SHA256"]and retained==full[:64]and dg(retained)==p["preview_SHA256"],"independent retained prefix")
            before=copy.deepcopy(r)
            r["raw_path"]=p["derived_preview_path"];r["raw_SHA256"]=p["preview_SHA256"]
            demand(r["raw_length"]==len(full),"pdu.length retains consumed chunk count")
            if not p["unretained_source_bytes"]:demand(r==before,"no adaptation for shorter opaque source")
            else:
                restored=copy.deepcopy(r);restored["raw_path"]=before["raw_path"];restored["raw_SHA256"]=before["raw_SHA256"]
                demand(restored==before,"preview policy cannot alter a semantic/range/alias field")
        return old,eof,api,rows,physical,closes
    base.load=project
    return base

def receipt(path,export,cap,args):
    r=read(path)
    demand(r["exit_code"]==0 and r["export_SHA256"]==sha(export),"closed producer export SHA")
    demand(r["capture_SHA256"]==sha(cap),"source full PCAP hash bound by closed producer")
    demand(r["arguments"][1:]==args,"exact default producer arguments; custom preview budget not permitted")
    matches=[p for p,h in r["inputs"].items()if Path(p).resolve()==cap.resolve()and h==sha(cap)]
    demand(len(matches)==1,"producer must bind the full source capture, not just SDK preview")
    for p,h in r["inputs"].items():demand(sha(p)==h,"closed producer input "+p)
    return{"path":str(path),"SHA256":sha(path),"source":r["source"],"export_SHA256":sha(export),"capture_SHA256":sha(cap)}

def selftest(plan,out):
    out.mkdir(exist_ok=False);base=load(plan);events,CLI=base.synthetic(plan);result=base.verify_all(plan,events,CLI);neg=[]
    def bad(name,change):
        x,y=copy.deepcopy(events),copy.deepcopy(CLI);change(x,y)
        try:base.verify_all(plan,x,y)
        except(AssertionError,KeyError,ValueError,TypeError):neg.append(name)
        else:raise AssertionError("bad preview policy accepted "+name)
    find=lambda x,n:next(e for e in x if e.get("raw")and e["source_bytes"]["PacketRefs"][0]["Number"]==n)
    bad("opaque-preview-must-not-pretend-fullRaw",lambda x,y:find(x,13).update(raw=base64.b64encode(Path(next(r for r in read(OLDPLAN/"source-rows.json")if r["frame"]==13)["raw_path"]).read_bytes()).decode()))
    bad("consumed-length-must-not-be-preview-length",lambda x,y:next(v for v in find(x,13)["field_values"]["values"]if v["name"]=="pdu.length").update(value=64,exact_value="64"))
    bad("Raw-hash-cannot-claim-full192",lambda x,y:find(x,13)["source_bytes"].update(SHA256=next(r for r in read(OLDPLAN/"source-rows.json")if r["frame"]==13)["raw_SHA256"]))
    bad("CompletedIn-is-not-authenticated-completion",lambda x,y:find(x,13)["source_bytes"].update(CompletedIn={"Number":13,"Domain":base.ZERO}))
    bad("retained64-prefix-mutation",lambda x,y:find(x,13).update(raw=base64.b64encode(bytes(64)).decode()))
    bad("role-malformed-full-frame-must-not-be-clipped",lambda x,y:find(x,89).update(raw=base64.b64encode(base64.b64decode(find(x,89)["raw"])[:64]).decode()))
    bad("opaque-preview-cannot-publish-hostkey",lambda x,y:find(x,13).update(session={"Host Key Fingerprint":"forged"}))
    bad("other-source-fields-remain-exact",lambda x,y:find(x,61)["fields"]["Payload"].update({"Server Exchange Value":"Ag=="}))
    result.update(SDK_opaque_retained_bytes=112,source_opaque_consumed_bytes=240,unretained_source_tail_bytes=128,
                  SDK_full_tail_coverage_claimed=False)
    put(out/"summary.json",{"passed":True,"plan_SHA256":sha(plan/"manifest.json"),"checker_SHA256":sha(__file__),
        "counts":result,"negative_controls":neg,"candidate_payload_values_read":False,"original_expected_field_rows_unchanged":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"negative_controls":len(neg),"counts":result}))

def verify(plan,events,CLI,descriptors,out,events_producer=None,CLI_producer=None):
    out.mkdir(exist_ok=False);base=load(plan);inputs={str(p.resolve()):sha(p)for p in(events,CLI,descriptors,plan/"manifest.json",Path(__file__))}
    failure=[];counts={};bindings=[]
    try:
        cap=Path(read(SOURCE/"manifest.json")["capture"]["path"]);cmd=read(OLDPLAN/"runtime-command-plan.json")
        ep=events_producer or events.parent/"events-producer.json";cp=CLI_producer or CLI.parent/"yak-producer.json"
        bindings=[receipt(ep,events,cap,cmd["events"]),receipt(cp,CLI,cap,cmd["CLI"])]
        for p in(ep,cp):inputs[str(p.resolve())]=sha(p)
        old,eof,api,rows,physical,closes=base.load(plan)
        count=old.descriptor_check(read(descriptors),api)
        counts=base.verify_all(plan,base.lines(events),base.lines(CLI));counts["SSH_descriptors"]=count
        counts.update(SDK_opaque_retained_bytes=112,source_opaque_consumed_bytes=240,unretained_source_tail_bytes=128,SDK_full_tail_coverage_claimed=False)
    except Exception as e:failure.append({"type":type(e).__name__,"message":str(e)})
    demand(inputs=={p:sha(p)for p in inputs},"closed inputs changed during audit")
    put(out/"differences.json",failure);put(out/"summary.json",{"passed":not failure,"inputs":inputs,"counts":counts,
      "capture_producer_bindings":bindings,"scope":read(plan/"scope.json"),"original64_fields_unchanged":True,
      "differences_SHA256":sha(out/"differences.json"),"expected_not_derived_from_actual_payload":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"passed":not failure}))
    if failure:raise SystemExit(1)
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
    else:verify(a.plan,a.events,a.CLI,a.descriptors,a.out,a.events_producer,a.CLI_producer)
if __name__=="__main__":main()

