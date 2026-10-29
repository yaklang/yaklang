"""Prepared native-console1MiB execution, with an independent cell verifier.
The old complete-HTTP3-detail tour is intentionally not its success criterion.
No UI or executable was run while preparing this driver.
"""
import argparse,hashlib,json,os,re,subprocess,sys,time
from pathlib import Path
here=Path(__file__).resolve().parent
ap=argparse.ArgumentParser()
ap.add_argument("--binary",type=Path,required=True)
ap.add_argument("--source",required=True)
ap.add_argument("--out",type=Path,required=True)
ap.add_argument("--helper",type=Path,default=Path(r"C:\tmp\yak-shark-ESP-SSH-UI-20261028\windows-PIM-helper26\native-console.exe"))
ap.add_argument("--renderer",type=Path)
ap.add_argument("--receipt",type=Path)
ap.add_argument("--helper-source",type=Path)
a=ap.parse_args()
def sha(p):
 h=hashlib.sha256()
 with Path(p).open("rb")as f:
  for b in iter(lambda:f.read(1<<20),b""):h.update(b)
 return h.hexdigest()
plan=json.loads((here/"plan.json").read_text())
receipt_path=a.receipt or a.binary.parent/"yak-receipt.json"
producer=json.loads(receipt_path.read_text())
assert producer["source"]==a.source and producer["exit_code"]==0 and sha(a.binary)==producer["binary_SHA256"]
assert sha(plan["capture"])==plan["capture_SHA256"] and sha(plan["keylog"])==plan["keylog_SHA256"]
helper_receipt=a.helper.parent/"receipt.json"
hr=json.loads(helper_receipt.read_text())
assert hr["exit_code"]==0 and sha(a.helper)==hr["binary_SHA256"]
a.out.mkdir(exist_ok=False)
frozen=a.out/"frozen-inputs";frozen.mkdir()
bundle=json.loads((here/"manifest.json").read_text())
small={"driver":Path(__file__),"plan":here/"plan.json","source_manifest":here/"manifest.json",
 "app_build_receipt":receipt_path,"helper_build_receipt":helper_receipt,
 "mixed_replay_source":here/"mixed_identity_replay.go"}
helper_source=a.helper_source or Path(plan["helper_source"])
small["native_console_helper_source"]=helper_source
if a.renderer:small["renderer"]=a.renderer
large={"application_binary":a.binary,"console_helper_binary":a.helper,
 "capture":Path(plan["capture"]),"external_keylog":Path(plan["keylog"])}
before={};copied={}
for name,p in small.items():
 data=p.read_bytes()
 assert len(data)<=1<<20,("unexpected source snapshot size",name,len(data))
 before[name]={"path":str(p),"SHA256":hashlib.sha256(data).hexdigest(),"bytes":len(data)}
 target=frozen/(name+(".json"if p.suffix==".json"else p.suffix or".bin"))
 target.write_bytes(data);copied[name]={"path":str(target),"SHA256":sha(target)}
for name,p in large.items():before[name]={"path":str(p),"SHA256":sha(p),"bytes":p.stat().st_size}
assert sha(helper_source)==hr["source_file_SHA256"],"helper source does not match its actual old producer"
for name,info in bundle["files"].items():
 assert sha(here/name)==info["SHA256"],("frozen bundle source changed",name)
(a.out/"inputs-before.json").write_text(json.dumps({"inputs":before,"source_snapshots":copied},indent=2)+"\n")
dest=a.out/"console"
query="fields pdu.id == 299"
field="Protocol detail absent or evicted"
argv=[str(a.helper),str(dest),str(a.binary),plan["capture"],plan["keylog"],query,field,
 "--ignore-embedded-secrets","--protocol-history-mib","1"]
# The helper's legacy strict-protocol path requires full HTTP3 fields. Here the
# expected correct result is retained row identity + explicitly absent detail.
# Its native ConPTY capture remains unchanged; strict assertions follow below.
env=dict(os.environ,GOMAXPROCS="1",GOMEMLIMIT="1GiB",SHARK_QA_EXPECT_PROTOCOL="",
 SHARK_QA_EXPECT_QUERY=query,SHARK_QA_EXPECT_PACKETS="1865",SHARK_QA_EXPECT_ADAPTIVE="",
 SHARK_QA_FIELD_DOWN="",SHARK_QA_SELECT_FIELD="",SHARK_QA_BYTE_FIELD="",SHARK_QA_NATIVE_WIRE="")
start=time.monotonic()
try:
 run=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,creationflags=0x08000000,timeout=120)
except subprocess.TimeoutExpired as error:
 class TimedOut:pass
 run=TimedOut();run.returncode=-124;run.stdout=error.stdout or b"";run.stderr=(error.stderr or b"")+b"\nNative helper timeout preserved."
after={name:{"path":entry["path"],"SHA256":sha(entry["path"]),"bytes":Path(entry["path"]).stat().st_size}
 for name,entry in before.items()}
