package cve

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"strings"
	"testing"
	"time"
)

func TestQueryCWE(t *testing.T) {
	err := TranslatingCWE("/Users/v1ll4n/yakit-projects/openai-key.txt", 1, "")
	if err != nil {
		panic(err)
	}
	//escdb := consts.GetGormCVEDescriptionDatabase()
	//descdb.AutoMigrate(&cveresources.CWE{})
	//for cwe := range cveresources.YieldCWEs(consts.GetGormCVEDatabase().Model(&cveresources.CWE{}), context.Background()) {
	//	cwe := cwe
	//
	//	cwe, err := MakeOpenAITranslateCWE(cwe, getKey(), `http://127.0.0.1:7890`)
	//	if err != nil {
	//		panic(err)
	//	}
	//	err = cveresources.CreateOrUpdateCWE(descdb, cwe.IdStr, cwe)
	//	if err != nil {
	//		log.Error(err)
	//	}
	//}
}

func TestQuery(t *testing.T) {
	_, num := cvequeryops.Query("./date.db", cvequeryops.CVE("CVE-2017-0144"))
	if num != 1 {
		fmt.Println("option cve Fail")
	}

	resCve, num := cvequeryops.Query("./date.db", cvequeryops.CWE("CWE-89"))
	for _, cve := range resCve {
		if !cve.CWE("CWE-89") {
			fmt.Println("option CWE Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Product("php"))
	fmt.Println(len(resCve))

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Product("iis"))
	for _, cve := range resCve {
		if !strings.Contains(cve.Product, "internet_information_server") {
			fmt.Println("option product Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Vendor("apple"))
	for _, cve := range resCve {
		if !strings.Contains(cve.Vendor, "apple") {
			fmt.Println("option vendor Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Before(2022, 1, 3))
	for _, cve := range resCve {
		formatTime := "2022-01-03 00:00:00"
		testTime, err := time.Parse("2006-01-02 15:04:05", formatTime)
		if err != nil {
			panic("parse time error")
		}
		if !cve.PublishedDate.Before(testTime) {
			fmt.Println("option before Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.After(2022, 1, 3))
	for _, cve := range resCve {
		formatTime := "2022-01-03 00:00:00"
		testTime, err := time.Parse("2006-01-02 15:04:05", formatTime)
		if err != nil {
			panic("parse time error")
		}
		if !cve.PublishedDate.After(testTime) {
			fmt.Println("option after Fail ")
			break
		}
	}
}

func TestFunc(t *testing.T) {
	cvequeryops.MakeCtScript("php", "./date.db", "php", "./")
}

func TestMigrate(t *testing.T) {
	_migrateTable()
}

func TestQueryCVEFixName(t *testing.T) {
	baseInfoString := `apt 2.4.9
base-files 12ubuntu4.3
base-passwd 3.5.52build1
bash 5.1-6ubuntu1
bsdutils 1:2.37.2-4ubuntu3
coreutils 8.32-4.1ubuntu1
dash 0.5.11+git20210903+057cd650a4ed-3build1
debconf 1.5.79ubuntu1
debianutils 5.5-1ubuntu2
diffutils 1:3.8-0ubuntu2
dpkg 1.21.1ubuntu2.2
e2fsprogs 1.46.5-2ubuntu1.1
findutils 4.8.0-1ubuntu3
gcc-12-base 12.1.0-2ubuntu1~22.04
gpgv 2.2.27-3ubuntu2.1
grep 3.7-1build1
gzip 1.10-4ubuntu4.1
hostname 3.23ubuntu2
init-system-helpers 1.62
libacl1 2.3.1-1
libapt-pkg6.0 2.4.9
libattr1 1:2.5.1-1build1
libaudit-common 1:3.0.7-1build1
libaudit1 1:3.0.7-1build1
libblkid1 2.37.2-4ubuntu3
libbz2-1.0 1.0.8-5build1
libc-bin 2.35-0ubuntu3.1
libc6 2.35-0ubuntu3.1
libcap-ng0 0.7.9-2.2build3
libcap2 1:2.44-1ubuntu0.22.04.1
libcom-err2 1.46.5-2ubuntu1.1
libcrypt1 1:4.4.27-1
libdb5.3 5.3.28+dfsg1-0.8ubuntu3
libdebconfclient0 0.261ubuntu1
libext2fs2 1.46.5-2ubuntu1.1
libffi8 3.4.2-4
libgcc-s1 12.1.0-2ubuntu1~22.04
libgcrypt20 1.9.4-3ubuntu3
libgmp10 2:6.2.1+dfsg-3ubuntu1
libgnutls30 3.7.3-4ubuntu1.2
libgpg-error0 1.43-3
libgssapi-krb5-2 1.19.2-2ubuntu0.2
libhogweed6 3.7.3-1build2
libidn2-0 2.3.2-2build1
libk5crypto3 1.19.2-2ubuntu0.2
libkeyutils1 1.6.1-2ubuntu3
libkrb5-3 1.19.2-2ubuntu0.2
libkrb5support0 1.19.2-2ubuntu0.2
liblz4-1 1.9.3-2build2
liblzma5 5.2.5-2ubuntu1
libmount1 2.37.2-4ubuntu3
libncurses6 6.3-2ubuntu0.1
libncursesw6 6.3-2ubuntu0.1
libnettle8 3.7.3-1build2
libnsl2 1.3.0-2build2
libp11-kit0 0.24.0-6build1
libpam-modules 1.4.0-11ubuntu2.3
libpam-modules-bin 1.4.0-11ubuntu2.3
libpam-runtime 1.4.0-11ubuntu2.3
libpam0g 1.4.0-11ubuntu2.3
libpcre2-8-0 10.39-3ubuntu0.1
libpcre3 2:8.39-13ubuntu0.22.04.1
libprocps8 2:3.3.17-6ubuntu2
libseccomp2 2.5.3-2ubuntu2
libselinux1 3.3-1build2
libsemanage-common 3.3-1build2
libsemanage2 3.3-1build2
libsepol2 3.3-1build1
libsmartcols1 2.37.2-4ubuntu3
libss2 1.46.5-2ubuntu1.1
libssl3 3.0.2-0ubuntu1.10
libstdc++6 12.1.0-2ubuntu1~22.04
libsystemd0 249.11-0ubuntu3.9
libtasn1-6 4.18.0-4build1
libtinfo6 6.3-2ubuntu0.1
libtirpc-common 1.3.2-2ubuntu0.1
libtirpc3 1.3.2-2ubuntu0.1
libudev1 249.11-0ubuntu3.9
libunistring2 1.0-1
libuuid1 2.37.2-4ubuntu3
libxxhash0 0.8.1-1
libzstd1 1.4.8+dfsg-3build1
login 1:4.8.1-2ubuntu2.1
logsave 1.46.5-2ubuntu1.1
lsb-base 11.1.0ubuntu4
mawk 1.3.4.20200120-3
mount 2.37.2-4ubuntu3
ncurses-base 6.3-2ubuntu0.1
ncurses-bin 6.3-2ubuntu0.1
passwd 1:4.8.1-2ubuntu2.1
perl-base 5.34.0-3ubuntu1.2
procps 2:3.3.17-6ubuntu2
sed 4.8-1ubuntu2
sensible-utils 0.0.17
sysvinit-utils 3.01-1ubuntu1
tar 1.34+dfsg-1ubuntu0.1.22.04.1
ubuntu-keyring 2021.03.26
usrmerge 25ubuntu2
util-linux 2.37.2-4ubuntu3
zlib1g 1:1.2.11.dfsg-2ubuntu9.2
adduser 3.118ubuntu5
apt 2.4.9
base-files 12ubuntu4.3
base-passwd 3.5.52build1
bash 5.1-6ubuntu1
bsdutils 1:2.37.2-4ubuntu3
ca-certificates 20230311ubuntu0.22.04.1
coreutils 8.32-4.1ubuntu1
curl 7.81.0-1ubuntu1.10
dash 0.5.11+git20210903+057cd650a4ed-3build1
debconf 1.5.79ubuntu1
debianutils 5.5-1ubuntu2
diffutils 1:3.8-0ubuntu2
dpkg 1.21.1ubuntu2.2
e2fsprogs 1.46.5-2ubuntu1.1
findutils 4.8.0-1ubuntu3
gcc-12-base 12.1.0-2ubuntu1~22.04
gpgv 2.2.27-3ubuntu2.1
grep 3.7-1build1
gzip 1.10-4ubuntu4.1
hostname 3.23ubuntu2
init-system-helpers 1.62
libacl1 2.3.1-1
libapt-pkg6.0 2.4.9
libattr1 1:2.5.1-1build1
libaudit-common 1:3.0.7-1build1
libaudit1 1:3.0.7-1build1
libblkid1 2.37.2-4ubuntu3
libbrotli1 1.0.9-2build6
libbz2-1.0 1.0.8-5build1
libc-bin 2.35-0ubuntu3.1
libc6 2.35-0ubuntu3.1
libcap-ng0 0.7.9-2.2build3
libcap2 1:2.44-1ubuntu0.22.04.1
libcom-err2 1.46.5-2ubuntu1.1
libcrypt1 1:4.4.27-1
libcurl4 7.81.0-1ubuntu1.10
libdb5.3 5.3.28+dfsg1-0.8ubuntu3
libdebconfclient0 0.261ubuntu1
libext2fs2 1.46.5-2ubuntu1.1
libffi8 3.4.2-4
libgcc-s1 12.1.0-2ubuntu1~22.04
libgcrypt20 1.9.4-3ubuntu3
libgmp10 2:6.2.1+dfsg-3ubuntu1
libgnutls30 3.7.3-4ubuntu1.2
libgpg-error0 1.43-3
libgssapi-krb5-2 1.19.2-2ubuntu0.2
libhogweed6 3.7.3-1build2
libidn2-0 2.3.2-2build1
libk5crypto3 1.19.2-2ubuntu0.2
libkeyutils1 1.6.1-2ubuntu3
libkrb5-3 1.19.2-2ubuntu0.2
libkrb5support0 1.19.2-2ubuntu0.2
libldap-2.5-0 2.5.14+dfsg-0ubuntu0.22.04.2
libldap-common 2.5.14+dfsg-0ubuntu0.22.04.2
liblz4-1 1.9.3-2build2
liblzma5 5.2.5-2ubuntu1
libmount1 2.37.2-4ubuntu3
libncurses6 6.3-2ubuntu0.1
libncursesw6 6.3-2ubuntu0.1
libnettle8 3.7.3-1build2
libnghttp2-14 1.43.0-1build3
libnsl2 1.3.0-2build2
libp11-kit0 0.24.0-6build1
libpam-modules 1.4.0-11ubuntu2.3
libpam-modules-bin 1.4.0-11ubuntu2.3
libpam-runtime 1.4.0-11ubuntu2.3
libpam0g 1.4.0-11ubuntu2.3
libpcre2-8-0 10.39-3ubuntu0.1
libpcre3 2:8.39-13ubuntu0.22.04.1
libprocps8 2:3.3.17-6ubuntu2
libpsl5 0.21.0-1.2build2
librtmp1 2.4+20151223.gitfa8646d.1-2build4
libsasl2-2 2.1.27+dfsg2-3ubuntu1.2
libsasl2-modules 2.1.27+dfsg2-3ubuntu1.2
libsasl2-modules-db 2.1.27+dfsg2-3ubuntu1.2
libseccomp2 2.5.3-2ubuntu2
libselinux1 3.3-1build2
libsemanage-common 3.3-1build2
libsemanage2 3.3-1build2
libsepol2 3.3-1build1
libsmartcols1 2.37.2-4ubuntu3
libss2 1.46.5-2ubuntu1.1
libssh-4 0.9.6-2ubuntu0.22.04.1
libssl3 3.0.2-0ubuntu1.10
libstdc++6 12.1.0-2ubuntu1~22.04
libsystemd0 249.11-0ubuntu3.9
libtasn1-6 4.18.0-4build1
libtinfo6 6.3-2ubuntu0.1
libtirpc-common 1.3.2-2ubuntu0.1
libtirpc3 1.3.2-2ubuntu0.1
libudev1 249.11-0ubuntu3.9
libunistring2 1.0-1
libuuid1 2.37.2-4ubuntu3
libxxhash0 0.8.1-1
libzstd1 1.4.8+dfsg-3build1
login 1:4.8.1-2ubuntu2.1
logsave 1.46.5-2ubuntu1.1
lsb-base 11.1.0ubuntu4
mawk 1.3.4.20200120-3
mount 2.37.2-4ubuntu3
ncurses-base 6.3-2ubuntu0.1
ncurses-bin 6.3-2ubuntu0.1
openssl 3.0.2-0ubuntu1.10
passwd 1:4.8.1-2ubuntu2.1
perl-base 5.34.0-3ubuntu1.2
procps 2:3.3.17-6ubuntu2
publicsuffix 20211207.1025-1
sed 4.8-1ubuntu2
sensible-utils 0.0.17
sysvinit-utils 3.01-1ubuntu1
tar 1.34+dfsg-1ubuntu0.1.22.04.1
ubuntu-keyring 2021.03.26
usrmerge 25ubuntu2
util-linux 2.37.2-4ubuntu3
zlib1g 1:1.2.11.dfsg-2ubuntu9.2`
	info := strings.Split(baseInfoString, "\n")
	var productTest []productWithVersion
	for _, s := range info {
		temp := strings.Split(s, " ")
		productTest = append(productTest, productWithVersion{
			name:    temp[0],
			version: temp[1],
		})
	}
	for _, item := range productTest {
		cveRes, num := cvequeryops.Query("C:/Users/27970/yakit-projects/default-cve.db", cvequeryops.ProductWithVersion(item.name, item.version))
		fmt.Printf("product: [%s]:[%s] find cve %d\n", item.name, item.version, num)
		if num > 0 {
			for _, cve := range cveRes {
				fmt.Println(cve.CVE.CVE)
			}
		}
	}
}

type productWithVersion struct {
	name    string
	version string
}

func TestClean(t *testing.T) {
	cpeObject := cveresources.Configurations{}
	jsonSTR := `{"CVE_data_version":"4.0","nodes":[{"operator":"OR","cpe_match":[{"vulnerable":true,"cpe23Uri":"cpe:2.3:o:redhat:linux:*:*:*:*:*:*:*:*","versionStartExcluding":"","versionEndExcluding":"","versionStartIncluding":"","versionEndIncluding":""},{"vulnerable":true,"cpe23Uri":"cpe:2.3:o:freebsd:freebsd:6.2:stable:*:*:*:*:*:*","versionStartExcluding":"","versionEndExcluding":"","versionStartIncluding":"","versionEndIncluding":""}],"children":[]}]}`
	err := json.Unmarshal([]byte(jsonSTR), &cpeObject)
	if err != nil {
		return
	}

}
