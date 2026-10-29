#!/usr/bin/env python3
"""Independent source50 role/primitive reference mapping, then offline comparison."""
import argparse
import collections
import hashlib
import json
from pathlib import Path
import re
import xml.etree.ElementTree as ET

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
SOURCE=PHASE/"SSH-role-source50-01"/"plan02"
META=PHASE/"SSH-Wireshark-source49-01"/"metadata01"
RUNNER=PHASE/"SSH-Wireshark-source49-01"/"run_ssh_reference49.py"
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2);f.write("\n")
def prepare(out):
    out.mkdir(exist_ok=False)
    source=read(SOURCE/"manifest.json")
    assert sha(SOURCE/"manifest.json")=="ae73e1329488cc159cb0958f483c529e7dd50e270b53249dae8521324a2553af"
    for p,h in source["inputs"].items():assert sha(p)==h
    cases=read(SOURCE/"source-facts.json")["cases"];mapping=[];contexts=[];physical=[]
    def add(c,s,alias,span,raw,kind="primitive"):
        mapping.append({"frame":s["frame"],"sourcecase":c["sourcecase"],"alias":alias,"span":[54+x for x in span],
                        "wire_SHA256":dg(raw[span[0]:span[1]]),"wire_hex":raw[span[0]:span[1]].hex(),"kind":kind,
                        "role_facts_independent_of_reference_field_value":True})
    for c in cases:
        for s in c["steps"]:
            physical.append({"frame":s["frame"],"sourcecase":c["sourcecase"],"source":s["source"],"destination":s["destination"],
                "TCP_seq":s["TCP_seq"],"TCP_ack":s["TCP_ack"],"caplen":s["caplen"],"origlen":s["origlen"],"TCP_flags":s["TCP_flags"],
                "known_SYN_role":c["role_evidence"]["kind"]=="observed-SYN-and-SYNACK"})
            if not s["payload_bytes"]:continue
            raw=Path(s["input_path"]).read_bytes();assert dg(raw)==s["payload_SHA256"]
            ctx={"frame":s["frame"],"sourcecase":c["sourcecase"],"kind":s["kind"],"method":c["method"],
                 "observed_role":s["observed_client_server_role"],"assessment":s.get("assessment"),
                 "known_SYN_role":c["role_evidence"]["kind"]=="observed-SYN-and-SYNACK",
                 "raw_SHA256":dg(raw),"payload_span":[54,54+len(raw)],"source_message_code":s.get("source_message_code"),
                 "forbid_hostkey_context":s.get("forbidden_hostkey_derivation",False)}
            contexts.append(ctx)
            if s["kind"]=="protected-sentinel":continue
            if s["kind"]=="identification":
                assert raw.endswith(b"\r\n");add(c,s,"ssh.protocol",[0,len(raw)-2],raw);continue
            n=int.from_bytes(raw[:4],"big");pad=raw[4];end=len(raw)-pad
            assert n+4==len(raw)and len(raw)%8==0
            for alias,sp in(("ssh.packet_length",[0,4]),("ssh.padding_length",[4,5]),("ssh.message_code",[5,6]),("ssh.padding_string",[end,len(raw)])):add(c,s,alias,sp,raw)
            msg=raw[5];at=6
            def atom(alias,size):
                nonlocal at
                start=at;assert size<=end-at;at+=size;add(c,s,alias,[start,at],raw);return raw[start:at]
            def string(alias,la=None):
                nonlocal at
                start=at;size=int.from_bytes(raw[at:at+4],"big")
                if la:atom(la,4)
                else:at+=4
                b=atom(alias,size);return b,start
            def host_signature(exchange):
                nonlocal at
                host,st=string("ssh.host_key.data","ssh.host_key.length")
                # The broad host_key.data alias is a fallback and not necessarily
                # rendered for a selected Ed25519 layout; classify as owned blob.
                mapping[-1]["kind"]="optional-owned-blob"
                sub=st+4;size=int.from_bytes(raw[sub:sub+4],"big")
                add(c,s,"ssh.host_key.type_length",[sub,sub+4],raw);sub+=4
                add(c,s,"ssh.host_key.type",[sub,sub+size],raw);sub+=size
                assert host[4:4+size]==b"ssh-ed25519"
                size=int.from_bytes(raw[sub:sub+4],"big");add(c,s,"ssh.host_key.eddsa.key_length",[sub,sub+4],raw);sub+=4
                add(c,s,"ssh.host_key.eddsa.key",[sub,sub+size],raw)
                string(exchange,"ssh.ecdh.q_s_length"if exchange=="ssh.ecdh.q_s"else"ssh.mpint_length")
                sig,st=string("ssh.host_sig.data","ssh.host_sig.length")
                mapping[-1]["kind"]="optional-owned-blob"
                sub=st+4;size=int.from_bytes(raw[sub:sub+4],"big")
                add(c,s,"ssh.host_sig.type_length",[sub,sub+4],raw);sub+=4;add(c,s,"ssh.host_sig.type",[sub,sub+size],raw);sub+=size
                add(c,s,"ssh.host_sig.data",[sub,st+4+len(sig)],raw,"aggregate-signature-owner")
            if msg==20:
                atom("ssh.cookie",16)
                aliases=["kex_algorithms","server_host_key_algorithms","encryption_algorithms_client_to_server","encryption_algorithms_server_to_client","mac_algorithms_client_to_server","mac_algorithms_server_to_client","compression_algorithms_client_to_server","compression_algorithms_server_to_client","languages_client_to_server","languages_server_to_client"]
                for alias in aliases:string("ssh."+alias,"ssh."+alias+"_length")
                atom("ssh.first_kex_packet_follows",1);atom("ssh.kex.reserved",4)
            elif msg==21:pass
            elif c["profile"]=="ecdh-curve25519":
                if msg==30:string("ssh.ecdh.q_c","ssh.ecdh.q_c_length")
                elif msg==31:host_signature("ssh.ecdh.q_s")
            elif c["profile"]=="dh-gex":
                if msg==34:
                    for alias in("ssh.dh_gex.min","ssh.dh_gex.nbits","ssh.dh_gex.max"):atom(alias,4)
                elif msg==30:atom("ssh.dh_gex.nbits",4)
                elif msg==31:
                    string("ssh.dh_gex.p","ssh.mpint_length");string("ssh.dh_gex.g","ssh.mpint_length")
                elif msg==32:string("ssh.dh.e","ssh.mpint_length")
                elif msg==33:host_signature("ssh.dh.f")
            else:
                # Unknown private method has no selected message-body layout.
                at=end
            assert at==end,(c["sourcecase"],msg,at,end)
    inventory={line.split("\t")[2]for line in(META/"fields.txt").read_text().splitlines()if line.startswith("F\t")}
    aliases={r["alias"]for r in mapping}
    aliases|={"frame.number","frame.cap_len","frame.len","frame.protocols","_ws.col.Protocol","_ws.expert.message","ip.src","ip.dst",
              "tcp.srcport","tcp.dstport","tcp.seq_raw","tcp.ack_raw","tcp.stream","ssh.direction","ssh.encrypted_packet","ssh.packet_length_encrypted","ssh.payload"}
    assert aliases-inventory=={"_ws.col.Protocol"}
    put(out/"mapping.json",mapping);put(out/"contexts.json",contexts);put(out/"physical.json",physical);put(out/"fields-selection.json",sorted(aliases))
    put(out/"reference-policy.json",{
        "same_pinned_TShark_and_empty_profile_policy_as_source49":True,"max_concurrent_TShark":1,
        "modes":["natural heuristic","explicit -d tcp.port==2222,ssh reference adaptation"],
        "role_truth":"frozen RFC5656/RFC8731/RFC4419 table plus observed SYN role; never TShark assumed direction",
        "source_forbidden_context":"wrong-role reply cannot become a server hostkey observation even if all candidate layout primitives are present",
        "primitive_agreement_not_role_agreement":True,"role_unconfirmed":"without SYN, these two banners and port2222 do not establish client/server",
        "protected_sentinels":"not conforming authenticated ciphertext; no role or common plaintext header interpretation after own NEWKEYS",
        "SYN_reuse":"source50 has9 distinct tuples; source49 case06->07 remains the separate frozen same-tuple generation control",
        "SDK_candidate_output_used":False})
    inputs={str(p.resolve()):sha(p)for p in[Path(__file__),RUNNER,SOURCE/"manifest.json",SOURCE/"source-facts.json",SOURCE/"method-role-contract.json",META/"producer.json",META/"fields.txt",Path(source["capture"]["path"])]}
    put(out/"manifest.json",{"capture":source["capture"],"inputs":inputs,"files":{p.name:sha(p)for p in out.iterdir()if p.is_file()},
         "mapping_rows":len(mapping),"contexts":len(contexts),"source_plan_frozen":True,"candidate_runtime_read":False})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),"mapping_rows":len(mapping),"contexts":len(contexts)}))
