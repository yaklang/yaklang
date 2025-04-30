package xmlfmt_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/utils/xmlfmt"
)

const xml1 = `<root><this><is>a</is><test /><message><!-- with comment --><org><cn>Some org-or-other</cn><ph>Wouldnt you like to know</ph></org><contact><fn>Pat</fn><ln>Califia</ln></contact></message></this></root>`

const xml2 = `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns="http://example.com/ns"><soapenv:Header/><soapenv:Body><ns:request><ns:customer><ns:id>123</ns:id><ns:name type="NCHZ">John Brown</ns:name></ns:customer></ns:request></soapenv:Body></soapenv:Envelope>`

const xml3 = `<Envelope xmlns="http://schemas.xmlsoap.org/soap/envelope/" xmlns:_xmlns="xmlns" _xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" _xmlns:ns="http://example.com/ns"><Header xmlns="http://schemas.xmlsoap.org/soap/envelope/"></Header><Body xmlns="http://schemas.xmlsoap.org/soap/envelope/"><request xmlns="http://example.com/ns"><customer xmlns="http://example.com/ns"><id xmlns="http://example.com/ns">123</id><name xmlns="http://example.com/ns" type="NCHZ">John Brown</name></customer></request></Body></Envelope>`

const xml4 = `<?xml version="1.0" encoding="UTF-8">

  <Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`

const xml5 = `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns="http://example.com/ns"><soapenv:Header/><soapenv:Body><ns:request><ns:customer><ns:id>123</ns:id><ns:name type="NCHZ">John Brown super long super long super long super long super long super long super long super longlong super long super long super long super long super long super long super longlong super long super long super long super long super long super long super long</ns:name></ns:customer></ns:request></soapenv:Body></soapenv:Envelope>`

func Example_output() {
	x3 := xmlfmt.FormatXML(xml3, "\t", " ")
	x2 := xmlfmt.FormatXML(xml2, "x ", " ")
	_ = x2
	_ = x3
	x1 := xmlfmt.FormatXML(xml1, "", "  ")
	fmt.Println(x1)
	// Output:
	// <root>
	//   <this>
	//     <is>a</is>
	//     <test />
	//     <message>
	//       <!-- with comment -->
	//       <org>
	//         <cn>Some org-or-other</cn>
	//         <ph>Wouldnt you like to know</ph>
	//       </org>
	//       <contact>
	//         <fn>Pat</fn>
	//         <ln>Califia</ln>
	//       </contact>
	//     </message>
	//   </this>
	// </root>
}

const w1 = `..
..<root>
..  <this>
..    <is>a</is>
..    <test />
..    <message>
..      <!-- with comment -->
..      <org>
..        <cn>Some org-or-other</cn>
..        <ph>Wouldnt you like to know</ph>
..      </org>
..      <contact>
..        <fn>Pat</fn>
..        <ln>Califia</ln>
..      </contact>
..    </message>
..  </this>
..</root>`

const w2 = `x 
x <soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns="http://example.com/ns">
x  <soapenv:Header/>
x  <soapenv:Body>
x   <ns:request>
x    <ns:customer>
x     <ns:id>123</ns:id>
x     <ns:name type="NCHZ">John Brown</ns:name>
x    </ns:customer>
x   </ns:request>
x  </soapenv:Body>
x </soapenv:Envelope>`

const w3 = `
<Envelope xmlns="http://schemas.xmlsoap.org/soap/envelope/" xmlns:_xmlns="xmlns" _xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" _xmlns:ns="http://example.com/ns">
 <Header xmlns="http://schemas.xmlsoap.org/soap/envelope/"></Header>
 <Body xmlns="http://schemas.xmlsoap.org/soap/envelope/">
  <request xmlns="http://example.com/ns">
   <customer xmlns="http://example.com/ns">
    <id xmlns="http://example.com/ns">123</id>
    <name xmlns="http://example.com/ns" type="NCHZ">John Brown</name>
   </customer>
  </request>
 </Body>
</Envelope>`

const w4 = `
<?xml version="1.0" encoding="UTF-8">
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
 <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
 <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`

