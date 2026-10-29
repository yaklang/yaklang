#!/usr/bin/env python3
"""Supplement frozen76 SSH payloads with11 source-derived FIN close diagnostics."""
import argparse
import collections
import copy
import hashlib
import importlib.util
import json
from pathlib import Path

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
OLDROOT=PHASE/"SSH-SDK-CLI-source49-01"
OLDPLAN=OLDROOT/"plan01"
ORIGINAL=PHASE/"SSH-source49-audit01"/"plan01"
ZERO={"Section":0,"Interface":0,"Encapsulation":""}
PUBSCOPE="available_during_processing; byte_contribution_is_defined_by_byte_source"
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2);f.write("\n")
def module(path):
    spec=importlib.util.spec_from_file_location("frozen_ssh_payload_checker49",path)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def strict(a,b):return json.dumps(a,sort_keys=True,separators=(",",":"))==json.dumps(b,sort_keys=True,separators=(",",":"))
def demand(ok,why):
    if not ok:raise AssertionError(why)
def lines(path):return[json.loads(s)for s in Path(path).read_text(encoding="utf-8-sig").splitlines()if s.strip()]
def old_module():return module(OLDROOT/"ssh_native_cli49.py")

def build_close_rows():
    cases=read(ORIGINAL/"source-facts.json")["cases"]
    result=[]
    for c in cases:
        nk={s["role"]for s in c["steps"]if s.get("SSH_message")==21}
        fins=[s for s in c["steps"]if s["TCP Flags"]&1]
        assert len(fins)==2 and {s["role"]for s in fins}=={"client","server"}
        if nk=={"client","server"}:continue
        banners={s["role"]:Path(s["input_path"]).read_bytes().rstrip(b"\r\n").decode("ascii")
                 for s in c["steps"]if s["kind"]=="identification"}
        trigger=max(s["frame"]for s in fins)
        result.append({"sourcecase":c["sourcecase"],"trigger_frame":trigger,"close_reason":"fin",
          "source":c["tuple"]["client"],"destination":c["tuple"]["server"],"direction":0,
          "session":{"Identification":[banners["client"],banners["server"]]},
          "newkeys_roles_observed":sorted(nk),"FIN_source_frames":sorted(s["frame"]for s in fins),
          "expected_summary":"fin: SSH handshake ended before NEWKEYS",
          "source_identity":"This is a connection close observation, not a captured PDU; trigger frame is not a byte contributor."})
    assert len(result)==11 and all(r["sourcecase"]!="07-standard-curve-markers-and-reuse"for r in result)
    return result

def empty_source():
    return {"Kind":"reassembled","SHA256":"","ParentPDU":0,"ParentPDUs":None,"PacketRefs":None}

def close_snapshot(row,ident):
    ip_src=row["source"].rsplit(":",1)[0];ip_dst=row["destination"].rsplit(":",1)[0]
    facts={"destination":row["destination"],"flow.id":ident["flow"],"ip.addr":[ip_src,ip_dst],"pdu.id":ident["id"],
           "pdu.length":0,"protocol":"ssh","source":row["source"],"status":"incomplete","tcp.flow.id":ident["tcp_flow_id"]}
    values=[]
    for name in sorted(facts):
        v=facts[name]
        for x in v if isinstance(v,list)else[v]:
            item={"name":name,"value":x,"origin":"event"}
            if name in("flow.id","pdu.id","pdu.length","tcp.flow.id"):item["exact_value"]=str(x)
            values.append(item)
    # The existing string descriptor cannot normalize a [2]string Identification.
    # Omitted private alias is a projection limitation, not proven absence.
    return {"schema_version":1,"pdu":ident["id"],"source":empty_source(),"values":values,"incomplete":True,"completeness":"incomplete"}

def close_event(row,ident):
    return {"admission":"","carrier_layers":None,"completeness":"incomplete","destination":row["destination"],
       "direction":0,"domain":ZERO,"entry":"","expert":"","field_values":close_snapshot(row,ident),
       "fields":None,"fields_error":"protocol parser: event is incomplete, not an exact message","flow":ident["flow"],"id":ident["id"],
       "profile":"ssh","protocol":"ssh","raw":None,"response_to":0,"session":row["session"],"source":row["source"],
       "source_bytes":empty_source(),"status":"incomplete","summary":row["expected_summary"],"tcp_flow_id":ident["tcp_flow_id"],
       "transaction":0,"transport":"tcp"}

