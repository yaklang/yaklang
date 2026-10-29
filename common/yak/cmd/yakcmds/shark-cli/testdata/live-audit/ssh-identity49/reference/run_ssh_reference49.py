#!/usr/bin/env python3
"""Pinned, isolated TShark metadata and SSH reference execution; no SDK imports."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
TOOL=Path(r"C:\tmp\yak-shark-protocol-audit-20261024\wireshark-portable\App\Wireshark\tshark.exe")
TOOL_SHA="662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b"
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p,v):
    with p.open("x",encoding="utf-8")as f:json.dump(v,f,sort_keys=True,indent=2);f.write("\n")
def isolated(folder):
    p=folder/"empty-profile";p.mkdir()
    env=dict(os.environ,APPDATA=str(p),WIRESHARK_CONFIG_DIR=str(p))
    for k in ("SSLKEYLOGFILE","WIRESHARK_PLUGIN_DIR","WIRESHARK_EXTCAP_DIR"):env.pop(k,None)
    return env,p
def execute(args,out,env,context):
    start=time.monotonic()
    stderr=out.with_suffix(out.suffix+".stderr")
    with out.open("xb")as a,stderr.open("xb")as b:
        p=subprocess.run([str(TOOL),*args],stdout=a,stderr=b,env=env,timeout=60,creationflags=0x08000000)
    record={"context":context,"argv":[str(TOOL),*args],"exit_code":p.returncode,"seconds":time.monotonic()-start,
            "output":str(out.resolve()),"output_bytes":out.stat().st_size,"output_SHA256":sha(out),
            "stderr":str(stderr.resolve()),"stderr_SHA256":sha(stderr)}
    print(json.dumps({"context":context,"exit_code":p.returncode,"output_bytes":out.stat().st_size}),flush=True)
    assert p.returncode==0
    return record
def metadata(out):
    out.mkdir(exist_ok=False);assert sha(TOOL)==TOOL_SHA
    env,profile=isolated(out);runs=[]
    for label,args in (("version",["-v"]),("fields",["-G","fields"]),("heuristic-decodes",["-G","heuristic-decodes"]),
                       ("decodes",["-G","decodes"]),("currentprefs",["-G","currentprefs"])):
        runs.append(execute(args,out/(label+".txt"),env,label))
    assert sha(TOOL)==TOOL_SHA
    put(out/"producer.json",{"mode":"metadata only; no source PCAP execution", "tool":str(TOOL),"tool_SHA256":TOOL_SHA,
        "driver":str(Path(__file__).resolve()),"driver_SHA256":sha(__file__),"empty_profile":str(profile.resolve()),
        "runs":runs,"all_processes_closed":True,"SDK_or_candidate_read":False})
    print(json.dumps({"producer":str(out/"producer.json"),"SHA256":sha(out/"producer.json")}))
def replay(plan,out):
    out.mkdir(exist_ok=False);m=read(plan/"manifest.json");assert sha(TOOL)==TOOL_SHA
    for p,s in m["inputs"].items():assert sha(p)==s
    for p,s in m["files"].items():assert sha(plan/p)==s
    cap=Path(m["capture"]["path"]);assert sha(cap)==m["capture"]["SHA256"]
    aliases=read(plan/"fields-selection.json");runs=[]
    inputs={**m["inputs"],str(plan/"manifest.json"):sha(plan/"manifest.json"),str(TOOL):sha(TOOL),
            str(Path(__file__).resolve()):sha(__file__)}
    for mode,adapt in (("natural",[]),("explicit-ssh-reference",["-d","tcp.port==2222,ssh"])):
        folder=out/mode;folder.mkdir();env,profile=isolated(folder)
        for kind in ("pdml","fields"):
            args=["-n","-2",*adapt,"-r",str(cap),"-T",kind]
            if kind=="fields":
                args+=["-E","header=y","-E","separator=/t","-E","quote=d","-E","occurrence=a"]
                for name in aliases:args+=["-e",name]
            p=folder/("reference.pdml" if kind=="pdml" else "reference-fields.tsv")
            r=execute(args,p,env,mode+"/"+kind);r.update(profile=str(profile.resolve()),capture_SHA256=sha(cap),reference_adapter=adapt)
            runs.append(r)
    assert inputs=={p:sha(p)for p in inputs}
    put(out/"producer.json",{"plan_SHA256":sha(plan/"manifest.json"),"driver_SHA256":sha(__file__),
        "inputs":inputs,"runs":runs,"all_processes_closed":True,"source_mapping_frozen_before_packet_execution":True,
        "reference_decode_as_is_not_content_admission_proof":True,"SDK_or_candidate_read":False})
    print(json.dumps({"producer":str(out/"producer.json"),"SHA256":sha(out/"producer.json")}))
if __name__=="__main__":
    p=argparse.ArgumentParser();sub=p.add_subparsers(dest="cmd",required=True)
    q=sub.add_parser("metadata");q.add_argument("--out",type=Path,required=True)
    q=sub.add_parser("replay");q.add_argument("--plan",type=Path,required=True);q.add_argument("--out",type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="metadata":metadata(a.out)
    else:replay(a.plan,a.out)

