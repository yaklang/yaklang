package bin_parser

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Mixed captures contain real protocols other than their representative label.
// Keep these explicit per-frame contracts separate from the one-disposition
// capture matrix and the fixed historical roadmap. Neither ledger gains a new
// application-decoding claim merely because an unrelated frame is understood.
type protocolCorpusSupplementalProfileSpec struct {
	CaptureID       string
	Frame           int // one-based physical capture record
	FrameOffset     int
	InputLength     int
	FrameTailLength int    // retained bytes after this explicitly bounded message
	BoundaryNote    string // required when the selected message does not end the frame
	Contract        protocolCorpusParseContract
	ExactValues     map[string]any
	ExactMetadata   map[string]any
}

var protocolCorpusSupplementalProfiles = map[string]protocolCorpusSupplementalProfileSpec{
	"Memcached Stats Request Fields": {
		CaptureID: "ndpi-memcached", Frame: 4, FrameOffset: 66, InputLength: 7,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/memcached_fields.yaml", EntryNode: "MemcachedStatsRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Command": "stats"},
		ExactMetadata: map[string]any{"Layout Context": "stats-request", "Command Executed": false, "Context Is Caller Supplied": true},
	},
	"Memcached Stats Response Fields": {
		CaptureID: "ndpi-memcached", Frame: 6, FrameOffset: 66, InputLength: 1028,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/memcached_fields.yaml", EntryNode: "MemcachedStatsResponseFields", Layer: "L7"},
		ExactValues:   map[string]any{"Response Terminator": "END"},
		ExactMetadata: map[string]any{"Layout Context": "stats-response", "Command Executed": false, "Context Is Caller Supplied": true},
	},
	"Memcached Binary GET Request Fields": {
		CaptureID: "pr5023-gen-memcache-bin", Frame: 4, FrameOffset: 54, InputLength: 27,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/memcached_fields.yaml", EntryNode: "MemcachedBinaryGetRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Key": []byte("foo"), "Total Body Length": uint64(3), "Opaque": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "binary-get-request", "Command Executed": false, "Context Is Caller Supplied": true},
	},
	"CQL Options v4 Fields": {
		CaptureID: "ndpi-cassandra", Frame: 4, FrameOffset: 66, InputLength: 9,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLOptions4Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(0 + 4), "Body Length": uint64(0)},
		ExactMetadata: map[string]any{"Layout Context": "options4", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"CQL Supported v4 Fields": {
		CaptureID: "ndpi-cassandra", Frame: 6, FrameOffset: 66, InputLength: 61,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLSupported4Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(128 + 4), "Body Length": uint64(52)},
		ExactMetadata: map[string]any{"Layout Context": "supported4", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"CQL Startup v4 Fields": {
		CaptureID: "ndpi-cassandra", Frame: 8, FrameOffset: 66, InputLength: 31,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLStartup4Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(0 + 4), "Body Length": uint64(22)},
		ExactMetadata: map[string]any{"Layout Context": "startup4", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"CQL Options v5 Initial Fields": {
		CaptureID: "ndpi-cassandra", Frame: 12, FrameOffset: 66, InputLength: 9,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLOptions5InitialFields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(0 + 5), "Body Length": uint64(0)},
		ExactMetadata: map[string]any{"Layout Context": "options5-initial", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"CQL Supported v5 Initial Fields": {
		CaptureID: "ndpi-cassandra", Frame: 14, FrameOffset: 66, InputLength: 111,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLSupported5InitialFields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(128 + 5), "Body Length": uint64(102)},
		ExactMetadata: map[string]any{"Layout Context": "supported5-initial", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"CQL Startup v5 Initial Fields": {
		CaptureID: "ndpi-cassandra", Frame: 16, FrameOffset: 66, InputLength: 92,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CQLStartup5InitialFields", Layer: "L7"},
		ExactValues:   map[string]any{"Version": uint64(0 + 5), "Body Length": uint64(83)},
		ExactMetadata: map[string]any{"Layout Context": "startup5-initial", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"Cassandra Internode Initiate Fields": {
		CaptureID: "ndpi-cassandra", Frame: 20, FrameOffset: 66, InputLength: 19,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/cassandra_fields.yaml", EntryNode: "CassandraInternodeInitiateFields", Layer: "L7"},
		ExactValues:   map[string]any{"Protocol Magic": uint64(0xca552dfa), "Connection Flags": uint64(0x0c0a0c11), "Message CRC32": uint64(0xf3cb19b3)},
		ExactMetadata: map[string]any{"Layout Context": "internode-initiate-modern", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Connect 315 Fields": {
		CaptureID: "ndpi-oracle", Frame: 4, FrameOffset: 54, InputLength: 212,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSConnect315Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(212), "Packet Type": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "connect315", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Accept 315 Fields": {
		CaptureID: "ndpi-oracle", Frame: 10, FrameOffset: 54, InputLength: 41,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSAccept315Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(41), "Packet Type": uint64(2)},
		ExactMetadata: map[string]any{"Layout Context": "accept315", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Resend 16 Fields": {
		CaptureID: "ndpi-oracle", Frame: 6, FrameOffset: 54, InputLength: 8,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSResend16Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(8), "Packet Type": uint64(11)},
		ExactMetadata: map[string]any{"Layout Context": "resend16", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Services 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 11, FrameOffset: 54, InputLength: 164,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSServices32Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(164), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "services32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Protocol Request 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 14, FrameOffset: 54, InputLength: 38,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSProtocolRequest32Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(38), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "protocol-request32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Protocol Response 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 16, FrameOffset: 54, InputLength: 239,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSProtocolResponse32Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(239), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "protocol-response32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Types Request Native 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 17, FrameOffset: 54, InputLength: 82,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSTypesRequestNative32Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(82), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "types-request-native32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Types Response Native 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 19, FrameOffset: 54, InputLength: 26,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSTypesResponseNative32Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(26), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "types-response-native32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TNS Parameters Native LE64 32 Fields": {
		CaptureID: "ndpi-oracle", Frame: 20, FrameOffset: 54, InputLength: 233,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tns_fields.yaml", EntryNode: "TNSParametersNativeLE6432Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(233), "Packet Type": uint64(6)},
		ExactMetadata: map[string]any{"Layout Context": "parameters-native-le64-32", "Context Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS SQL Batch 7.1 Fields": {
		CaptureID: "ndpi-mssql", Frame: 5, FrameOffset: 54, InputLength: 44,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSBatch71Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(1), "Length": uint64(44), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "batch71", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS SQL Batch 7.2 Fields": {
		CaptureID: "ndpi-mssql", Frame: 1, FrameOffset: 66, InputLength: 190,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSBatch72Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(1), "Length": uint64(190), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "batch72", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS RPC 7.1 Fields": {
		CaptureID: "ndpi-mssql", Frame: 8, FrameOffset: 54, InputLength: 1082,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSRPC71Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(3), "Length": uint64(1082), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "rpc71", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS RPC 7.2 Fields": {
		CaptureID: "ndpi-mssql", Frame: 3, FrameOffset: 66, InputLength: 292,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSRPC72Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(3), "Length": uint64(292), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "rpc72", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS Response 7.1 Fields": {
		CaptureID: "ndpi-mssql", Frame: 6, FrameOffset: 54, InputLength: 17,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSResponse71Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(4), "Length": uint64(17), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "response71", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"TDS Response 7.2 Fields": {
		CaptureID: "ndpi-mssql", Frame: 4, FrameOffset: 66, InputLength: 358,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tds_fields.yaml", EntryNode: "TDSResponse72Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Type": uint64(4), "Length": uint64(358), "PacketID": uint64(1)},
		ExactMetadata: map[string]any{"Layout Context": "response72", "TDS Packet Count": 1, "Version Is Caller Supplied": true, "Session State Validated": false},
	},
	"LDAP Bind Request Fields": {
		CaptureID: "gen-ldap", Frame: 4, FrameOffset: 54, InputLength: 14,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/ldap_fields.yaml", EntryNode: "LDAPBindRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message ID": uint64(1), "Version": uint64(3), "Directory Name": "", "Simple Octets": []byte{}},
		ExactMetadata: map[string]any{"Layout Context": "bind-request", "Choice": "simple", "Session State Validated": false, "Transport Context Validated": false},
	},
	"PostgreSQL Startup Fields": {
		CaptureID: "ndpi-postgresql", Frame: 7, FrameOffset: 66, InputLength: 38,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLStartupFields", Layer: "L7"},
		ExactValues:   map[string]any{"Length": uint64(38), "Protocol Code": uint64(196608)},
		ExactMetadata: map[string]any{"Layout Context": "startup", "Session State Validated": false},
	},
	"PostgreSQL SSL Request Fields": {
		CaptureID: "ndpi-postgresql", Frame: 51, FrameOffset: 66, InputLength: 8,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLSSLRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Protocol Code": uint64(80877103)},
		ExactMetadata: map[string]any{"Layout Context": "ssl-request", "Payload Decrypted": false},
	},
	"PostgreSQL SSL Response Fields": {
		CaptureID: "ndpi-postgresql", Frame: 53, FrameOffset: 66, InputLength: 1,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLSSLResponseFields", Layer: "L7"},
		ExactValues:   map[string]any{"Transport Response": uint64(78)},
		ExactMetadata: map[string]any{"Layout Context": "ssl-response", "Positive Response Observed": false},
	},
	"PostgreSQL GSS Request Fields": {
		CaptureID: "ndpi-postgresql", Frame: 43, FrameOffset: 66, InputLength: 8,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLGSSRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Protocol Code": uint64(80877104)},
		ExactMetadata: map[string]any{"Layout Context": "gss-request", "Payload Decrypted": false},
	},
	"PostgreSQL GSS Response Fields": {
		CaptureID: "ndpi-postgresql", Frame: 45, FrameOffset: 66, InputLength: 1,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLGSSResponseFields", Layer: "L7"},
		ExactValues:   map[string]any{"Transport Response": uint64(71)},
		ExactMetadata: map[string]any{"Layout Context": "gss-response", "Positive Response Observed": true},
	},
	"PostgreSQL Frontend Fields": {
		CaptureID: "ndpi-postgresql", Frame: 21, FrameOffset: 66, InputLength: 32, FrameTailLength: 124,
		BoundaryNote:  "The first complete Parse message is selected; nine following complete messages remain outside this boundary.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLFrontendFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Type": uint64(80), "Length": uint64(31)},
		ExactMetadata: map[string]any{"Layout Context": "frontend", "Message Name": "Parse", "Query Executed": false},
	},
	"PostgreSQL Backend Block Fields": {
		CaptureID: "ndpi-postgresql", Frame: 25, FrameOffset: 66, InputLength: 98,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLBackendBlockFields", Layer: "L7"},
		ExactValues:   map[string]any{"Table OID": uint64(17215), "Type OID": uint64(23)},
		ExactMetadata: map[string]any{"Layout Context": "backend-block", "Message Count": 9, "Session State Validated": false},
	},
	"PostgreSQL Password Fields": {
		CaptureID: "ndpi-postgresql", Frame: 15, FrameOffset: 66, InputLength: 41,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLPasswordFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Type": uint64(112), "Length": uint64(40)},
		ExactMetadata: map[string]any{"Layout Context": "password", "Message Name": "PasswordMessage", "Session State Validated": false},
	},
	"PostgreSQL SASL Initial Fields": {
		CaptureID: "ndpi-postgresql", Frame: 83, FrameOffset: 66, InputLength: 55,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLSASLInitialFields", Layer: "L7"},
		ExactValues:   map[string]any{"Mechanism Name": "SCRAM-SHA-256"},
		ExactMetadata: map[string]any{"Layout Context": "sasl-initial", "Empty SCRAM Username Observed": true, "Session State Validated": false},
	},
	"PostgreSQL Backend SCRAM Fields": {
		CaptureID: "ndpi-postgresql", Frame: 84, FrameOffset: 66, InputLength: 93,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLBackendSCRAMFields", Layer: "L7"},
		ExactValues:   map[string]any{"Method Code": uint64(11)},
		ExactMetadata: map[string]any{"Layout Context": "backend-scram", "Session State Validated": false},
	},
	"PostgreSQL SCRAM Response Fields": {
		CaptureID: "ndpi-postgresql", Frame: 86, FrameOffset: 66, InputLength: 109,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/postgresql_fields.yaml", EntryNode: "PostgreSQLSASLSCRAMResponseFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Type": uint64(112), "Length": uint64(108)},
		ExactMetadata: map[string]any{"Layout Context": "sasl-scram-response", "Session State Validated": false},
	},
	"MySQL Greeting Fields": {
		CaptureID: "ndpi-mysql", Frame: 19, FrameOffset: 66, InputLength: 78,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MySQLGreetingFields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(74), "Connection ID": uint64(12), "Server Version": "8.0.36", "Plugin Name": "caching_sha2_password"},
		ExactMetadata: map[string]any{"Layout Context": "greeting", "Capabilities": uint64(3758096383), "Session State Validated": false},
	},
	"MariaDB Greeting Fields": {
		CaptureID: "ndpi-mysql", Frame: 4, FrameOffset: 66, InputLength: 110,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MySQLGreetingFields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(106), "Connection ID": uint64(32), "MariaDB Extended Capabilities": uint64(29)},
		ExactMetadata: map[string]any{"Layout Context": "greeting", "Capabilities": uint64(2181036030), "Session State Validated": false},
	},
	"MariaDB Handshake Response Fields": {
		CaptureID: "ndpi-mysql", Frame: 6, FrameOffset: 66, InputLength: 218,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MariaDBHandshakeResponse41Fields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(214), "Client Capabilities": uint64(553625220), "Maximum Packet Size": uint64(16777216), "MariaDB Extended Capabilities": uint64(29)},
		ExactMetadata: map[string]any{"Layout Context": "mariadb-response41", "Session State Validated": false},
	},
	"MySQL Command Fields": {
		CaptureID: "ndpi-mysql", Frame: 9, FrameOffset: 66, InputLength: 37,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MySQLCommandFields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(33), "Command": uint64(3)},
		ExactMetadata: map[string]any{"Layout Context": "command", "Command Name": "COM_QUERY", "Query Executed": false},
	},
	"MySQL OK Session Track Fields": {
		CaptureID: "ndpi-mysql", Frame: 8, FrameOffset: 66, InputLength: 11,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MySQLOKSessionTrackFields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(7), "Sequence ID": uint64(2), "Status Flags": uint64(2), "Warnings": uint64(0)},
		ExactMetadata: map[string]any{"Layout Context": "ok41-session-track", "Affected Rows": uint64(0), "Last Insert ID": uint64(0), "Session State Validated": false},
	},
	"MySQL SSL Request Fields": {
		CaptureID: "ndpi-mysql", Frame: 21, FrameOffset: 66, InputLength: 36,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MySQLSSLRequestFields", Layer: "L7"},
		ExactValues:   map[string]any{"Payload Length": uint64(32), "Client Capabilities": uint64(436186757), "Maximum Packet Size": uint64(16777216)},
		ExactMetadata: map[string]any{"Layout Context": "ssl-request", "TLS Requested": true, "Payload Decrypted": false},
	},
	"MariaDB Text Result Set Fields": {
		CaptureID: "ndpi-mysql", Frame: 10, FrameOffset: 66, InputLength: 85,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mysql_fields.yaml", EntryNode: "MariaDBTextResultSetFields", Layer: "L7"},
		ExactValues:   map[string]any{"Metadata Follows": uint64(1), "Character Set": uint64(33), "Maximum Column Length": uint64(36), "Column Type": uint64(253)},
		ExactMetadata: map[string]any{"Layout Context": "mariadb-text-resultset", "Column Count": uint64(1), "Row Count": 1, "Session State Validated": false},
	},
	"IMAP Command Fields": {
		CaptureID: "ndpi-imap", Frame: 29, FrameOffset: 66, InputLength: 73,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/imap_fields.yaml", EntryNode: "IMAPCommandFields", Layer: "L7"},
		ExactValues:   map[string]any{"Tag": "C00005", "Command": "UID", "UID Command": "FETCH", "Sequence Number": "1", "Sequence Wildcard": []byte("*")},
		ExactMetadata: map[string]any{"Layout Context": "command", "Command Name": "UID", "UID Command": "FETCH", "Session State Validated": false},
	},
	"IMAP Response Fields": {
		CaptureID: "ndpi-imap", Frame: 4, FrameOffset: 66, InputLength: 42,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/imap_fields.yaml", EntryNode: "IMAPResponseFields", Layer: "L7"},
		ExactValues:   map[string]any{"Response Marker": []byte("*"), "Response Name": "OK", "Response Text": "IMAP4Rev1 Server Version 4.9.04.012"},
		ExactMetadata: map[string]any{"Layout Context": "response", "Response Name": "OK", "Session State Validated": false},
	},
	"IMAP Response Block Fields": {
		CaptureID: "ndpi-imap", Frame: 27, FrameOffset: 66, InputLength: 259,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/imap_fields.yaml", EntryNode: "IMAPResponseBlockFields", Layer: "L7"},
		ExactValues:   map[string]any{"Observed Number": "1", "Response Code": "UIDVALIDITY", "Flag Name": "Seen"},
		ExactMetadata: map[string]any{"Layout Context": "response-block", "Response Count": 6, "Literal Count": 0, "Mailbox State Validated": false},
	},
	"POP3 Command Fields": {
		CaptureID: "ndpi-pop3", Frame: 13, FrameOffset: 66, InputLength: 6,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3CommandFields", Layer: "L7"},
		ExactValues:   map[string]any{"Command": "list", "CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "command", "Command Name": "LIST", "Session State Validated": false},
	},
	"POP3 Status Fields": {
		CaptureID: "ndpi-pop3", Frame: 4, FrameOffset: 66, InputLength: 35,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3StatusFields", Layer: "L7"},
		ExactValues:   map[string]any{"Status": "+OK", "CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "status", "Positive Status Observed": true, "Session State Validated": false},
	},
	"POP3 List Fields": {
		CaptureID: "ndpi-pop3", Frame: 14, FrameOffset: 66, InputLength: 25,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3ListFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Number": "1", "Message Octets": "16196", "Response Terminator": []byte(".\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "list", "Item Count": 2, "Mailbox State Validated": false},
	},
	"POP3 UIDL Fields": {
		CaptureID: "ndpi-pop3", Frame: 17, FrameOffset: 66, InputLength: 64,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3UIDLFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Number": "1", "Unique ID": "0LnyLp-1TsNI72MLw-00g8zj", "Response Terminator": []byte(".\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "uidl", "Item Count": 2, "Mailbox State Validated": false},
	},
	"POP3 Capabilities Fields": {
		CaptureID: "ndpi-pop3", Frame: 39, FrameOffset: 54, InputLength: 91,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3CapabilitiesFields", Layer: "L7"},
		ExactValues:   map[string]any{"Capability Name": "TOP", "Value": "PLAIN", "Response Terminator": []byte(".\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "capa", "Item Count": 6, "Session State Validated": false},
	},
	"POP3 Stat Fields": {
		CaptureID: "ndpi-pop3", Frame: 107, FrameOffset: 54, InputLength: 13, FrameTailLength: 5,
		BoundaryNote:  "The original five nonzero link-padding bytes are outside the IPv4 total length and TCP payload.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3StatFields", Layer: "L7"},
		ExactValues:   map[string]any{"Message Count": "3", "Message Octets": "19191"},
		ExactMetadata: map[string]any{"Layout Context": "stat", "Mailbox State Validated": false},
	},
	"POP3 Challenge Fields": {
		CaptureID: "ndpi-pop3", Frame: 55, FrameOffset: 54, InputLength: 4, FrameTailLength: 14,
		BoundaryNote:  "Four TCP payload bytes form the empty + SP CRLF challenge; fourteen original link-tail bytes remain outside it.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3ChallengeFields", Layer: "L7"},
		ExactValues:   map[string]any{"Continuation Marker": []byte("+"), "CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "challenge", "Decoded Response Octets": 0, "Mechanism Payload Decoded": false},
	},
	"POP3 Client Continuation Fields": {
		CaptureID: "ndpi-pop3", Frame: 56, FrameOffset: 54, InputLength: 62,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/pop3_fields.yaml", EntryNode: "POP3ClientContinuationFields", Layer: "L7"},
		ExactValues:   map[string]any{"CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Layout Context": "client-continuation", "Decoded Response Octets": 43, "Mechanism Payload Decoded": false},
	},
	"SMTP Command Fields": {
		CaptureID: "ndpi-smtp", Frame: 11, FrameOffset: 54, InputLength: 36,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/smtp_fields.yaml", EntryNode: "SMTPCommandFields", Layer: "L7"},
		ExactValues:   map[string]any{"Command": "MAIL", "Path Keyword": "From", "Path Open": []byte("<"), "Path Close": []byte(">"), "CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Command Name": "MAIL", "Parameter Count": 0, "Session State Validated": false, "Delivery Outcome Validated": false},
	},
	"MQTT 3.1 Packet Fields": {
		CaptureID: "ndpi-mqtt", Frame: 3, FrameOffset: 66, InputLength: 32,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mqtt_fields.yaml", EntryNode: "MQTT31PacketFields", Layer: "L7"},
		ExactValues:   map[string]any{"Fixed Header": uint64(0x82), "Packet Identifier": uint64(1), "Topic Filter": "astr/s720/02D5050223D3/99", "Requested QoS": uint64(0)},
		ExactMetadata: map[string]any{"Protocol Level Context": 3, "Packet Type": uint64(8), "List Item Count": 1, "Session State Validated": false},
	},
	"MQTT 3.1.1 Packet Fields": {
		CaptureID: "ndpi-mqtt", Frame: 9, FrameOffset: 70, InputLength: 285,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/mqtt_fields.yaml", EntryNode: "MQTT311PacketFields", Layer: "L7"},
		ExactValues:   map[string]any{"Fixed Header": uint64(0x10), "Protocol Name": "MQTT", "Protocol Level": uint64(4), "Keep Alive": uint64(600)},
		ExactMetadata: map[string]any{"Protocol Level Context": 4, "Packet Type": uint64(1), "Remaining Length": 282, "Delivery Outcome Validated": false},
	},
	"Kerberos Message Fields": {
		CaptureID: "ndpi-kerberos-error", Frame: 1, FrameOffset: 46, InputLength: 287,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/kerberos_fields.yaml", EntryNode: "KerberosMessageFields", Layer: "L7"},
		ExactValues:   map[string]any{"DER Tag": []byte{0x6a}, "Value": []byte{5}},
		ExactMetadata: map[string]any{"Message Type": int64(10), "Ticket Count": 0, "Encrypted Part Count": 1, "Decryption Performed": false},
	},
	"Kerberos TCP Fields": {
		CaptureID: "ndpi-kerberos-login", Frame: 31, FrameOffset: 66, InputLength: 1555,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/kerberos_fields.yaml", EntryNode: "KerberosTCPFields", Layer: "L7"},
		ExactValues:   map[string]any{"Record Length": uint64(1551), "DER Tag": []byte{0x6c}, "Value": []byte{5}},
		ExactMetadata: map[string]any{"Message Type": int64(12), "Ticket Count": 1, "Encrypted Part Count": 3, "Checksums Verified": false, "TCP Reassembly Performed": false},
	},
	"HTTP/2 Frame Fields": {
		CaptureID: "ndpi-http2", Frame: 2, FrameOffset: 68, InputLength: 102,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/http2_fields.yaml", EntryNode: "HTTP2FrameFields", Layer: "L7"},
		ExactValues:   map[string]any{"Length": uint64(93), "Type": uint64(1), "Stream Identifier": uint64(3)},
		ExactMetadata: map[string]any{"Frame Count": 1, "HPACK Decoded": false, "TCP Reassembly Performed": false},
	},
	"HTTP/2 Initial Client Direction": {
		CaptureID: "ndpi-http2", Frame: 1, FrameOffset: 68, InputLength: 64,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/http2_fields.yaml", EntryNode: "HTTP2InitialClientStream", Layer: "L7"},
		ExactValues:   map[string]any{"Client Preface": []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"), "Type": uint64(4)},
		ExactMetadata: map[string]any{"Frame Count": 2, "Header Count": 0, "HPACK Decoded": true, "Peer Settings Applied": false},
	},
	"HTTP/2 Initial Server Direction": {
		CaptureID: "ndpi-http2", Frame: 4, FrameOffset: 68, InputLength: 33,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/http2_fields.yaml", EntryNode: "HTTP2InitialServerStream", Layer: "L7"},
		ExactValues:   map[string]any{"Length": uint64(24), "Type": uint64(4), "Stream Identifier": uint64(0)},
		ExactMetadata: map[string]any{"Frame Count": 1, "Header Count": 0, "HPACK Decoded": true, "Connection Lifecycle Validated": false},
	},
	"SSH Initial Plaintext Packet": {
		CaptureID: "ndpi-ssh", Frame: 8, FrameOffset: 66, InputLength: 904,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/ssh_plaintext.yaml", EntryNode: "SSHPlaintextPacket", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(900), "Message Number": uint64(20)},
		ExactMetadata: map[string]any{"Payload Layout Decoded": true, "Negotiation Validated": false},
	},
	"SSH DH Group Exchange": {
		CaptureID: "ndpi-ssh", Frame: 12, FrameOffset: 66, InputLength: 24,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/ssh_plaintext.yaml", EntryNode: "SSHPlaintextDHGEX", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(20), "Message Number": uint64(34)},
		ExactMetadata: map[string]any{"Exchange Layout": "dh-gex", "Key Mathematics Validated": false},
	},
	"SSH Fixed Group DH": {
		CaptureID: "ndpi-ssh", Frame: 269, FrameOffset: 54, InputLength: 272,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/ssh_plaintext.yaml", EntryNode: "SSHPlaintextDH", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(268), "Message Number": uint64(30), "Client Exchange Value Length": uint64(256)},
		ExactMetadata: map[string]any{"Exchange Layout": "dh", "Signature Validated": false},
	},
	"SSH P-256 ECDH": {
		CaptureID: "ndpi-ssh", Frame: 295, FrameOffset: 66, InputLength: 80,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/ssh_plaintext.yaml", EntryNode: "SSHPlaintextECDHP256", Layer: "L7"},
		ExactValues:   map[string]any{"Packet Length": uint64(76), "Message Number": uint64(30), "Client Point Format": uint64(4)},
		ExactMetadata: map[string]any{"Exchange Layout": "ecdh-nistp256", "Peer Identity Validated": false},
	},
	"X.509 Certificate DER with Public Key": {
		CaptureID: "ndpi-dot", Frame: 6, FrameOffset: 149, InputLength: 1567, FrameTailLength: 1419,
		BoundaryNote:  "Exact first DER certificate with RSA public-key fields; subsequent certificate bytes remain outside this boundary.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/x509_certificate.yaml", EntryNode: "X509CertificateDERWithPublicKey", Layer: "L7"},
		ExactValues:   map[string]any{"DER Tag": []byte{0x30}, "Value": uint64(2)},
		ExactMetadata: map[string]any{"Certificate Version": 3, "Extension Count": 10, "Decoded Extension Count": 10, "Public Key Fields Decoded": true, "Public Key Validated": false, "Signature Verified": false, "Certificate Trust Validated": false},
	},
	"X.509 Certificate DER with Extensions": {
		CaptureID: "ndpi-dot", Frame: 6, FrameOffset: 149, InputLength: 1567, FrameTailLength: 1419,
		BoundaryNote:  "Exact first DER certificate inside the Certificate vector; subsequent certificate bytes remain outside the explicit boundary.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/x509_certificate.yaml", EntryNode: "X509CertificateDERWithExtensions", Layer: "L7"},
		ExactValues:   map[string]any{"DER Tag": []byte{0x30}, "Value": uint64(2)},
		ExactMetadata: map[string]any{"Certificate Version": 3, "Extension Count": 10, "Decoded Extension Count": 10, "Opaque Extension Count": 0, "Extension Contents Decoded": true, "Extension Semantics Validated": false, "SCT Signatures Verified": false, "Certificate Trust Validated": false},
	},
	"X.509 Certificate DER": {
		CaptureID: "ndpi-anydesk", Frame: 85, FrameOffset: 132, InputLength: 684, FrameTailLength: 51,
		BoundaryNote:  "Exact DER certificate inside the TLS Certificate vector; subsequent TLS control records are outside this boundary.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/x509_certificate.yaml", EntryNode: "X509CertificateDER", Layer: "L7"},
		ExactValues:   map[string]any{"DER Tag": []byte{0x30}, "Value": []byte{1}},
		ExactMetadata: map[string]any{"Certificate Version": 1, "Extension Count": 0, "DER Framing Parsed": true, "Extension Contents Decoded": false, "Signature Verified": false, "Certificate Trust Validated": false},
	},
	"TLS ChangeCipherSpec": {
		CaptureID: "ndpi-anydesk", Frame: 20, FrameOffset: 1097, InputLength: 6, FrameTailLength: 45,
		BoundaryNote:  "Complete plaintext CCS between CertificateVerify and a separate protected handshake record; the following bytes are not CCS fields.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_change_cipher_spec.yaml", EntryNode: "TLSChangeCipherSpecRecord", Layer: "L7"},
		ExactValues:   map[string]any{"Content Type": uint64(20), "Legacy Record Version": uint64(0x0303), "Record Length": uint64(1), "ChangeCipherSpec Value": uint64(1)},
		ExactMetadata: map[string]any{"Plaintext Context Supplied By Caller": true, "Protocol Version Inferred": false, "Cipher State Transition Validated": false, "Handshake Completion Validated": false},
	},
	"TLS 1.2 ServerKeyExchange": {
		CaptureID: "ndpi-anydesk", Frame: 128, FrameOffset: 951, InputLength: 147, FrameTailLength: 48,
		BoundaryNote:  "Complete ECDHE ServerKeyExchange before CertificateRequest and ServerHelloDone; paired ServerHello selects cipher c02c.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_key_exchange.yaml", EntryNode: "TLS12ECDHEServerKeyExchange", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(12), "Handshake Length": uint64(143), "Curve Type": uint64(3), "Named Group": uint64(23), "Server EC Point Length": uint64(65), "Hash Algorithm": uint64(6), "Signature Algorithm": uint64(3), "Signature Length": uint64(70)},
		ExactMetadata: map[string]any{"Profile": "TLS 1.2 ECDHEServer layout", "Public Value Validated": false, "Signature Verified": false, "Cipher Suite Correlation Validated": false},
	},
	"TLS 1.2 ClientKeyExchange": {
		CaptureID: "ndpi-anydesk", Frame: 20, FrameOffset: 758, InputLength: 70, FrameTailLength: 320,
		BoundaryNote:  "Complete ECDHE ClientKeyExchange before CertificateVerify and subsequent records; paired ServerHello selects cipher c02c.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_key_exchange.yaml", EntryNode: "TLS12ECDHEClientKeyExchange", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(16), "Handshake Length": uint64(66), "Client EC Point Length": uint64(65)},
		ExactMetadata: map[string]any{"Profile": "TLS 1.2 ECDHEClient layout", "Public Value Validated": false, "Premaster Decrypted": false, "Cipher Suite Correlation Validated": false},
	},
	"TLS 1.2 Certificate": {
		CaptureID: "ndpi-anydesk", Frame: 85, FrameOffset: 122, InputLength: 694, FrameTailLength: 51,
		BoundaryNote:  "Complete Certificate after ServerHello; following CertificateRequest and ServerHelloDone records remain outside this message boundary.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_certificate.yaml", EntryNode: "TLSHandshakeCertificate", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(11), "Handshake Length": uint64(690), "Certificate List Length": uint64(687), "Certificate Length": uint64(684)},
		ExactMetadata: map[string]any{"Certificate Count": 1, "DER Parsed": false, "Certificate Trust Validated": false, "TCP Reassembly Performed": false},
	},
	"TLS 1.2 CertificateRequest": {
		CaptureID: "ndpi-anydesk", Frame: 85, FrameOffset: 821, InputLength: 42, FrameTailLength: 4,
		BoundaryNote:  "Complete CertificateRequest followed by a distinct ServerHelloDone handshake in the same record.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_certificate_auth.yaml", EntryNode: "TLS12CertificateRequest", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(13), "Handshake Length": uint64(38), "Certificate Types Length": uint64(3), "Signature Algorithms Length": uint64(30), "Distinguished Names Length": uint64(0)},
		ExactMetadata: map[string]any{"Certificate Type Count": 3, "Signature Algorithm Count": 15, "Distinguished Name Count": 0, "Sender Conformance Validated": false, "Request Correlation Validated": false},
	},
	"TLS 1.2 CertificateVerify": {
		CaptureID: "ndpi-anydesk", Frame: 20, FrameOffset: 833, InputLength: 264, FrameTailLength: 51,
		BoundaryNote:  "Complete CertificateVerify before ChangeCipherSpec and a protected record; those bytes are not part of this handshake.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_certificate_auth.yaml", EntryNode: "TLS12CertificateVerify", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(15), "Handshake Length": uint64(260), "Hash Algorithm": uint64(6), "Signature Algorithm": uint64(1), "Signature Length": uint64(256)},
		ExactMetadata: map[string]any{"Profile": "TLS 1.2 CertificateVerify layout", "Signature Verified": false, "Handshake Transcript Validated": false, "Peer Identity Validated": false},
	},
	"TLS 1.2 ServerHelloDone": {
		CaptureID: "ndpi-anydesk", Frame: 85, FrameOffset: 863, InputLength: 4,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_control_handshake.yaml", EntryNode: "TLS12ServerHelloDone", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(14), "Handshake Length": uint64(0)},
		ExactMetadata: map[string]any{"Handshake Message": "ServerHelloDone", "Handshake Completion Validated": false, "Direction Validated": false},
	},
	"TLS 1.2 NewSessionTicket": {
		CaptureID: "ndpi-anydesk", Frame: 81, FrameOffset: 59, InputLength: 858, FrameTailLength: 51,
		BoundaryNote:  "Complete NewSessionTicket followed by ChangeCipherSpec and a protected record, retained separately by the full-frame test.",
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_control_handshake.yaml", EntryNode: "TLS12NewSessionTicket", Layer: "L7"},
		ExactValues:   map[string]any{"Handshake Type": uint64(4), "Handshake Length": uint64(854), "Ticket Lifetime Hint": uint64(7200), "Ticket Length": uint64(848)},
		ExactMetadata: map[string]any{"Ticket Validated": false, "Session Resumption Validated": false, "Lifetime Hint Is Verified Expiry": false},
	},
	"TLS ServerHello": {
		CaptureID: "ndpi-netease-games", Frame: 10, FrameOffset: 66, InputLength: 96,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/tls_server_hello.yaml", EntryNode: "TLSServerHelloRecord", Layer: "L7"},
		ExactValues:   map[string]any{"Content Type": uint64(22), "Record Version": uint64(0x0303), "Record Length": uint64(91), "Handshake Type": uint64(2), "Handshake Length": uint64(87), "Hello Version": uint64(0x0303), "Session ID Length": uint64(32), "Cipher Suite": uint64(0xc02f), "Compression Method": uint64(0), "Extensions Length": uint64(15)},
		ExactMetadata: map[string]any{"Profile": "TLS 1.2 ServerHello layout", "Record Wrapped": true, "ParsedFirstHandshakeOnly": true, "Following Handshake Byte Count": 0, "Negotiation Validated": false, "Handshake Completion Validated": false, "TCP Reassembly Performed": false},
	},
	"Length-prefixed zlib JSON": {
		CaptureID: "ndpi-tencent-games", Frame: 20, FrameOffset: 40, InputLength: 338,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/zlib_json.yaml", EntryNode: "ZlibJSONRecord", Layer: "L7"},
		ExactValues:   map[string]any{"Compressed Length": uint64(334), "CMF": uint64(0x78), "FLG": uint64(1)},
		ExactMetadata: map[string]any{"Decoded Byte Length": 509, "Decoded SHA256": "10d488868cba9cae4d6aadd651dd70aceaaa283e05f6723d988f7dc719163e86", "Adler32 Verified": true, "Decoded Values Have Wire Spans": false, "Application Semantics Decoded": false},
	},
	"KCP": {
		CaptureID: "ndpi-netease-games", Frame: 19, FrameOffset: 58, InputLength: 81,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/kcp.yaml", EntryNode: "KCPDatagram", Layer: "L7"},
		ExactValues:   map[string]any{"Conversation ID": uint64(768), "Command": uint64(81), "Window": uint64(128), "Timestamp": uint64(35242), "Sequence Number": uint64(0), "Unacknowledged Sequence": uint64(0), "Data Length": uint64(57)},
		ExactMetadata: map[string]any{"Segment Count": 1, "Application Data Decoded": false, "Outer Prefix Decoded": false},
	},
	"SMTP Reply": {
		CaptureID: "ndpi-smtps", Frame: 4, FrameOffset: 54, InputLength: 179,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/smtp_reply.yaml", EntryNode: "SMTPReply", Layer: "L7"},
		ExactValues:   map[string]any{"Code": "220", "Separator": uint64('-'), "CRLF": []byte("\r\n")},
		ExactMetadata: map[string]any{"Line Count": 3, "Multiline": true, "TLS Handshake Validated": false},
	},
	"Browser Mailslot Datagram": {
		CaptureID: "ndpi-wechat", Frame: 1022, FrameOffset: 42, InputLength: 212,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/browser_mailslot.yaml", EntryNode: "BrowserMailslotDatagram", Layer: "L7"},
		ExactValues:   map[string]any{"Datagram Message Type": uint64(17), "Datagram Length": uint64(198), "SMB Command": uint64(37), "Data Offset": uint64(86), "Browser Opcode": uint64(12), "Mailslot Name": `\MAILSLOT\BROWSE`, "Local Master Browser Name": "GIOVANNI-PC"},
		ExactMetadata: map[string]any{"Data Alignment Conformant": false, "Byte Count Validated": true},
	},
	"GQUIC Q035": {
		CaptureID: "ndpi-wechat", Frame: 49, FrameOffset: 42, InputLength: 1350,
		Contract:      protocolCorpusParseContract{RuleFile: "application-layer/gquic.yaml", EntryNode: "GQUIC35ClientPacket", Layer: "L7"},
		ExactValues:   map[string]any{"Public Flags": uint64(13), "Connection ID": uint64(0x8b24930db663d631), "Version Tag": "Q035", "Wire Packet Number": uint64(1)},
		ExactMetadata: map[string]any{"Null Checksum Verified": false, "Packet Number Reconstructed": false, "Body Format Decoded": false},
	},
}

