"""Prepare only: original full-Weber Windows ESP NAT-T source/field witness."""
from pathlib import Path
import base64,hashlib,json
root=Path(__file__).resolve().parent;phase=root.parent
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb') as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
native_plan=phase/'ESP-NATT-whole-plan-before-01/frozen01/seven-complete-native-expectations.json'
source=next(r for r in json.loads(native_plan.read_text()) if r['frame']==46978)
raw=Path(source['frame_wire_file']).read_bytes();wire=base64.b64decode(source['ESP_wire_base64'],validate=True)
assert len(raw)==138 and source['ESP_wire_range']==[42,138] and wire==raw[42:138] and len(wire)==96
assert wire[:4].hex()=='f487bfcf' and wire[4:8].hex()=='00000002'
helper=phase/'windows-PIM-helper26/native-console.exe';helper_receipt=helper.parent/'receipt.json';hr=json.loads(helper_receipt.read_text())
assert sha(helper)==hr['binary_SHA256']=='d18d9f2ae0eb965d5c1cebdbb2de3f93ef189f96c7dc0186dfc7ab20bcd7bb5f'
old_driver=phase/'windows-replay-parameterized-01/windows_console_qa.py'
renderer=phase/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
capture=Path(r'C:\tmp\yak-shark-public-corpus-20261024\weber-20260710\The-Ultimate-PCAP.pcapng');keylog=capture.with_name('tls.keys')
assert sha(capture)=='ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4'
case=dict(capture=str(capture),capture_SHA256=sha(capture),keys=str(keylog),keylog_SHA256=sha(keylog),
 physical_packets=51328,packets=20000,default_packet_cache=20000,default_protocol_history_mib=64,
 default_protocol_message_capacity=80000,pdu=37903,physical_frame=46978,protocol='ESP',field='SPI: 4102537167',
 select_field='SPI',select_field_value='4102537167',query='fields pdu.id == 37903',
 progressive_stack=['Ethernet','IPv4','UDP','ESP'],physical_layers=['Ethernet','IPv4','UDP'],
 ESP_payload_physical_range=[42,138],ESP_payload_bytes=96,ESP_payload_SHA256=hashlib.sha256(wire).hexdigest(),
 physical_frame_SHA256=hashlib.sha256(raw).hexdigest(),SPI_range=[0,4],SPI_wire='f487bfcf',Sequence_range=[4,8],Sequence_wire='00000002',
 physical_frame_file=source['frame_wire_file'],full_native_expectation=source['expected_native_event'],
 native_plan_file=str(native_plan),native_plan_SHA256=sha(native_plan),
 detail_must_be_retained=True,no_implicit_history_budget_increase=True,
 EOF_packet_window_contract='51328 physical records replayed; only last20000 retained packet rows. Target46978 remains in that window.',
 source_cycle_contract='initial Source0=138 frame bytes; selected SPI Source1=96 ESP bytes; b then Source0, b then Source1')
(root/'cases.json').write_text(json.dumps({'ESP-NATT-Weber46978':case},indent=2,ensure_ascii=False)+'\n',encoding='utf8')
plan=dict(scope=__doc__,prepared_only=True,no_Go_or_native_execution=True,
 helper=str(helper),helper_SHA256=sha(helper),helper_receipt=str(helper_receipt),helper_receipt_SHA256=sha(helper_receipt),
 helper_source_file_SHA256=hr['source_file_SHA256'],helper_build_source=hr['source'],
 base_parameter_driver=str(old_driver),base_parameter_driver_SHA256=sha(old_driver),
 renderer=str(renderer),renderer_SHA256=sha(renderer),case='ESP-NATT-Weber46978',case_file='cases.json',
 case_file_SHA256=sha(root/'cases.json'),execution='run_windows_NATT.py --bins ACTUAL_BIN_DIR --source ACTUAL_FULL_COMMIT --out NEW_OUTPUT_DIR',
 source_field_mapping='SPI physical42..46 equals PDU0..4; sequence physical46..50 equals PDU4..8. This does not claim SA/decryption or field offsets on ciphertext internals.',
 history_preflight='No native budget execution performed by child. Default64MiB target detail availability must actually pass fresh console checks; absence cannot produce a success certificate.',
 old_tail_observation='Closed SDK30 events37903..41400 contain3498 objects and12585891JSON bytes; this is not an exact RetainedBytes/RSS bound.',
 generated_plan_constructor_SHA256=sha(Path(__file__)))
(root/'plan.json').write_text(json.dumps(plan,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
snap=root/'baseline-source';snap.mkdir(exist_ok=False)
for name,path in [('parameter-driver.txt',old_driver),('helper-receipt.json',helper_receipt),('renderer.txt',renderer)]:
 (snap/name).write_bytes(path.read_bytes())
print(json.dumps(dict(plan=str(root/'plan.json'),SHA256=sha(root/'plan.json'),case_SHA256=sha(root/'cases.json'))))