const w5 = `x 
x <soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns="http://example.com/ns">
x  <soapenv:Header/>
x  <soapenv:Body>
x   <ns:request>
x    <ns:customer>
x     <ns:id>123</ns:id>
x     <ns:name type="NCHZ">John Brown super long super long super long super long super long super long super long super longlong super long super long super long super long super long super long super longlong super long super long super long super long super long super long super long</ns:name>
x    </ns:customer>
x   </ns:request>
x  </soapenv:Body>
x </soapenv:Envelope>`

func TestFormatXML_t0(t *testing.T) {
	xmlfmt.NL = "\n"
}

func TestFormatXML_t1(t *testing.T) {
	x1 := xmlfmt.FormatXML(xml1, "..", "  ")
	if x1 != w1 {
		t.Errorf("got:\n%s, want:\n%s.", x1, w1)
	}
}

func TestFormatXML_t2(t *testing.T) {
	x2 := xmlfmt.FormatXML(xml2, "x ", " ")
	if x2 != w2 {
		t.Errorf("got:\n%s, want:\n%s.", x2, w2)
	}
}

func TestFormatXML_t3(t *testing.T) {
	x3 := xmlfmt.FormatXML(xml3, "", " ")
	if x3 != w3 {
		t.Errorf("got:\n%s, want:\n%s.", x3, w3)
	}
}

func TestFormatXML_t4(t *testing.T) {
	x4 := xmlfmt.FormatXML(xml4, "", " ")
	if x4 != w4 {
		t.Errorf("got:\n%s, want:\n%s.", x4, w4)
	}
}

func TestFormatXML_t5(t *testing.T) {
	x5 := xmlfmt.FormatXML(xml5, "x ", " ")
	if x5 != w5 {
		t.Errorf("got:\n%s, want:\n%s.", x5, w5)
	}
}

const xmlc1 = `
<book> <author>Fred</author>
<!--
<price>20</price><currency>USD</currency>
-->
 <isbn>23456</isbn> </book>
<!-- c1 --> <?xml version="1.0" encoding="utf-8"?> <message name="DIS_USER_SSVC" tid="1591918441"> <!-- c2 --> <Result>0</Result> <parameter> <SsvcList> <!-- c3 --> <CFU>2</CFU> <!-- c4 --> <!-- <DATA>0261216281</DATA> --> </SsvcList> </parameter> </message>`
const wc1 = `

<book>
  <author>Fred</author>
  <!-- <price>20</price><currency>USD</currency> -->
  <isbn>23456</isbn>
</book>
<!-- c1 -->
<?xml version="1.0" encoding="utf-8"?>
<message name="DIS_USER_SSVC" tid="1591918441">
  <!-- c2 -->
  <Result>0</Result>
  <parameter>
    <SsvcList>
      <!-- c3 -->
      <CFU>2</CFU>
      <!-- c4 -->
      <!-- <DATA>0261216281</DATA> -->
    </SsvcList>
  </parameter>
</message>`

func TestFormatXML_comments_t1(t *testing.T) {
	x1 := xmlfmt.FormatXML(xmlc1, "", "  ", true)
	if x1 != wc1 {
		t.Errorf("got:\n%s, want:\n%s.", x1, wc1)
	}
}

////////////////////////////////////////////////////////////////////////////
// Benchmarking

/*
BenchmarkFormatXML show compare metrics between different xml strings.
*/
func BenchmarkFormatXML(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000, 10000, 100000} { // , 1000000
		benchmarkFormatXML(b, size)
	}
}

func benchmarkFormatXML(b *testing.B, size int) {
	b.Run(fmt.Sprintf("XML1_%d", size), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			x := xmlfmt.FormatXML(xml1, "..", "  ")
			_ = x
		}
	})

	b.Run(fmt.Sprintf("XML2_%d", size), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			x := xmlfmt.FormatXML(xml2, "x ", " ")
			_ = x
		}
	})

	b.Run(fmt.Sprintf("XML3_%d", size), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			x := xmlfmt.FormatXML(xml3, "", " ")
			_ = x
		}
	})
}
