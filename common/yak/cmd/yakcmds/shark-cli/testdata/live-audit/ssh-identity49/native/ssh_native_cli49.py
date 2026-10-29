#!/usr/bin/env python3
"""Prepare/selftest/offline-verify SSH source49. Never invokes a producer."""
import argparse
import base64
import collections
import copy
import datetime
import hashlib
import importlib.util
import ipaddress
import json
from pathlib import Path
import re
import struct

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
ZERO={"Section":0,"Interface":0,"Encapsulation":""}

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,v):
    with p.open("x",encoding="utf-8") as f:json.dump(v,f,sort_keys=True,indent=2,ensure_ascii=False);f.write("\n")
def module(p):
    spec=importlib.util.spec_from_file_location("independent_ssh_API49",p)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def strict(a,b):return json.dumps(a,sort_keys=True,separators=(",",":"))==json.dumps(b,sort_keys=True,separators=(",",":"))
def demand(ok,why):
    if not ok:raise AssertionError(why)
def lines(p):return [json.loads(x) for x in Path(p).read_text(encoding="utf-8-sig").splitlines() if x.strip()]

def pcap_rows(path):
    w=Path(path).read_bytes();assert w[:4]==bytes.fromhex("d4c3b2a1") and int.from_bytes(w[20:24],"little")==1
    at=24;rows=[]
    while at<len(w):
        sec,us,cap,orig=struct.unpack("<IIII",w[at:at+16]);at+=16;raw=w[at:at+cap];at+=cap
        assert len(raw)==cap==orig and raw[14]==0x45 and raw[23]==6
        ip=raw[14:34];tcp=raw[34:54];payload=raw[54:]
        src=str(ipaddress.ip_address(ip[12:16]));dst=str(ipaddress.ip_address(ip[16:20]))
        sport,dport,seq,ack=struct.unpack("!HHII",tcp[:12])
        assert int.from_bytes(ip[2:4],"big")+14==len(raw)
        rows.append({"number":len(rows)+1,"time_ns":sec*10**9+us*1000,"length":orig,"captured_length":cap,
                     "physical_SHA256":dg(raw),"source":src,"destination":dst,
                     "source_port":sport,"destination_port":dport,"TCP_sequence":seq,"TCP_ack":ack,
                     "TCP_flags":tcp[13],"TCP_payload_SHA256":dg(payload),"TCP_payload_length":len(payload),
                     "physical_TCP_payload_span":[54,len(raw)],"layers":["Ethernet","IPv4","TCP"],"capture_domain":ZERO})
    assert at==len(w)
    return rows

def sourcebytes(row):
    ref={"Number":row["frame"],"Domain":ZERO}
    out={"Kind":"reassembled","SHA256":row["raw_SHA256"],"ParentPDU":0,"ParentPDUs":None,"PacketRefs":[ref]}
    if row["expected_status"]=="decoded":out["CompletedIn"]=ref
    return out

def expected_snapshot(row,identity):
    src=sourcebytes(row);a=row["anchor"]
    facts={"destination":row["destination"],"flow.id":identity["flow"],"ip.addr":[a["source"],a["destination"]],
           "pdu.id":identity["id"],"pdu.length":row["raw_length"],"protocol":"ssh",
           "source":row["source"],"status":row["expected_status"],"tcp.flow.id":identity["tcp_flow_id"]}
    if row["expected_status"]=="decoded":facts["pdu.completed_in"]=row["frame"]
    vals=[]
    for name in sorted(facts):
        value=facts[name]
        for v in value if isinstance(value,list) else [value]:
            item={"name":name,"value":v,"origin":"event"}
            if name in ("flow.id","pdu.id","pdu.length","pdu.completed_in","tcp.flow.id"):item["exact_value"]=str(v)
            vals.append(item)
    vals+=copy.deepcopy(row["private_aliases"])
    return {"schema_version":1,"pdu":identity["id"],"source":src,"values":vals,"completeness":row["expected_completeness"]}

def fieldmap(snapshot):
    out=collections.defaultdict(list)
    for v in snapshot["values"]:out[v["name"]].append(v["value"])
    return dict(out)

