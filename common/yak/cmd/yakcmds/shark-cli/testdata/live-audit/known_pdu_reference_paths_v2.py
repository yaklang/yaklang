"""Two public SDK schemas: generated transaction/media/FTP references included.

The v1 helper remains frozen with its narrow historical certificate. Every
extra path here has an explicit writer, protocol guard and observed schema.
No recursive integer/name matching or endpoint/time matching is performed.
"""
from pathlib import Path
import hashlib,importlib.util

OLD=Path(__file__).with_name('known_pdu_reference_paths.py')
assert hashlib.sha256(OLD.read_bytes()).hexdigest()=='3d895308ab555eb0dcebe821f9f0ea7fba4fc266b133d76dc8ebfb166a38d8b6'
spec=importlib.util.spec_from_file_location('explicit_refs_v1',OLD);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)

# Each writer assigns e.ID/an observed request or command's native PDU ID.
# Protocol wire transaction IDs stay in their literal fields/session trees.
TRANSACTION_PDU_PROTOCOLS=frozenset(['http','ipp','dhcp','dhcpv6','dns','llmnr','ntp','imap','ftp-data','radius','bfd'])

def remap_known_pdu_references(event,mapping):
 out=old.remap_known_pdu_references(event,mapping)
 protocol=event['protocol']
 def one(value,path):
  assert type(value) is int and value>=0,path
  if value==0:return 0
  assert value in mapping,(path,'unmapped generated PDU',value)
  result=mapping[value];assert type(result) is int and result>0,path
  return result
 if 'transaction' in event:assert type(event['transaction']) is int and event['transaction']>=0,'transaction'
 if event.get('transaction',0)!=0:
  assert protocol in TRANSACTION_PDU_PROTOCOLS,('transaction meaning not admitted by this two-public-input schema',protocol)
  out['transaction']=one(event['transaction'],'transaction/'+protocol)
 for root in ['fields','session']:
  node=out.get(root)
  if not isinstance(node,dict):continue
  if protocol in ['ftp','ftp-data'] and 'Data Transfer ID' in node:
   node['Data Transfer ID']=one(node['Data Transfer ID'],root+'/Data Transfer ID')
  if protocol=='rtp' and 'SDP Event IDs' in node:
   assert isinstance(node['SDP Event IDs'],list)
   node['SDP Event IDs']=[one(v,root+'/SDP Event IDs') for v in node['SDP Event IDs']]
  if protocol=='rtsp' and 'RTSP SETUP Request Event IDs' in node:
   assert isinstance(node['RTSP SETUP Request Event IDs'],list)
   node['RTSP SETUP Request Event IDs']=[one(v,root+'/RTSP SETUP Request Event IDs') for v in node['RTSP SETUP Request Event IDs']]
 return out
