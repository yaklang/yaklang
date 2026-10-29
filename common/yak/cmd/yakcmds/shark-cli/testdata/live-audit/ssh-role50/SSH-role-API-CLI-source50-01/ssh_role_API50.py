"""Source50 -> existing SSH API labels, independently decode bounded GEX values."""
import hashlib
import importlib.util
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
BASE=ROOT/"SSH-SDK-CLI-source49-01"/"ssh_source_API49.py"
assert hashlib.sha256(BASE.read_bytes()).hexdigest()=="061ba6afdb24cd78c07302b89c406d3d507889c9f78e851258d1418de5a3ff68"
spec=importlib.util.spec_from_file_location("frozen_source_SSH_API49",BASE)
base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)
SPECS=base.SPECS
def fields(raw,profile):
    msg=raw[5]
    if profile!="dh-gex"or msg not in(30,31,32,33):return base.plain_fields(raw,profile)
    assert len(raw)>=16 and len(raw)%8==0 and int.from_bytes(raw[:4],"big")+4==len(raw)
    pad=raw[4];end=len(raw)-pad;leaves=[]
    h=base.Reader(raw,0,5,[],leaves)
    tree={"Packet Length":h.uint("Packet Length",4),"Padding Length":h.uint("Padding Length",1)};h.done()
    r=base.Reader(raw,5,end,["Payload"],leaves)
    p={"Message Number":r.uint("Message Number",1)};host=None
    def mp(name):
        value,w=r.string(name)
        assert not w or not w[0]&128 and(w[0]!=0 or len(w)>1 and w[1]&128),"canonical nonnegative source mpint"
        p[name+" Length"]=len(w);p[name]=value
    if msg==30:p["Preferred Group Bits"]=r.uint("Preferred Group Bits",4)
    elif msg==31:mp("Group Prime");mp("Group Generator")
    elif msg==32:mp("Client Exchange Value")
    elif msg==33:
        value,host=base.hostkey(r);p.update(value);mp("Server Exchange Value");p.update(base.signature(r))
    r.done();tree["Payload"]=p
    tail=base.Reader(raw,end,len(raw),[],leaves);tree["Padding"]=tail.take("Padding",pad)[0];tail.done()
    return tree,leaves,host

