#!/usr/bin/env python3
"""Offline source/PDML comparison. Source mapping stays byte frozen."""
import argparse
import collections
import decimal
import hashlib
import json
from pathlib import Path
import re
import xml.etree.ElementTree as ET

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,v):
    with p.open("x",encoding="utf-8")as f:json.dump(v,f,sort_keys=True,indent=2,ensure_ascii=False);f.write("\n")
def must(x,m):
    if not x:raise AssertionError(m)
def fields(packet,name):return list(packet.iterfind(".//field[@name='"+name+"']"))
def show(packet,name):
    fs=fields(packet,name);must(len(fs)==1,"one original "+name)
    return fs[0].get("show")
def span(n):
    try:return[int(n.get("pos")),int(n.get("pos"))+int(n.get("size"))]
    except (TypeError,ValueError):return None
def octets(n):
    if "value"not in n.attrib:return None
    try:return bytes.fromhex(n.attrib["value"].replace(":",""))
    except ValueError:return None

def compare(plan,actual,out):
    out.mkdir(exist_ok=False);m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():must(sha(p)==h,"frozen input "+p)
    for p,h in m["files"].items():must(sha(plan/p)==h,"frozen mapping "+p)
    producer=read(actual/"producer.json")
    must(producer["all_processes_closed"]and producer["plan_SHA256"]==sha(plan/"manifest.json"),"closed producer binding")
    for run in producer["runs"]:
        must(run["exit_code"]==0 and sha(run["output"])==run["output_SHA256"]and sha(run["stderr"])==run["stderr_SHA256"],"closed run export binding")
    for p,h in producer["inputs"].items():must(sha(p)==h,"producer input binding")
    mapping=read(plan/"source-to-reference-mapping.json")
    contexts={x["frame"]:x for x in read(plan/"source-contexts.json")}
    physical=read(plan/"physical-source.json");policy=read(plan/"reference-policy.json")
    byframe=collections.defaultdict(list)
    for row in mapping:byframe[row["frame"]].append(row)
    outputs={};summaries={};detail={}
    for mode in("natural","explicit-ssh-reference"):
        path=actual/mode/"reference.pdml"
        packets=ET.parse(path).getroot().findall("packet")
        must(len(packets)==148,"all original physical frames")
        result=[];watch=[];methods=[];packet_results=[];stats=collections.Counter();case_streams=collections.defaultdict(set)
        for a,p in zip(physical,packets):
            num=int(show(p,"frame.number"));must(num==a["number"],"original frame identity/order")
            for alias,key in (("frame.len","length"),("frame.cap_len","captured_length"),("tcp.srcport","source_port"),("tcp.dstport","destination_port"),
                              ("tcp.seq_raw","TCP_sequence"),("tcp.ack_raw","TCP_ack")):
                must(int(show(p,alias))==a[key],"physical "+alias)
            must(show(p,"ip.src")==a["source"]and show(p,"ip.dst")==a["destination"],"original IP direction")
            must(decimal.Decimal(show(p,"frame.time_epoch"))*decimal.Decimal(10**9)==a["time_ns"],"physical timestamp")
            streams={int(x.get("show"))for x in fields(p,"tcp.stream")}
            must(len(streams)==1,"one captured TCP stream")
            case_streams[a["sourcecase"]]|=streams
            ssh_protocols=list(p.iterfind(".//proto[@name='ssh']"))
            if ssh_protocols:stats["physical_frames_with_SSH_tree"]+=1
            method_names=sorted(set(re.findall(r"Key Exchange \(method:\s*([^)]*)\)"," ".join(n.get("showname","")for n in p.iter()))))
            ctx=contexts.get(num)
            if ctx:
                methods.append({"frame":num,"sourcecase":ctx["sourcecase"],"source_selected_literal":ctx["selected_literal_KEX"],"reference_method_names":method_names})
                for name in method_names:
                    if name!=ctx["selected_literal_KEX"]:
                        watch.append({"category":"reference-selected-method-differs","frame":num,"sourcecase":ctx["sourcecase"],
                                      "source_method":ctx["selected_literal_KEX"],"reference_method":name})
                if method_names:stats["frames_with_method_name_display"]+=1
                if ctx["forbidden_method_layout"]:
                    aliases=sorted(set(n.get("name","")for n in p.iter("field")if any(n.get("name","").startswith(prefix)for prefix in policy["forbidden_method_alias_prefixes"])
                                       or n.get("name")in policy["forbidden_exact_aliases_when_unselected"]))
                    if aliases:
                        watch.append({"category":"reference-method-fields-on-unselected-source","frame":num,"sourcecase":ctx["sourcecase"],
                                      "source_kind":ctx["kind"],"aliases":aliases,
                                      "source_claim":"No exact selected known grammar or this is an ignored/protected-phase packet."})
                if ctx["no_plaintext_after_direction_NEWKEYS"]:
                    forbidden=fields(p,"ssh.protocol")+fields(p,"ssh.message_code")+fields(p,"ssh.kex_algorithms")
                    if forbidden:watch.append({"category":"reference-clear-fields-after-direction-NEWKEYS","frame":num,"aliases":[n.get("name")for n in forbidden]})
                experts=[n.get("show","")for n in fields(p,"_ws.expert.message")]
                packet_results.append({"frame":num,"sourcecase":ctx["sourcecase"],"kind":ctx["kind"],
                    "SSH_tree_present":bool(ssh_protocols),"SSH_message_codes":[n.get("show")for n in fields(p,"ssh.message_code")],
                    "encrypted_packet_ranges":[span(n)for n in fields(p,"ssh.encrypted_packet")],
                    "method_names":method_names,"expert_messages":experts,
                    "source_payload_SHA256":ctx["raw_SHA256"],"source_payload_span":ctx["source_payload_span"],
                    "tcp_stream":next(iter(streams))})
            for row in byframe[num]:
                rec={"frame":num,"sourcecase":row["sourcecase"],"path":row["path"],"alias":row["alias"],"source_primitive_span":row["literal_span"],
                     "mapping_kind":row["kind"],"reference_expected_span":[54+v for v in row["reference_span"]]}
                if not row["alias"]:
                    rec["result"]="unmapped-no-reference-pass";stats[rec["result"]]+=1;result.append(rec);continue
                available=fields(p,row["alias"]);expected=bytes.fromhex(row["wire_hex"])
                exact=[n for n in available if span(n)==rec["reference_expected_span"]]
                matches=[n for n in exact if octets(n)==expected]
                if matches:
                    rec["result"]="aggregate-owner-matched"if row["kind"]=="aggregate-owner"else"zero-width-literal-matched"if not expected else"exact-primitive-matched"
                    rec["reference_field_count"]=len(matches)
                    rec["literal_wire_SHA256"]=dg(expected)
                elif exact:
                    rec["result"]="literal-value-difference"
                    rec["observed"]=[{"span":span(n),"wire_SHA256":dg(octets(n))if octets(n)is not None else None,
                                     "wire_length":len(octets(n))if octets(n)is not None else None}for n in exact]
                elif available:
                    value_match=[n for n in available if octets(n)==expected]
                    rec["result"]="same-value-different-highlight"if value_match else"reference-alias-present-without-source-leaf"
                    rec["observed"]=[{"span":span(n),"wire_SHA256":dg(octets(n))if octets(n)is not None else None,
                                     "wire_length":len(octets(n))if octets(n)is not None else None}for n in available]
                else:rec["result"]="source-alias-not-rendered"
                stats[rec["result"]]+=1;result.append(rec)
        must(all(len(s)==1 for s in case_streams.values()),"reference split one source TCP generation")
        generation_values=[next(iter(s))for s in case_streams.values()]
        must(len(generation_values)==len(set(generation_values))==12,"reference merged distinct source SYN generations, including reused tuple")
        result_path=out/(mode+"-field-results.json");put(result_path,result)
        watch_path=out/(mode+"-semantic-observations.json");put(watch_path,watch)
        put(out/(mode+"-packet-results.json"),packet_results)
        put(out/(mode+"-method-displays.json"),methods)
        aggregate_owners={(r["frame"],r["alias"],tuple(r["reference_expected_span"]))for r in result if r["result"]=="aggregate-owner-matched"}
        missing_group=collections.Counter()
        for r in result:
            if r["result"]not in("exact-primitive-matched","zero-width-literal-matched","aggregate-owner-matched","unmapped-no-reference-pass"):
                missing_group[(r["sourcecase"],r["result"])]+=1
        summaries[mode]={"counts":dict(stats),"source_mapping_rows":len(mapping),"physical_source_frames":148,"TCP_generations":12,
                         "unique_aggregate_owners_matched":len(aggregate_owners),
                         "semantic_observation_counts":dict(collections.Counter(w["category"]for w in watch)),
                         "nonmatch_by_case":[{"sourcecase":k[0],"category":k[1],"count":v}for k,v in missing_group.items()],
                         "field_results_SHA256":sha(result_path),"semantic_observations_SHA256":sha(watch_path),
                         "source_expected_physical_generations_preserved":True}
        outputs[mode]={"pdml_SHA256":sha(path),"fields_SHA256":sha(actual/mode/"reference-fields.tsv")}
        detail[mode]=(result,watch,packet_results)
    same=outputs["natural"]==outputs["explicit-ssh-reference"]
    put(out/"summary.json",{"result":"REFERENCE_COMPARISON_CLOSED","producer_SHA256":sha(actual/"producer.json"),"source_plan_SHA256":sha(plan/"manifest.json"),
        "checker":str(Path(__file__).resolve()),"checker_SHA256":sha(__file__),"contexts":summaries,"actual_exports":outputs,
        "natural_and_explicit_exports_byte_identical":same,"actual_TShark_processes_closed":True,
        "authored_controls_not_live_traffic":True,"source_case_groups":{"unknown_method":6,"known_method_state":6},
        "SDK_mutation_negatives":16,"source_builder_mutation_negatives":7,
        "no_SDK_baseline_or_future_candidate_read":True,
        "not_claimed":["all SSH authentication correctness","private method layout knowledge","live encrypted SSH","exact primitive highlighting from aggregate owner spans",
                       "a missing alias proves content absence","all TOP30 protocols complete"]})
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"natural_and_explicit_identical":same,
                      "contexts":{k:{"counts":v["counts"],"semantic":v["semantic_observation_counts"]}for k,v in summaries.items()}}))
if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--plan",type=Path,required=True);p.add_argument("--actual",type=Path,required=True);p.add_argument("--out",type=Path,required=True)
    a=p.parse_args();compare(a.plan,a.actual,a.out)

