package antlr4nasl

import "testing"

func TestKeys(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	engine.Eval(`
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
assert(sum1 == "ac", "keys error");

`)
}

func TestSplit(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	engine.Eval(`
a = "a.b.c.d";
b = split(a, sep:".");
foreach k(b){
	assert(k == "a" || k == "b" || k == "c" || k == "d", "split error");
}
`)
}
func TestGetKbList(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	engine.Eval(`
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
