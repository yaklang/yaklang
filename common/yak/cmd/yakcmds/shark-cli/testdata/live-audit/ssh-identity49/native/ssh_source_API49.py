"""Independent frozen-source SSH -> existing API naming bridge. No product import."""
import base64
import hashlib
import struct

SPECS = [
    ("ssh.protocol", "Identification", "context", "string", 0),
    ("ssh.message_code", "Message Number", "pdu", "unsigned", 8),
    ("ssh.kex_algorithms", "Kex Algorithms", "pdu", "string", 0),
    ("ssh.server_host_key_algorithms", "Host Key Algorithms", "pdu", "string", 0),
    ("ssh.negotiation.context", "Negotiation Context", "context", "string", 0),
    ("ssh.identification.conformance", "Identification Conformance", "context", "string", 0),
    ("ssh.identification.warning", "Identification Warning", "context", "string", 0),
    ("ssh.kex.selected", "Kex Algorithm", "context", "string", 0),
    ("ssh.host_key.selected", "Host Key Algorithm", "context", "string", 0),
    ("ssh.host_key.sha256", "Host Key Fingerprint", "derived", "string", 0),
    ("ssh.signature.verified", "Signature Verified", "derived", "boolean", 0),
    ("ssh.peer_identity.verified", "Peer Identity Verified", "derived", "boolean", 0),
]
LISTS = ["Kex Algorithms", "Host Key Algorithms", "Encryption C2S", "Encryption S2C",
         "MAC C2S", "MAC S2C", "Compression C2S", "Compression S2C", "Languages C2S", "Languages S2C"]

def b64(w):
    return base64.b64encode(w).decode()
def dg(w):
    return hashlib.sha256(w).hexdigest()

class Reader:
    def __init__(self, raw, start, end, owner, leaves):
        self.raw, self.at, self.end = raw,start,end
        self.owner,self.leaves = list(owner),leaves
    def take(self,name,n,kind="bytes"):
        assert 0<=n<=self.end-self.at,(name,"parent boundary")
        start=self.at;self.at+=n;raw=self.raw[start:self.at]
        value=int.from_bytes(raw,"big") if kind=="unsigned" else raw.decode("ascii") if kind=="string" else b64(raw)
        self.leaves.append({"API_path":self.owner+[name],"span":[start,self.at],"value":value,"kind":kind})
        return value,raw
    def uint(self,name,n):
        return self.take(name,n,"unsigned")[0]
    def string(self,name,kind="bytes"):
        n=self.uint(name+" Length",4)
        value,raw=self.take(name,n,kind)
        return value,raw
    def nested(self,name,decoder):
        n=self.uint(name+" Length",4)
        assert n<=self.end-self.at
        start=self.at;self.at+=n
        child=Reader(self.raw,start,self.at,self.owner+[name],self.leaves)
        result=decoder(child);child.done()
        return n,result,self.raw[start:self.at]
    def done(self):
        assert self.at==self.end,(self.owner,self.at,self.end)

def point(r,name):
    def decode(c):
        out={name+" Format":c.uint(name+" Format",1)}
        assert out[name+" Format"]==4
        for axis in ("X","Y"):
            out[name+" "+axis]=c.take(name+" "+axis,32)[0]
        return out
    n,value,wire=r.nested(name,decode)
    assert n==65
    return {name+" Length":n,name:value}

def hostkey(r):
    n,value,wire=r.nested("Host Key",lambda c:{
        "Host Key Format Length":c.uint("Host Key Format Length",4),
        # Fixed source public format avoids dependent rewinds or product helpers.
        "Host Key Format":c.take("Host Key Format",11,"string")[0],
        "Ed25519 Public Key Length":c.uint("Ed25519 Public Key Length",4),
        "Ed25519 Public Key":c.take("Ed25519 Public Key",32)[0]})
    assert value["Host Key Format Length"]==11 and value["Host Key Format"]=="ssh-ed25519" and value["Ed25519 Public Key Length"]==32
    return {"Host Key Length":n,"Host Key":value},wire

def signature(r):
    n,value,wire=r.nested("Signature",lambda c:{
        "Signature Format Length":c.uint("Signature Format Length",4),
        "Signature Format":c.take("Signature Format",11,"string")[0],
        "Ed25519 Signature Bytes Length":c.uint("Ed25519 Signature Bytes Length",4),
        "Ed25519 Signature Bytes":c.take("Ed25519 Signature Bytes",64)[0]})
    assert value["Signature Format Length"]==11 and value["Signature Format"]=="ssh-ed25519" and value["Ed25519 Signature Bytes Length"]==64
    return {"Signature Length":n,"Signature":value}

