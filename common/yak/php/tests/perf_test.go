package tests

import (
	"embed"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

//go:embed perfdata/***
var perfFs embed.FS

func benchmarkFrontendFixture(b *testing.B, fixture string) {
	var (
		raw []byte
		err error
	)
	switch {
	case strings.HasPrefix(fixture, "syntax/"):
		raw, err = syntaxFs.ReadFile(fixture)
	case strings.HasPrefix(fixture, "perfdata/"):
		raw, err = perfFs.ReadFile(fixture)
	default:
		b.Fatalf("unsupported fixture root for %s", fixture)
	}
	if err != nil {
		b.Fatalf("read fixture %s: %v", fixture, err)
	}

	src := string(raw)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := php2ssa.Frontend(src, phpTestAntlrCache); err != nil {
			b.Fatalf("parse fixture %s: %v", fixture, err)
		}
	}
}

func BenchmarkFrontendHelloFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "syntax/hello.php")
}

func BenchmarkFrontendPfsenseSystemInformationFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "syntax/pfsense/system_information.widget.php")
}

func BenchmarkFrontendGravUtilsFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Utils.php.fixture")
}

func BenchmarkFrontendGravDebuggerFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Debugger.php.fixture")
}

func BenchmarkFrontendGravPageFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Page__Page.php.fixture")
}

func BenchmarkFrontendGravPagesFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Page__Pages.php.fixture")
}

func BenchmarkFrontendGravExtensionFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Twig__Extension__GravExtension.php.fixture")
}

func BenchmarkFrontendGravPageCollectionFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Common__Page__Collection.php.fixture")
}

func BenchmarkFrontendGravFlexCollectionFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Framework__Flex__FlexCollection.php.fixture")
}

func BenchmarkFrontendGravFlexIndexFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Framework__Flex__FlexIndex.php.fixture")
}

func BenchmarkFrontendGravFlexObjectFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/grav/system__src__Grav__Framework__Flex__FlexObject.php.fixture")
}

func BenchmarkFrontendPrestaShopAdminControllerFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/prestashop/classes__controller__AdminController.php.fixture")
}

func BenchmarkFrontendPrestaShopMailPreviewVariablesBuilderFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/prestashop/src__Adapter__MailTemplate__MailPreviewVariablesBuilder.php.fixture")
}

func BenchmarkFrontendPrestaShopCarrierFeatureContextFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "perfdata/prestashop/tests__Integration__Behaviour__Features__Context__Domain__Carrier__CarrierFeatureContext.php.fixture")
}