input_changes=[name for name in before if before[name]!=after[name]]
(a.out/"inputs-after.json").write_text(json.dumps({"inputs":after,"changes":input_changes},indent=2)+"\n")
(a.out/"stdout.log").write_bytes(run.stdout);(a.out/"stderr.log").write_bytes(run.stderr)
proof={"source":a.source,"binary_SHA256":sha(a.binary),"build_receipt_SHA256":sha(receipt_path),
 "helper_source":hr["source"],"helper_SHA256":sha(a.helper),"helper_receipt_SHA256":sha(helper_receipt),
 "driver_SHA256":sha(__file__),"plan_SHA256":sha(here/"plan.json"),
 "capture_SHA256":plan["capture_SHA256"],"keylog_SHA256":plan["keylog_SHA256"],
 "arguments":argv,"environment_overrides":{k:v for k,v in env.items()if k.startswith("SHARK_QA_")},
 "exit_code":run.returncode,"seconds":time.monotonic()-start,"snapshots":{},"errors":[],"visual_review_pending":True}
proof["source_snapshot_SHA256"]=sha(a.out/"inputs-before.json")
proof["source_after_SHA256"]=sha(a.out/"inputs-after.json")
proof["input_bytes_unchanged"]=not input_changes
proof["input_changes"]=input_changes
def check(ok,msg):
 if not ok:proof["errors"].append(msg)
check(not input_changes,"execution input bytes changed; do not close/relabel this run")
for p in sorted(dest.glob("*.json")):
 try:s=json.loads(p.read_text(encoding="utf8"))
 except Exception as e:proof["errors"].append(str(e));continue
 if "Rows"not in s:continue
 check(len(s["Rows"])==len(s["Attributes"])==s["Height"],"row/attribute height mismatch")
 check(all(len(r)==len(at)==s["Width"]for r,at in zip(s["Rows"],s["Attributes"])),"cell width mismatch")
 proof["snapshots"][p.stem]={"cells_SHA256":sha(p),"width":s["Width"],"height":s["Height"]}
 if a.renderer:
  rr=subprocess.run([sys.executable,str(a.renderer),str(p),str(p.with_suffix(".png"))],
   stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
  check(rr.returncode==0,"renderer failed "+p.name)
  if p.with_suffix(".png").exists():proof["snapshots"][p.stem]["PNG_SHA256"]=sha(p.with_suffix(".png"))
first=dest/"01-protocol-fields.json"
if first.exists():
 s=json.loads(first.read_text());text="\n".join(s["Rows"])
 check(" EOF "in s["Rows"][0],"not an actual completed replay")
 check(query in s["Rows"][1] and "▏"not in s["Rows"][1],"field query was not committed")
 check(bool(re.search(r"buffer\s+1865\s*/",text)),"not all1865 source records drained")
 check(bool(re.search(r"· 1 visible(?:\s|╮)",text)),"must have exactly one visible row")
 selected=[r for r in s["Rows"]if re.search(r"(?:^|\s)›294\s+\S+\s+HTTP/3(?:\s|$)",r)]
 check(len(selected)==1,"selected physical294/HTTP3 row missing or ambiguous")
 check("Field filter partial: incomplete field evidence"in text,"partial filter notice missing")
 check("retained history"in text,"history uncertainty must stay explicit")
 check(field in text,"PDU detail unavailability is not displayed")
 check("#294"in text,"detail frame caption changed")
 check(bool(re.search(r"2 HTTP/3 FIELDS(?:\s|─)",text)),"observed HTTP3 identity lost from field-pane title")
 check("PDU #0"not in text,"coverage placeholder was drawn as a fabricated PDU0")
 check(not re.search(r"\bMESSAGE\s*·\s*PDU",text),"retention diagnostic was drawn as a MESSAGE protocol")
 reason="Protocol detail absent or evicted from bounded message history"
 check(text.count(reason)==1,"missing-detail reason duplicated or absent")
 proof["selected_frame"]=294;proof["selected_PDU_query"]=299;proof["visible_rows"]=1
else:check(False,"no fresh first protocol-fields cell snapshot")
# An ID witness cannot manufacture authenticated plaintext bytes after eviction.
for name in ("02-first-byte-source","02-byte-sources"):
 p=dest/(name+".json")
 if p.exists():
  text="\n".join(json.loads(p.read_text())["Rows"])
  headers=[r for r in text.splitlines()if"3 "in r and("BYTES"in r or"·"in r)]
  check(not any("authenticated plaintext"in r for r in headers),"identity witness fabricated a plaintext byte source")
restored=dest/"restored.json"
if restored.exists():
 proof["restored"]=json.loads(restored.read_text())
 check(proof["restored"]=={"error":"<nil>","input_cp":936,"output_cp":936},"console codepage/exit state not restored")
else:check(False,"no codepage restoration receipt")
check(run.returncode==0,"native helper failed; preserve its logs and cells")
proof["passed"]=not proof["errors"]
(a.out/"proof.json").write_text(json.dumps(proof,indent=2,ensure_ascii=False)+"\n",encoding="utf8")
print(json.dumps({"proof":str(a.out/"proof.json"),"SHA256":sha(a.out/"proof.json"),"passed":proof["passed"],"errors":proof["errors"]},ensure_ascii=False))
assert proof["passed"],proof["errors"]
