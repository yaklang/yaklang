package tests

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_Extend_Class(t *testing.T) {
	t.Run("test extend constant ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			int a = 0; 
		}
	public class B extends A{}
	public class C extends B{}
	public class Main{
		public static void main(String[] args) {
		C C = new C();
		println(C.a); // 0
}}
		`, []string{
			"0",
		}, t)
	})
	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class Q {
		int a = 0; 
		public void getA() {
			return this.a;
		}
	}
	class A extends Q{}
	public class Main{
	public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.a=1;
		println(a.getA());
	}
}
		`, []string{
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
		}, t)
	})
	t.Run("test function call", func(t *testing.T) {
		code := `
class A{
	public static void b(){
		return "1";
	}
}
public class main{
	public static void main(String[] args){
		A.a();
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `A.a() as $call`, map[string][]string{
			"call": {"Undefined-A.a(A)"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public class Q {
			int a = 0; 
			public void getA(){
				return this.a;
			}
			
			public void setA(int par){
				this.a=par;
			}
		}
		class A extends Q{}
		public class Main{
			public static void main(String[] args) {
				A a = new A(); 
				println(a.getA());
				a.setA(1);
				println(a.getA());
			}
		}
		`, []string{
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-par, this.a)]",
		}, t)
	})

	t.Run("test class value", func(t *testing.T) {
		code := `
class A {
	int a;

	A(int a){
		this.a = a;
	}
}

class B {
    A A;

	B(A a){
		this.A = a;
	}
}

public class C {
    public static void main(String[] args) {
 		A a = new A(1);
        B b = new B(a);
		
		b.A.a = 2; 
        int o1 = a.a; 	
        int o2 = b.A.a;	
        a.a = 3; 
        int o3 = a.a; 	
        int o4 = b.A.a;
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `
			o1 #-> as $o1
			o2 #-> as $o2
			o3 #-> as $o3
			o4 #-> as $o4
		`, map[string][]string{
			"o1": {"2"},
			"o2": {"2"},
			"o3": {"3"},
			"o4": {"3"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	// todo: 跨过程会导致指针失效
	t.Run("test class value cross function", func(t *testing.T) {
		t.Skip()
		code := `
		class A {
			int a;
		
			A(int a){
				this.a = a;
			}
			public static void main(String[] args) {
		
			}
		}
		
		class B {
			A A;
		
			B(A a){
				this.A = a;
			}
		
			public static void main(String[] args) {
				
			}
		}
		
		class Main {
			static B test(A a) {
				return new B(a);
			}
		
			public static void main(String[] args) {
				A a = new A(1);
				B b = test(a);
				
				b.A.a = 2; 
				int o1 = a.a;
				int o2 = b.A.a;
				a.a = 3; 
				int o3 = a.a;
				int o4 = b.A.a;
			}
		}
		 `
		ssatest.CheckSyntaxFlow(t, code, `
		 o1 #-> as $o1
		 o2 #-> as $o2
		 o3 #-> as $o3
		 o4 #-> as $o4
	 `, map[string][]string{
			"o1": {"2"},
			"o2": {"2"},
			"o3": {"3"},
			"o4": {"3"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	// todo
	t.Run("test class value if", func(t *testing.T) {
		t.Skip()
		code := `
class A {
	int a;

	A(int a){
		this.a = a;
	}
}

class B {
    A A;

	B(A a){
		this.A = a;
	}
}

public class C {
    public static void main(String[] args) {
 		A a1 = new A(1);
		A a2 = new A(2);

        B b = new B(a1);
        if (a.a == 2) {
            B b = new B(a2); 
        }

		a1.a = 3; 
        int o1 = a2.a; 	
        int o2 = b.A.a;	
        a2.a = 4; 
        int o3 = a1.a; 	
        int o4 = b.A.a;	
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `
			o1 #-> as $o1
			o2 #-> as $o2
			o3 #-> as $o3
			o4 #-> as $o4
		`, map[string][]string{
			"": {""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestJava_Construct(t *testing.T) {
	t.Run("no construct", func(t *testing.T) {
		code := `
	public	class A {
			int num = 0;
			public int getNum() {
				super();
				return this.num;
			}
		}
public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getNum());
		}
}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		code := `
public class A {
	private int num1=0;
	private int num2=0;
	
	public A(int num1,int num2) {
		this.num1 = num1;
		this.num2 = num2;
	}
	public int getNum1() {
		return this.num1;
	}
	public int getNum2(){
		return this.num2;
	}
}
public class Main{
	public static void main(String[] args) {
		A a = new A(1,2);
		println(a.getNum1());
		println(a.getNum2());
	}
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-a.getNum1(valid)(Function-A.A(Undefined-A,1,2)) member[side-effect(Parameter-num1, this.num1)]",
			"Undefined-a.getNum2(valid)(Function-A.A(Undefined-A,1,2)) member[side-effect(Parameter-num2, this.num2)]",
		}, t)
	})
}

func TestJava_OOP_Enum(t *testing.T) {
	t.Skip()
	t.Run("test simple top-level enum", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A,B,C;
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a);
			}
		}
		`, []string{
			"Undefined-a(valid)",
		}, t)
	})

	t.Run("test  top-level enum with constructor", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A(1,2),
			B(3,4),
			C(4,5);
			private final int num1;
			private final int num2;

			A(int par1,int par2){
				this.num1=par1;
				this.num2=par2;
			}

			public int getNum1(){
			return this.num1;
			}

			public int getNum2(){
			return this.num2;
			}
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a.getNum1());
			println(a.getNum2());
			}
		}
		`, []string{
			"Undefined-a.getNum1(valid)(Undefined-a(valid)) member[Undefined-a.num1(valid)]",
			"Undefined-a.getNum2(valid)(Undefined-a(valid)) member[Undefined-a.num2(valid)]",
		}, t)
	})

}

