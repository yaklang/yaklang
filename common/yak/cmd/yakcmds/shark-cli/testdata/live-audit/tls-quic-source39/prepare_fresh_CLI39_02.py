"""Correct only a public transport-name assumption against immutable API code."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
path='common/yak/cmd/yakcmds/shark-cli/decode.go'
def blob(rev):return subprocess.check_output(['git','show',rev+':'+path],cwd=repo)
old=blob('68527c4d9355d2df37c5c0c9b8bc19f7766ca9c6');current=blob('d6772965c669e288705f7e0d0e25c62972226832')
assert old==current and b'transport = "tcp"'in old and b'transport = "udp"'in old and b'row.Transport, row.SourcePort, row.DestinationPort = transport'in old
before=root/'audit_fresh_CLI39.py';after=root/'audit_fresh_CLI39_02.py';assert not after.exists()
s=before.read_text('utf8');anchor='destination_port=dport,transport=transport,layers='
assert s.count(anchor)==1;s=s.replace(anchor,'destination_port=dport,transport=transport.lower(),layers=')
assert s.count("p['transport']=='UDP'")==1;s=s.replace("p['transport']=='UDP'","p['transport']=='udp'")
after.write_text(s,encoding='utf8')
proof=dict(scope=__doc__,historical_failure='Source reader incorrectly expected uppercase transport JSON enum;no protocol values/ranges/labels changed.',old_auditor_SHA256=hashlib.sha256(before.read_bytes()).hexdigest(),new_auditor_SHA256=hashlib.sha256(after.read_bytes()).hexdigest(),old_and_current_API_source_equal=True,immutable_API_SHA256=hashlib.sha256(old).hexdigest(),case_values_from_candidate_used=False)
(root/'fresh-CLI-source39-audit01/failure-policy-bridge.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(proof))