def prepare(out):
    out.mkdir(exist_ok=False)
    old=old_module();m,source_rows,physical,api=old.load(OLDPLAN)
    assert sha(OLDPLAN/"manifest.json")=="6152f55c293920a466756dcc090e1b4d0627f8ca7379d33e21cb175c37c2d85d"
    assert sha(OLDPLAN/"source-rows.json")=="d9421805893b738fa47d95b6baa177cc907df37475e2dae30c90dd45547dcf73"
    for name in("source-rows.json","physical-rows.json","private-alias-schema.json","windows-unknown-reply-source-plan.json"):
        (out/name).write_bytes((OLDPLAN/name).read_bytes())
    close_rows=build_close_rows();put(out/"close-source-rows.json",close_rows)
    put(out/"payload-unchanged-bridge.json",{"old_plan":str(OLDPLAN),"old_manifest_SHA256":sha(OLDPLAN/"manifest.json"),
        "payload_rows":76,"payload_value_or_span_changes":0,
        "old_rows_SHA256":sha(OLDPLAN/"source-rows.json"),"new_rows_SHA256":sha(out/"source-rows.json"),
        "literal_bytes_equal":(OLDPLAN/"source-rows.json").read_bytes()==(out/"source-rows.json").read_bytes(),
        "original_payload_checker_used_unmodified":str(OLDROOT/"ssh_native_cli49.py"),
        "original_payload_checker_SHA256":sha(OLDROOT/"ssh_native_cli49.py")})
    repo=Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
    paths=["common/pcapx/pcaputil/bin_parser_session.go","common/pcapx/pcaputil/protocol_field_registry.go",
           "common/pcapx/pcaputil/protocol_evidence.go","common/yak/cmd/yakcmds/shark-cli/capture.go",
           "common/yak/cmd/yakcmds/shark-cli/packet_observation.go"]
    put(out/"supplement-schema-binding.json",[{"path":str(repo/p),"SHA256":sha(repo/p),
       "role":"preexisting finishSession/projection/publication schema; packet facts remain frozen source49"}for p in paths])
    put(out/"scope.json",{
        "correction":"plan01 omitted source-known finishSession diagnostics; this is a checker/schema cardinality error, not an SSH algorithm failure",
        "native_events":87,"physical_rows":148,"payload_events":76,"payload_statuses":{"decoded":74,"context-required":2},
        "nonphysical_close_diagnostics":11,"standalone_nonphysical_CLI_records":0,"published_PDU_entries_on_FIN_rows":11,
        "native_payload_fields_unchanged":True,"source_TCP_generations":12,"only_both_NEWKEYS_case":7,
        "close_source":"11 authored TCP generations have complete banners/KEXINIT but no two-direction NEWKEYS; second actual FIN closes the source generation",
        "close_ownership":"emptyRaw has no PacketRefs, no CompletedIn, no rawSHA and no capturedPDU; use same client-role endpoints/flow/TCP generation",
        "projection":"Identification[2]string stays in Session; existing ssh.protocol string alias cannot project it, so Incomplete=true is required and no absence proof is claimed",
        "CLI_projection":"exact published_pdus compact snapshot independently verified; only after this verification is the frozen original payload checker applied to a copy without those verified supplemental entries",
        "not_claimed":"no new live traffic, authenticated SSH, all algorithm/state/crypto semantics or every control-frame annotation",
        "actual_read_before_freeze":"only the11 emptyRaw diagnostic DTO/CLI publication schema; no76-payload SDK values were read or used",
        "old_failure_directories":["SSH-before-c04a49-01/independent-failure01","SSH-native-cc9d49-01/independent-audit"],
    })
    cmd=read(OLDPLAN/"runtime-command-plan.json")
    cmd["verify_argv"]=[str(Path(__file__).resolve()),"verify","--plan",str(out),"--events","<closed>","--CLI","<closed>","--descriptors","<closed>","--out","<fresh>"]
    put(out/"runtime-command-plan.json",cmd)
    inputs=[Path(__file__),OLDROOT/"ssh_native_cli49.py",OLDROOT/"ssh_source_API49.py",OLDPLAN/"manifest.json",
            OLDPLAN/"source-rows.json",OLDPLAN/"physical-rows.json",ORIGINAL/"source-facts.json"]
    inputs+=list(p for p in out.iterdir()if p.is_file())
    put(out/"manifest.json",{"schema":"SSH-source49-SDK-CLI-plan-v2-with-explicit-close",
         "inputs":{str(p.resolve()):sha(p)for p in inputs},"original_plan":str(OLDPLAN),"original_plan_SHA256":sha(OLDPLAN/"manifest.json"),
         "payload_rows_SHA256":sha(out/"source-rows.json"),"close_rows_SHA256":sha(out/"close-source-rows.json"),
         "checker":str(Path(__file__).resolve()),"source_payload_unchanged":True,
         "no_payload_candidate_values_read_for_expectation":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),
                     "close_source_rows_SHA256":sha(out/"close-source-rows.json"),"unchanged_payload_SHA256":sha(out/"source-rows.json")}))

