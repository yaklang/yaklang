"""Emit only public numeric/name metadata from frozen official-source tables."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent;repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
tables=[('gtpv2MessageNames',root/'GTPv2-boundary-source43-01/message-type-names.json','efaf9934beb6810930dedd7c46cd2116ec6de118ae01ce92db5998353b308029'),('gtpv2IENames',root/'GTPv2-IE-names43-01/IE-type-names.json','217f6fc772b7e2607a70d800604436c7fc1df032589683f5b9fd65b42d24bbcf')]
lines=['package stream_parser','','import ("fmt"; "strings")','','// Public numeric/name metadata from pinned Wireshark v4.6.9 tables.','// Original source hashes and extraction receipts remain outside the repository.']
for symbol,path,digest in tables:
 assert hashlib.sha256(path.read_bytes()).hexdigest()==digest
 names=json.loads(path.read_text('utf8'))['names'];lines.append('var '+symbol+' = map[uint64]string{')
 for key,value in sorted(names.items(),key=lambda r:int(r[0])):
  assert str(int(key))==key and 0<=int(key)<=255 and type(value)is str and len(value)<200
  lines.append('\t'+key+': '+json.dumps(value,ensure_ascii=True)+',')
 lines.extend(['}',''])
lines.extend(['// Whitespace in the original display labels is trimmed explicitly; wire values stay literal.','func GTPv2MessageName(typ uint64) string { if label, ok := gtpv2MessageNames[typ]; ok { return strings.TrimSpace(label) }; return fmt.Sprintf("Unselected message type %d", typ) }','func GTPv2IEName(typ uint64) string { if label, ok := gtpv2IENames[typ]; ok { return strings.TrimSpace(label) }; return fmt.Sprintf("Unselected IE type %d", typ) }',''])
target=repo/'common/bin-parser/parser/stream_parser/gtpv2_names.go';assert not target.exists();target.write_text('\n'.join(lines),encoding='utf8');print(json.dumps(dict(output=str(target),message_names=102,IE_names=168)))
