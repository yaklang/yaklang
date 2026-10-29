#!/usr/bin/env python3
"""Prepare/selftest/offline verify source50; no Go/SDK/TShark execution."""
import argparse,base64,collections,copy,hashlib,importlib.util,json
from pathlib import Path
HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
SOURCE=PHASE/"SSH-role-source50-01"/"plan02"
ZERO={"Section":0,"Interface":0,"Encapsulation":""}
OLDROOT=PHASE/"SSH-SDK-CLI-source49-01"
EOFROOT=PHASE/"SSH-SDK-CLI-source49-02"
PUBSCOPE="available_during_processing; byte_contribution_is_defined_by_byte_source"
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2,ensure_ascii=False);f.write("\n")
def mod(path,name):
    s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def deps():
    old=mod(OLDROOT/"ssh_native_cli49.py","source_payload_wrapper49")
    eof=mod(EOFROOT/"ssh_native_cli49_v02.py","source_close_wrapper49")
    api=mod(HERE/"ssh_role_API50.py","independent_role_API50")
    return old,eof,api
def strict(a,b):return json.dumps(a,sort_keys=True,separators=(",",":"))==json.dumps(b,sort_keys=True,separators=(",",":"))
def demand(x,why):
    if not x:raise AssertionError(why)
def lines(p):return[json.loads(s)for s in Path(p).read_text(encoding="utf-8-sig").splitlines()if s.strip()]

def prepare(out):
    out.mkdir(exist_ok=False);src=read(SOURCE/"manifest.json")
    assert sha(SOURCE/"manifest.json")=="ae73e1329488cc159cb0958f483c529e7dd50e270b53249dae8521324a2553af"
    for p,h in src["inputs"].items():assert sha(p)==h
    old,eof,api=deps();cap=Path(src["capture"]["path"]);physical=old.pcap_rows(cap)
    rows=[];closes=[];known={}
    cases=read(SOURCE/"source-facts.json")["cases"]
    for generation,c in enumerate(cases,1):
        values,close=api.source_case(c,physical)
        for r in values:r["CLI_generation"]=generation
        rows+=values
        if close:closes.append(close)
        for s in c["steps"]:
            a=physical[s["frame"]-1];a.update(sourcecase=c["sourcecase"],CLI_generation=generation)
            assert a["physical_SHA256"]==s["physical_SHA256"]and a["TCP_payload_SHA256"]==s["payload_SHA256"]
    assert len(physical)==115 and len(rows)==64 and len(closes)==7
    statuses=dict(collections.Counter(r["expected_status"]for r in rows))
    assert statuses=={"decoded":58,"context-required":2,"malformed":4}
    put(out/"source-rows.json",rows);put(out/"physical-rows.json",physical);put(out/"close-source-rows.json",closes)
    put(out/"alias-schema.json",[{"name":n,"API_key":k,"origin":o,"type":t,"bits":b}for n,k,o,t,b in api.SPECS])
    repo=Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
    names=["common/pcapx/pcaputil/bin_parser_ssh.go","common/pcapx/pcaputil/bin_parser_session.go",
      "common/pcapx/pcaputil/protocol_field_registry.go","common/pcapx/pcaputil/protocol_evidence.go","common/pcapx/pcaputil/bin_parser.go",
      "common/pcapx/pcaputil/trafficpool.go","common/pcapx/pcaputil/reassembly.go",
      "common/yak/cmd/yakcmds/shark-cli/presentation.go","common/yak/cmd/yakcmds/shark-cli/protocol.go",
      "common/yak/cmd/yakcmds/shark-cli/conversation.go","common/yak/cmd/yakcmds/shark-cli/decode.go",
      "common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events/main.go"]
    put(out/"schema-binding.json",{"source_commit_for_pure_names":"26f01f4baa5fd39b88deadf901257a70bc5c1dfe",
       "files":[{"path":str(repo/p),"SHA256":sha(repo/p),"scope":"names/type/layout and legacy publication contract only; values from frozen wire"}for p in names]})
    put(out/"runtime-command-plan.json",{"events":["replay",str(cap),"-","--ignore-embedded-secrets"],
       "CLI":["shark","-r",str(cap),"--json","--ignore-embedded-secrets"],"descriptors":["shark","--list-fields","--json"],
       "execution":"parent only after all producer/build source guards closed; this checker never starts a producer",
       "verify":[str(Path(__file__).resolve()),"verify","--plan",str(out),"--events","<closed>","--CLI","<closed>","--descriptors","<closed>","--out","<new>"]})
    put(out/"scope.json",{"authored":True,"real_capture_count":0,"physical_frames":115,"payload_callbacks":64,
       "payload_statuses":statuses,"nonphysical_FIN_diagnostics":7,"total_SDK_callbacks":71,
       "standalone_nonphysical_CLI_records":0,"published_PDU_entries_on_second_FIN":7,"descriptor_count":12,
       "payload_fields":"every Session/GetFields tree and private typed snapshot; malformed sender role has no successful fields/hostkey",
       "published_wire_spans":"independent source ledger only; existing SSH API/private aliases have no invented WR or pdu_range",
       "scope_boundary":"15 valid method-role messages plus4 positive contradictions,1 opaque private method,2 noSYN role-unknown and2 synthetic protected payloads; transport messages counted separately",
       "unknown_role":"flow orientation slot0 is first captured endpointB in case09, never proof of client; no TCP SYN means no sender role assertion",
       "CLI_diagnostic":"existing applyApplication keeps SSH+malformed+connection-associated diagnostic+SSH stack/suffix; visibility empty omitempty is deliberately absent",
       "source_close":"FIN container is analysis publication only; emptyRaw has no PacketRefs/SHA/CompletedIn and does not make FIN its PDU source",
       "not_claimed":["all SSH order/negotiation conformance","cryptographic authentication","live network capture","all control-frame conversation annotations"],
       "no_candidate_SDK_value_read":True,"original_failed_source50_prepare01_preserved":True})
    inputs=[Path(__file__),HERE/"ssh_role_API50.py",OLDROOT/"ssh_source_API49.py",OLDROOT/"ssh_native_cli49.py",
       EOFROOT/"ssh_native_cli49_v02.py",SOURCE/"manifest.json",SOURCE/"source-facts.json",SOURCE/"method-role-contract.json",cap]
    inputs+=list(Path(r["raw_path"])for r in rows)
    inputs+=list(p for p in out.iterdir()if p.is_file())
    put(out/"manifest.json",{"schema":"source50-independent-API-CLI-v1","inputs":{str(p.resolve()):sha(p)for p in inputs},
        "source50_manifest_SHA256":sha(SOURCE/"manifest.json"),"source64_rows_SHA256":sha(out/"source-rows.json"),
        "physical_rows":115,"payload_rows":64,"close_rows":7,"candidate_values_used":False})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),
          "counts":counts(rows),"close_rows":len(closes)}))