def load(plan):
    m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():demand(sha(p)==h,"frozen supplementary input "+p)
    old=old_module();old.load(OLDPLAN)
    demand((plan/"source-rows.json").read_bytes()==(OLDPLAN/"source-rows.json").read_bytes(),"no payload rewrite allowed")
    return old,read(plan/"source-rows.json"),read(plan/"physical-rows.json"),read(plan/"close-source-rows.json")

def verify_all(plan,events,CLI):
    old,rows,physical,closes=load(plan)
    demand(len(events)==87,"exact87 callback cardinality:76 frozen payload+11 strictly source-bound close diagnostics")
    payloads=[e for e in events if e.get("raw")not in(None,"")]
    diagnostics=[e for e in events if e.get("raw")in(None,"")]
    demand(len(payloads)==76 and len(diagnostics)==11,"payload/emptyRaw partition must exhaust complete SDK export")
    source_by_frame={r["frame"]:r for r in rows};case_identity={};used=set();expected_order=[]
    for e in payloads:
        refs=e["source_bytes"]["PacketRefs"];demand(len(refs)==1 and refs[0]["Number"]in source_by_frame,"payload original frame")
        row=source_by_frame[refs[0]["Number"]];ident=old.verify_event(e,row)
        demand(ident["id"]not in used,"duplicate payload id");used.add(ident["id"])
        pair=(ident["flow"],ident["tcp_flow_id"]);case=row["sourcecase"]
        if case in case_identity:demand(case_identity[case]==pair,"payload generation consistency")
        else:case_identity[case]=pair
        expected_order.append((row["frame"],"payload",e["id"]))
    demand(len(case_identity)==12 and len({p[0]for p in case_identity.values()})==12 and len({p[1]for p in case_identity.values()})==12,
           "source SYN generations cannot merge")
    close_by_flow={case_identity[r["sourcecase"]][0]:r for r in closes}
    close_events={};seenclose=set()
    for e in diagnostics:
        demand(e.get("flow")in close_by_flow,"no unidentified or completed-generation emptyRaw callback")
        row=close_by_flow[e["flow"]];case=row["sourcecase"]
        demand(case not in seenclose,"one source close callback per incomplete generation")
        seenclose.add(case);demand(type(e["id"])is int and e["id"]>0 and e["id"]not in used,"new close id");used.add(e["id"])
        f,t=case_identity[case];expected=close_event(row,{"id":e["id"],"flow":f,"tcp_flow_id":t})
        demand(strict(e,expected),"whole independently source-built close DTO "+case)
        close_events[row["trigger_frame"]]=e
        expected_order.append((row["trigger_frame"],"close",e["id"]))
    demand(len(close_events)==11,"all source EOF/FIN contracts")
    demand([x[2]for x in sorted(expected_order)]==[e["id"]for e in events],"SDK publication order matches original payload/FIN chronology")
    physicalCLI=[c for c in CLI if "number"in c];other=[c for c in CLI if "number"not in c]
    demand(len(physicalCLI)==148 and not other,"no extra physical or standalone nonphysical records in FIN-complete capture")
    projected=copy.deepcopy(physicalCLI);published=0
    for c in projected:
        frame=c["number"];entries=c.get("published_pdus")or[]
        if frame in close_events:
            e=close_events[frame];expected=old.compact_expected(e)
            demand(strict(entries,[expected]),"entire close SDK->CLI compact snapshot on correct source FIN trigger")
            demand(c.get("published_pdu_scope")==PUBSCOPE,"publication scope never claims byte ownership")
            published+=1
            del c["published_pdus"]
            # This field is itself validated, and belongs only to the validated
            # supplement. Removing it here cannot hide a payload/source field.
            del c["published_pdu_scope"]
        else:
            demand(not entries and "published_pdu_scope"not in c,"unexpected nonphysical publication on another frame")
    demand(published==11,"all explicit supplemental CLI bridges")
    payload_counts=old.verify_rows(rows,physical,payloads,projected)
    return {"SDK_total_events":87,"physical_CLI_rows":148,"frozen_payload_events":76,"close_diagnostic_events":11,
        "standalone_nonphysical_CLI_records":0,"close_published_PDU_entries":11,"whole_close_DTOs":11,
        "close_snapshot_event_occurrences":110,"private_SSH_close_occurrences":0,"payload_counts":payload_counts,
        "old_payload_rows_bytes_unchanged":True}

