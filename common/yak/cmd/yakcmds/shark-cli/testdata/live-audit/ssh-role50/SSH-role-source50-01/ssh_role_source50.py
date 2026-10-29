#!/usr/bin/env python3
"""Authored source-only SSH endpoint-role controls. No product decoder/import."""
import argparse
import copy
import hashlib
import importlib.util
import ipaddress
import json
from pathlib import Path
import re
import struct

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
OLD=PHASE/"SSH-source49-audit01"
ROLE_TABLE={
    "dh":{30:"client",31:"server"},
    "ecdh-nistp256":{30:"client",31:"server"},
    "ecdh-curve25519":{30:"client",31:"server"},
    "dh-gex":{30:"client",31:"server",32:"client",33:"server",34:"client"},
}
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,x):
    with p.open("x",encoding="utf-8")as f:json.dump(x,f,sort_keys=True,indent=2);f.write("\n")
def module(path):
    spec=importlib.util.spec_from_file_location("frozen_ssh_source_wire_builder49",path)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def assess(profile,code,observed_role,protected):
    if protected:return"protected-opaque"
    if observed_role is None:return"role-unconfirmed"
    if profile is None:return"unselected-private-opaque"
    wanted=ROLE_TABLE.get(profile,{}).get(code)
    if wanted is None:return"transport-or-unselected-message"
    return"role-consistent"if wanted==observed_role else"known-role-contradiction"
def prime():
    text=(HERE/"primary01/rfc3526.txt").read_text()
    section=re.search(r"\n3\.\s+2048-bit MODP Group\s*\n(.*?)\n4\.\s+3072-bit",text,re.S).group(1)
    hexpart=section.split("hexadecimal value is:")[1].split("The generator is:")[0]
    words=re.findall(r"\b[0-9A-F]{8}\b",hexpart)
    b=bytes.fromhex("".join(words));assert len(b)==256 and int.from_bytes(b,"big").bit_length()==2048
    return b
def source_inputs():
    manifest=read(OLD/"plan01/manifest.json")
    assert sha(OLD/"plan01/manifest.json")=="37e004f4266ed3ce1bf3fd65c0df789ef2d16a11f0691713471ea8c69a2ab4b0"
    assert sha(OLD/"build_ssh_source49.py")=="c8f7d30e7d057544432b26cbafe92ceac184c0f9d9cdddde789665bee1ee4dfb8"
    for r in manifest["files"]+manifest["primary_manifests"]:assert sha(r["path"])==r["SHA256"]
    facts=read(OLD/"plan01/source-facts.json")
    frames={s["frame"]:s for c in facts["cases"]for s in c["steps"]}
    raw={}
    for n in(80,81,120,121,132,69,82):
        path=Path(frames[n]["input_path"]);b=path.read_bytes();assert dg(b)==frames[n]["payload_SHA256"];raw[n]=b
    return raw,frames,module(OLD/"build_ssh_source49.py")
def change_code(w,n):
    assert len(w)>=16 and w[5]in(30,31)
    return w[:5]+bytes([n])+w[6:]