def prepare(out):
    assert not out.exists();out.mkdir()
    source=PHASE/"SSH-source49-audit01";plan=source/"plan01";closure=source/"closure01"
    assert sha(plan/"manifest.json")=="37e004f4266ed3ce1bf3fd65c0df789ef2d16a11f0691713471ea8c69a2ab4b0"
    assert sha(closure/"manifest.json")=="171cecd0689736efa0cdc3a5c818f41e3f10028a79308bf08a847afbd2505244"
    m=read(plan/"manifest.json")
    for r in m["files"]+m["primary_manifests"]:assert sha(r["path"])==r["SHA256"]
    api=module(HERE/"ssh_source_API49.py")
    cap=Path(m["capture"]["path"]);physical=pcap_rows(cap)
    frozen_cases=read(plan/"source-facts.json")["cases"]
    rows=[];case_by_frame={}
    for case in frozen_cases:
        case_no=len(set(case_by_frame.values()))+1
        for step in case["steps"]:case_by_frame[step["frame"]]=case["sourcecase"]
        prepared=api.case_expected(case)
        for r in prepared:
            anchor=physical[r["frame"]-1]
            assert anchor["TCP_payload_SHA256"]==r["raw_SHA256"]
            r["anchor"]=anchor;r["expected_direction"]=0 if r["role"]=="client" else 1
            r["source"]=case["tuple"][r["role"]]
            r["destination"]=case["tuple"]["server" if r["role"]=="client" else "client"]
            r["capture_domain"]=ZERO
        rows+=prepared
    assert len(rows)==76 and len(physical)==148
    counts=collections.Counter(r["expected_status"] for r in rows)
    assert counts=={"decoded":74,"context-required":2}
    for r in physical:r["sourcecase"]=case_by_frame[r["number"]]
    put(out/"source-rows.json",rows);put(out/"physical-rows.json",physical)
    put(out/"private-alias-schema.json",[{"name":n,"API_key":k,"origin":o,"type":t,"bits":b}for n,k,o,t,b in api.SPECS])
    runtime=read(closure/"runtime-command-plan.json")
    runtime["field_descriptor_args"]=["shark","--list-fields","--json"]
    runtime["verify_argv"]=[str(HERE/"ssh_native_cli49.py"),"verify","--plan",str(out),"--events","<closed events.jsonl>","--CLI","<closed yak.jsonl>","--descriptors","<closed descriptors.json>","--out","<fresh audit directory>"]
    put(out/"runtime-command-plan.json",runtime)
    windows_row=next(r for r in rows if r["frame"]==69)
    wraw=Path(windows_row["raw_path"]).read_bytes()
    opaque=base64.b64decode(windows_row["fields"]["Payload"]["Opaque Message Data"])
    put(out/"windows-unknown-reply-source-plan.json",{
        "scope":"unexecuted Windows source plan, frame69; no pixel proof claimed",
        "capture":str(cap),"capture_SHA256":sha(cap),"sourcecase":windows_row["sourcecase"],
        "frame":69,"filter":"fields pdu.completed_in == 69","target_PDU_id":"derive from independently source-verified frame69 event; never hardcode",
        "expected_protocol":"SSH","expected_packet_name":"KEY_EXCHANGE_31 (layout unconfirmed)",
        "expected_literal_method":"private@example.test","required_stack":["Ethernet","IPv4","TCP","SSH"],
        "capture_physical_layers":["Ethernet","IPv4","TCP"],"expected_visibility":"reassembled",
        "expected_status":"decoded","source_TCP_payload_physical_span":windows_row["anchor"]["physical_TCP_payload_span"],
        "whole_PDU":{"path":windows_row["raw_path"],"bytes":len(wraw),"SHA256":dg(wraw)},
        "opaque_field":{"API_path":["Payload","Opaque Message Data"],"bytes":len(opaque),"SHA256":dg(opaque),
                        "independent_source_span":[6,len(wraw)-wraw[4]],
                        "UI_expected_source":"owned extracted-field-value source, or complete PDU source; existing API supplies no frame/PDU range for this leaf",
                        "not_allowed":"byte-value search is not evidence of a captured frame offset"},
        "forbidden_labels":["Host Key Fingerprint","Host Key SHA256","Host Key Format","KEXDH_REPLY","KEX_ECDH_REPLY","Kex Family: DH","Kex Family: ECDH"],
        "unknown_method_policy":"Preserve common SSH framing and arbitrary opaque payload; do not reinterpret first string as a public host key.",
        "default_protocol_history_mib":64,"default_cache_packets":20000,
    })
    repo=Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
    metadata=[
        "common/pcapx/pcaputil/protocol_field_registry.go","common/pcapx/pcaputil/protocol_evidence.go",
        "common/pcapx/pcaputil/bin_parser.go","common/pcapx/pcaputil/bin_parser_ssh.go",
        "common/pcapx/pcaputil/bin_parser_session.go","common/bin-parser/parser/stream_parser/ssh_plaintext.go",
        "common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events/main.go",
        "common/yak/cmd/yakcmds/shark-cli/decode.go","common/yak/cmd/yakcmds/shark-cli/presentation.go",
        "common/yak/cmd/yakcmds/shark-cli/packet_observation.go","common/yak/cmd/yakcmds/shark-cli/command.go"]
    put(out/"schema-metadata-binding.json",[{"path":str(repo/p),"SHA256":sha(repo/p),
         "scope":"name/type/layout schema only; future production files may change, not value oracle"}for p in metadata])
    put(out/"scope.json",{
        "authored":True,"real_peer_or_live_capture_count":0,"physical_frames":148,"expected_native_payload_events":76,
        "expected_status_counts":dict(counts),"TCP_generations":12,"four_tuple_reuse_pairs":1,
        "method_allowlist":"frozen12 exact complete grammar names; unknown prefix/substring never selects method fields",
        "fields":"complete Session and GetFields trees from frozen source; every source-backed API literal compared",
        "ranges":"independent literal span ledger is NOT a claimed API WR; current SSH GetFields and private alias API expose no pdu_range",
        "private_aliases":"all12 existing SSH descriptors; complete per-event occurrence shape and current types/origins",
        "native_metadata":"whole raw, exact source/domain/PacketRefs/CompletedIn, TCP roles, unique protocol/TCP generations, zero transaction/response",
        "CLI":"all148 original physical metadata+76 exact payload captions/scopes/visibility/stack; all compact PDUs and private snapshots bridge to independently verified SDK",
        "not_whole_object_claim":"Control-frame nonessential conversation annotations and every event error prose are not independently frozen; extra keys remain visible in actual exports",
        "encrypted":"two protected-phase sentinels are authored nonconformant ciphertext placeholders, not authenticated encryption proof",
        "physical_vs_publication":"148 physical source rows; nonphysical publication rows must be counted separately and source-bound, never added to physical count",
        "no_candidate_runtime_read":True,
    })
    inputs=[HERE/"ssh_source_API49.py",Path(__file__),plan/"manifest.json",plan/"source-facts.json",
            plan/"allowlist-and-markers.json",closure/"manifest.json",closure/"runtime-command-plan.json",cap]
    inputs+=list(Path(r["raw_path"])for r in rows)
    inputs+=list(p for p in out.iterdir()if p.is_file())
    put(out/"manifest.json",{"schema":"SSH-source49-SDK-CLI-plan-v1","inputs":{str(p.resolve()):sha(p)for p in inputs},
         "source_plan":str(plan),"source_plan_SHA256":sha(plan/"manifest.json"),"source_API":str(HERE/"ssh_source_API49.py"),
         "checker":str(Path(__file__).resolve()),"physical_frames":148,"payload_events":76,
         "no_SSH_baseline_or_future_actual_read":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),
          "source_rows_SHA256":sha(out/"source-rows.json"),"counts":count_source(rows)}))