def source_case(c,physical):
    """Endpoint A is a client only when source SYN proves it; noSYN B is dir0."""
    known=c["role_evidence"]["kind"]=="observed-SYN-and-SYNACK"
    orientations=["A","B"]if known else[c["steps"][0]["endpoint"],"A"if c["steps"][0]["endpoint"]=="B"else"B"]
    idents={s["endpoint"]:Path(s["input_path"]).read_bytes().rstrip(b"\r\n").decode("ascii")for s in c["steps"]if s["kind"]=="identification"}
    have=set();nk=set();out=[]
    selected=base.selected({"selected_literal_KEX":c["method"]})
    for s in c["steps"]:
        if not s["payload_bytes"]:continue
        raw=Path(s["input_path"]).read_bytes();assert base.dg(raw)==s["payload_SHA256"]
        row={"sourcecase":c["sourcecase"],"frame":s["frame"],"kind":s["kind"],"endpoint":s["endpoint"],
             "source":s["source"],"destination":s["destination"],"expected_direction":orientations.index(s["endpoint"]),
             "raw_path":s["input_path"],"raw_SHA256":base.dg(raw),"raw_length":len(raw),"anchor":physical[s["frame"]-1],
             "source_role_assessment":s.get("assessment"),"role_proven_by_SYN":known,"fields_error_exact":None}
        if s["kind"]=="protected-sentinel":
            assert s["endpoint"]in nk
            row.update(expected_status="context-required",expected_completeness="context-required",expected_entry="",
                       session=None,fields=None,API_wire_leaves=[],private_aliases=[],fields_error_required=True,expert="Encrypted",
                       expected_summary=None,expected_error=None,CLI_protocol="SSH",CLI_status="encrypted",
                       CLI_visibility="encrypted",CLI_scope="connection envelope",
                       CLI_info_base="SSH encrypted transport · NEWKEYS observed; plaintext unavailable")
            out.append(row);continue
        if s["kind"]=="identification":
            session={"Packet Name":"identification","Identification":idents[s["endpoint"]],"Version":"2.0","Context Level":"observed"}
            tree=session.copy();leaves=[];host=None
        else:
            msg=raw[5];profile=c["profile"]if known and len(have)==2 else"transport"
            if profile is None:profile="transport"
            selected_now=selected if known and len(have)==2 else{}
            session={"Packet Name":base.message_name(msg,profile),"Message Number":msg,"Context Level":"observed","Identification":idents[s["endpoint"]]}
            if msg!=20:session.update(selected_now)
            if not known and(msg==20 or len(have)==2):
                session["Negotiation Context"]="client/server role not observed";session["Context Level"]="partial"
            if s.get("assessment")=="known-role-contradiction":
                # Only common framing is source-proven acceptable; publication
                # deliberately contains no successful standard method fields.
                fields(raw,"transport")
                expected=s["normative_sender_role_if_known_method"];observed=s["observed_client_server_role"]
                session["Packet Name"]=f"KEY_EXCHANGE_{msg} (unexpected sender)"
                session.update({"Expected Sender Role":expected,"Observed Sender Role":observed,
                     "Payload Interpretation":"not decoded: sender contradicts observed TCP role","Error Scope":"message"})
                cause=f"SSH {base.message_name(msg,c['profile'])} requires the observed {expected}, received from the observed {observed}"
                err="MalformedMessage: "+cause
                a=row["anchor"]
                tcp=f"{a['source_port']} \u2192 {a['destination_port']} [ACK,PSH] Seq={a['TCP_sequence']} Ack={a['TCP_ack']} Payload={a['TCP_payload_length']}"
                row.update(expected_status="malformed",expected_completeness="malformed",expected_entry="SSHPacket",
                     session=session,fields=None,API_wire_leaves=[],private_aliases=base.private_aliases(session),
                     fields_error_required=True,fields_error_exact=err,expert="MalformedMessage",expected_summary="ssh",expected_error=err,
                     CLI_protocol="SSH",CLI_status="malformed",CLI_visibility=None,CLI_scope="connection-associated diagnostic",
                     CLI_info_base="SSH \u00b7 MalformedMessage \u00b7 "+cause+" \u00b7 "+tcp)
                out.append(row);continue
            tree,leaves,host=fields(raw,profile)
            if msg==20:
                p=tree["Payload"]
                session.update({"Kex Algorithms":p["Kex Algorithms"],"Host Key Algorithms":p["Host Key Algorithms"]})
                for key in("Encryption C2S","Encryption S2C","MAC C2S","MAC S2C","Compression C2S","Compression S2C"):session["Offered "+key]=p[key]
                for key in("Languages C2S","Languages S2C","First Kex Packet Follows","Reserved"):session[key]=p[key]
                have.add(s["endpoint"])
                if known and len(have)==2:session.update(selected);session["Kex Family"]=base.family(c["profile"])
            elif msg==21:
                nk.add(s["endpoint"])
                if len(nk)==2:session.update({"Encrypted":True,"Protocol Transition":"ssh->encrypted"})
            elif msg==30:
                session["Kex Family"]=base.family(profile)
            elif msg in(31,33):
                session["Kex Family"]=base.family(profile)
                reply=(profile=="dh-gex"and msg==33)or(profile in("dh","ecdh-curve25519","ecdh-nistp256")and msg==31)
                if reply:
                    assert host is not None
                    digest=hashlib.sha256(host).digest()
                    session.update({"Host Key Format":"ssh-ed25519","Host Key Fingerprint":"SHA256:"+__import__("base64").b64encode(digest).decode().rstrip("="),
                          "Host Key SHA256":digest.hex(),"Signature Verified":False,"Peer Identity Verified":False})
            if 30<=msg<=34 and profile!="transport":
                session["Initial Packet Fields"]=tree;session["Signature Verified"]=session["Peer Identity Verified"]=False
        row.update(expected_status="decoded",expected_completeness="envelope",
             expected_entry="SSHIdentification"if s["kind"]=="identification"else"SSHPacket",
             session=session,fields=tree,API_wire_leaves=leaves,private_aliases=base.private_aliases(session),
             fields_error_required=False,expert="",expected_summary="SSH "+session["Packet Name"],expected_error=None,
             CLI_protocol="SSH",CLI_status="decoded",CLI_visibility="reassembled",CLI_scope="reassembled message",
             CLI_info_base=base.secure_summary(session))
        out.append(row)
    close=None
    if nk!={"A","B"}:
        ep=orientations[0];other=orientations[1]
        close={"sourcecase":c["sourcecase"],"trigger_frame":max(s["frame"]for s in c["steps"]if s["TCP_flags"]&1),
             "source":c["endpoints"][ep],"destination":c["endpoints"][other],"direction":0,
             "session":{"Identification":[idents[ep],idents[other]]},"expected_summary":"fin: SSH handshake ended before NEWKEYS",
             "source_role_evidence":c["role_evidence"]["kind"]}
    return out,close