def cases(raw,builder):
    p=prime()
    group=builder.frame(b"\x1f"+builder.string(b"\0"+p)+builder.mpint(2))
    newreq=raw[132]
    oldreq=builder.frame(b"\x1e"+struct.pack("!I",2048))
    init=change_code(raw[120],32)
    reply=change_code(raw[121],33)
    def s(endpoint,w,kind="method"):return{"endpoint":endpoint,"wire":w,"kind":kind}
    curve="curve25519-sha256";gex="diffie-hellman-group-exchange-sha256"
    return [
        {"sourcecase":"01-curve-correct-server-first-reverse-address","method":curve,"profile":"ecdh-curve25519","known_SYN_role":True,"reverse_addresses":True,"server_first":True,
         "steps":[s("A",raw[80]),s("B",raw[81]),s("B",raw[82],"NEWKEYS"),s("B",raw[80],"protected-sentinel"),s("A",raw[82],"NEWKEYS"),s("A",raw[81],"protected-sentinel")]},
        {"sourcecase":"02-curve-server-sends-INIT","method":curve,"profile":"ecdh-curve25519","known_SYN_role":True,
         "steps":[s("B",raw[80])]},
        {"sourcecase":"03-curve-client-sends-REPLY","method":curve,"profile":"ecdh-curve25519","known_SYN_role":True,
         "steps":[s("A",raw[80]),s("A",raw[81])]},
        {"sourcecase":"04-private-client31-stays-opaque","method":"private-role@example.test","profile":None,"known_SYN_role":True,
         "steps":[s("A",raw[69])]},
        {"sourcecase":"05-GEX-modern-correct-roles","method":gex,"profile":"dh-gex","known_SYN_role":True,
         "steps":[s("A",newreq),s("B",group),s("A",init),s("B",reply),s("B",raw[82],"NEWKEYS"),s("A",raw[82],"NEWKEYS")]},
        {"sourcecase":"06-GEX-old-request-correct-incomplete","method":gex,"profile":"dh-gex","known_SYN_role":True,
         "steps":[s("A",oldreq),s("B",group),s("A",init),s("B",reply)]},
        {"sourcecase":"07-GEX-client-sends-GROUP","method":gex,"profile":"dh-gex","known_SYN_role":True,
         "steps":[s("A",newreq),s("A",group)]},
        {"sourcecase":"08-GEX-client-sends-REPLY","method":gex,"profile":"dh-gex","known_SYN_role":True,
         "steps":[s("A",newreq),s("B",group),s("A",init),s("A",reply)]},
        {"sourcecase":"09-noSYN-role-not-from-banner-or-port","method":curve,"profile":"ecdh-curve25519","known_SYN_role":False,"server_first":True,
         "steps":[s("B",raw[80]),s("A",raw[81])]},
    ]