def load(plan):
    m=read(plan/"manifest.json")
    for p,s in m["inputs"].items():demand(sha(p)==s,"frozen file changed: "+p)
    return m,read(plan/"source-rows.json"),read(plan/"physical-rows.json"),module(m["source_API"])

def count_source(rows):
    return {"source_payloads":len(rows),"API_literal_occurrences":sum(len(r["API_wire_leaves"])for r in rows),
            "source_span_occurrences":sum(len(r["source_fields_from_plan01"])for r in rows),
            "private_alias_occurrences":sum(len(r["private_aliases"])for r in rows),
            "status_counts":dict(collections.Counter(r["expected_status"]for r in rows))}

def descriptor_check(data,api):
    schema=data["fields"] if isinstance(data,dict) else data
    selected=[d for d in schema if d["name"].startswith("ssh.")]
    demand({d["name"]for d in selected}=={n for n,*_ in api.SPECS},"SSH descriptor set")
    for n,k,o,t,b in api.SPECS:
        d=next(v for v in selected if v["name"]==n)
        demand(d["origin"]==o and d["type"]==t and d.get("bits",0)==b,"SSH descriptor type/origin "+n)
    return len(selected)

def at_path(v,path):
    for k in path:v=v[k]
    return v

def verify_event(e,row):
    raw=Path(row["raw_path"]).read_bytes()
    demand(base64.b64decode(e["raw"],validate=True)==raw,"SDK entire raw bytes")
    for key,value in {"protocol":"ssh","status":row["expected_status"],"profile":"ssh",
         "completeness":row["expected_completeness"],"entry":row["expected_entry"],
         "transport":"tcp","source":row["source"],"destination":row["destination"],
         "direction":row["expected_direction"],"domain":ZERO,"transaction":0,"response_to":0}.items():
        demand(strict(e[key],value),"SDK "+key+" "+row["sourcecase"])
    demand(strict(e["source_bytes"],sourcebytes(row)),"exact owned source bytes/ref/domain/complete contributor")
    demand(strict(e.get("session"),row["session"]),"entire source-built Session")
    demand(strict(e.get("fields"),row["fields"]),"entire source-built GetFields")
    demand(bool(e.get("fields_error")) is row["fields_error_required"],"fields error outcome")
    demand(e.get("expert","")==row["expert"],"expert identity")
    if row["expected_status"]=="context-required":
        demand("NEWKEYS" in str(e.get("error",""))+" "+e.get("summary",""),"encrypted cause")
    else:
        demand(not e.get("error"),"unexpected decode refusal")
        demand(e["summary"]=="SSH "+row["session"]["Packet Name"],"SDK named method summary")
    for leaf in row["API_wire_leaves"]:
        demand(strict(at_path(e["fields"],leaf["API_path"]),leaf["value"]),"source-backed literal "+repr(leaf["API_path"]))
    ids={k:e[k]for k in ("id","flow","tcp_flow_id")}
    demand(all(type(v)is int and v>0 for v in ids.values()),"real positive generated PDU/session/TCP ids")
    expected=expected_snapshot(row,ids)
    demand(strict(e["field_values"],expected),"entire source-built typed/private snapshot")
    return ids