func TestJava_OOP_MemberClass(t *testing.T) {
	t.Skip()
	t.Run("test no-static inner class ", func(t *testing.T) {
		code := `
public class Outer {
    public  class Inner{
        int a = 1;
		// TODO: if this constructor is defined, it will be an error
        // public Inner(int par){
        //     this.a=par;
        // }
        public int getA(){
            return this.a;
        }
    }
}

public class Main{
    public static void main(String[] args) {
        Outer outer = new Outer();
        Outer.Inner inner =outer.new Inner(5);
        println(inner);
		println(inner.getA());
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-inner",
		}, t)
	})
}

func TestJava_OOP_Static_Member(t *testing.T) {
	t.Run("test call self static member", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public static void main(String[] args) {
            println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test static variable and method within a class (arg is a)", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public void main(String[] args) {
           println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test static variable and  method within a class (arg is this.a)", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public void main(String[] args) {
            println(this.a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and  method within a class ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(this.a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and  method within a class ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and static method within a class", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test cross class static variable calls ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
package org.example;
public class Test {
		static int a = 1;
	}

public class Main {
    public void main(String[] args) {
           println(Test.a);
        }
 }
	
			`, []string{"1"}, t)
	})

}

func TestJava_Package(t *testing.T) {
	t.Run("simple test", func(t *testing.T) {
		code := `
	package org.example.A;
	public	class A {
			int num = 0;
			public int getNum() {
				return this.num;
			}
		}
public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getNum());
		}
}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
		}, t)
	})
	t.Run("test no package with constructor and direct use member", func(t *testing.T) {
		code := `package com.example.A;
public class A{
	public int num1=0;
	public A(int num1){
		this.num1 = num1;
	}
}
class Main{
	public static void main(String[] args) {
		A a = new A();
		println(a.num1);
	}
}
`
		ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-num1, this.num1)"}, t)
	})
	t.Run("test no package with constructor and no direct use member", func(t *testing.T) {
		code := `package com.example.A;
public class A{
	public int num1=0;
	public A(int num1){
		this.num1 = num1;
	}
	public int getNum(){
		return this.num1;
	}
}
class Main{
	public static void main(String[] args) {
		A a = new A();
		println(a.getNum());
	}
}
`
		ssatest.CheckPrintlnValue(code, []string{"Undefined-a.getNum(valid)(Function-A.A(Undefined-A)) member[side-effect(Parameter-num1, this.num1)]"}, t)
	})
	t.Run("test package with constructor", func(t *testing.T) {
		code := `
	package com.example.A;
	public class A {
	private int num1=0;
	private int num2=0;
	
	public A(int num1,int num2) {
		this.num1 = num1;
		this.num2 = num2;

	}
	public int getNum1() {
		return this.num1;
	}
	public int getNum2(){
		return this.num2;
	}
	}
	public class Main{
			public static void main(String[] args) {
			A a = new A(1,2);
			println(a.getNum1());
			println(a.getNum2());
			}
	}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-a.getNum1(valid)(Function-A.A(Undefined-A,1,2)) member[side-effect(Parameter-num1, this.num1)]", "Undefined-a.getNum2(valid)(Function-A.A(Undefined-A,1,2)) member[side-effect(Parameter-num2, this.num2)]",
		}, t)
	})
}

