"""Check native semantics against this run's independent application hashes.

Usage: verify.py CAPTURE_DIR (after compare.py). No expected output is taken
from Yak's own decoder. This audit intentionally exports test-owned plaintext.
"""
import base64, collections, csv, hashlib, json, struct, sys
from pathlib import Path

cap = Path(sys.argv[1])
m = json.loads((cap/'manifest.json').read_text())
events = [json.loads(x) for x in (cap/'comparison/events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
csv.field_size_limit(8000000)
ws = list(csv.DictReader((cap/'comparison/tshark.tsv').open(encoding='utf-8-sig'), delimiter='\t', quoting=csv.QUOTE_NONE))
b64 = lambda x: base64.b64decode(x or '')
sha = lambda x: hashlib.sha256(x).hexdigest()
checks = {}
def select(proto): return [e for e in events if e['protocol'] == proto and e['status'] == 'decoded']
def expected(proto, kind=None):
    return collections.Counter(o['Hash'] for o in m['Operations'] if o['Protocol']==proto and (kind is None or o['Kind']==kind))
def equal(name, actual, oracle):
    actual=collections.Counter(actual)
    assert actual==oracle, (name,actual,oracle)
    checks[name]=sum(actual.values())
def raw(e): return b64(e['raw'])

groups=collections.defaultdict(list)
for e in select('ftp-data'): groups[e['flow']].append(e)
ftp_hashes=collections.Counter()
for flow, chunks in groups.items():
    chunks.sort(key=lambda e:e['session']['Stream Offset']); data=bytearray()
    for e in chunks:
        assert len(data)==e['session']['Stream Offset']
        assert (e['source_bytes']['Kind']=='decrypted')==e['session']['Payload Decrypted']
        data+=raw(e)
    ftp_hashes[sha(data)]+=1
equal('FTP and FTPS complete data streams',ftp_hashes,expected('ftp')+expected('ftps'))
equal('MQTT publish and delivery', [sha(b64(e['session']['Payload'])) for e in select('mqtt') if e['session']['Packet Name']=='PUBLISH'],expected('mqtt'))
equal('WebSocket decompressed binary echo',[sha(b64(e['session']['Application'])) for e in select('websocket') if e['session']['Opcode']==2], expected('websocket')+expected('websocket'))
equal('SMTP plain and STARTTLS bodies',[sha(raw(e)[:-3]) for e in select('smtp') if e['session']['Packet Name']=='DATA Body'],expected('smtp'))
equal('POP3 RETR bodies',[sha(raw(e)[:-3]) for e in select('pop3') if e['session'].get('Packet Name')=='Multiline' and e['session'].get('In Reply To')=='RETR'],expected('pop3'))
equal('IMAP literals',[sha(raw(e)) for e in select('imap') if e['session']['Packet Name']=='Literal'],expected('imap'))
equal('PostgreSQL rows',[sha(b64(v['Value'])) for e in select('postgresql') if e['session'].get('Message Name')=='DataRow' for v in e['session']['Values']],expected('postgresql'))
equal('MySQL text result rows',[sha(b64(r['Column Value'])) for e in select('mysql') if e['entry']=='MySQLTextResultSet41Fields' for r in e['fields']['Rows']],expected('mysql'))
equal('LDAP cn values',[sha(b64(v['Attribute Value'])) for e in select('ldap') if e['session']['Message Name']=='SearchResultEntry' for a in e['fields']['LDAPMessage']['SearchResultEntry']['Attributes Encoding']['Attributes'] if a['Attribute Description Encoding']['Attribute Description']=='cn' for v in a['Values Encoding']['Values']],expected('ldap'))
equal('SNMP response varbind values',[sha(b64(v['Value'])) for e in select('snmp') if e['session']['Packet Name']=='Response' for v in e['session']['Variable Bindings']],expected('snmp'))
smb_files=collections.defaultdict(bytearray)
for e in select('smb2'):
    s=e['session']
    if s['Packet Name']=='READ' and s['Response'] and s['Status']==0:
        assert s['Association Status']=='matched' and s['File Context']=='payload.bin'
        w=raw(e); offset=w[70];length=struct.unpack_from('<I',w,72)[0]
        smb_files[e['flow']]+=w[4+offset:4+offset+length]
equal('SMB READ files',[sha(v) for v in smb_files.values()],expected('smb2','read-file'))
ntp=select('ntp'); assert len(ntp)==2
assert ntp[0]['session']['Transmit Timestamp']==ntp[1]['session']['Origin Timestamp']
checks['NTP origin timestamp matches request']=1
equal('DHCP observed replies',[sha(raw(e)) for e in select('dhcp') if e['session']['Message Type'] in (2,5)],expected('dhcp'))

# Compare protocol IDs from the independent dissector per contributing frame.
for proto, field, session_field in [('ldap','ldap.messageID','Message ID'),('snmp','snmp.request_id','Request ID'),('smb2','smb2.msg_id','Message ID')]:
    checked=0
    for e in select(proto):
        vals=set()
        for ref in e['source_bytes'].get('PacketRefs') or []:
            vals.update(ws[ref['Number']-1].get(field,'').split(','))
        assert str(e['session'][session_field]) in vals,(proto,e['id'],vals)
        checked+=1
    checks[proto+' IDs agree with Wireshark per source frame']=checked
assert not [e for e in events if e['status'] not in ('decoded','deferred') and not (e['protocol']=='ssh' and e['expert']=='Encrypted')]
assert all(e['session'].get('Authentication Verified') is True for e in select('tls') if e['session'].get('Payload Decrypted') is True)
(cap/'comparison/semantic-verification.json').write_text(json.dumps(checks,indent=2))
print(json.dumps(checks,indent=2))