func TestProtocolCorpusSupplementalProfiles(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}
	for name, spec := range protocolCorpusSupplementalProfiles {
		t.Run(name, func(t *testing.T) {
			capture, ok := captures[spec.CaptureID]
			require.True(t, ok, "supplemental profile points to missing capture")
			data := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
			require.Equal(t, capture.SHA256, fmt.Sprintf("%x", sha256.Sum256(data)))
			records := protocolCorpusAuditPackets(t, filepath.Join(corpusDir, capture.CaptureFile))
			require.Len(t, records, capture.PacketCount)
			require.Greater(t, spec.Frame, 0)
			require.LessOrEqual(t, spec.Frame, len(records))
			frame := records[spec.Frame-1]
			require.Greater(t, spec.InputLength, 0)
			require.GreaterOrEqual(t, spec.FrameOffset, 0)
			require.GreaterOrEqual(t, spec.FrameTailLength, 0)
			if spec.FrameTailLength > 0 {
				require.NotEmpty(t, spec.BoundaryNote, "embedded messages require an explicit boundary explanation")
			}
			require.Equal(t, len(frame), spec.FrameOffset+spec.InputLength+spec.FrameTailLength, "the profile and retained tail must account for the exact physical record")
			input := frame[spec.FrameOffset : spec.FrameOffset+spec.InputLength]
			rule := strings.TrimSuffix(strings.ReplaceAll(spec.Contract.RuleFile, "/", "."), ".yaml")
			node := protocolCorpusRequireBoundedRuleParse(t, input, rule, spec.Contract.EntryNode)
			require.Equal(t, input, NodeToBytes(node))
			require.NotEmpty(t, spec.ExactValues)
			for field, value := range spec.ExactValues {
				protocolCorpusRequireValue(t, node, field, value)
			}
			metadata, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
			require.True(t, ok, "supplemental profile must retain explicit scope metadata")
			for field, value := range spec.ExactMetadata {
				require.Equal(t, value, metadata[field], "metadata %s", field)
			}
		})
	}
}