def plain_fields(raw, profile, ignored=False):
    """Return API tree and independent source ranges, never insert new API WR."""
    assert len(raw)>=16 and len(raw)%8==0
    n=int.from_bytes(raw[:4],"big");pad=raw[4]
    assert n+4==len(raw) and 4<=pad<=n-2
    end=len(raw)-pad;leaves=[]
    h=Reader(raw,0,5,[],leaves)
    tree={"Packet Length":h.uint("Packet Length",4),"Padding Length":h.uint("Padding Length",1)}
    h.done()
    r=Reader(raw,5,end,["Payload"],leaves);msg=r.uint("Message Number",1)
    p={"Message Number":msg};host=None
    def s(name,typ="bytes"):
        value,wire=r.string(name,typ)
        p[name+" Length"]=len(wire);p[name]=value
    if msg==20:
        p["Cookie"]=r.take("Cookie",16)[0]
        for name in LISTS:s(name,"string")
        p["First Kex Packet Follows"]=r.uint("First Kex Packet Follows",1)
        p["Reserved"]=r.uint("Reserved",4)
    elif msg==21:
        pass
    elif not ignored and profile=="dh-gex" and msg==34:
        for name in ("Minimum Group Bits","Preferred Group Bits","Maximum Group Bits"):p[name]=r.uint(name,4)
    elif not ignored and profile=="dh" and msg==30:
        s("Client Exchange Value")
    elif not ignored and profile=="ecdh-curve25519" and msg==30:
        s("Client Curve25519 Public Key");assert p["Client Curve25519 Public Key Length"]==32
    elif not ignored and profile=="ecdh-nistp256" and msg==30:
        p.update(point(r,"Client Point"))
    elif not ignored and profile in ("dh","ecdh-curve25519","ecdh-nistp256") and msg==31:
        value,host=hostkey(r);p.update(value)
        if profile=="dh":s("Server Exchange Value")
        elif profile=="ecdh-curve25519":s("Server Curve25519 Public Key")
        else:p.update(point(r,"Server Point"))
        p.update(signature(r))
    else:
        p["Opaque Message Data"]=r.take("Opaque Message Data",r.end-r.at)[0]
    r.done();tree["Payload"]=p
    t=Reader(raw,end,len(raw),[],leaves);tree["Padding"]=t.take("Padding",pad)[0];t.done()
    return tree,leaves,host

def selected(case):
    return {"Kex Algorithm":case["selected_literal_KEX"],"Host Key Algorithm":"ssh-ed25519",
            "Encryption C2S":"aes128-ctr","Encryption S2C":"aes128-ctr",
            "MAC C2S":"hmac-sha2-256","MAC S2C":"hmac-sha2-256",
            "Compression C2S":"none","Compression S2C":"none"}

def family(profile):
    return "ECDH" if profile and profile.startswith("ecdh") else "DH" if profile in ("dh","dh-gex") else ""

def message_name(msg,profile):
    if msg==20:return "KEXINIT"
    if msg==21:return "NEWKEYS"
    if profile=="dh-gex":
        return {30:"KEX_DH_GEX_REQUEST_OLD",31:"KEX_DH_GEX_GROUP",32:"KEX_DH_GEX_INIT",33:"KEX_DH_GEX_REPLY",34:"KEX_DH_GEX_REQUEST"}.get(msg,f"KEY_EXCHANGE_{msg} (layout unconfirmed)")
    if profile and profile.startswith("ecdh") and msg in (30,31):return "KEX_ECDH_INIT" if msg==30 else "KEX_ECDH_REPLY"
    if profile=="dh" and msg in (30,31):return "KEXDH_INIT" if msg==30 else "KEXDH_REPLY"
    return f"KEY_EXCHANGE_{msg} (layout unconfirmed)"

def private_aliases(session):
    out=[]
    for name,key,origin,typ,bits in SPECS:
        if session is not None and key in session and session[key] not in (None,""):
            out.append({"name":name,"value":session[key],"origin":origin})
    # Existing Session exposes no WR for SSH, so no pdu_range is manufactured.
    return sorted(out,key=lambda x:x["name"])

