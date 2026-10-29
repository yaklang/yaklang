#!/usr/bin/env python3
"""Freeze source-only SSH->TShark aliases/spans before packet dissection."""
import argparse
import base64
import hashlib
import json
from pathlib import Path

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2);f.write("\n")
def make(out):
    out.mkdir(exist_ok=False)
    source=PHASE/"SSH-source49-audit01"/"plan01"
    api=PHASE/"SSH-SDK-CLI-source49-01"/"plan01"
    meta=HERE/"metadata01"
    assert sha(source/"manifest.json")=="37e004f4266ed3ce1bf3fd65c0df789ef2d16a11f0691713471ea8c69a2ab4b0"
    assert sha(api/"manifest.json")=="6152f55c293920a466756dcc090e1b4d0627f8ca7379d33e21cb175c37c2d85d"
    assert sha(meta/"producer.json")=="3cc4ed8e568fd220605ff541366ace87653447d96134017eb82e13feca25ef0f"
    metadata=read(meta/"producer.json")
    assert metadata["all_processes_closed"] and metadata["mode"].startswith("metadata only")
    inventory={}
    for line in (meta/"fields.txt").read_text(encoding="utf-8").splitlines():
        parts=line.split("\t")
        if len(parts)>=4 and parts[0]=="F":inventory[parts[2]]={"name":parts[1],"type":parts[3]}
    all_names=read(source/"allowlist-and-markers.json")
    rows=read(api/"source-rows.json")
    original=read(source/"source-facts.json")
    byframe={s["frame"]:(c,s)for c in original["cases"]for s in c["steps"]}
    common={"Packet Length":"ssh.packet_length","Padding Length":"ssh.padding_length","Padding":"ssh.padding_string","Message Number":"ssh.message_code","Cookie":"ssh.cookie","First Kex Packet Follows":"ssh.first_kex_packet_follows","Reserved":"ssh.kex.reserved"}
    lists={"Kex Algorithms":"kex_algorithms","Host Key Algorithms":"server_host_key_algorithms","Encryption C2S":"encryption_algorithms_client_to_server","Encryption S2C":"encryption_algorithms_server_to_client","MAC C2S":"mac_algorithms_client_to_server","MAC S2C":"mac_algorithms_server_to_client","Compression C2S":"compression_algorithms_client_to_server","Compression S2C":"compression_algorithms_server_to_client","Languages C2S":"languages_client_to_server","Languages S2C":"languages_server_to_client"}
    exact={
        "Minimum Group Bits":"ssh.dh_gex.min","Preferred Group Bits":"ssh.dh_gex.nbits","Maximum Group Bits":"ssh.dh_gex.max",
        "Client Exchange Value":"ssh.dh.e","Server Exchange Value":"ssh.dh.f",
        "Client Exchange Value Length":"ssh.mpint_length","Server Exchange Value Length":"ssh.mpint_length",
        "Client Curve25519 Public Key":"ssh.ecdh.q_c","Server Curve25519 Public Key":"ssh.ecdh.q_s",
        "Client Curve25519 Public Key Length":"ssh.ecdh.q_c_length","Server Curve25519 Public Key Length":"ssh.ecdh.q_s_length",
        "Client Point Length":"ssh.ecdh.q_c_length","Server Point Length":"ssh.ecdh.q_s_length",
        "Host Key Length":"ssh.host_key.length","Host Key Format Length":"ssh.host_key.type_length","Host Key Format":"ssh.host_key.type",
        "Ed25519 Public Key Length":"ssh.host_key.eddsa.key_length","Ed25519 Public Key":"ssh.host_key.eddsa.key",
        "Signature Length":"ssh.host_sig.length","Signature Format Length":"ssh.host_sig.type_length","Signature Format":"ssh.host_sig.type",
    }
    for name,alias in lists.items():exact[name]="ssh."+alias;exact[name+" Length"]="ssh."+alias+"_length"
    mapping=[];contexts=[]
    for r in rows:
        case,step=byframe[r["frame"]];raw=Path(r["raw_path"]).read_bytes()
        ctx={"frame":r["frame"],"sourcecase":r["sourcecase"],"role":r["role"],"kind":r["kind"],
             "raw_SHA256":r["raw_SHA256"],"raw_length":len(raw),"source_payload_span":[54,54+len(raw)],
             "selected_literal_KEX":case["selected_literal_KEX"],"selected_profile":case["selected_method_profile"],
             "source_packet_name":r["session"]["Packet Name"]if r["session"]else None,
             "guessed_packet_must_be_ignored":bool(step.get("ignored_guess")),
             "forbidden_method_layout":case["selected_method_profile"]is None or bool(step.get("ignored_guess")) or r["kind"]=="protected-phase-sentinel",
             "no_plaintext_after_direction_NEWKEYS":r["kind"]=="protected-phase-sentinel"}
        contexts.append(ctx)
        if r["kind"]=="identification":
            mapping.append({"frame":r["frame"],"sourcecase":r["sourcecase"],"path":["Identification"],"literal_span":[0,len(raw)],
                  "alias":"ssh.protocol","reference_span":[0,len(raw)],"kind":"exact-primitive",
                  "wire_hex":raw.hex(),"source_display":raw.rstrip(b"\r\n").decode()})
            continue
        for leaf in r["API_wire_leaves"]:
            name=leaf["API_path"][-1];alias=common.get(name)or exact.get(name);span=leaf["span"];refspan=span;kind="exact-primitive" if alias else "unmapped"
            if name in ("Ed25519 Signature Bytes Length","Ed25519 Signature Bytes"):
                alias="ssh.host_sig.data";kind="aggregate-owner"
                owner=[x for x in r["API_wire_leaves"]if x["API_path"][:-1]==leaf["API_path"][:-1]and x["API_path"][-1] in ("Ed25519 Signature Bytes Length","Ed25519 Signature Bytes")]
                refspan=[min(x["span"][0]for x in owner),max(x["span"][1]for x in owner)]
            if len(leaf["API_path"])>=3 and leaf["API_path"][-2]in("Client Point","Server Point"):
                ownername=leaf["API_path"][-2]
                alias="ssh.ecdh.q_c"if ownername=="Client Point"else"ssh.ecdh.q_s";kind="aggregate-owner"
                owner=[x for x in r["API_wire_leaves"]if x["API_path"][:-1]==leaf["API_path"][:-1]]
                refspan=[min(x["span"][0]for x in owner),max(x["span"][1]for x in owner)]
            mapping.append({"frame":r["frame"],"sourcecase":r["sourcecase"],"path":leaf["API_path"],
                "literal_span":span,"alias":alias,"reference_span":refspan,"kind":kind,
                "wire_hex":raw[refspan[0]:refspan[1]].hex(),"literal_value":leaf["value"],"source_kind":leaf["kind"],
                "reference_field_type":inventory[alias]["type"]if alias else None})
    wanted={"frame.number","frame.cap_len","frame.len","frame.time_epoch","frame.protocols","_ws.col.Protocol","_ws.expert.message",
            "ip.src","ip.dst","tcp.srcport","tcp.dstport","tcp.seq_raw","tcp.ack_raw","tcp.stream","tcp.flags",
            "ssh.encrypted_packet","ssh.packet_length_encrypted","ssh.payload","ssh.direction"}
    wanted.update(m["alias"]for m in mapping if m["alias"])
    # Registered method fields are watchlisted even on frames where source selects
    # no layout. Unexpected presence is reported, never normalized into correctness.
    forbidden_prefixes=["ssh.host_key.","ssh.host_sig.","ssh.dh.","ssh.dh_gex.","ssh.ecdh."]
    watched=sorted(n for n in inventory if any(n.startswith(p)for p in forbidden_prefixes))
    wanted.update(watched)
    missing=wanted-set(inventory)-{"_ws.col.Protocol"}
    assert not missing,sorted(missing)
    heuristic_lines=[line for line in (meta/"heuristic-decodes.txt").read_text().splitlines()if "\tssh\t"in line]
    port_lines=[line for line in (meta/"decodes.txt").read_text().splitlines()if line.endswith("\tssh")and line.startswith("tcp.port")]
    put(out/"source-to-reference-mapping.json",mapping);put(out/"source-contexts.json",contexts)
    put(out/"physical-source.json",read(api/"physical-rows.json"));put(out/"fields-selection.json",sorted(wanted))
    put(out/"reference-policy.json",{
        "source_cases":12,"physical_frames":148,"authored":True,"real_capture_count":0,
        "source_case_groups":{"unknown_method_controls":6,"known_method_or_state_controls":6},
        "SDK_checker_mutation_negatives":16,"source_builder_mutation_negatives":7,
        "heuristic_metadata":heuristic_lines,"registered_tcp_port_metadata":port_lines,
        "natural_reference":"No decode-as; determine actual default heuristic result without assuming port recognition.",
        "explicit_reference":"-d tcp.port==2222,ssh is an explicit reference adaptation only, not content identification proof.",
        "spans":"Compare literal physical span54+source span. Aggregate-owner fields are counted separately and never certified as exact inner primitives.",
        "method_names":"Compare dynamic Key Exchange(method:...) display metadata if present; sender offer aliases are separately literal.",
        "forbidden_method_alias_prefixes":forbidden_prefixes,
        "forbidden_exact_aliases_when_unselected":["ssh.mpint_length","ssh.kex_hybrid_blob_client","ssh.kex_hybrid_blob_server"],
        "unknown":"A source-private method stays unselected even if bytes resemble a familiar hostkey/exchange. Missing reference fields are reported, not borrowed from another frame.",
        "protected_phase":"No plaintext banner/KEX/method fields after that direction's NEWKEYS. Sentinels are not conforming ciphertext/authentication proof.",
        "fingerprint":"No registered SSH hostkey SHA256 fingerprint alias exists in this pinned field inventory; do not invent reference support or turn missing display into a parser bug.",
        "first_guess":"Wrong guessed frame143 has no selected method fields; frame144 and145 restore correct Curve25519. Correct guess frame96 remains selected.",
        "reuse":"case06 and07 share tuple but have distinct SYN ISNs; tcp.stream should not merge them.",
        "per_packet_counts":"Reference packets/clear PDUs/registered fields/display labels are independent counts, not equal-correctness proxies.",
        "no_SDK_baseline_or_candidate_read":True})
    files={p.name:sha(p)for p in out.iterdir()if p.is_file()}
    cap=read(source/"manifest.json")["capture"]
    inputs={str(p.resolve()):sha(p)for p in [Path(__file__),HERE/"run_ssh_reference49.py",source/"manifest.json",source/"source-facts.json",
             source/"allowlist-and-markers.json",api/"manifest.json",api/"source-rows.json",api/"physical-rows.json",meta/"producer.json",meta/"fields.txt",
             meta/"heuristic-decodes.txt",meta/"decodes.txt",meta/"currentprefs.txt",Path(cap["path"])]}
    put(out/"manifest.json",{"schema":"SSH-source-reference49-v1","capture":cap,"inputs":inputs,"files":files,
         "mapping_rows":len(mapping),"selected_alias_rows":sum(bool(x["alias"])for x in mapping),
         "source_contexts":len(contexts),"reference_plan_frozen_before_source_dissection":True,
         "no_SDK_or_candidate_output_read":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),"mapping_rows":len(mapping),
                     "selected_alias_rows":sum(bool(x["alias"])for x in mapping),"heuristic":heuristic_lines}))
if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--out",type=Path,required=True);a=p.parse_args();make(a.out)