def selftest(plan,out):
    old,rows,physical,closes=load(plan);out.mkdir(exist_ok=False)
    _,_,_,api=old.load(OLDPLAN);payloads,CLI=old.synthetic(rows,physical,api)
    cases={r["sourcecase"]:(e["flow"],e["tcp_flow_id"])for r,e in zip(rows,payloads)}
    extended=[(r["frame"],e)for r,e in zip(rows,payloads)]
    for i,row in enumerate(closes,9001):
        flow,tcp=cases[row["sourcecase"]]
        e=close_event(row,{"id":i,"flow":flow,"tcp_flow_id":tcp});extended.append((row["trigger_frame"],e))
        c=CLI[row["trigger_frame"]-1];c["published_pdus"]=[old.compact_expected(e)];c["published_pdu_scope"]=PUBSCOPE
    events=[e for _,e in sorted(extended)]
    counts=verify_all(plan,events,CLI);neg=[]
    def reject(label,change):
        x,y=copy.deepcopy(events),copy.deepcopy(CLI);change(x,y)
        try:verify_all(plan,x,y)
        except(AssertionError,KeyError,ValueError,TypeError):neg.append(label)
        else:raise AssertionError("bad source supplement accepted "+label)
    eof=lambda x:next(e for e in x if e.get("raw")is None)
    reject("close-invented-contributor",lambda x,y:eof(x)["source_bytes"].update({"PacketRefs":[{"Number":11,"Domain":ZERO}]}))
    reject("close-invented-CompletedIn",lambda x,y:eof(x)["source_bytes"].update({"CompletedIn":{"Number":11,"Domain":ZERO}}))
    reject("close-pretends-captured-raw",lambda x,y:eof(x).update({"raw":"AA=="}))
    reject("close-inherited-algorithm-context",lambda x,y:eof(x)["session"].update({"Kex Algorithm":"private@example.test"}))
    reject("close-false-field-coverage",lambda x,y:eof(x)["field_values"].pop("incomplete"))
    reject("close-private-identification-string-forgery",lambda x,y:eof(x)["field_values"]["values"].append({"name":"ssh.protocol","value":"invented","origin":"context"}))
    reject("close-attached-as-physical-PDU",lambda x,y:y[10].update({"pdus":y[10].pop("published_pdus")}))
    reject("close-ownership-scope-wrong",lambda x,y:y[10].update({"published_pdu_scope":"captured bytes"}))
    reject("drop-incomplete-generation-diagnostic",lambda x,y:x.pop(next(i for i,e in enumerate(x)if e.get("raw")is None)))
    reject("duplicate-close-generation",lambda x,y:x.append(copy.deepcopy(eof(x))))
    # The old payload predicates remain active inside the87-event whole checker.
    find=lambda x,n:next(e for e in x if e.get("raw")and e["source_bytes"]["PacketRefs"][0]["Number"]==n)
    reject("payload-prefix-name-still-rejected",lambda x,y:find(x,8)["session"].update({"Kex Algorithm":"curve25519-sha256"}))
    reject("payload-false-hostkey-still-rejected",lambda x,y:find(x,69)["session"].update({"Host Key Fingerprint":"SHA256:invented"}))
    put(out/"summary.json",{"result":"PASS","plan_SHA256":sha(plan/"manifest.json"),"checker_SHA256":sha(__file__),
       "counts":counts,"mutation_controls":neg,"source76_semantics_unchanged":True,"no_actual_payload_read_during_selftest":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"negative_controls":len(neg),"counts":counts}))

def verify(plan,events,CLI,descriptors,out):
    old,rows,physical,closes=load(plan);out.mkdir(exist_ok=False);inputs={str(p.resolve()):sha(p)for p in(events,CLI,descriptors,plan/"manifest.json",Path(__file__))}
    failure=[];counts={}
    try:
        _,_,_,api=old.load(OLDPLAN);n=old.descriptor_check(read(descriptors),api)
        counts=verify_all(plan,lines(events),lines(CLI));counts["SSH_descriptor_count"]=n
    except Exception as e:failure.append({"type":type(e).__name__,"message":str(e)})
    demand(inputs=={p:sha(p)for p in inputs},"closed exports mutated during offline audit")
    put(out/"differences.json",failure);put(out/"summary.json",{"passed":not failure,"inputs":inputs,"counts":counts,"scope":read(plan/"scope.json"),
       "payload_unchanged_bridge_SHA256":sha(plan/"payload-unchanged-bridge.json"),"differences_SHA256":sha(out/"differences.json")})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"passed":not failure,"differences":len(failure)}))
    if failure:raise SystemExit(1)

def main():
    p=argparse.ArgumentParser();sub=p.add_subparsers(dest="cmd",required=True)
    q=sub.add_parser("prepare");q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("selftest");q.add_argument("--plan",type=Path,required=True);q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("verify")
    for n in("plan","events","CLI","descriptors","out"):q.add_argument("--"+n,type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.out)
    elif a.cmd=="selftest":selftest(a.plan,a.out)
    else:verify(a.plan,a.events,a.CLI,a.descriptors,a.out)
if __name__=="__main__":main()