def load(plan):
    m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():demand(sha(p)==h,"frozen source/API input changed "+p)
    return *deps(),read(plan/"source-rows.json"),read(plan/"physical-rows.json"),read(plan/"close-source-rows.json")
def counts(rows):
    return{"payload_rows":len(rows),"status_counts":dict(collections.Counter(r["expected_status"]for r in rows)),
           "API_literal_occurrences":sum(len(r["API_wire_leaves"])for r in rows),
           "private_SSH_occurrences":sum(len(r["private_aliases"])for r in rows),
           "role_diagnostics":sum(r["expected_status"]=="malformed"for r in rows)}

def verify_payload(old,e,r):
    raw=Path(r["raw_path"]).read_bytes()
    demand(base64.b64decode(e["raw"],validate=True)==raw,"whole source PDU bytes")
    for k,v in{"protocol":"ssh","status":r["expected_status"],"profile":"ssh","completeness":r["expected_completeness"],
      "entry":r["expected_entry"],"transport":"tcp","source":r["source"],"destination":r["destination"],"direction":r["expected_direction"],
      "domain":ZERO,"transaction":0,"response_to":0}.items():demand(strict(e[k],v),"SDK "+k+" frame"+str(r["frame"]))
    demand(strict(e["source_bytes"],old.sourcebytes(r)),"complete immutable raw source/ref/domain")
    demand(strict(e.get("session"),r["session"]),"entire source-derived Session frame"+str(r["frame"]))
    demand(strict(e.get("fields"),r["fields"]),"entire source-derived GetFields frame"+str(r["frame"]))
    demand(bool(e.get("fields_error"))is r["fields_error_required"],"GetFields error policy")
    if r["fields_error_exact"]is not None:demand(e.get("fields_error")==r["fields_error_exact"],"private role refusal not a generic retry")
    demand(e.get("expert","")==r["expert"],"typed diagnostic expert")
    if r["expected_error"]is not None:demand(e.get("error")==r["expected_error"],"exact role error cause")
    elif r["expected_status"]=="decoded":demand(not e.get("error"),"unexpected source decode failure")
    else:demand("NEWKEYS"in str(e.get("error",""))+" "+e.get("summary",""),"observed protection context")
    if r["expected_summary"]is not None:demand(e.get("summary")==r["expected_summary"],"SDK schema summary")
    for leaf in r["API_wire_leaves"]:demand(strict(old.at_path(e["fields"],leaf["API_path"]),leaf["value"]),"independent primitive")
    ident={k:e[k]for k in("id","flow","tcp_flow_id")};demand(all(type(v)is int and v>0 for v in ident.values()),"real generated identities")
    demand(strict(e["field_values"],old.expected_snapshot(r,ident)),"whole private typed occurrence snapshot")
    return ident

