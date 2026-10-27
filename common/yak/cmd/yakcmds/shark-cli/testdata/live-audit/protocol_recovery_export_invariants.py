"""Compare all other SDK records across explicitly selected protocol recovery.

Use the original complete-record wire/provenance bijection and strict declared
PDU-reference contracts. Selection never relaxes unrelated values, types, rows,
order, flow/domain identity or literal wire transactions. Selected recovered
semantics need their separate independent original-capture audit.
"""
import argparse
import json
import re
from pathlib import Path

import mail_rtsp_export_invariants as exact


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['before','after','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--selected',required=True,help='comma-separated native protocol names with independent wire audits')
    a=ap.parse_args()
    for p in [a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    selected=set(a.selected.split(','))
    assert selected and all(re.fullmatch('[a-z][a-z0-9-]*',name)for name in selected)
    exact.SELECTED=selected
    # Observed IMAP commands use request PDU identities for their transaction
    # contract; these are not tag strings or other on-wire identifiers.
    exact.REQUEST_PDU_TRANSACTIONS|={'imap'}
    proof=exact.audit(a.before,a.after)
    proof['scope']=__doc__;proof['selected_protocols']=sorted(selected)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:proof[k]for k in ['selected_protocols','nonselected_records_compared','renumbered_records','selected_counts_before','selected_counts_after','all_other_values_types_order_flows_and_wire_fields_exact']}))
