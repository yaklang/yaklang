package types

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

// TestSlashToDot verifies the fast '/'->'.' conversion against a table of edge cases.
func TestSlashToDot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"X", "X"},
		{"/", "."},
		{"a/b", "a.b"},
		{"java/lang/String", "java.lang.String"},
		{"no_slash_here", "no_slash_here"},
		{"/leading", ".leading"},
		{"trailing/", "trailing."},
		{"a//b", "a..b"},
		{"com/hazelcast/Foo$Bar", "com.hazelcast.Foo$Bar"},
	}
	for _, c := range cases {
		if got := SlashToDot(c.in); got != c.want {
			t.Errorf("SlashToDot(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSlashToDotEquivalence is a property test: SlashToDot must be byte-identical to the
// strings.Replace(s, "/", ".", -1) it replaces, for randomized inputs.
func TestSlashToDotEquivalence(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	alphabet := []byte("ab/.$_/0/")
	for i := 0; i < 5000; i++ {
		n := r.Intn(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		s := string(b)
		want := strings.Replace(s, "/", ".", -1)
		if got := SlashToDot(s); got != want {
			t.Fatalf("mismatch for %q: SlashToDot=%q strings.Replace=%q", s, got, want)
		}
	}
}

// TestSlashToDotNoAllocWhenClean documents the fast path: an input without '/' must be
// returned without copying (same backing string header), so the common case is free.
func TestSlashToDotNoAllocWhenClean(t *testing.T) {
	s := "already.dotted.name"
	got := SlashToDot(s)
	if got != s {
		t.Fatalf("SlashToDot(%q) = %q", s, got)
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = SlashToDot("java.lang.String")
	})
	if allocs != 0 {
		t.Errorf("SlashToDot on a slash-free string allocated %.0f times, want 0", allocs)
	}
}

// TestCachedClassTypeInterning verifies the flyweight: the same internal name yields the
// same *JavaClass instance (interned) with the dotted Name, and distinct names differ.
func TestCachedClassTypeInterning(t *testing.T) {
	a := cachedClassType("java/lang/Object")
	b := cachedClassType("java/lang/Object")
	if a != b {
		t.Fatalf("expected identical interned *JavaClass for the same name, got %p vs %p", a, b)
	}
	if a.Name != "java.lang.Object" {
		t.Fatalf("interned Name = %q, want java.lang.Object", a.Name)
	}
	c := cachedClassType("java/util/List")
	if c == a {
		t.Fatalf("distinct class names must not share an instance")
	}
	if c.Name != "java.util.List" {
		t.Fatalf("Name = %q, want java.util.List", c.Name)
	}
}

// TestCachedClassTypeConcurrent stresses concurrent interning; run with -race to prove the
// sync.Map + soft-cap counter is race-free and never returns an inconsistent Name.
func TestCachedClassTypeConcurrent(t *testing.T) {
	const goroutines = 50
	const names = 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < names; i++ {
				internal := fmt.Sprintf("pkg/sub%d/Cls%d", i%17, i)
				jc := cachedClassType(internal)
				want := SlashToDot(internal)
				if jc.Name != want {
					t.Errorf("concurrent: Name=%q want=%q", jc.Name, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestParseJavaDescriptionPrimitives checks every primitive descriptor and the remaining
// string returned by the parser.
func TestParseJavaDescriptionPrimitives(t *testing.T) {
	cases := map[string]string{
		"B": JavaByte, "C": JavaChar, "D": JavaDouble, "F": JavaFloat,
		"I": JavaInteger, "J": JavaLong, "S": JavaShort, "Z": JavaBoolean, "V": JavaVoid,
	}
	for desc, wantName := range cases {
		typ, rest, err := ParseJavaDescription(desc + "REST")
		if err != nil {
			t.Fatalf("ParseJavaDescription(%q) err: %v", desc, err)
		}
		if rest != "REST" {
			t.Errorf("desc %q rest = %q, want REST", desc, rest)
		}
		p, ok := typ.RawType().(*JavaPrimer)
		if !ok {
			t.Fatalf("desc %q: RawType %T, want *JavaPrimer", desc, typ.RawType())
		}
		if p.Name != wantName {
			t.Errorf("desc %q: name %q, want %q", desc, p.Name, wantName)
		}
	}
}

// TestParseJavaDescriptionClass verifies L...; parsing produces a dotted class name and
// consumes exactly through the ';'.
func TestParseJavaDescriptionClass(t *testing.T) {
	typ, rest, err := ParseJavaDescription("Ljava/lang/String;Lnext/Type;")
	if err != nil {
		t.Fatal(err)
	}
	if rest != "Lnext/Type;" {
		t.Errorf("rest = %q, want Lnext/Type;", rest)
	}
	jc, ok := typ.RawType().(*JavaClass)
	if !ok {
		t.Fatalf("RawType %T, want *JavaClass", typ.RawType())
	}
	if jc.Name != "java.lang.String" {
		t.Errorf("name %q, want java.lang.String", jc.Name)
	}
}

// TestParseJavaDescriptionArray checks single and multi-dimensional arrays.
func TestParseJavaDescriptionArray(t *testing.T) {
	t1, _, err := ParseJavaDescription("[I")
	if err != nil {
		t.Fatal(err)
	}
	if !t1.IsArray() || t1.ArrayDim() != 1 {
		t.Errorf("[I: IsArray=%v dim=%d, want true/1", t1.IsArray(), t1.ArrayDim())
	}
	t2, _, err := ParseJavaDescription("[[Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	if !t2.IsArray() || t2.ArrayDim() != 2 {
		t.Errorf("[[L...: IsArray=%v dim=%d, want true/2", t2.IsArray(), t2.ArrayDim())
	}
}

// TestParseMethodDescriptor checks param counts and return types for representative method
// descriptors -- the hot path that previously re-ran strings.Replace per occurrence.
func TestParseMethodDescriptor(t *testing.T) {
	cases := []struct {
		desc       string
		paramCount int
		retName    string // JavaPrimer name when applicable
	}{
		{"()V", 0, JavaVoid},
		{"(II)I", 2, JavaInteger},
		{"(Ljava/lang/String;)Z", 1, JavaBoolean},
		{"(Ljava/lang/String;[IJ)V", 3, JavaVoid},
	}
	for _, c := range cases {
		typ, err := ParseMethodDescriptor(c.desc)
		if err != nil {
			t.Fatalf("ParseMethodDescriptor(%q) err: %v", c.desc, err)
		}
		ft := typ.FunctionType()
		if ft == nil {
			t.Fatalf("desc %q: FunctionType nil", c.desc)
		}
		if len(ft.ParamTypes) != c.paramCount {
			t.Errorf("desc %q: params=%d, want %d", c.desc, len(ft.ParamTypes), c.paramCount)
		}
		if p, ok := ft.ReturnType.RawType().(*JavaPrimer); ok {
			if p.Name != c.retName {
				t.Errorf("desc %q: ret %q, want %q", c.desc, p.Name, c.retName)
			}
		}
	}
}

// TestParseMethodDescriptorInternsClasses proves the optimization end-to-end: parsing two
// method descriptors that both reference java/lang/String reuses the same interned leaf.
func TestParseMethodDescriptorInternsClasses(t *testing.T) {
	a, err := ParseMethodDescriptor("(Ljava/lang/String;)V")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseMethodDescriptor("(ILjava/lang/String;)I")
	if err != nil {
		t.Fatal(err)
	}
	pa := a.FunctionType().ParamTypes[0].RawType()
	pb := b.FunctionType().ParamTypes[1].RawType()
	if pa != pb {
		t.Fatalf("expected the java/lang/String leaf to be interned and shared across descriptors")
	}
}

// BenchmarkSlashToDot vs BenchmarkStringsReplaceSlash is the algorithm comparison for the
// '/'->'.' conversion on a typical class name.
func BenchmarkSlashToDot(b *testing.B) {
	s := "com/hazelcast/client/impl/protocol/DefaultMessageTaskFactoryProvider"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SlashToDot(s)
	}
}

func BenchmarkStringsReplaceSlash(b *testing.B) {
	s := "com/hazelcast/client/impl/protocol/DefaultMessageTaskFactoryProvider"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = strings.Replace(s, "/", ".", -1)
	}
}

// BenchmarkParseMethodDescriptorCached vs Uncached shows the flyweight effect on the hot
// descriptor path (same descriptor parsed repeatedly, as in a real constant pool).
func BenchmarkParseMethodDescriptor(b *testing.B) {
	desc := "(Ljava/lang/String;Ljava/util/Map;I[Ljava/lang/Object;)Ljava/lang/String;"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ParseMethodDescriptor(desc)
	}
}