def verify_all(plan,events,CLI):
    old,eof,api,rows,physical,closes=load(plan)
    demand(len(events)==71,"exact64 payload+7 source FIN close callbacks")
    payload=[e for e in events if e.get("raw")not in(None,"")];closed=[e for e in events if e.get("raw")in(None,"")]
    demand(len(payload)==64 and len(closed)==7,"exhaustive callback partition")
    byframe={r["frame"]:r for r in rows};observed={};generations={};ids=set();chronology=[]
    for e in payload:
        refs=e["source_bytes"]["PacketRefs"];demand(len(refs)==1 and refs[0]["Number"]in byframe,"source original byte contributor")
        frame=refs[0]["Number"];demand(frame not in observed,"one source payload per physical record")
        r=byframe[frame];ident=verify_payload(old,e,r)
        demand(ident["id"]not in ids,"PDU identity not reused");ids.add(ident["id"]);observed[frame]=e
        pair=(ident["flow"],ident["tcp_flow_id"])
        if r["sourcecase"]in generations:demand(generations[r["sourcecase"]]==pair,"transport generation")
        else:generations[r["sourcecase"]]=pair
        chronology.append((frame,e["id"]))
    demand(len(generations)==9 and len({v[0]for v in generations.values()})==9 and len({v[1]for v in generations.values()})==9,"all9 source generations isolated")
    close_by_flow={generations[r["sourcecase"]][0]:r for r in closes};close_map={}
    for e in closed:
        demand(e["flow"]in close_by_flow,"no diagnostic can borrow unrelated or completed generation")
        r=close_by_flow[e["flow"]];trigger=r["trigger_frame"];demand(trigger not in close_map,"duplicate close callback")
        f,t=generations[r["sourcecase"]]
        demand(e["id"]>0 and e["id"]not in ids,"new close PDU identity");ids.add(e["id"])
        demand(strict(e,eof.close_event(r,{"id":e["id"],"flow":f,"tcp_flow_id":t})),"whole emptyRaw close DTO")
        close_map[trigger]=e;chronology.append((trigger,e["id"]))
    demand([id for _,id in sorted(chronology)]==[e["id"]for e in events],"source publication order")
    physicalCLI=[c for c in CLI if"number"in c];publication=[c for c in CLI if"number"not in c]
    demand(len(physicalCLI)==115 and not publication,"physical count cannot include supplemental diagnostics")
    demand([c["number"]for c in physicalCLI]==list(range(1,116)),"original physical identities")
    pub=0
    for c,a in zip(physicalCLI,physical):
        frame=a["number"]
        for k in("number","length","captured_length","source","destination","source_port","destination_port","layers","capture_domain"):
            demand(strict(c.get(k),a[k]),"CLI physical source "+k+" frame"+str(frame))
        demand(c.get("transport")=="tcp"and old.ns(c["time"])==a["time_ns"],"captured transport/time")
        demand(c.get("tcp_flow_id")==generations[a["sourcecase"]][1]and strict(c.get("tcp_flow_domain"),ZERO),"captured generation/domain")
        r=byframe.get(frame);pdus=c.get("pdus")or[]
        demand(len(pdus)==(1 if r else 0),"no cross-frame byte association")
        if r:
            demand(strict(pdus,[old.compact_expected(observed[frame])]),"full independently checked SDK->CLI PDU/snapshot bridge")
            generation=a["CLI_generation"]
            demand(c.get("stream_id")==generation,"source-ordered connection namespace")
            for k,v in{"protocol":"SSH","protocol_status":r["CLI_status"],"protocol_scope":r["CLI_scope"],
                "protocol_stack":["Ethernet","IPv4","TCP","SSH"],"connection_protocol":"SSH","connection_stack":["SSH"],
                "connection_evidence":"validated protocol events"}.items():demand(strict(c.get(k),v),"CLI semantic "+k+" frame"+str(frame))
            if r["CLI_visibility"]is None:demand("visibility"not in c,"existing diagnostic visibility is omitted, not invented")
            else:demand(c.get("visibility")==r["CLI_visibility"],"CLI source visibility")
            suffix=" [connection #"+str(generation)+": SSH]"
            demand(c.get("info")==r["CLI_info_base"]+suffix,"exact source caption/diagnostic plus checked source connection suffix frame"+str(frame))
        pubs=c.get("published_pdus")or[]
        if frame in close_map:
            demand(strict(pubs,[old.compact_expected(close_map[frame])]),"source FIN publication bridge")
            demand(c.get("published_pdu_scope")==PUBSCOPE,"diagnostic trigger is not raw contributor");pub+=1
        else:demand(not pubs and"published_pdu_scope"not in c,"unexpected late publication")
    demand(pub==7,"all source close publications verified")
    return{"physical_frames":115,"SDK_callbacks":71,"payload_callbacks":64,"close_DTOs":7,"published_close_entries":7,
           "TCP_generations":9,"payload_SDK_CLI_bridges":64,"close_SDK_CLI_bridges":7,**counts(rows)}