def secure_summary(session):
    parts=[session["Packet Name"]]
    for key in ("Identification Warning","Negotiation Context","Kex Algorithm","Host Key Algorithm","Host Key Fingerprint","Identification"):
        if session.get(key):parts.append(session[key])
    if session.get("Encrypted") is True:parts.append("encrypted transport; plaintext unavailable")
    return " · ".join(parts)

def case_expected(case):
    """Mechanical naming bridge over the already frozen exact method selection."""
    role_ident={s["role"]:__import__("pathlib").Path(s["input_path"]).read_bytes().rstrip(b"\r\n").decode("ascii")
                for s in case["steps"] if s["kind"]=="identification"}
    have=set();newkeys=set();out=[]
    for step in case["steps"]:
        if step["kind"]=="tcp-control":continue
        raw=__import__("pathlib").Path(step["input_path"]).read_bytes()
        row={"sourcecase":case["sourcecase"],"frame":step["frame"],"kind":step["kind"],"role":step["role"],
             "raw_path":step["input_path"],"raw_SHA256":dg(raw),"raw_length":len(raw),
             "source_fields_from_plan01":step.get("source_fields",[])}
        if step["kind"]=="identification":
            session={"Packet Name":"identification","Identification":role_ident[step["role"]],"Version":"2.0","Context Level":"observed"}
            fields=session.copy();leaves=[]
        elif step["kind"]=="protected-phase-sentinel":
            assert step["role"] in newkeys
            row.update(expected_status="context-required",expected_completeness="context-required",
                       expected_entry="",session=None,fields=None,API_wire_leaves=[],private_aliases=[],
                       fields_error_required=True,expert="Encrypted",CLI_protocol="SSH",CLI_status="encrypted",
                       CLI_visibility="encrypted",CLI_scope="connection envelope",
                       CLI_info="SSH encrypted transport · NEWKEYS observed; plaintext unavailable")
            out.append(row);continue
        else:
            profile=case["selected_method_profile"]
            fields,leaves,host=plain_fields(raw,profile,step.get("ignored_guess",False))
            msg=fields["Payload"]["Message Number"]
            session={"Packet Name":message_name(msg,profile),"Message Number":msg,"Context Level":"observed","Identification":role_ident[step["role"]]}
            if msg!=20:session.update(selected(case))
            if step.get("ignored_guess"):
                session["Packet Name"]="Ignored guessed key-exchange packet"
                session["Payload Interpretation"]="discarded after algorithm mismatch"
            elif msg==20:
                p=fields["Payload"]
                session.update({"Kex Algorithms":p["Kex Algorithms"],"Host Key Algorithms":p["Host Key Algorithms"]})
                for key in ("Encryption C2S","Encryption S2C","MAC C2S","MAC S2C","Compression C2S","Compression S2C"):
                    session["Offered "+key]=p[key]
                for key in ("Languages C2S","Languages S2C","First Kex Packet Follows","Reserved"):session[key]=p[key]
                have.add(step["role"])
                if len(have)==2:session.update(selected(case));session["Kex Family"]=family(profile)
            elif msg==21:
                newkeys.add(step["role"])
                if len(newkeys)==2:session["Encrypted"]=True;session["Protocol Transition"]="ssh->encrypted"
            elif msg==30:
                session["Kex Family"]=family(profile)
            elif msg in (31,33):
                session["Kex Family"]=family(profile)
                if host is not None:
                    digest=hashlib.sha256(host).digest()
                    session.update({"Host Key Format":"ssh-ed25519","Host Key Fingerprint":"SHA256:"+base64.b64encode(digest).decode().rstrip("="),
                                    "Host Key SHA256":digest.hex(),"Signature Verified":False,"Peer Identity Verified":False})
            if not step.get("ignored_guess") and 30<=msg<=34 and profile is not None:
                session["Initial Packet Fields"]=fields
                session["Signature Verified"]=session["Peer Identity Verified"]=False
        row.update(expected_status="decoded",expected_completeness="envelope",
                   expected_entry="SSHIdentification" if step["kind"]=="identification" else "SSHPacket",
                   session=session,fields=fields,API_wire_leaves=leaves,private_aliases=private_aliases(session),
                   fields_error_required=False,expert="",CLI_protocol="SSH",CLI_status="decoded",
                   CLI_visibility="reassembled",CLI_scope="reassembled message",CLI_info=secure_summary(session))
        out.append(row)
    return out

