package rust

import (
	"debug/elf"
	"os"
	"strings"
	"testing"
)

func TestDemangleV0AndLegacy(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"_RNvCs15kBYyAo9fc_7mycrate3add", "mycrate::add"},
		{"__RNvCs15kBYyAo9fc_7mycrate3add", "mycrate::add"},
		{"_ZN3foo3bar17h0123456789abcdefE", "foo::bar"},
		{"_ZN3foo3bar17h0123456789abcdefE.llvm.123", "foo::bar"},
		{"_RINvMNtNtCs4NRVxsYgnAr_4core3fmt2rtNtB3_8Argument11new_displayxECs4FrXkKnA8Dq_10rust_hello", "<core::fmt::rt::Argument>::new_display::<i64>"},
		{"_RNvCs4FrXkKnA8Dq_10rust_hello3add", "rust_hello::add"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Demangle(tc.name)
			if !ok || got != tc.want {
				t.Fatalf("Demangle = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}
}

func TestDemangleRejectsNonRustNames(t *testing.T) {
	for _, name := range []string{
		"main.Add", "main", "_ZN3foo3barEv", "_Z3fooi", "_RNonsense", "_RNv",
		"_ZN3foo3bar17hzzzzzzzzzzzzzzzzE", "_ZN3foo3bar17h0123E",
	} {
		if got, ok := Demangle(name); ok {
			t.Errorf("%q decoded to %q", name, got)
		}
		if IsMangled("main.Add") {
			t.Fatal("plain name reported as mangled")
		}
	}
}

func TestMalformedInputDoesNotPanic(t *testing.T) {
	for _, name := range []string{
		"_R", "_RN", "_RNv", "_RI", "_RB", "_RB_", "_RINvE",
		"_R" + strings.Repeat("N", 64), "_RNvC", "_RINv",
		"_RNvCs15kBYyAo9fc_9999x",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Demangle(%q) panicked: %v", name, r)
				}
			}()
			_, _ = Demangle(name)
		}()
	}
}

func TestIsMangledRequiresRustHashForLegacy(t *testing.T) {
	if !IsMangled("_ZN3foo3bar17h0123456789abcdefE") {
		t.Fatal("legacy Rust name rejected")
	}
	if IsMangled("_ZN3foo3barEv") {
		t.Fatal("C++ name accepted as Rust")
	}
	if !IsMangled("__RNvCs15kBYyAo9fc_7mycrate3add") {
		t.Fatal("Mach-O v0 name rejected")
	}
}

func TestExpandPunycode(t *testing.T) {
	// RFC 3492 example: bcher-kva decodes to bücher.
	got, err := expandPunycode("bcher_kva")
	if err != nil || got != "bücher" {
		t.Fatalf("expandPunycode = %q, %v", got, err)
	}
	if _, err := expandPunycode("abc-!!"); err == nil {
		t.Fatal("invalid punycode accepted")
	}
}

func TestV0RealForms(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		// An empty identifier is a single 0; the next length starts right after.
		{"_RNvNCNvNtCs2AWtUsOyxgP_3std9panicking12default_hook011FIRST_PANIC", "std::panicking::default_hook::{closure#0}::FIRST_PANIC"},
		// Nested closures, one with disambiguator 1.
		{"_RNCNCNCNvNtCs2AWtUsOyxgP_3std2rt19lang_start_internal00s_0B9_", "std::rt::lang_start_internal::{closure#0}::{closure#0}::{closure#1}"},
		// Const generic namespace K and a compiler-internal static.
		{"_RNvNCNKNvNtNtCs2AWtUsOyxgP_3std9panicking11panic_count17LOCAL_PANIC_COUNT0s_023___RUST_STD_INTERNAL_VAL", "std::panicking::panic_count::LOCAL_PANIC_COUNT::{K#0}::{closure#1}::__RUST_STD_INTERNAL_VAL"},
		// Generic instantiation whose argument is a closure type.
		{"_RNCINvNtNtCs2AWtUsOyxgP_3std6thread7current17with_current_nameNCNCNvNtB8_9panicking12default_hook00uE0B8_", "std::thread::current::with_current_name::<std::panicking::default_hook::{closure#0}::{closure#0}, ()>::{closure#0}"},
	} {
		got, ok := Demangle(tc.name)
		if !ok || got != tc.want {
			t.Fatalf("Demangle = %q, %v; want %q", got, ok, tc.want)
		}
	}
}

// TestRealBinaryCorpus decodes the symbols of a Rust binary. Set
// REIFY_RUST_CORPUS to an ELF file; the test is skipped otherwise so the suite
// stays hermetic.
func TestRealBinaryCorpus(t *testing.T) {
	path := os.Getenv("REIFY_RUST_CORPUS")
	if path == "" {
		t.Skip("set REIFY_RUST_CORPUS to a Rust ELF binary")
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	symbols, err := file.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	total, decoded := 0, 0
	for _, s := range symbols {
		if !IsMangled(s.Name) {
			continue
		}
		total++
		if _, ok := Demangle(s.Name); ok {
			decoded++
			continue
		}
		if total-decoded <= 8 {
			t.Logf("not decoded: %s", s.Name)
		}
	}
	t.Logf("Rust symbols: total=%d decoded=%d failed=%d", total, decoded, total-decoded)
	if total == 0 {
		t.Fatal("no Rust symbols in corpus")
	}
	if decoded*100 < total*98 {
		t.Fatalf("decoded %d of %d Rust symbols", decoded, total)
	}
}
