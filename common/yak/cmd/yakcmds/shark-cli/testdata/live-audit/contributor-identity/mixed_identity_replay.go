// Source-frozen external replay probe; not a production package/test.
// Build only from the requested clean repository source. No Go was run while
// preparing this file. Keep this source and build/execution receipts together.
package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "strconv"

    "github.com/yaklang/yaklang/common/pcapx/pcaputil"
)
func must(err error) { if err!=nil { panic(err) } }
func fileHash(p string) string { b,e:=os.ReadFile(p);must(e); h:=sha256.Sum256(b);return hex.EncodeToString(h[:]) }
type decision struct {
    Expression string
    FullReference pcaputil.DisplayFilterResult
    IdentityAndRetained pcaputil.DisplayFilterResult
}
func main() {
    capture:=flag.String("capture","","source-frozen sparse pcap")
    keylog:=flag.String("keylog","","external TLS keylog")
    output:=flag.String("out","","new JSON result path")
    flag.Parse()
    if *capture=="" || *keylog=="" || *output=="" { panic("--capture --keylog --out required") }
    kb,err:=os.ReadFile(*keylog);must(err)
    keys,err:=pcaputil.ParseTLSKeyLog(string(kb));must(err)
    mixed,err:=pcaputil.NewProtocolInspector(1,1<<20);must(err)
    history,err:=pcaputil.NewProtocolInspector(80000,1<<20);must(err)
    var originals []pcaputil.DisplayFieldSnapshot
    var callbackFacts []map[string]any
    var mixedRef,targetRef pcaputil.PacketReference
    total:=0
    var cost int
    err=pcaputil.ReplayPcapFile(*capture,
        pcaputil.WithTLSSecrets(keys),pcaputil.WithIgnoreEmbeddedSecrets(true),
        pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
            total++;cost+=e.RetainedBytes();history.OnEvent(e)
            if e.ID==299 && len(e.SourceBytes.PacketRefs)>0 { targetRef=e.SourceBytes.PacketRefs[0] }
            // Actual provenance membership, not ID guesses or current-frame
            // timing. Exactly two native observations have source frame1.
            member:=false
            for _,ref:=range e.SourceBytes.PacketRefs {
                if ref.Number==1 { mixedRef=ref;member=true;break }
            }
            if !member { return }
            callbackFacts=append(callbackFacts,map[string]any{
                "id":e.ID,"protocol":e.Protocol,"status":e.Status,
                "completeness":e.Completeness,"source_bytes":e.SourceBytes,
            })
            originals=append(originals,e.DisplayFieldValues())
            mixed.OnEvent(e)
        }))
    must(err)
    var problems []string
    check:=func(ok bool,text string) { if !ok { problems=append(problems,text) } }
    check(total==1880,"source replay must produce1880 native PDU observations")
    check(len(callbackFacts)==2,"source frame1 must have exactly two actual callback observations")
    check(mixedRef.Number==1,"mixed reference must remain actual physical frame1")
    ids,complete:=mixed.PacketEventWindow(mixedRef,true)
    w:=mixed.PacketContributorWindow(mixedRef,true,4096)
    first,firstErr:=mixed.Details(1)
    last,lastErr:=mixed.Details(2)
    check(len(ids)==1 && ids[0]==2,"retained mixed window must remain only PDU2")
    check(!complete,"first mixed query must report incomplete contributor detail coverage")
    check(w.Complete && len(w.Identities)==2 && w.Identities[0].ID==1 && w.Identities[1].ID==2,"independent source index must preserve IDs1and2")
    check(first==nil && firstErr!=nil,"PDU1 detail must actually be absent")
    check(last!=nil && lastErr==nil,"PDU2 must retain its complete actual padding observation")
    check(mixed.Evicted()==1,"mixed history must have exactly one payload eviction")
    // This typed witness contains ONLY independently observed source-index IDs.
    // It has no byte source or recovered fields. The full original snapshots
    // above are used solely as reference, NEVER to fill omitted identity data.
    witness:=pcaputil.DisplayFieldSnapshot{
        SchemaVersion:pcaputil.DisplayFieldSchemaVersion,
        Incomplete:true,Completeness:"limited",
    }
    seen:=map[uint64]bool{}
    for _,proof:=range w.Identities {
        if seen[proof.ID] { continue };seen[proof.ID]=true
        witness.Values=append(witness.Values,pcaputil.DisplayFieldValue{
            Name:"pdu.id",Value:proof.ID,ExactValue:strconv.FormatUint(proof.ID,10),Origin:"event",
        })
    }
    union:=[]pcaputil.DisplayFieldSnapshot{witness}
    var retained pcaputil.DisplayFieldSnapshot
    if last!=nil { retained=last.DisplayFieldValues();union=append(union,retained) }
    var decisions []decision
    for _,expr:=range []string{"pdu.id == 1","not quic.token_length","not (pdu.id == 1)","quic.token_length == 0","not http.request.method"} {
        f,e:=pcaputil.CompileDisplayFilter(expr);must(e)
        a,b:=f.EvaluateSnapshots(originals),f.EvaluateSnapshots(union)
        decisions=append(decisions,decision{expr,a,b})
        switch expr {
        case "pdu.id == 1":
            check(a.Matched && a.Complete && !a.Partial,"full source must witness actual PDU1")
            check(b.Matched && b.Complete && b.Partial,"ID1 must survive as partial positive witness")
        case "not (pdu.id == 1)":
            check(!a.Matched && a.Complete,"full reference negated ID1 must be false")
            check(!b.Matched && b.Complete && b.Partial,"negated witnessed ID1 must be false/complete/partial")
        case "not quic.token_length":
            check(!a.Matched && a.Complete,"actual Initial token-length must refute absence")
            check(!b.Matched && !b.Complete && b.Partial,"missing Initial token-length cannot prove absence")
        case "quic.token_length == 0":
            check(a.Matched && a.Complete,"actual Initial has token-length0")
            check(!b.Matched && !b.Complete && b.Partial,"do not fill missing token-length from old reference")
        case "not http.request.method":
            check(a.Matched && a.Complete && !a.Partial,"full source absent HTTP field remains decidable")
            check(!b.Matched && !b.Complete && b.Partial,"identity-only projection cannot certify other absent fields")
        }
    }
    target,targetErr:=history.Details(299)
    exact,exactComplete:=history.PacketEventWindow(targetRef,false)
    physical,physicalComplete:=history.PacketEventWindow(targetRef,true)
    targetProof:=history.PacketContributorWindow(targetRef,true,4096)
    targetPresent:=false
    for _,proof:=range targetProof.Identities { if proof.ID==299 {targetPresent=true} }
    check(targetRef.Number==294,"PDU299 must originate from actual physical294")
    check(target==nil && targetErr!=nil,"1MiB actual target detail must really be evicted")
    check(len(exact)==0 && len(physical)==0 && !exactComplete && !physicalComplete,"retained-only target windows must be empty AND incomplete")
    check(targetProof.Complete && targetPresent,"source index must preserve actual299 contributor independently")
    result:=map[string]any{
        "scope":"actual native replay with typed identity-only witness; not a claim of payload recovery",
        "capture_SHA256":fileHash(*capture),"keylog_SHA256":fileHash(*keylog),
        "analyzed_PDUs":total,"logical_event_cost":cost,
        "mixed":map[string]any{"actual_callbacks":callbackFacts,"full_reference_snapshots":originals,
            "retained_ids":ids,"detail_coverage_complete":complete,"source_identity_window":w,
            "PDU1_detail_present":first!=nil,"PDU2_detail_present":last!=nil,
            "retained_snapshot":retained,"identity_only_snapshot":witness,"decisions":decisions,
            "history_evictions":mixed.Evicted()},
        "overbudget":map[string]any{"byte_budget":1<<20,"target_frame":targetRef,
            "PDU299_detail_present":target!=nil,"detail_error":fmt.Sprint(targetErr),
            "exact_retained_ids":exact,"exact_complete":exactComplete,
            "physical_retained_ids":physical,"physical_complete":physicalComplete,
            "source_identity_window":targetProof,"evictions":history.Evicted(),"retained_rows":len(history.Rows("",0))},
        "errors":problems,"passed":len(problems)==0,
    }
    b,err:=json.MarshalIndent(result,"","  ");must(err)
    out,err:=os.OpenFile(*output,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);must(err)
    _,err=out.Write(append(b,'\n'));must(err);must(out.Close())
    if len(problems)>0 { fmt.Fprintln(os.Stderr,problems);os.Exit(1) }
    fmt.Println("PASS: real1880 replay; mixed source window and partial ID union; actual299/294 survives1MiB detail eviction")
}