def build(out):
    out.mkdir(exist_ok=False);(out/"raw").mkdir()
    raw,orig,builder=source_inputs();controls=cases(raw,builder)
    captures=[];facts=[];number=0;clock=0
    for ci,c in enumerate(controls,1):
        ips={"A":"192.0.2.2"if c.get("reverse_addresses")else"192.0.2.1",
             "B":"192.0.2.1"if c.get("reverse_addresses")else"192.0.2.2"}
        ports={"A":52000+ci,"B":2222}
        seq={"A":ci*100000+1000,"B":ci*100000+50000}
        peer={"A":"B","B":"A"};events=[];newkeys=set()
        def emit(ep,data=b"",flags=0x18,kind="TCP-control",extra=None):
            nonlocal number,clock
            dst=peer[ep];ack=0 if flags==2 else seq[dst]
            w=builder.ethernet_tcp(ips[ep],ips[dst],ports[ep],ports[dst],seq[ep],ack,flags,data,number+1)
            number+=1;sec=1700000050+clock//1000000;sub=clock%1000000;clock+=2000
            captures.append(struct.pack("<IIII",sec,sub,len(w),len(w))+w)
            row={"frame":number,"endpoint":ep,"source":f"{ips[ep]}:{ports[ep]}","destination":f"{ips[dst]}:{ports[dst]}",
                 "TCP_seq":seq[ep],"TCP_ack":ack,"TCP_flags":flags,"kind":kind,
                 "caplen":len(w),"origlen":len(w),"physical_SHA256":dg(w),"payload_bytes":len(data),
                 "payload_SHA256":dg(data),"PDU_physical_span":[54,len(w)],
                 "observed_client_server_role":("client"if ep=="A"else"server")if c["known_SYN_role"]else None}
            if data:
                f=out/"raw"/f"{c['sourcecase']}-frame{number:03d}.bin";f.write_bytes(data);row["input_path"]=str(f.resolve())
            if extra:row.update(extra)
            events.append(row)
            seq[ep]+=len(data)+bool(flags&2)+bool(flags&1)
            return row
        if c["known_SYN_role"]:
            syn=emit("A",flags=2);synack=emit("B",flags=18);emit("A",flags=16)
            role_evidence={"kind":"observed-SYN-and-SYNACK","client_endpoint":"A","server_endpoint":"B","SYN_frame":syn["frame"],"SYNACK_frame":synack["frame"],
                           "client_SYN_flag_span":[47,48],"server_SYNACK_flag_span":[47,48]}
        else:role_evidence={"kind":"unconfirmed","client_endpoint":None,"server_endpoint":None,
                            "reason":"no SYN; both endpoints can send SSH identification first; software label and2222 are not role proof"}
        order=("B","A")if c.get("server_first")else("A","B")
        for ep in order:emit(ep,f"SSH-2.0-Endpoint{ep}50\r\n".encode(),0x18,"identification")
        for ep in order:emit(ep,builder.frame(builder.kex_payload([c["method"]])),0x18,"KEXINIT",
                              {"sender_offer_literal":c["method"],"first_kex_packet_follows":False})
        for step in c["steps"]:
            ep=step["endpoint"];w=step["wire"];kind=step["kind"]
            if kind=="protected-sentinel":
                assert ep in newkeys
                assessment="protected-opaque";code=None;wanted=None
            else:
                assert ep not in newkeys
                code=w[5];wanted=ROLE_TABLE.get(c["profile"],{}).get(code)
                assessment=assess(c["profile"],code,("client"if ep=="A"else"server")if c["known_SYN_role"]else None,False)
            row=emit(ep,w,kind=kind,extra={
                "method_literal":c["method"],"selected_field_profile_only_if_negotiation_role_proven":c["profile"]if c["known_SYN_role"]else None,
                "source_message_code":code,"source_message_code_span":None if code is None else[5,6],
                "common_clear_framing_spans":None if code is None else{"Packet Length":[0,4],"Padding Length":[4,5],"Message Number":[5,6]},
                "normative_sender_role_if_known_method":wanted,
                "assessment":assessment,"source_both_direction_NEWKEYS_before":len(newkeys)==2,
                "source_sender_NEWKEYS_before":ep in newkeys,
                "required_outcome":"known-role contradiction diagnostic; preserve SSH/raw source and do not publish role-owned hostkey context"if assessment=="known-role-contradiction"else
                    "opaque; do not parse protected bytes as clear method fields"if assessment=="protected-opaque"else
                    "no role contradiction can be proved; no client/server key ownership can be inferred"if assessment=="role-unconfirmed"else
                    "opaque private method; no global msg31 hostkey/server role rule"if assessment=="unselected-private-opaque"else
                    "role/framing conformant only; signature, peer identity, procedure completion and key math unverified",
                "forbidden_hostkey_derivation":assessment in("known-role-contradiction","unselected-private-opaque","role-unconfirmed","protected-opaque"),
                "synthetic_ciphertext_warning":"not actual encrypted/authenticated wire"if kind=="protected-sentinel"else None})
            if kind=="NEWKEYS":assert code==21;newkeys.add(ep)
        emit("A",flags=17);emit("B",flags=17);emit("A",flags=16)
        facts.append({"sourcecase":c["sourcecase"],"method":c["method"],"profile":c["profile"],"authored":True,
             "role_evidence":role_evidence,"endpoints":{"A":f"{ips['A']}:{ports['A']}","B":f"{ips['B']}:{ports['B']}"},
             "NEWKEYS_observed_directions":sorted(newkeys),"both_NEWKEYS_observed":newkeys=={"A","B"},"steps":events})
    capture=out/"ssh-role-source50-authored9.pcap"
    capture.write_bytes(struct.pack("<IHHIIII",0xa1b2c3d4,2,4,0,0,262144,1)+b"".join(captures))
    put(out/"source-facts.json",{"cases":facts,"scope":"source endpoint-role facts, not candidate API object expectations"})
    put(out/"method-role-contract.json",{"table":{k:{str(n):v for n,v in t.items()}for k,t in ROLE_TABLE.items()},
         "only_exact12_source49_methods":True,
         "wire_bytes_do_not_establish_role_without_transport_or_independent_role_evidence":True,
         "non_goals":["complete KEX message-order FSM","signature or host verification","SFTP","real encrypted SSH","reinterpretation of captured unknown private method semantics"],
         "shortage_order_boundary":"a missing prior opposite-direction packet can reflect capture loss/reordering; do not infer an order violation from absence alone",
         "role_boundary":"a packet from a source-proven server carrying a method-client message is a positive contradiction independent of missing prior messages",
         "guard_placement":"after deciding ignored guessed packet and protected phase, before deriving role-owned method fields/hostkey context; pure stateless wire codec has no endpoint-role input",
         "public_prime":{"RFC":3526,"section":"3","bytes":256,"unsigned_bits":2048,"SHA256":dg(prime())},
         "source50_primary":[
             {"path":str(OLD/"primary01/rfc5656.txt"),"SHA256":sha(OLD/"primary01/rfc5656.txt"),"sections":["4","7.1"],"lines":[343,370,375,377,699]},
             {"path":str(OLD/"primary01/rfc8731.txt"),"SHA256":sha(OLD/"primary01/rfc8731.txt"),"sections":["3"],"lines":[94,99,103]},
             {"path":str(OLD/"primary01/rfc4253.txt"),"SHA256":sha(OLD/"primary01/rfc4253.txt"),"sections":["8"],"lines":[1208,1213]},
             {"path":str(OLD/"primary01/rfc4419.txt"),"SHA256":sha(OLD/"primary01/rfc4419.txt"),"sections":["3","5"],"lines":[99,103,106,119,254,258,265]}]})
    all_steps=[s for c in facts for s in c["steps"]]
    observations=collections_counter(s["assessment"]for s in all_steps if "assessment"in s)
    inputs=[Path(__file__),OLD/"build_ssh_source49.py",OLD/"plan01/manifest.json",OLD/"plan01/source-facts.json",OLD/"plan01/allowlist-and-markers.json",
            HERE/"primary01/manifest.json",HERE/"primary01/rfc3526.txt",capture,out/"source-facts.json",out/"method-role-contract.json"]
    inputs+=list(Path(s["input_path"])for s in all_steps if "input_path"in s)
    put(out/"manifest.json",{"schema":"SSH-role-source50-v1","authored_cases":9,"known_SYN_generations":8,"half_open_no_role_cases":1,
         "physical_frames":number,"payload_steps":sum(bool(s["payload_bytes"])for s in all_steps),"role_assessments":observations,
         "capture":{"path":str(capture.resolve()),"SHA256":sha(capture),"DLT":1},"inputs":{str(p.resolve()):sha(p)for p in inputs},
         "candidate_source_read_only_for_gap_location":True,"no_Go_TShark_SDK_native_process":True,"candidate_runtime_read":False,
         "real_capture_count":0,"frozen_source49_inputs_unchanged":True})
    print(json.dumps({"manifest":str(out/"manifest.json"),"SHA256":sha(out/"manifest.json"),"capture_SHA256":sha(capture),
                     "physical_frames":number,"payload_steps":sum(bool(s["payload_bytes"])for s in all_steps),"roles":observations}))