def ns(text):
    m=re.fullmatch(r"(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)",text);demand(bool(m),"timestamp schema")
    dt=datetime.datetime.strptime(m[1],"%Y-%m-%dT%H:%M:%S").replace(tzinfo=datetime.timezone.utc)
    value=int(dt.timestamp())*10**9+int((m[2]or"").ljust(9,"0"))
    if m[3]!="Z":value-=(int(m[3][1:3])*3600+int(m[3][4:])*60)*10**9*(1 if m[3][0]=="+" else -1)
    return value

def compact_expected(e):
    s=e["field_values"]
    out={"id":e["id"],"protocol":"ssh","status":e["status"],"completeness":e["completeness"],
         "response_to":0,"byte_source":e["source_bytes"],"fields":fieldmap(s),"field_values":s,"tcp_flow_id":e["tcp_flow_id"]}
    if e.get("error") or e.get("expert"):
        out.update(summary=e["summary"],error=e.get("error",""),expert=e.get("expert",""))
    return out

def verify_rows(rows,physical,events,CLI):
    demand(len(events)==76,"exact native payload-event cardinality; EOF/other events not silently discarded")
    expected_by_frame={r["frame"]:r for r in rows};byframe={};identities={};used=set()
    for e in events:
        refs=e["source_bytes"]["PacketRefs"]
        demand(len(refs)==1 and refs[0]["Number"]in expected_by_frame,"every event has its actual sole byte-contributor")
        frame=refs[0]["Number"];demand(frame not in byframe,"duplicate native physical-event witness")
        row=expected_by_frame[frame];ids=verify_event(e,row)
        demand(ids["id"] not in used,"PDU id reuse");used.add(ids["id"]);byframe[frame]=e
        key=row["sourcecase"]
        pair=(ids["flow"],ids["tcp_flow_id"])
        if key in identities:demand(identities[key]==pair,"one TCP/SYN generation split or flow reuse")
        else:identities[key]=pair
    demand(len(identities)==12 and len({v[0]for v in identities.values()})==12 and len({v[1]for v in identities.values()})==12,
           "distinct SYN generations, including reused tuple, need distinct protocol and TCP flow ids")
    physicalCLI=[r for r in CLI if "number"in r];publications=[r for r in CLI if "number"not in r]
    demand(len(physicalCLI)==148,"physical frame cardinality")
    demand([r["number"]for r in physicalCLI]==list(range(1,149)),"source frame order/identity")
    bridge_counts=collections.Counter()
    for c,a in zip(physicalCLI,physical):
        for key in ("number","length","captured_length","source","destination","source_port","destination_port","layers","capture_domain"):
            demand(strict(c.get(key),a[key]),"CLI original physical "+key+" frame"+str(a["number"]))
        demand(c["transport"]=="tcp" and ns(c["time"])==a["time_ns"],"CLI captured transport/time")
        demand(c.get("tcp_flow_id")==identities[a["sourcecase"]][1],"CLI actual TCP generation")
        demand(strict(c.get("tcp_flow_domain"),ZERO),"CLI actual TCP domain")
        expected=expected_by_frame.get(a["number"])
        if expected:
            for key,ek in (("protocol","CLI_protocol"),("protocol_status","CLI_status"),("visibility","CLI_visibility"),("protocol_scope","CLI_scope"),("info","CLI_info")):
                demand(c.get(key)==expected[ek],"CLI source-selected "+key+" frame"+str(a["number"]))
            demand(c.get("protocol_stack")==["Ethernet","IPv4","TCP","SSH"],"progressive stack without fabricated physical SSH layer")
        observed=c.get("pdus",[])
        demand(len(observed)==(1 if expected else 0),"direct source callback presence, no cross-frame inheritance")
        for p in observed:
            demand(strict(p,compact_expected(byframe[a["number"]])),"entire SDK->CLI compact PDU/snapshot bridge")
            bridge_counts["physical_PDU_bridges"]+=1
        demand(not c.get("published_pdus"),"these contiguous one-record payloads have no delayed publications")
    demand(not publications,"no expected EOF diagnostics in closed authored full-wire controls; publications cannot inflate physical count")
    return {"physical_rows":148,"native_events":76,"TCP_generations":12,"publication_rows":len(publications),
            **count_source(rows),**bridge_counts}

