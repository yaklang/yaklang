package tests

import (
	"testing"
)

func TestBuildInLib(t *testing.T) {
	Exec(`
s = "password: 1234567890";
res1 = ereg_replace(string:s,pattern:"[0-9]",replace:"*");

res2 = ereg_replace(replace:"*",pattern:"[0-9]",string:s);
assert(res1 == "password: **********","res1 != **********");
dump(res2);
assert(res2 == res1, "res1 != res2");

location = "123";
display( 'DEBUG: Location header is pointing to "' + location + '" on the same host/ip. Returning this location.\n' );

if (isnull(get_kb_item(name:"unknown_os_or_service/available")))
	display("isNULL");
`)
}

func TestString(t *testing.T) {
	DebugExec(`
a =string("a\nb\nc");
res = split(a,sep:"\n");
assert(res[0]=="a","res[0]!=a");
`)
}
func TestEregmatch(t *testing.T) {
	DebugExec(`
if (a = eregmatch(string:"a",pattern:"aaa")){
	assert(0,"a!=NULL");
}

`)
}
func TestEgrep(t *testing.T) {
	DebugExec(`
a = egrep( pattern:"^User-Agent:.+", string:"User-Agent: aaa", icase:TRUE );
dump(a);
`)
}

func TestStrStr(t *testing.T) {
	DebugExec(`
assert(strstr("asdfasdCVE 2023","CVE ") == "CVE 2023","strstr error");
`)
}

func TestSubString(t *testing.T) {
	DebugExec(`
assert("aaa<b>aaa"-"<b>" == "aaaaaa","sub string error");
`)
}
func TestMapElement(t *testing.T) {
	DebugExec(`
array = make_array("a",1);
assert(array[NULL] == NULL,"array[NULL] != NULL");
`)
}

func TestMkword(t *testing.T) {
	DebugExec(`
function mkword(){
	return _FCT_ANON_ARGS[0];
}
dump(mkword(100));
assert(mkword(100) == 100,"mkword error");
`)
}

func TestKeys(t *testing.T) {
	Exec(`
a = make_list(1,2,3);
sum = 0;
foreach k(keys(a)){
	sum+= k;
}
assert(sum == 3, "keys error");

a = make_array("a", "b", "c","d");
sum1 = "";
foreach k(keys(a)){
	sum1 += k;
}
assert(sum1 == "ac" || sum1 == "ca", "keys error");

`)
}

func TestSplit(t *testing.T) {
	Exec(`
a = "a.b.c.d";
b = split(a, sep:".");
foreach k(b){
	assert(k == "a" || k == "b" || k == "c" || k == "d", "split error");
}
`)
}
func TestGetKbList(t *testing.T) {
	Exec(`
set_kb_item(name:"Ports/tcp/80", value:1);
set_kb_item(name:"Ports/tcp/443", value:1);
tcp_ports = get_kb_list("Ports/tcp/*");
if( ! tcp_ports || ! is_array( tcp_ports ) ) {
	log_message( port:0, data:"Open TCP ports: [None found]" );
  exit( 0 );
}
keys = sort( keys( tcp_ports ) );

foreach port( keys ) {

  _port = eregmatch( string:port, pattern:"Ports/tcp/([0-9]+)" );
dump(_port);

}

`)
}