func TestConstruct(t *testing.T) {
	code := `package com.example.demo1;

class Main {
    public int a = 1;

    public Main(int a) {
        this.a = a;
    }
}
class Test{
    public static void main(){
        Main main = new Main(2);
        println(main.a);
    }
}`
	ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-a, this.a)"}, t)
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"2"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
func TestJava_Instantiation(t *testing.T) {
	t.Run("Instantiate a non-existent object", func(t *testing.T) {
		code := `
public class Main{
    public static void main(String[] args) {
        File tempFile = new File();
		println(tempFile);
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-File(Undefined-File)",
		}, t)
	})

	t.Run("instantiate an existing object ", func(t *testing.T) {
		code := `
public class File{
}

public class Main{
    public static void main(String[] args) {
        File tempFile = new File();
		println(tempFile);
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"Undefined-File(Undefined-File)",
		}, t)
	})
	t.Run("test undefind function call", func(t *testing.T) {
		code := `class tes1 {
    public void function(test t) {
        for (int a = 0; ; ) {
            println(t.a());
        }
    }
}`
		ssatest.CheckPrintlnValue(code, []string{"ParameterMember-parameter[1].a(Parameter-t)"}, t)
	})
}

func TestJava_Method(t *testing.T) {
	t.Run("get static method by variable name", func(t *testing.T) {
		code := `
public class ImageUtils{
    public  InputStream getFile(String imagePath){
    }
    public static byte[] readFile(String url){
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `*readFile as $fun`, map[string][]string{
			"fun": {"Function-ImageUtils.readFile"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("new java blueprint by fullName", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.java", `
package com.example.demo1;
class A{
	public void method(int a){
		println(a);
	}
}
`)
		fs.AddFile("b.java", `
package com.example.demo2;
import com.example.demo1.A;
class B{
	public static void main(string[] args){
		com.example.demo1.A a = new com.example.demo1.A();
		
	}
}`)
		ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
func TestImport1(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.java", `package com.simp.sso.service.imp;

import java.security.InvalidKeyException;
import java.security.NoSuchAlgorithmException;
import javax.crypto.Cipher;
import javax.crypto.NoSuchPaddingException;
import javax.crypto.SecretKey;
import javax.crypto.spec.SecretKeySpec;

/* loaded from: iam.app.5.0.enc.jar:com/simp/sso/service/imp/Encryption.class */
public class Encryption {
    public static final int ENC_ALG_DES3_CODE_NULL = 0;
    public static final String ENC_ALG_DES3 = "3des";
    public static final int ENC_ALG_DES3_CODE = 100;
    public static final String ENC_ALG_DES3_EX = "3des_ex";
    public static final int ENC_ALG_DES3_EX_CODE = 200;
    public static final String DES3 = "TripleDES";
    public static final String DES3_TAIL = "/ECB/PKCS5Padding";
    private static final String str1 = "acftyuij";
    private static final String str2 = "7653!$#@";
    private static final String str3 = "R$GV*&(<";
    private byte[] kbs = null;
    private int encryptAlg = 0;
    private Cipher cipherEnc = null;
    private Cipher cipherDec = null;

    public Encryption(int encAlg, String keyStr) {
        ini(encAlg, keyStr);
    }

    public Encryption(String encAlg, String keyStr) {
        ini(encAlg, keyStr);
    }

    public boolean ini(int encryptAlg, String keyStr) {
        if (encryptAlg == 0) {
            return false;
        }
        this.kbs = "acftyuij7653!$#@R$GV*&(<".getBytes();
        this.encryptAlg = encryptAlg;
        saltKey(keyStr);
        return true;
    }

    public boolean ini(String encAlg, String keyStr) {
        int alg;
        if ("3des".equalsIgnoreCase(encAlg)) {
            alg = 100;
        } else if ("3des_ex".equalsIgnoreCase(encAlg)) {
            alg = 200;
        } else {
            alg = 0;
        }
        return ini(alg, keyStr);
    }

    private void saltKey(String keyStr) {
        byte[] bs = keyStr.getBytes();
        byte a = bs[0];
        byte b = bs[bs.length - 1];
        byte c = bs[bs.length / 2];
        this.kbs[0] = a;
        this.kbs[this.kbs.length / 2] = b;
        this.kbs[this.kbs.length - 1] = c;
    }

    private byte[] saltBySid(String sid) {
        byte[] bs = sid.getBytes();
        byte a = bs[bs.length - 3];
        byte b = bs[bs.length - 2];
        byte c = bs[bs.length - 1];
        byte[] rv = new byte[this.kbs.length];
        System.arraycopy(this.kbs, 0, rv, 0, rv.length);
        rv[0] = (byte) (rv[0] ^ a);
        int length = this.kbs.length / 2;
        rv[length] = (byte) (rv[length] ^ b);
        int length2 = this.kbs.length - 1;
        rv[length2] = (byte) (rv[length2] ^ c);
        return rv;
    }

    public Cipher getCipherEnc(String sid) {
        if (this.encryptAlg == 0) {
            return null;
        }
        if (100 == this.encryptAlg) {
            if (this.cipherEnc == null) {
                this.cipherEnc = createCipher(1, this.kbs);
            }
            return this.cipherEnc;
        } else if (200 == this.encryptAlg) {
            byte[] ks = saltBySid(sid);
            return createCipher(1, ks);
        } else {
            return null;
        }
    }

    public int getEncryptAlg() {
        return this.encryptAlg;
    }

    public Cipher getCipherDec(int decAlg, String sid) {
        if (decAlg == 0) {
            return null;
        }
        if (100 == decAlg) {
            if (this.cipherDec == null) {
                this.cipherDec = createCipher(2, this.kbs);
            }
            return this.cipherDec;
        } else if (200 == decAlg) {
            byte[] ks = saltBySid(sid);
            return createCipher(2, ks);
        } else {
            return null;
        }
    }

    private Cipher createCipher(int mode, byte[] ks) {
        try {
            SecretKey key = new SecretKeySpec(ks, DES3);
            Cipher rv = Cipher.getInstance("TripleDES/ECB/PKCS5Padding");
            rv.init(mode, key);
            return rv;
        } catch (InvalidKeyException e) {
            e.printStackTrace();
            return null;
        } catch (NoSuchAlgorithmException e2) {
            e2.printStackTrace();
            return null;
        } catch (NoSuchPaddingException e3) {
            e3.printStackTrace();
            return null;
        }
    }
}`)
	ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		for _, program := range prog {
			program.Show()
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestJava_Super_Class(t *testing.T) {
	t.Run("test super class's field", func(t *testing.T) {
		code := `
		class ParentClass {
			public String parentString = "This is a parent string.";
		}

		class ChildClass extends ParentClass {
			public String childString = "This is a child string.";

			public void printParentString() {
			println(super.parentString);
		}
		}
		`
		ssatest.CheckPrintlnValue(code, []string{"\"This is a parent string.\""}, t)
	})

	t.Run("test super class's static field", func(t *testing.T) {
		code := `
class ParentClass {
    public static String parentString = "This is a parent string.";
}

class ChildClass extends ParentClass {
    public String childString = "This is a child string.";

    public void printParentString() {
        println(super.parentString);
    }
}
`
		ssatest.CheckPrintlnValue(code, []string{"\"This is a parent string.\""}, t)
	})

	t.Run("test super class method", func(t *testing.T) {
		code := `
class ParentClass {
    public String getName() {
		return "Parent";
    }
}

class ChildClass extends ParentClass {
    @Override
    public String getName() {
		println(super.getName());
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"\"Parent\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test super class static method", func(t *testing.T) {
		code := `
class ParentClass {
    public static String getName() {
		return "Parent";
    }
}

class ChildClass extends ParentClass {
    public String getName() {
		println(super.getName());
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"\"Parent\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestXXECompareConst(t *testing.T) {
	code := `import org.xml.sax.SAXException;
import org.xml.sax.XMLReader;
import org.xml.sax.helpers.XMLReaderFactory;
import javax.xml.parsers.ParserConfigurationException;
import javax.xml.parsers.SAXParser;
import javax.xml.parsers.SAXParserFactory;
import org.xml.sax.helpers.DefaultHandler;


public class XMLReaderFactorySafe {
    public void parseXml(String xml) {
        try {
            XMLReader reader = XMLReaderFactory.createXMLReader();
            reader.setFeature("http://apache.org/xml/features/demo-xxx", true);
            reader.setContentHandler(new DefaultHandler());
            reader.parse(xml);
        } catch (SAXException | ParserConfigurationException e) {
            e.printStackTrace();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}`
	ssatest.CheckSyntaxFlow(t, code, `
XMLReaderFactory?{<typeName>?{have:'org.xml.sax.helpers.XMLReaderFactory'}} as $factory;
$factory.createXMLReader() as $reader;
$reader.setFeature?(,*?{=="http://xml.org/sax/features/external-general-entities"},*?{==false}) as $excludeCall;
`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