def synthetic(rows,physical,api):
    caseorder=list(dict.fromkeys(r["sourcecase"]for r in rows));ids={c:(i+11,i+101)for i,c in enumerate(caseorder)}
    events=[]
    for i,r in enumerate(rows,1001):
        f,t=ids[r["sourcecase"]]
        e={"id":i,"flow":f,"tcp_flow_id":t,"protocol":"ssh","status":r["expected_status"],"profile":"ssh",
           "completeness":r["expected_completeness"],"summary":"SSH "+r["session"]["Packet Name"]if r["session"] else"Encrypted: SSH transport is encrypted after NEWKEYS",
           "expert":r["expert"],"source_bytes":sourcebytes(r),"fields":r["fields"],"session":r["session"],
           "field_values":expected_snapshot(r,{"id":i,"flow":f,"tcp_flow_id":t}),
           "raw":base64.b64encode(Path(r["raw_path"]).read_bytes()).decode(),"entry":r["expected_entry"],"admission":"",
           "source":r["source"],"destination":r["destination"],"direction":r["expected_direction"],
           "transport":"tcp","domain":ZERO,"carrier_layers":None,"transaction":0,"response_to":0}
        if r["fields_error_required"]:e["fields_error"]="protocol parser: event is context-required, not an exact message";e["error"]="Encrypted: SSH transport is encrypted after NEWKEYS"
        events.append(e)
    byframe={r["frame"]:e for r,e in zip(rows,events)};rowbyframe={r["frame"]:r for r in rows}
    CLI=[]
    for a in physical:
        c={k:a[k]for k in ("number","length","captured_length","source","destination","source_port","destination_port","layers","capture_domain")}
        dt=datetime.datetime.fromtimestamp(a["time_ns"]//10**9,datetime.timezone.utc)
        c.update(transport="tcp",time=dt.strftime("%Y-%m-%dT%H:%M:%S")+f".{a['time_ns']%10**9:09d}Z",
                 tcp_flow_id=ids[a["sourcecase"]][1],tcp_flow_domain=ZERO,protocol="TCP",info="synthetic control")
        if a["number"]in byframe:
            e=byframe[a["number"]];r=rowbyframe[a["number"]]
            c.update(protocol=r["CLI_protocol"],protocol_status=r["CLI_status"],visibility=r["CLI_visibility"],
                     protocol_scope=r["CLI_scope"],info=r["CLI_info"],protocol_stack=["Ethernet","IPv4","TCP","SSH"],pdus=[compact_expected(e)])
        CLI.append(c)
    return events,CLI

def selftest(plan,out):
    m,rows,physical,api=load(plan);out.mkdir(exist_ok=False)
    e,c=synthetic(rows,physical,api);counts=verify_rows(rows,physical,e,c);reject=[]
    def bad(label,mutator):
        x,y=copy.deepcopy(e),copy.deepcopy(c);mutator(x,y)
        try:verify_rows(rows,physical,x,y)
        except (AssertionError,KeyError,ValueError,TypeError):reject.append(label)
        else:raise AssertionError("negative accepted "+label)
    find=lambda x,n:next(z for z in x if z["source_bytes"]["PacketRefs"][0]["Number"]==n)
    bad("prefix-marker-removal-wrong-selected-literal",lambda x,y:find(x,8)["session"].update({"Kex Algorithm":"curve25519-sha256"}))
    bad("unknown-DH-standard-fields",lambda x,y:find(x,32)["fields"]["Payload"].update({"Client Exchange Value":"Ag=="}))
    bad("unknown-GEX-family",lambda x,y:find(x,44)["session"].update({"Kex Family":"DH"}))
    bad("unknown-curve-family",lambda x,y:find(x,56)["session"].update({"Kex Family":"ECDH"}))
    bad("unknown-first-string-fingerprint",lambda x,y:find(x,69)["session"].update({"Host Key Fingerprint":"SHA256:invented"}))
    bad("unknown-private-fingerprint-alias",lambda x,y:find(x,69)["field_values"]["values"].append({"name":"ssh.host_key.sha256","value":"SHA256:invented","origin":"derived"}))
    bad("invented-private-byte-range",lambda x,y:next(v for v in find(x,69)["field_values"]["values"]if v["name"]=="ssh.message_code").update({"pdu_range":{"offset":5,"length":1}}))
    bad("wrong-guess-was-decoded",lambda x,y:find(x,143)["session"].update({"Packet Name":"KEXDH_INIT"}))
    bad("protected-byte-banner-reinterpretation",lambda x,y:find(x,83).update({"entry":"SSHIdentification","status":"decoded"}))
    bad("tuple-reuse-inherited-previous-method",lambda x,y:find(x,80)["session"].update({"Kex Algorithm":"private@example.test"}))
    bad("reverse-role-lost",lambda x,y:find(x,108).update({"direction":1}))
    bad("CLI-opaque-labelled-DH-reply",lambda x,y:y[68].update({"info":"KEXDH_REPLY"}))
    bad("CLI-progressive-layer-missing",lambda x,y:y[68].update({"protocol_stack":["Ethernet","IPv4","TCP"]}))
    bad("literal-libssh-alias-rewritten",lambda x,y:find(x,96)["session"].update({"Kex Algorithm":"curve25519-sha256"}))
    bad("false-signature-verified",lambda x,y:find(x,97)["session"].update({"Signature Verified":True}))
    bad("wrong-physical-source-ref",lambda x,y:find(x,69)["source_bytes"]["PacketRefs"][0].update({"Number":68}))
    put(out/"summary.json",{"result":"PASS","plan_manifest_SHA256":sha(plan/"manifest.json"),"checker_SHA256":sha(__file__),
        "source_API_SHA256":sha(HERE/"ssh_source_API49.py"),"counts":counts,"negative_controls":reject,
        "synthetic_positive_harness_only":True,"no_candidate_runtime_read":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"negative_controls":len(reject),"counts":counts}))