def fields(p,name):return list(p.iterfind(".//field[@name='"+name+"']"))
def show(p,name):
    x=fields(p,name);assert len(x)==1,name
    return x[0].get("show")
def octets(n):
    if"value"in n.attrib:
        try:return bytes.fromhex(n.get("value").replace(":",""))
        except ValueError:return None
    if n.get("size")=="0"and n.get("show")=="":return b""
    return None
def span(n):return[int(n.get("pos")),int(n.get("pos"))+int(n.get("size"))]
def compare(plan,actual,out):
    out.mkdir(exist_ok=False);m=read(plan/"manifest.json");prod=read(actual/"producer.json")
    for p,h in m["inputs"].items():assert sha(p)==h
    for p,h in m["files"].items():assert sha(plan/p)==h
    assert prod["all_processes_closed"]and prod["plan_SHA256"]==sha(plan/"manifest.json")
    for r in prod["runs"]:assert r["exit_code"]==0 and sha(r["output"])==r["output_SHA256"]and sha(r["stderr"])==r["stderr_SHA256"]
    mappings=read(plan/"mapping.json");physical=read(plan/"physical.json");ctxs={r["frame"]:r for r in read(plan/"contexts.json")}
    by=collections.defaultdict(list)
    for r in mappings:by[r["frame"]].append(r)
    summaries={}
    for mode in("natural","explicit-ssh-reference"):
        packets=ET.parse(actual/mode/"reference.pdml").getroot().findall("packet")
        assert len(packets)==115
        observations=[];matches=[];stats=collections.Counter();streams=collections.defaultdict(set)
        for a,p in zip(physical,packets):
            num=a["frame"];assert int(show(p,"frame.number"))==num
            assert int(show(p,"frame.cap_len"))==a["caplen"]and int(show(p,"frame.len"))==a["origlen"]
            assert show(p,"ip.src")+":"+show(p,"tcp.srcport")==a["source"]and show(p,"ip.dst")+":"+show(p,"tcp.dstport")==a["destination"]
            assert int(show(p,"tcp.seq_raw"))==a["TCP_seq"]and int(show(p,"tcp.ack_raw"))==a["TCP_ack"]
            streams[a["sourcecase"]].add(int(show(p,"tcp.stream")))
            c=ctxs.get(num)
            if c:
                dirs=[n.get("showname")for n in fields(p,"ssh.direction")]
                experts=[n.get("show")for n in fields(p,"_ws.expert.message")]
                keyaliases=sorted(set(n.get("name")for n in p.iter("field")if n.get("name","").startswith(("ssh.host_key.","ssh.host_sig."))))
                obs={"frame":num,"sourcecase":c["sourcecase"],"source_role":c["observed_role"],"source_assessment":c["assessment"],
                     "reference_directions":dirs,"reference_message_codes":[n.get("show")for n in fields(p,"ssh.message_code")],
                     "reference_hostkey_or_signature_aliases":keyaliases,"reference_experts":experts,
                     "reference_clear_packet_ssh_protocol":[n.get("show")for n in fields(p,"ssh.protocol")],
                     "reference_method_names":sorted(set(re.findall(r"Key Exchange \(method:\s*([^)]*)\)"," ".join(n.get("show","")+" "+n.get("showname","")for n in p.iter()))))}
                if c["observed_role"]is not None:
                    want="True"if c["observed_role"]=="server"else"False"
                    obs["direction_matches_SYN"] = all(n.get("show")==want for n in fields(p,"ssh.direction"))
                else:obs["direction_matches_SYN"]=None
                if c["assessment"]=="known-role-contradiction":
                    obs["RFC_role_contradiction_independent_of_reference"]=True
                    obs["reference_role_rejection_observed"]=any("role"in v.lower()or"direction"in v.lower()for v in experts)
                    stats["source_wrong_role_packets"]+=1
                    if not obs["reference_role_rejection_observed"]:stats["wrong_role_without_reference_role_expert"]+=1
                    if keyaliases:stats["wrong_role_with_hostkey_fields"]+=1
                if c["kind"]=="protected-sentinel":
                    assert not fields(p,"ssh.message_code")and not fields(p,"ssh.protocol"),"reference plaintext leaked after ownNEWKEYS"
                    stats["protected_sentinels_without_clear_fields"]+=1
                if obs["direction_matches_SYN"]is False:stats["reference_direction_differs_from_SYN"]+=1
                observations.append(obs)
            for r in by[num]:
                nodes=fields(p,r["alias"]);expected=bytes.fromhex(r["wire_hex"])
                ok=[n for n in nodes if span(n)==r["span"]and octets(n)==expected]
                category="matched-primitive"if ok and r["kind"]=="primitive"else"matched-aggregate-owner"if ok else"optional-owned-blob-not-rendered"if r["kind"]=="optional-owned-blob"else"not-matched"
                if ok and not expected:category="matched-zero-width-no-highlight"
                matches.append({"frame":num,"alias":r["alias"],"source_span":r["span"],"mapping_kind":r["kind"],"result":category})
                stats[category]+=1
        assert len(streams)==9 and all(len(s)==1 for s in streams.values())and len(set(next(iter(s))for s in streams.values()))==9
        put(out/(mode+"-packet-observations.json"),observations);put(out/(mode+"-field-results.json"),matches)
        summaries[mode]={"counts":dict(stats),"physical_frames":115,"source_TCP_carriers":9,
             "packet_observations_SHA256":sha(out/(mode+"-packet-observations.json")),"fields_SHA256":sha(out/(mode+"-field-results.json"))}
    put(out/"summary.json",{"result":"REFERENCE_CLOSED_WITH_RFC_ROLE_TRUTH_SEPARATE","source_plan_SHA256":sha(plan/"manifest.json"),
       "producer_SHA256":sha(actual/"producer.json"),"checker_SHA256":sha(__file__),"contexts":summaries,
       "all_TShark_closed":True,"source50_unchanged":True,"SDK_candidate_read":False,"authored_not_live":True,
       "reference_role_checking_not_assumed_correct":True})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"counts":{k:v["counts"]for k,v in summaries.items()}}))
if __name__=="__main__":
    p=argparse.ArgumentParser();sub=p.add_subparsers(dest="cmd",required=True)
    q=sub.add_parser("prepare");q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("compare")
    for n in("plan","actual","out"):q.add_argument("--"+n,type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.out)
    else:compare(a.plan,a.actual,a.out)

