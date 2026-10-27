"""Capture read-only RPC endpoint-map inquiries made by the Windows RPC DLL.

Usage: dcerpc_windows_query_live.py OUTPUT_DIR TSHARK
Force authentication off before querying local TCP/135. Never register/delete
endpoints, use credentials, request annotations or invoke discovered services.
API receipts count library operations; they do not invent one wire PDU per call.
"""
import ctypes as C
import hashlib
import json
import sys
import time
from pathlib import Path

from capture_ready import start_capture

out=Path(sys.argv[1]).resolve()
assert not any((p/'.git').exists()for p in[out,*out.parents])
out.mkdir(exist_ok=False,parents=True)
DWORD=C.c_uint32;HANDLE=C.c_void_p

class GUID(C.Structure):
    _fields_=[('Data1',DWORD),('Data2',C.c_uint16),('Data3',C.c_uint16),('Data4',C.c_ubyte*8)]
class IFID(C.Structure):
    _fields_=[('Uuid',GUID),('VersMajor',C.c_uint16),('VersMinor',C.c_uint16)]
assert C.sizeof(GUID)==16 and C.sizeof(IFID)==20
rpc=C.WinDLL('Rpcrt4.dll')
signatures={
    'RpcBindingFromStringBindingW':[C.c_wchar_p,C.POINTER(HANDLE)],
    'RpcBindingSetAuthInfoW':[HANDLE,C.c_wchar_p,DWORD,DWORD,HANDLE,DWORD],
    'RpcMgmtSetComTimeout':[HANDLE,DWORD],
    'RpcMgmtEpEltInqBegin':[HANDLE,DWORD,C.POINTER(IFID),DWORD,C.POINTER(GUID),C.POINTER(HANDLE)],
    'RpcMgmtEpEltInqNextW':[HANDLE,C.POINTER(IFID),C.POINTER(HANDLE),C.POINTER(GUID),C.POINTER(C.c_wchar_p)],
    'RpcMgmtEpEltInqDone':[C.POINTER(HANDLE)],
    'RpcBindingFree':[C.POINTER(HANDLE)],
}
for name,args in signatures.items():
    function=getattr(rpc,name);function.argtypes=args;function.restype=DWORD
receipts=[];failures=[]

def record(round_number,operation,status,**facts):
    receipts.append(dict(round=round_number,operation=operation,status=int(status),time_ns=time.time_ns(),**facts))
    assert status==0,(operation,status)

cap,marker=start_capture(sys.argv[2],out/'live.pcap','tcp port 135',18)
try:
    for round_number in range(3):
        binding=HANDLE();context=HANDLE()
        record(round_number,'RpcBindingFromStringBindingW',rpc.RpcBindingFromStringBindingW('ncacn_ip_tcp:127.0.0.1[135]',C.byref(binding)))
        try:
            # RPC_C_AUTHN_LEVEL_NONE=1, RPC_C_AUTHN_NONE=0, RPC_C_AUTHZ_NONE=0.
            record(round_number,'RpcBindingSetAuthInfoW',rpc.RpcBindingSetAuthInfoW(binding,None,1,0,None,0),authentication='explicitly disabled')
            record(round_number,'RpcMgmtSetComTimeout',rpc.RpcMgmtSetComTimeout(binding,3))
            # RPC_C_EP_ALL_ELTS=0; version/interface/object selectors are ignored.
            record(round_number,'RpcMgmtEpEltInqBegin',rpc.RpcMgmtEpEltInqBegin(binding,0,None,1,None,C.byref(context)))
            try:
                for index in range(8):
                    interface=IFID()
                    status=rpc.RpcMgmtEpEltInqNextW(context,C.byref(interface),None,None,None)
                    record(round_number,'RpcMgmtEpEltInqNextW',status,index=index,interface_bytes_sha256=hashlib.sha256(bytes(interface)).hexdigest())
            finally:
                if context.value:record(round_number,'RpcMgmtEpEltInqDone',rpc.RpcMgmtEpEltInqDone(C.byref(context)))
        finally:
            if binding.value:record(round_number,'RpcBindingFree',rpc.RpcBindingFree(C.byref(binding)))
        time.sleep(.15)
except Exception as exc:
    failures.append(type(exc).__name__+': '+str(exc));raise
finally:
    _,stderr=cap.communicate(timeout=30);(out/'capture.stderr').write_bytes(stderr)
    (out/'API-receipts.json').write_text(json.dumps(receipts,indent=2)+'\n',encoding='utf8')
    manifest=dict(generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),capture_sha256=hashlib.sha256((out/'live.pcap').read_bytes()).hexdigest(),marker_port=marker,client='Windows Rpcrt4.dll through independent Python ctypes declarations',target='actual local Windows endpoint mapper over TCP/135',binding_rounds=3,inquiry_next_operations=sum(r['operation']=='RpcMgmtEpEltInqNextW'for r in receipts),API_receipts=len(receipts),authentication='explicit RPC_C_AUTHN_NONE before inquiries',endpoint_registration_or_service_invocation=False,failures=failures)
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
assert cap.returncode==0 and not failures
print(json.dumps(dict(inquiry_next_operations=manifest['inquiry_next_operations'],API_receipts=len(receipts),capture_sha256=manifest['capture_sha256'])))