def verify(plan,events,CLI,descriptors,out):
    m,rows,physical,api=load(plan);out.mkdir(exist_ok=False)
    inputs={str(p.resolve()):sha(p)for p in (events,CLI,descriptors,plan/"manifest.json",Path(__file__),HERE/"ssh_source_API49.py")}
    failures=[];counts={}
    try:
        descriptor_count=descriptor_check(read(descriptors),api)
        counts=verify_rows(rows,physical,lines(events),lines(CLI))
        counts["private_alias_descriptors"]=descriptor_count
    except Exception as err:
        failures.append({"type":type(err).__name__,"message":str(err)})
    demand(inputs=={p:sha(p)for p in inputs},"closed inputs changed during offline audit")
    put(out/"differences.json",failures)
    put(out/"summary.json",{"passed":not failures,"scope":read(plan/"scope.json"),"inputs":inputs,"counts":counts,
         "differences_SHA256":sha(out/"differences.json"),"no_expected_value_changed_after_candidate_read":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"passed":not failures,"differences":len(failures)}))
    if failures:raise SystemExit(1)

def main():
    p=argparse.ArgumentParser();sub=p.add_subparsers(dest="cmd",required=True)
    q=sub.add_parser("prepare");q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("selftest");q.add_argument("--plan",type=Path,required=True);q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("verify")
    for n in ("plan","events","CLI","descriptors","out"):q.add_argument("--"+n,type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.out)
    elif a.cmd=="selftest":selftest(a.plan,a.out)
    else:verify(a.plan,a.events,a.CLI,a.descriptors,a.out)
if __name__=="__main__":main()