def collections_counter(values):
    from collections import Counter
    return dict(Counter(values))

def selftest(plan,out):
    out.mkdir(exist_ok=False);m=read(plan/"manifest.json")
    for p,h in m["inputs"].items():assert sha(p)==h,p
    facts=read(plan/"source-facts.json")["cases"];cap=Path(m["capture"]["path"]).read_bytes();at=24;packets=[]
    while at<len(cap):
        sec,us,n,orig=struct.unpack("<IIII",cap[at:at+16]);at+=16;w=cap[at:at+n];at+=n
        assert n==orig==len(w)
        packets.append(w)
    assert len(packets)==m["physical_frames"]and at==len(cap)
    _,_,builder=source_inputs();checks=0
    for c in facts:
        rolemap={endpoint:role for role,endpoint in (("client",c["role_evidence"]["client_endpoint"]),("server",c["role_evidence"]["server_endpoint"]))if endpoint}
        if rolemap:
            syn=c["role_evidence"]["SYN_frame"];synack=c["role_evidence"]["SYNACK_frame"]
            assert packets[syn-1][47]==2 and packets[synack-1][47]==18
        for s in c["steps"]:
            w=packets[s["frame"]-1];assert dg(w)==s["physical_SHA256"]and dg(w[54:])==s["payload_SHA256"]
            ip=w[14:34];tcp=w[34:]
            assert builder.internet_checksum(ip)==0 and builder.internet_checksum(ip[12:20]+b"\0\x06"+struct.pack("!H",len(tcp))+tcp)==0
            src=str(ipaddress.ip_address(ip[12:16]))+":"+str(int.from_bytes(tcp[:2],"big"))
            dst=str(ipaddress.ip_address(ip[16:20]))+":"+str(int.from_bytes(tcp[2:4],"big"))
            assert src==s["source"]==c["endpoints"][s["endpoint"]]and dst==s["destination"]
            assert s["observed_client_server_role"]==rolemap.get(s["endpoint"])
            if "assessment"in s:
                raw=w[54:];assert len(raw)%8==0 and len(raw)>=16 and int.from_bytes(raw[:4],"big")+4==len(raw)
                assert 4<=raw[4]<=len(raw)-6
                protected=s["kind"]=="protected-sentinel"
                code=None if protected else raw[5]
                expected=assess(c["profile"],code,rolemap.get(s["endpoint"]),protected)
                assert s["source_message_code"]==code and s["assessment"]==expected
                if expected=="known-role-contradiction":assert s["forbidden_hostkey_derivation"]is True
            checks+=1
    assert {int(k):v for k,v in read(plan/"method-role-contract.json")["table"]["dh-gex"].items()}=={30:"client",31:"server",32:"client",33:"server",34:"client"}
    # Finite source classifier contrast assertions; no product decoder or SDK output.
    negatives=[]
    for label,got,wanted in[
        ("server-INIT-cannot-be-role-consistent",assess("ecdh-curve25519",30,"server",False),"role-consistent"),
        ("client-REPLY-cannot-be-role-consistent",assess("ecdh-curve25519",31,"client",False),"role-consistent"),
        ("client-GEX-GROUP-cannot-be-server-proof",assess("dh-gex",31,"client",False),"role-consistent"),
        ("client-GEX-REPLY-cannot-own-hostkey",assess("dh-gex",33,"client",False),"role-consistent"),
        ("private31-cannot-use-standard-server-rule",assess(None,31,"client",False),"known-role-contradiction"),
        ("noSYN-cannot-infer-role-from-first-banner-or-port",assess("ecdh-curve25519",30,None,False),"known-role-contradiction"),
        ("postNEWKEYS-bytes-cannot-be-clear-role-violation",assess("ecdh-curve25519",30,"server",True),"known-role-contradiction")]:
        assert got!=wanted;negatives.append(label)
    result={"result":"PASS","manifest_SHA256":sha(plan/"manifest.json"),"source_rows_SHA256":sha(plan/"source-facts.json"),
            "source_step_checks":checks,"negative_controls":negatives,"no_candidate_runtime_read":True,
            "no_Go_TShark_SDK_process":True,"source_role_assessments":m["role_assessments"],"checker_SHA256":sha(__file__)}
    put(out/"summary.json",result)
    print(json.dumps({"summary":str(out/"summary.json"),"SHA256":sha(out/"summary.json"),"checks":checks,"negative_controls":len(negatives)}))
if __name__=="__main__":
    p=argparse.ArgumentParser();sub=p.add_subparsers(dest="cmd",required=True)
    q=sub.add_parser("prepare");q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("selftest");q.add_argument("--plan",type=Path,required=True);q.add_argument("--out",type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":build(a.out)
    else:selftest(a.plan,a.out)