def synthetic(plan):
    old,eof,api,rows,physical,closes=load(plan)
    cases=list(dict.fromkeys(r["sourcecase"]for r in rows));ids={c:(i+30,i+130)for i,c in enumerate(cases)};payload=[];e_by={}
    for i,r in enumerate(rows,2001):
        f,t=ids[r["sourcecase"]]
        e={"id":i,"flow":f,"tcp_flow_id":t,"protocol":"ssh","status":r["expected_status"],"profile":"ssh","completeness":r["expected_completeness"],
           "summary":r["expected_summary"]or"Encrypted: SSH transport is encrypted after NEWKEYS","expert":r["expert"],
           "source_bytes":old.sourcebytes(r),"fields":r["fields"],"session":r["session"],
           "field_values":old.expected_snapshot(r,{"id":i,"flow":f,"tcp_flow_id":t}),"raw":base64.b64encode(Path(r["raw_path"]).read_bytes()).decode(),
           "entry":r["expected_entry"],"admission":"","source":r["source"],"destination":r["destination"],"direction":r["expected_direction"],
           "transport":"tcp","domain":ZERO,"carrier_layers":None,"transaction":0,"response_to":0}
        if r["fields_error_required"]:e["fields_error"]=r["fields_error_exact"]or"protocol parser: event is context-required, not an exact message"
        if r["expected_error"]is not None:e["error"]=r["expected_error"]
        elif r["expected_status"]=="context-required":e["error"]="Encrypted: SSH transport is encrypted after NEWKEYS"
        payload.append((r["frame"],e));e_by[r["frame"]]=e
    close_by={}
    for i,r in enumerate(closes,9001):
        f,t=ids[r["sourcecase"]];e=eof.close_event(r,{"id":i,"flow":f,"tcp_flow_id":t});payload.append((r["trigger_frame"],e));close_by[r["trigger_frame"]]=e
    out=[]
    import datetime
    r_by={r["frame"]:r for r in rows}
    for a in physical:
        c={k:a[k]for k in("number","length","captured_length","source","destination","source_port","destination_port","layers","capture_domain")}
        dt=datetime.datetime.fromtimestamp(a["time_ns"]//10**9,datetime.timezone.utc)
        c.update(time=dt.strftime("%Y-%m-%dT%H:%M:%S")+f".{a['time_ns']%10**9:09d}Z",transport="tcp",
                 tcp_flow_id=ids[a["sourcecase"]][1],tcp_flow_domain=ZERO,protocol="TCP",info="synthetic control")
        r=r_by.get(a["number"])
        if r:
            g=a["CLI_generation"]
            c.update(protocol="SSH",protocol_status=r["CLI_status"],protocol_scope=r["CLI_scope"],protocol_stack=["Ethernet","IPv4","TCP","SSH"],
                     connection_protocol="SSH",connection_stack=["SSH"],connection_evidence="validated protocol events",stream_id=g,
                     info=r["CLI_info_base"]+" [connection #"+str(g)+": SSH]",pdus=[old.compact_expected(e_by[a["number"]])])
            if r["CLI_visibility"]is not None:c["visibility"]=r["CLI_visibility"]
        if a["number"]in close_by:c.update(published_pdus=[old.compact_expected(close_by[a["number"]])],published_pdu_scope=PUBSCOPE)
        out.append(c)
    return[e for _,e in sorted(payload)],out

def selftest(plan,out):
    out.mkdir(exist_ok=False);events,CLI=synthetic(plan);result=verify_all(plan,events,CLI);neg=[]
    def bad(name,fn):
        x,y=copy.deepcopy(events),copy.deepcopy(CLI);fn(x,y)
        try:verify_all(plan,x,y)
        except(AssertionError,KeyError,ValueError,TypeError):neg.append(name)
        else:raise AssertionError("source checker accepted "+name)
    find=lambda x,n:next(e for e in x if e.get("raw")and e["source_bytes"]["PacketRefs"][0]["Number"]==n)
    bad("wrong-server-INIT-admitted",lambda x,y:find(x,24).update(status="decoded"))
    bad("client-REPLY-fake-hostkey",lambda x,y:find(x,36)["session"].update({"Host Key Fingerprint":"SHA256:invented"}))
    bad("client-GEX-REPLY-fake-private-alias",lambda x,y:find(x,103)["field_values"]["values"].append({"name":"ssh.host_key.sha256","value":"fake","origin":"derived"}))
    bad("client-GEX-GROUP-successful-fields",lambda x,y:find(x,89).update(fields={"Group Prime":"fake"}))
    bad("private-client31-wrong-role-rule",lambda x,y:find(x,47).update(status="malformed"))
    bad("noSYN-role-inferred",lambda x,y:find(x,112)["session"].update({"Host Key Fingerprint":"SHA256:invented"}))
    bad("protected-bytes-reinterpreted",lambda x,y:find(x,11).update(entry="SSHPacket"))
    bad("GEX-old-request-length-unit",lambda x,y:find(x,74)["fields"]["Payload"].update({"Preferred Group Bits":256}))
    bad("GEX-group-prime-borrowed",lambda x,y:find(x,59)["fields"]["Payload"].update({"Group Prime":""}))
    bad("GEX-server-exchange-altered",lambda x,y:find(x,61)["fields"]["Payload"].update({"Server Exchange Value":"Ag=="}))
    bad("malformed-CompletedIn-invented",lambda x,y:find(x,36)["source_bytes"].update({"CompletedIn":{"Number":36,"Domain":ZERO}}))
    bad("invented-private-range",lambda x,y:next(v for v in find(x,61)["field_values"]["values"]if v["name"]=="ssh.message_code").update({"pdu_range":{"offset":5,"length":1}}))
    bad("diagnostic-hidden-as-TCP",lambda x,y:y[35].update(protocol="TCP"))
    bad("malformed-downgraded-to-context",lambda x,y:y[35].update(protocol_status="context-required"))
    bad("connection-suffix-wrong-generation",lambda x,y:y[35].update(info=y[35]["info"].replace("connection #3","connection #2")))
    bad("diagnostic-visibility-invented",lambda x,y:y[35].update(visibility="decrypted"))
    bad("EOF-frame-source-invented",lambda x,y:next(e for e in x if e.get("raw")is None)["source_bytes"].update({"PacketRefs":[{"Number":27,"Domain":ZERO}]}))
    put(out/"summary.json",{"result":"PASS","plan_SHA256":sha(plan/"manifest.json"),"checker_SHA256":sha(__file__),
       "counts":result,"negative_controls":neg,"no_candidate_actual_read":True,"positive_harness_is_synthetic":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"counts":result,"negative_controls":len(neg)}))
def verify(plan,events,CLI,descriptors,out):
    out.mkdir(exist_ok=False);old,eof,api,rows,physical,closes=load(plan)
    inputs={str(p.resolve()):sha(p)for p in(events,CLI,descriptors,plan/"manifest.json",Path(__file__))}
    failure=[];result={}
    try:
        n=old.descriptor_check(read(descriptors),api);result=verify_all(plan,lines(events),lines(CLI));result["SSH_descriptors"]=n
    except Exception as e:failure.append({"type":type(e).__name__,"message":str(e)})
    demand(inputs=={p:sha(p)for p in inputs},"closed inputs changed during offline audit")
    put(out/"differences.json",failure);put(out/"summary.json",{"passed":not failure,"inputs":inputs,"counts":result,"scope":read(plan/"scope.json"),
       "difference_SHA256":sha(out/"differences.json"),"no_expected_value_changed_after_actual":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"passed":not failure}))
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

