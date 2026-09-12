# P0 message parsing scope

2026-09-07. P0 completion and whole-protocol support are separate. The four
catalog entries remain `partial`. `TestP0RoadmapCovered` now executes each
scope's positive, negative, public-entry and original-record checks regardless
of its catalog label, then checks roadmap completion. Changing a name or label
cannot bypass these checks. Existing scores are retained conservatively; native
field coverage does not earn YAML schema or unmeasured branch points.

| Protocol | Explicit completed message scope | Executable evidence |
| --- | --- | --- |
| LDAP | LDAPv3 BindRequest/BindResponse, UnbindRequest, SearchRequest, SearchResultEntry/Done/Reference; definite BER, all ten filter choices, ordered attribute/value lists and controls | `TestP0LDAPOperationEntries`, `TestLDAPOperation*`, original BindRequest and CLDAP rejection audits |
| MySQL | V10/41 greeting/response, SSLRequest, supported classic commands, OK/ERR/EOF and existing fresh-column text resultset profiles | Original 49-record audit, twelve-profile boundaries/fallback and wide-integer checks |
| PostgreSQL | Existing sixteen bounded startup/SSL/GSS, authentication, frontend/backend and SCRAM message profiles | Original 88-record audit and direction/phase boundary/isolation checks |
| SMB3 | Existing 3.0/3.0.2/3.1.1 NEGOTIATE profiles and full bounded Transform carrier | Original and seven-companion negotiate checks, receive-context checks, `TestP0SMB3TransformEntry`, `TestSMB3TransformFieldsBoundariesAndTransactions` |

The executable mapping is `p0_scope_test.go`. LDAP operations and Transform
add explicit entries; historical entries, fields and byte-level contracts are
unchanged. A caller selects the profile and supplies an exact message bound.
Every added entry has a bounded raw fallback carrier. Invalid grammar does not
publish partially parsed fields. Structured generation of these new entries is
unsupported, as it is for the existing exact field profiles.

LDAP limits: 1 MiB, four BER length octets, 64 nested filters, a shared budget of
4096 operation items and the existing 4096-control budget. UTF-8 string encoding,
length/count/order and primitive tag boundaries are checked. Directory DN and
attribute schema validity, matching-rule evaluation, URI resolution, control
semantics, SASL verification and session/transport state are not implemented.
Unknown extensions require another explicit profile. Existing BindRequest
acceptance and metadata remain unchanged.

The SMB3 carrier retains all 52 header bytes and the encrypted payload. It
checks FD SMB, the known flags/algorithm value and the declared payload length,
with a 1 MiB input bound. Reserved bytes and cipher-dependent nonce padding are
retained rather than rejected as if the receiver were a sender. Encryption,
authentication, nonce uniqueness and negotiated sessions are not verified.

MySQL compressed/binary/prepared layouts, cached metadata and arbitrary
multi-result sessions remain outside the selected profiles. PostgreSQL
cross-message session/portal state and arbitrary binary data types remain
outside its profiles. Protocol support is not promoted to complete because a
message-layout scope passed.

Sources: [LDAP RFC 4511](https://www.rfc-editor.org/rfc/rfc4511.html),
[SMB2 Transform header](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/d6ce2327-a4c9-4793-be66-7b5bad2175fa).
Database sources and the original record ledgers remain in
`testdata/protocol-corpus/README.md`.

## Score audit for this scope

The existing cards continue to score the legacy YAML rule named by each card;
new native message profiles do not silently receive full YAML-schema or
exhaustive-branch credit. Recalculation with the existing rubric gives:

| Card | Schema | Traffic | Tests | Branches | Stack | Total / grade | Credit boundary |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| LDAP | 20 | 20 | 16 | 14 | 10 | 80 / B | Existing Bind sample and legacy stack; new BER/filter/control profiles require their independent scope tests |
| MySQL | 20 | 15 | 16 | 14 | 10 | 75 / B | Existing classic grammar and samples; no binary/prepared or arbitrary session credit |
| PostgreSQL | 20 | 15 | 16 | 20 | 10 | 81 / B | Existing rule's branches and explicit startup/query samples; sixteen native profiles do not imply arbitrary session coverage |
| SMB3 | 20 | 15 | 16 | 14 | 10 | 75 / B | Legacy Transform header grammar; retained ciphertext earns no decryption/session credit |

`TestP0ScorecardsCovered` recalculates totals and grades from these dimensions.
`TestP0RoadmapCovered` additionally runs the four executable message-scope
ledgers; a historical grade alone is insufficient for completion. Catalog
`partial` status and the fourteen existing deferrals remain unchanged.
