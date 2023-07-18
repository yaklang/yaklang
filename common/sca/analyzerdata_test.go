package sca

import (
	"testing"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

var APKWantPkgs = []*dxtypes.Package{

	{
		Name:         "alpine-baselayout",
		Version:      "3.4.3-r1",
		Verification: "sha1:cf0bca32762cd5be9974f4c127467b0f93f78f20",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "alpine-baselayout-data",
		Version:      "3.4.3-r1",
		Verification: "sha1:602007ee374ed96f35e9bf39b1487d67c6afe027",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "alpine-keys",
		Version:      "2.4-r1",
		Verification: "sha1:ec3a3d5ef4c7a168d09516097bb3219ca77c1534",
		License:      []string{"MIT"},
	},

	{
		Name:         "apk-tools",
		Version:      "2.14.0-r2",
		Verification: "sha1:8cde25f239ebf691cd135a3954e5193c1ac2ae13",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "brotli-libs",
		Version:      "1.0.9-r14",
		Verification: "sha1:48b2006d35cdde849a18f7cadbfaf17c9273130f",
		License:      []string{"MIT"},
	},

	{
		Name:         "busybox",
		Version:      "1.36.1-r0",
		Verification: "sha1:53bff6ed72a869ce4555a2e0af6750eebea002fc",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "busybox-binsh",
		Version:      "1.36.1-r0",
		Verification: "sha1:1819eefdc85da3f9baa0277b10d24062d53f0f84",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "ca-certificates",
		Version:      "20230506-r0",
		Verification: "sha1:146f0cfbbc3e7648d5f55cb49861565b6b78f83a",
		License:      []string{"MPL-2.0", "MIT"},
	},

	{
		Name:         "ca-certificates-bundle",
		Version:      "20230506-r0",
		Verification: "sha1:47f485d08670a9eb21ebf10e70ae65dc43ab6c3d",
		License:      []string{"MPL-2.0", "MIT"},
	},

	{
		Name:         "curl",
		Version:      "8.1.2-r0",
		Verification: "sha1:8bed52a52a72a65aa7f73b4576ce913fb788bfc5",
		License:      []string{"curl"},
	},

	{
		Name:         "libc-utils",
		Version:      "0.7.2-r5",
		Verification: "sha1:2e59dafeb8bca0786540846c686f121ae8348a42",
		License:      []string{"BSD-2-Clause", "BSD-3-Clause"},
	},

	{
		Name:         "libcrypto3",
		Version:      "3.1.1-r1",
		Verification: "sha1:c81bb336f4e54404b0469c02c2e81a23b24652be",
		License:      []string{"Apache-2.0"},
	},

	{
		Name:         "libcurl",
		Version:      "8.1.2-r0",
		Verification: "sha1:d91300aff047a12cc19b4ab4f2c738970da71855",
		License:      []string{"curl"},
	},

	{
		Name:         "libidn2",
		Version:      "2.3.4-r1",
		Verification: "sha1:7bc3cd824a388677844c8e6e75ccf5344cf42f6f",
		License:      []string{"GPL-2.0", "LGPL-3.0-or-later"},
	},

	{
		Name:         "libssl3",
		Version:      "3.1.1-r1",
		Verification: "sha1:f867d5ec872470b96cf651da758a53e6a1187c2a",
		License:      []string{"Apache-2.0"},
	},

	{
		Name:         "libunistring",
		Version:      "1.1-r1",
		Verification: "sha1:14ce8b4b122fdd33acb11cc7f106aa0561c219a1",
		License:      []string{"GPL-2.0", "LGPL-3.0-or-later"},
	},

	{
		Name:         "musl",
		Version:      "1.2.4-r0",
		Verification: "sha1:e779b680e85539adb9dc4c6c48e6f7dd75e1df6b",
		License:      []string{"MIT"},
	},

	{
		Name:         "musl-utils",
		Version:      "1.2.4-r0",
		Verification: "sha1:e96f86ad77fb1d0c3e93b26e63b6402235ab8189",
		License:      []string{"MIT", "BSD-2-Clause", "GPL-2.0"},
	},

	{
		Name:         "nghttp2-libs",
		Version:      "1.53.0-r0",
		Verification: "sha1:577c7f2ee19642ee1c2a2755a10a818fcdf51979",
		License:      []string{"MIT"},
	},

	{
		Name:         "scanelf",
		Version:      "1.3.7-r1",
		Verification: "sha1:e27abda38faea3635a2db4d50d007751ea280b43",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "ssl_client",
		Version:      "1.36.1-r0",
		Verification: "sha1:8722023d7e6cde7b861a7c076481000d05f0272e",
		License:      []string{"GPL-2.0"},
	},

	{
		Name:         "zlib",
		Version:      "1.2.13-r1",
		Verification: "sha1:2656e848992b378aa40dca24af8cde9e97161174",
		License:      []string{"Zlib"},
	},
}

var APKNegativePkgs = []*dxtypes.Package{
	{
		Name:         "ssl_client",
		Version:      "1.36.1-r0",
		Verification: "sha1:8722023d7e6cde7b861a7c076481000d05f0272e",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "zlib",
		Version:      "1.2.13-r1",
		Verification: "sha1:2656e848992b378aa40dca24af8cde9e97161174",
		License:      []string{"Zlib"},

		Potential: false,
	},
	{
		Name:         "so:libc.musl-x86_64.so.1",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "so:libcrypto.so.3",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "so:libssl.so.3",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
}

var DPKGWantPkgs = []*dxtypes.Package{
	{
		Name:         "adduser",
		Version:      "3.118ubuntu5",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "apt",
		Version:      "2.4.9",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "base-files",
		Version:      "12ubuntu4.3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "base-passwd",
		Version:      "3.5.52build1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "bash",
		Version:      "5.1-6ubuntu1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "bsdutils",
		Version:      "1:2.37.2-4ubuntu3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "ca-certificates",
		Version:      "20230311ubuntu0.22.04.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "coreutils",
		Version:      "8.32-4.1ubuntu1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "curl",
		Version:      "7.81.0-1ubuntu1.10",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "dash",
		Version:      "0.5.11+git20210903+057cd650a4ed-3build1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "passwd",
		Version:      "*",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "debconf|debconf-2.0",
		Version:      ">= 0.5|*",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "ubuntu-keyring",
		Version:      "*",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libc6",
		Version:      ">= 2.34",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libseccomp2",
		Version:      ">= 2.4.2",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libapt-pkg6.0",
		Version:      ">= 2.4.9",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libgcc-s1",
		Version:      ">= 3.3.1",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libgnutls30",
		Version:      ">= 3.7.0",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libstdc++6",
		Version:      ">= 11",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libsystemd0",
		Version:      "*",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "gpgv|gpgv1|gpgv2",
		Version:      "*|*|*",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libcrypt1",
		Version:      ">= 1:4.4.10-10ubuntu3",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libdebconfclient0",
		Version:      ">= 0.145",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "debianutils",
		Version:      ">= 2.15",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "openssl",
		Version:      ">= 1.1.1",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "libcurl4",
		Version:      "= 7.81.0-1ubuntu1.10",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "zlib1g",
		Version:      ">= 1:1.1.4",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
	{
		Name:         "dpkg",
		Version:      ">= 1.19.1",
		Verification: "",
		License:      nil,
		Potential:    true,
	},
}

var RPMWantPkgs = []*dxtypes.Package{
	{
		Name:         "mariner-release",
		Version:      "2.0",
		Verification: "md5:f7bd337ae2962162ac73a509ed7129f0",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "filesystem",
		Version:      "1.1",
		Verification: "md5:4aa036ed5ef7ddb03687f7f05eaf7c4e",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "glibc",
		Version:      "2.34",
		Verification: "md5:85c3258b72bf1a42444835ee87c54115",
		License:      []string{"BSD AND GPLv2+ AND Inner-Net AND ISC AND LGPLv2+ AND MIT"},

		Potential: false,
	},
	{
		Name:         "zlib",
		Version:      "1.2.11",
		Verification: "md5:26b5ae370d09fa0f57d2f4ba1452c5c3",
		License:      []string{"Zlib"},

		Potential: false,
	},
	{
		Name:         "openssl-libs",
		Version:      "1.1.1k",
		Verification: "md5:fdf144018e3048996693d3ed05b432b4",
		License:      []string{"OpenSSL"},

		Potential: false,
	},
	{
		Name:         "xz-libs",
		Version:      "5.2.5",
		Verification: "md5:96c01c54f8e16b2ff2a69b4108fa6043",
		License:      []string{"GPLv2+ and GPLv3+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "bzip2-libs",
		Version:      "1.0.8",
		Verification: "md5:dd11ab0fe74f3ac60d333e507c3042a3",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "zstd-libs",
		Version:      "1.5.0",
		Verification: "md5:f72d876024cef64bb9b5666be2ba58ce",
		License:      []string{"BSD or GPLv2"},

		Potential: false,
	},
	{
		Name:         "sqlite-libs",
		Version:      "3.34.1",
		Verification: "md5:db6df0437714d8cc12669fe5358b70eb",
		License:      []string{"Unlicense"},

		Potential: false,
	},
	{
		Name:         "elfutils-libelf",
		Version:      "0.185",
		Verification: "md5:b06d95eb513297033357ecc8471d6c49",
		License:      []string{"GPLv2+ OR LGPLv3+"},

		Potential: false,
	},
	{
		Name:         "popt",
		Version:      "1.16",
		Verification: "md5:4164c1fcf045b40d887bd2badc80d267",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libgcc",
		Version:      "11.2.0",
		Verification: "md5:17272f16ad5c3b7430d0d83d3114aa1f",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "openssl",
		Version:      "1.1.1k",
		Verification: "md5:7ca9fbff26805dd55e79cbb7cf598934",
		License:      []string{"OpenSSL"},

		Potential: false,
	},
	{
		Name:         "libcap",
		Version:      "2.26",
		Verification: "md5:bb7a06aae691f09c5717a92f7a86ffac",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "lua-libs",
		Version:      "5.3.5",
		Verification: "md5:d3d40054b6f1b7716001e454995fe8a8",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "pcre-libs",
		Version:      "8.44",
		Verification: "md5:65b24462e903d64b58e487c04b6be6b3",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "expat-libs",
		Version:      "2.4.3",
		Verification: "md5:82650673cdaeaf275084868238e93eb5",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libstdc++",
		Version:      "11.2.0",
		Verification: "md5:42f2cde1419ab48a0528a873affd5898",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "ncurses-libs",
		Version:      "6.2",
		Verification: "md5:472fac2ce5d3d3f7338bb2a3c237fa0e",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "readline",
		Version:      "8.1",
		Verification: "md5:f3d3a3bb05347f5bea78703a7ea9a99d",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "libffi",
		Version:      "3.4.2",
		Verification: "md5:70cbd94d7ff8936a2b45054763ec3c79",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "gmp",
		Version:      "6.2.1",
		Verification: "md5:d3eef70298546ecc500b5dd243a4a67e",
		License:      []string{"GPLv2+ AND GPLv3+ AND LGPLv3+"},

		Potential: false,
	},
	{
		Name:         "e2fsprogs-libs",
		Version:      "1.46.4",
		Verification: "md5:2ba5463d5fcc5bc3789bfc45a5502807",
		License:      []string{"GPLv2 AND LGPLv2 AND BSD AND MIT"},

		Potential: false,
	},
	{
		Name:         "p11-kit",
		Version:      "0.23.22",
		Verification: "md5:9c9ee2bb0c24dc159515bcc44fbb692d",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "pcre",
		Version:      "8.44",
		Verification: "md5:0fa8f6f26296829efcb486d5da014da0",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "libselinux",
		Version:      "3.2",
		Verification: "md5:9573fd90eb7ff806c51490984b64a0be",
		License:      []string{"Unlicense"},

		Potential: false,
	},
	{
		Name:         "coreutils",
		Version:      "8.32",
		Verification: "md5:9e431e0adc7a379e18415da52abff435",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "grep",
		Version:      "3.7",
		Verification: "md5:1861a4f6be5ecf733bcd0c7c7b60ca1e",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "bash",
		Version:      "5.1.8",
		Verification: "md5:8f5f6e54a8c0e4c0c9accf0ec7388905",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "libsepol",
		Version:      "3.2",
		Verification: "md5:d055d594d2ff83dc8e825ad9c64f84e1",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "libgpg-error",
		Version:      "1.43",
		Verification: "md5:ce492fb031015af7e8248f40cbcc5d1b",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "xz",
		Version:      "5.2.5",
		Verification: "md5:1c03d8ce0f048ddcdcbcf9adb2116e36",
		License:      []string{"GPLv2+ and GPLv3+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "libassuan",
		Version:      "2.5.5",
		Verification: "md5:20f2b6a76e1fb95d9b6efd80be02a635",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "krb5",
		Version:      "1.18",
		Verification: "md5:68e2400fe4186bf12a38c9271abbf6ff",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libgcrypt",
		Version:      "1.9.4",
		Verification: "md5:09707ad0dd6f584c0d651012aa4c45d9",
		License:      []string{"GPLv2+ and LGPLv2+ and BSD and MIT and Public Domain"},

		Potential: false,
	},
	{
		Name:         "cracklib",
		Version:      "2.9.7",
		Verification: "md5:7a9eafd2a3aff07d806ddd9a62988e7b",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "pam",
		Version:      "1.5.1",
		Verification: "md5:363f30b972a09d8b18f6bbee4d9b183d",
		License:      []string{"BSD and GPLv2+"},

		Potential: false,
	},
	{
		Name:         "nspr",
		Version:      "4.21",
		Verification: "md5:785bff4e7e077e9174d4036a393b965a",
		License:      []string{"MPLv2.0"},

		Potential: false,
	},
	{
		Name:         "mariner-rpm-macros",
		Version:      "2.0",
		Verification: "md5:0e2447ed26ae71a2a78e403493583503",
		License:      []string{"GPL+ AND MIT"},

		Potential: false,
	},
	{
		Name:         "rpm-libs",
		Version:      "4.17.0",
		Verification: "md5:2232802d536e3607f1df0ad8ef91a248",
		License:      []string{"GPLv2+ AND LGPLv2+ AND BSD"},

		Potential: false,
	},
	{
		Name:         "gzip",
		Version:      "1.11",
		Verification: "md5:42285a4d25ffa824e97c5026f7230eae",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "bzip2",
		Version:      "1.0.8",
		Verification: "md5:a74384d3884f27146018caf6e48613d0",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "slang",
		Version:      "2.3.2",
		Verification: "md5:cb4149334ca91cc785ca32790b3eb58e",
		License:      []string{"GNU General Public License"},

		Potential: false,
	},
	{
		Name:         "ncurses",
		Version:      "6.2",
		Verification: "md5:1d28c132fe503b2aaa2d8e9d0c440eab",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "expat",
		Version:      "2.4.3",
		Verification: "md5:a077a89ff0e9d8a2401c10ab91197407",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libssh2",
		Version:      "1.9.0",
		Verification: "md5:69e0a449811fdb86958beb22c50e058c",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "curl-libs",
		Version:      "7.76.0",
		Verification: "md5:020c15c1f41a135778c61b0c0f652fc0",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "curl",
		Version:      "7.76.0",
		Verification: "md5:b5f5369ae91df3672fa3338669ec5ca2",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "file-libs",
		Version:      "5.40",
		Verification: "md5:3e3714a2381c074c7752e294995b9241",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "libcap-ng",
		Version:      "0.7.9",
		Verification: "md5:2c3b12932d563261540f17a701cc8303",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "tar",
		Version:      "1.34",
		Verification: "md5:0aa396e03f4dca36f316a762c23dbf71",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "lz4",
		Version:      "1.9.2",
		Verification: "md5:6696a59d11719cb33c7f4e998a8c6974",
		License:      []string{"BSD 2-Clause and GPLv2"},

		Potential: false,
	},
	{
		Name:         "newt",
		Version:      "0.52.21",
		Verification: "md5:034fc478b65afee54d478255743315af",
		License:      []string{"LGPLv2"},

		Potential: false,
	},
	{
		Name:         "chkconfig",
		Version:      "1.20",
		Verification: "md5:d87f0f95e810abf39ea84d88ed750da6",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "libsolv",
		Version:      "0.7.19",
		Verification: "md5:e601712773ae56e8c0fbfb7648855d86",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "nss-libs",
		Version:      "3.44",
		Verification: "md5:c656916f80fc819ab13e44a0e6b7d0e8",
		License:      []string{"MPLv2.0"},

		Potential: false,
	},
	{
		Name:         "pinentry",
		Version:      "1.2.0",
		Verification: "md5:0560d426f3d258841576f873f8ffe976",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "kmod",
		Version:      "29",
		Verification: "md5:c4e39e796ce8d65a757acc9b22a75ca7",
		License:      []string{"LGPLv2.1+ AND GPLv2+"},

		Potential: false,
	},
	{
		Name:         "libksba",
		Version:      "1.6.0",
		Verification: "md5:5a6214af2bd6030c1de66ff1248b1036",
		License:      []string{"(LGPLv3+ or GPLv2+) and GPLv3+"},

		Potential: false,
	},
	{
		Name:         "zstd",
		Version:      "1.5.0",
		Verification: "md5:0102fd8bae48e29aea0b7cebd3ac7443",
		License:      []string{"BSD or GPLv2"},

		Potential: false,
	},
	{
		Name:         "unzip",
		Version:      "6.0",
		Verification: "md5:480c4b180e699cc7a0b0cef115c7717e",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "npth",
		Version:      "1.6",
		Verification: "md5:07974f161d13636f9ec947ec9fe30a1a",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "gnupg2",
		Version:      "2.3.3",
		Verification: "md5:d0b419b83ad6616c3b9ca87b33d2f893",
		License:      []string{"BSD and CC0 and GPLv2+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "gpgme",
		Version:      "1.16.0",
		Verification: "md5:9d213bfec2b404ed013d4841710b9ffb",
		License:      []string{"GPLv3+ and LGPLv2+ and MIT"},

		Potential: false,
	},
	{
		Name:         "vim",
		Version:      "8.2.4081",
		Verification: "md5:37b0ca888da3be40c867c550bb1cefe9",
		License:      []string{"Vim"},

		Potential: false,
	},
	{
		Name:         "libtool",
		Version:      "2.4.6",
		Verification: "md5:999ad391ed5ab6d2c449625bd63195e9",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "findutils",
		Version:      "4.8.0",
		Verification: "md5:6e78e7560afb386fadfbaae750a31ab3",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "mpfr",
		Version:      "4.1.0",
		Verification: "md5:3a63ab9f5ce8bb364d5235b2896fcb78",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "gawk",
		Version:      "5.1.0",
		Verification: "md5:702313964991c3a92bcacdc110dc82d3",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "gdbm",
		Version:      "1.21",
		Verification: "md5:6a30cff90bab80d49fd1342a4a900b8b",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "lua",
		Version:      "5.3.5",
		Verification: "md5:9f2f481b5228f128189b36e70e92d6fe",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libarchive",
		Version:      "3.4.2",
		Verification: "md5:325327330e4b80e28bb8383dae440d1b",
		License:      []string{"BSD AND Public Domain AND (ASL 2.0 OR CC0 1.0 OR OpenSSL)"},

		Potential: false,
	},
	{
		Name:         "openldap",
		Version:      "2.4.57",
		Verification: "md5:18c2f546e10e23ee86061a5ead0c8d58",
		License:      []string{"OpenLDAP"},

		Potential: false,
	},
	{
		Name:         "elfutils-libelf-devel",
		Version:      "0.185",
		Verification: "md5:58d0c2d532ef1d9bd348ecb699c0e706",
		License:      []string{"GPLv2+ OR LGPLv3+"},

		Potential: false,
	},
	{
		Name:         "libgomp",
		Version:      "11.2.0",
		Verification: "md5:4600af8787654a186c3876a20706ae32",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "cpio",
		Version:      "2.13",
		Verification: "md5:b9ad8230a6de91cada8446ef5264e26d",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "libtasn1",
		Version:      "4.14",
		Verification: "md5:0bfb1e05619099f58072129a03480275",
		License:      []string{"GPLv3+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "p11-kit-trust",
		Version:      "0.23.22",
		Verification: "md5:7d97b7919bff33e0f685b7e295616449",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "ca-certificates-tools",
		Version:      "2.0.0",
		Verification: "md5:5640b94408426231cedab443a74fa7b5",
		License:      []string{"MPLv2.0"},

		Potential: false,
	},
	{
		Name:         "json-c",
		Version:      "0.14",
		Verification: "md5:3845970bdc4d5de92897f0236f8f85a6",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "libpkgconf",
		Version:      "1.8.0",
		Verification: "md5:5cb6ab549d102d57ba55853fc14a23ee",
		License:      []string{"ISC"},

		Potential: false,
	},
	{
		Name:         "pkgconf",
		Version:      "1.8.0",
		Verification: "md5:e8aac528c94293827225e2b4248578f7",
		License:      []string{"ISC"},

		Potential: false,
	},
	{
		Name:         "tdnf-cli-libs",
		Version:      "2.1.0",
		Verification: "md5:56105162da491ebdfa47878db4032f23",
		License:      []string{"LGPLv2.1 AND GPLv2"},

		Potential: false,
	},
	{
		Name:         "tdnf",
		Version:      "2.1.0",
		Verification: "md5:1b48f3dac9841e00ec1dfbdc4cf3b938",
		License:      []string{"LGPLv2.1 AND GPLv2"},

		Potential: false,
	},
	{
		Name:         "tdnf-plugin-repogpgcheck",
		Version:      "2.1.0",
		Verification: "md5:58d3d6d63e6ae1935d1b0668408fcd1f",
		License:      []string{"LGPLv2.1 AND GPLv2"},

		Potential: false,
	},
	{
		Name:         "sed",
		Version:      "4.8",
		Verification: "md5:d4d6aa23e89e601fba25bb372652e7d8",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "net-tools",
		Version:      "1.60",
		Verification: "md5:a7b9895648a38e43df3535fce4c6ab2e",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "ca-certificates-shared",
		Version:      "2.0.0",
		Verification: "md5:6fbb05bac4fafbe9da167ac69465b711",
		License:      []string{"MPLv2.0"},

		Potential: false,
	},
	{
		Name:         "ca-certificates-base",
		Version:      "2.0.0",
		Verification: "md5:584e262af950f73e75f315c220f08d90",
		License:      []string{"MPLv2.0"},

		Potential: false,
	},
	{
		Name:         "pkgconf-m4",
		Version:      "1.8.0",
		Verification: "md5:e6f5b92bd1dcca980711ca327f928d49",
		License:      []string{"GPLv2+ WITH exceptions"},

		Potential: false,
	},
	{
		Name:         "pkgconf-pkg-config",
		Version:      "1.8.0",
		Verification: "md5:8cd1dc436091fd049a5b7b5e31a7fa21",
		License:      []string{"ISC"},

		Potential: false,
	},
	{
		Name:         "zstd-devel",
		Version:      "1.5.0",
		Verification: "md5:ae279036d6bebdd879ed8a942a091d89",
		License:      []string{"BSD or GPLv2"},

		Potential: false,
	},
	{
		Name:         "popt-devel",
		Version:      "1.16",
		Verification: "md5:2a45fd20e0f815c7f7aebfb898c9ba21",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "zlib-devel",
		Version:      "1.2.11",
		Verification: "md5:260594dcd232af5bb07cf5854d187393",
		License:      []string{"Zlib"},

		Potential: false,
	},
	{
		Name:         "xz-devel",
		Version:      "5.2.5",
		Verification: "md5:5be2749263a5f2d53230cf4c663020e6",
		License:      []string{"GPLv2+ and GPLv3+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "pcre-devel",
		Version:      "8.44",
		Verification: "md5:d05756d20fefc5399bb837bc0fcd1d02",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "libsepol-devel",
		Version:      "3.2",
		Verification: "md5:c8e3ed57403f50c640250a233896a76f",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "libselinux-devel",
		Version:      "3.2",
		Verification: "md5:38abe447e4de74c517f9195a9a4bda38",
		License:      []string{"Unlicense"},

		Potential: false,
	},
	{
		Name:         "util-linux",
		Version:      "2.37.2",
		Verification: "md5:a2a93d998f4427bd3d886403ffcf9872",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "util-linux-devel",
		Version:      "2.37.2",
		Verification: "md5:bdd34b2e7bee180786ad8828a29fc57d",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "python3",
		Version:      "3.9.9",
		Verification: "md5:1d82ece8bd5f763a83565af3e7756c3a",
		License:      []string{"PSF"},

		Potential: false,
	},
	{
		Name:         "python3-libs",
		Version:      "3.9.9",
		Verification: "md5:7223c8c1c5da4221e20f93dcc6b7ebe4",
		License:      []string{"PSF"},

		Potential: false,
	},
	{
		Name:         "glib",
		Version:      "2.60.1",
		Verification: "md5:b00a1ff8181a44b956a63a8d4bce95f3",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "wget",
		Version:      "1.20.3",
		Verification: "md5:772c6e8acc19ea2b060fc1db93baa097",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "e2fsprogs",
		Version:      "1.46.4",
		Verification: "md5:c0338b46b24028e4e13c3677f14194a1",
		License:      []string{"GPLv2 AND LGPLv2 AND BSD AND MIT"},

		Potential: false,
	},
	{
		Name:         "systemd-rpm-macros",
		Version:      "249.7",
		Verification: "md5:dc79fea691329218b1bb77da943e097c",
		License:      []string{"LGPLv2+ AND GPLv2+ AND MIT"},

		Potential: false,
	},
	{
		Name:         "elfutils-default-yama-scope",
		Version:      "0.185",
		Verification: "md5:7dc1a9e96daec9ef1b6e4a362fa77e1b",
		License:      []string{"GPLv2+ OR LGPLv3+"},

		Potential: false,
	},
	{
		Name:         "cryptsetup-libs",
		Version:      "2.3.3",
		Verification: "md5:0b173211e0199c9a0ab03fc7821a2d57",
		License:      []string{"GPLv2+ and LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "device-mapper-libs",
		Version:      "2.03.05",
		Verification: "md5:326f93d65793e99cc2f2f1035a12131c",
		License:      []string{"LGPLv2"},

		Potential: false,
	},
	{
		Name:         "systemd",
		Version:      "249.7",
		Verification: "md5:dd7d6a1ba5f1582680c4193441c5f690",
		License:      []string{"LGPLv2+ AND GPLv2+ AND MIT"},

		Potential: false,
	},
	{
		Name:         "elfutils",
		Version:      "0.185",
		Verification: "md5:6af46383940433a14545e2a9d4d082dd",
		License:      []string{"GPLv3+ AND (GPLv2+ OR LGPLv3+)"},

		Potential: false,
	},
	{
		Name:         "elfutils-devel",
		Version:      "0.185",
		Verification: "md5:183221f84ea0c3d4a4f68bb4a5e1bc64",
		License:      []string{"GPLv2+ OR LGPLv3+"},

		Potential: false,
	},
	{
		Name:         "rpm-build-libs",
		Version:      "4.17.0",
		Verification: "md5:eb4e11096a870a36628fbd15bdba8724",
		License:      []string{"GPLv2+ AND LGPLv2+ AND BSD"},

		Potential: false,
	},
	{
		Name:         "debugedit",
		Version:      "5.0",
		Verification: "md5:679555eefa8ee14986b2f9167b83b172",
		License:      []string{"GPL-3.0"},

		Potential: false,
	},
	{
		Name:         "rpm",
		Version:      "4.17.0",
		Verification: "md5:00b688156bfa64c8bcb4ef2e550906cc",
		License:      []string{"GPLv2+ AND LGPLv2+ AND BSD"},

		Potential: false,
	},
	{
		Name:         "rpm-devel",
		Version:      "4.17.0",
		Verification: "md5:238ece917c7523d00eb7747583bae13f",
		License:      []string{"GPLv2+ AND LGPLv2+ AND BSD"},

		Potential: false,
	},
	{
		Name:         "rpm-build",
		Version:      "4.17.0",
		Verification: "md5:ce59e4fa5cdbf57ac15f1dbb0c485d60",
		License:      []string{"GPLv2+ AND LGPLv2+ AND BSD"},

		Potential: false,
	},
	{
		Name:         "mariner-repos-shared",
		Version:      "2.0",
		Verification: "md5:a769b5c3d79b8d4c6d8a09fa8ddbbe49",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "mariner-repos-preview",
		Version:      "2.0",
		Verification: "md5:03dcc8cdf868a4b53f6da5141d1cbb60",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "mariner-repos-microsoft-preview",
		Version:      "2.0",
		Verification: "md5:619d34504c62c7b408b9a0f65978f24f",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "mariner-repos-extras-preview",
		Version:      "2.0",
		Verification: "md5:e8a7bb262fc5c730aed7a0d6d97dd730",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "mariner-repos-extended-preview",
		Version:      "2.0",
		Verification: "md5:4de7ee8f7d0f77eb6a55156ad221c7bc",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "audit-libs",
		Version:      "3.0.6",
		Verification: "md5:1fdb5bd82b0e63a2b82538031463cf03",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "audit",
		Version:      "3.0.6",
		Verification: "md5:0303b906ae3240f6bb451a89e82b2039",
		License:      []string{"GPL-2.0"},

		Potential: false,
	},
	{
		Name:         "libsemanage",
		Version:      "3.2",
		Verification: "md5:e2183e7c4b8955e487e7810865f8f705",
		License:      []string{"LGPLv2+"},

		Potential: false,
	},
	{
		Name:         "shadow-utils",
		Version:      "4.9",
		Verification: "md5:e7b3cbf6478e186ef0269c465b5daedf",
		License:      []string{"BSD-3-Clause"},

		Potential: false,
	},
	{
		Name:         "sudo",
		Version:      "1.9.5p2",
		Verification: "md5:be171e00e868015c67accbf8d0ec57eb",
		License:      []string{"ISC"},

		Potential: false,
	},
	{
		Name:         "core-packages-container",
		Version:      "2.0",
		Verification: "md5:b47617ace46b2e6258859718490d1e3b",
		License:      []string{"ASL 2.0"},

		Potential: false,
	},
	{
		Name:         "sqlite",
		Version:      "3.34.1",
		Verification: "md5:48da82892b7b8e46a1c64445d9df1389",
		License:      []string{"Unlicense"},

		Potential: false,
	},
	{
		Name:         "mv",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "cp",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "ln",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "rm",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "env",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
	{
		Name:         "python3.9",
		Version:      "*",
		Verification: "",
		License:      nil,

		Potential: true,
	},
}

var ConanWantPkgs = []*dxtypes.Package{
	{
		Name:    "openssl",
		Version: "3.0.5",
	},
	{
		Name:    "zlib",
		Version: "1.2.12",
	},
}
var GOBianryWantPkgs = []*dxtypes.Package{
	{
		Name:    "github.com/aquasecurity/go-pep440-version",
		Version: "v0.0.0-20210121094942-22b2f8951d46",
	},
	{
		Name:    "github.com/aquasecurity/go-version",
		Version: "v0.0.0-20210121072130-637058cfe492",
	},
	{
		Name:    "golang.org/x/xerrors",
		Version: "v0.0.0-20200804184101-5ec99f83aff1",
	},
}

var GoModWantPkgs = []*dxtypes.Package{
	{
		Name:    "github.com/aquasecurity/go-dep-parser",
		Version: "0.0.0-20220406074731-71021a481237",
	},
	{
		Name:    "golang.org/x/xerrors",
		Version: "0.0.0-20200804184101-5ec99f83aff1",
	},
}
var GoModLess117Pkgs = []*dxtypes.Package{
	{
		Name:    "github.com/aquasecurity/go-dep-parser",
		Version: "0.0.0-20230219131432-590b1dfb6edd",
	},
	{
		Name:    "github.com/BurntSushi/toml",
		Version: "0.3.1",
	},
}

var PHPComposerPkgs = []*dxtypes.Package{
	{
		Name:         "pear/log",
		Version:      "1.13.3",
		Verification: "",

		License: []string{"MIT"},
	},
	{
		Name:         "pear/pear_exception",
		Version:      "v1.0.2",
		Verification: "",
		License:      []string{"BSD-2-Clause"},
	},
}

var PHPComposerWrongJsonPkgs = []*dxtypes.Package{
	{
		Name:         "pear/log",
		Version:      "1.13.3",
		Verification: "",

		License: []string{"MIT"},
	},
	{
		Name:         "pear/pear_exception",
		Version:      "v1.0.2",
		Verification: "",

		License: []string{"BSD-2-Clause"},
	},
}
var PHPComposerNoJsonPkgs = []*dxtypes.Package{
	{
		Name:         "pear/log",
		Version:      "1.13.3",
		Verification: "",

		License: []string{"MIT"},
	},
	{
		Name:         "pear/pear_exception",
		Version:      "v1.0.2",
		Verification: "",

		License: []string{"BSD-2-Clause"},
	},
}

var PythonPackagingPkgs = []*dxtypes.Package{
	{
		Name:         "kitchen",
		Version:      "1.2.6",
		Verification: "",

		License: []string{"LGPLv2+"},
	},
}

var PythonPackagingEggPkg = []*dxtypes.Package{
	{
		Name:         "distlib",
		Version:      "0.3.1",
		Verification: "",

		License: []string{"Python license"},
	},
}
var PythonPackagingWheel = []*dxtypes.Package{
	{
		Name:         "distlib",
		Version:      "0.3.1",
		Verification: "",

		License: []string{"Python license"},
	},
}

var PythonPIPPkgs = []*dxtypes.Package{
	{
		Name:    "click",
		Version: "8.0.0",
	},
	{
		Name:    "Flask",
		Version: "2.0.0",
	},
	{
		Name:    "itsdangerous",
		Version: "2.0.0",
	},
}

var PythonPIPEnvPkgs = []*dxtypes.Package{
	{
		Name:    "pytz",
		Version: "2022.7.1",
	},
}

var PythonPoetryPkgs = []*dxtypes.Package{
	{
		Name:    "certifi",
		Version: "2022.12.7",
	},
	{
		Name:    "charset-normalizer",
		Version: "2.1.1",
	},
	{
		Name:    "click",
		Version: "7.1.2",
	},
	{
		Name:    "flask",
		Version: "1.1.4",
	},
	{
		Name:    "idna",
		Version: "3.4",
	},
	{
		Name:    "itsdangerous",
		Version: "1.1.0",
	},
	{
		Name:    "jinja2",
		Version: "2.11.3",
	},
	{
		Name:    "markupsafe",
		Version: "2.1.2",
	},
	{
		Name:    "requests",
		Version: "2.28.1",
	},
	{
		Name:    "urllib3",
		Version: "1.26.14",
	},
	{
		Name:    "werkzeug",
		Version: "1.0.1",
	},
}
var PythonPoetryNoProjectPkgs = []*dxtypes.Package{
	{
		Name:    "click",
		Version: "8.1.3",
	},
	{
		Name:    "colorama",
		Version: "0.4.6",
	},
}

var PythonPoetryWrongProjectPkgs = []*dxtypes.Package{
	{
		Name:    "click",
		Version: "8.1.3",
	},
	{
		Name:    "colorama",
		Version: "0.4.6",
	},
}

var JavaGradlePkgs = []*dxtypes.Package{
	{Name: "com.example:example",
		Version: "0.0.1",
	},
}
var JavaPomPkgs = []*dxtypes.Package{
	{
		Name:         "com.example:example",
		Version:      "1.0.0",
		Verification: "",

		License: []string{"Apache-2.0"},
	},
}
var JavaPomRequirementPkgs = []*dxtypes.Package{
	{
		Name:         "com.example:example",
		Version:      "2.0.0",
		Verification: "",

		License: []string{"Apache-2.0"},
	},
}
var JavaJarWarPkgs = []*dxtypes.Package{
	{
		Name:    "org.glassfish:javax.el",
		Version: "3.0.0",
	},
	{
		Name:    "com.fasterxml.jackson.core:jackson-databind",
		Version: "2.9.10.6",
	},
	{
		Name:    "com.fasterxml.jackson.core:jackson-annotations",
		Version: "2.9.10",
	},
	{
		Name:    "com.fasterxml.jackson.core:jackson-core",
		Version: "2.9.10",
	},
	{
		Name:    "org.slf4j:slf4j-api",
		Version: "1.7.30",
	},
	{
		Name:    "com.cronutils:cron-utils",
		Version: "9.1.2",
	},
	{
		Name:    "org.apache.commons:commons-lang3",
		Version: "3.11",
	},
	{
		Name:    "com.example:web-app",
		Version: "1.0-SNAPSHOT",
	},
}
var JavaJarParPkgs = []*dxtypes.Package{
	{
		Name:    "com.fasterxml.jackson.core:jackson-core",
		Version: "2.9.10",
	},
}
var JavaJarJarPkgs = []*dxtypes.Package{
	{
		Name:    "org.apache:tomcat-embed-websocket",
		Version: "9.0.65",
	},
}

var NodeNpmPkgs = []*dxtypes.Package{
	{
		Name:         "send",
		Version:      "0.16.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "safe-buffer",
		Version:      "5.1.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "parseurl",
		Version:      "~1.3.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "serve-static",
		Version:      "1.13.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "setprototypeof",
		Version:      "1.1.0",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "express",
		Version:      "4.16.4",
		Verification: "",
		License:      []string{"MIT"},

		Potential: false,
	},
	{
		Name:         "content-type",
		Version:      "~1.0.4",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "path-to-regexp",
		Version:      "0.1.7",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "encodeurl",
		Version:      "~1.0.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "content-disposition",
		Version:      "0.5.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "merge-descriptors",
		Version:      "1.0.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "vary",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "depd",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "utils-merge",
		Version:      "1.0.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "debug",
		Version:      "2.6.9",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "type-is",
		Version:      "~1.6.16",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "body-parser",
		Version:      "1.18.3",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "cookie",
		Version:      "0.3.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "array-flatten",
		Version:      "1.1.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "escape-html",
		Version:      "~1.0.3",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "finalhandler",
		Version:      "1.1.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "fresh",
		Version:      "0.5.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "cookie-signature",
		Version:      "1.0.6",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "qs",
		Version:      "6.5.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "statuses",
		Version:      "~1.4.0",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "on-finished",
		Version:      "~2.3.0",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "methods",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "etag",
		Version:      "~1.8.1",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "accepts",
		Version:      "~1.3.5",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "proxy-addr",
		Version:      "~2.0.4",
		Verification: "",
		License:      nil,

		Potential: false,
	},
	{
		Name:         "range-parser",
		Version:      "~1.2.0",
		Verification: "",
		License:      nil,

		Potential: false,
	},
}
var NodeNpmPkgsFolder = []*dxtypes.Package{
	{
		Name:         "array-flatten",
		Version:      "1.1.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "body-parser",
		Version:      "1.18.3",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "send",
		Version:      "0.16.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "iconv-lite",
		Version:      "0.4.23",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "escape-html",
		Version:      "~1.0.3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "etag",
		Version:      "~1.8.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "range-parser",
		Version:      "~1.2.0",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "finalhandler",
		Version:      "1.1.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "cookie",
		Version:      "0.3.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "http-errors",
		Version:      "~1.6.3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "debug",
		Version:      "2.6.9",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "setprototypeof",
		Version:      "1.1.0",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "on-finished",
		Version:      "~2.3.0",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "safe-buffer",
		Version:      "5.1.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "parseurl",
		Version:      "~1.3.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "content-disposition",
		Version:      "0.5.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "proxy-addr",
		Version:      "~2.0.4",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "utils-merge",
		Version:      "1.0.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "cookie-signature",
		Version:      "1.0.6",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "qs",
		Version:      "6.5.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "methods",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "testname",
		Version:      "1.0.0",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "merge-descriptors",
		Version:      "1.0.1",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "depd",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "raw-body",
		Version:      "2.3.3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "ansi-colors",
		Version:      "3.2.3",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "content-type",
		Version:      "~1.0.4",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "encodeurl",
		Version:      "~1.0.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "type-is",
		Version:      "~1.6.16",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "bytes",
		Version:      "3.0.0",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "ms",
		Version:      "2.0.0",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "ms",
		Version:      "2.1.1",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "vary",
		Version:      "~1.1.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "statuses",
		Version:      "~1.4.0",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "fresh",
		Version:      "0.5.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "path-to-regexp",
		Version:      "0.1.7",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "express",
		Version:      "4.16.4",
		Verification: "",
		License:      []string{"MIT"},
		Potential:    false,
	},
	{
		Name:         "accepts",
		Version:      "~1.3.5",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
	{
		Name:         "serve-static",
		Version:      "1.13.2",
		Verification: "",
		License:      nil,
		Potential:    false,
	},
}

var NodePnpmPkgs = []*dxtypes.Package{
	{
		// lodash-4.17.21
		Name:    "lodash",
		Version: "4.17.21",
	},
}

var RubyBundlerPkgs = []*dxtypes.Package{
	{
		Name:    "sawyer",
		Version: "0.9.2",
	},
	{
		Name:    "metasploit-model",
		Version: "5.0.1",
	},
	{
		Name:    "sinatra",
		Version: "3.0.6",
	},
	{
		Name:    "domain_name",
		Version: "0.5.20190701",
	},
	{
		Name:    "ruby-prof",
		Version: "1.4.2",
	},
	{
		Name:    "gyoku",
		Version: "1.4.0",
	},
	{
		Name:    "rubocop",
		Version: "1.52.0",
	},
	{
		Name:    "unix-crypt",
		Version: "1.3.1",
	},
	{
		Name:    "rkelly-remix",
		Version: "0.0.7",
	},
	{
		Name:    "aws-sdk-ssm",
		Version: "1.151.0",
	},
	{
		Name:    "yard",
		Version: "0.9.34",
	},
	{
		Name:    "unicode-display_width",
		Version: "2.4.2",
	},
	{
		Name:    "xdr",
		Version: "3.0.3",
	},
	{
		Name:    "net-smtp",
		Version: "0.3.3",
	},
	{
		Name:    "unf_ext",
		Version: "0.0.8.2",
	},
	{
		Name:    "recog",
		Version: "3.1.1",
	},
	{
		Name:    "ruby2_keywords",
		Version: "0.0.5",
	},
	{
		Name:    "thin",
		Version: "1.8.2",
	},
	{
		Name:    "ffi",
		Version: "1.15.5",
	},
	{
		Name:    "rack-test",
		Version: "2.1.0",
	},
	{
		Name:    "metasploit-credential",
		Version: "6.0.5",
	},
	{
		Name:    "simpleidn",
		Version: "0.2.1",
	},
	{
		Name:    "windows_error",
		Version: "0.1.5",
	},
	{
		Name:    "rex-encoder",
		Version: "0.1.6",
	},
	{
		Name:    "builder",
		Version: "3.2.4",
	},
	{
		Name:    "ed25519",
		Version: "1.3.0",
	},
	{
		Name:    "tilt",
		Version: "2.2.0",
	},
	{
		Name:    "faraday-retry",
		Version: "2.2.0",
	},
	{
		Name:    "swagger-blocks",
		Version: "3.0.0",
	},
	{
		Name:    "method_source",
		Version: "1.0.0",
	},
	{
		Name:    "msgpack",
		Version: "1.6.1",
	},
	{
		Name:    "rex-sslscan",
		Version: "0.1.9",
	},
	{
		Name:    "patch_finder",
		Version: "1.0.2",
	},
	{
		Name:    "packetfu",
		Version: "2.0.0",
	},
	{
		Name:    "metasploit_data_models",
		Version: "6.0.2",
	},
	{
		Name:    "rex-text",
		Version: "0.2.52",
	},
	{
		Name:    "rasn1",
		Version: "0.12.1",
	},
	{
		Name:    "addressable",
		Version: "2.8.4",
	},
	{
		Name:    "rexml",
		Version: "3.2.5",
	},
	{
		Name:    "net-ldap",
		Version: "0.18.0",
	},
	{
		Name:    "metasm",
		Version: "1.0.5",
	},
	{
		Name:    "rainbow",
		Version: "3.1.1",
	},
	{
		Name:    "httpclient",
		Version: "2.8.3",
	},
	{
		Name:    "rack",
		Version: "2.2.7",
	},
	{
		Name:    "octokit",
		Version: "4.25.1",
	},
	{
		Name:    "aws-sdk-kms",
		Version: "1.66.0",
	},
	{
		Name:    "openssl-cmac",
		Version: "2.0.2",
	},
	{
		Name:    "logging",
		Version: "2.3.1",
	},
	{
		Name:    "json",
		Version: "2.6.3",
	},
	{
		Name:    "network_interface",
		Version: "0.0.2",
	},
	{
		Name:    "chunky_png",
		Version: "1.4.0",
	},
	{
		Name:    "rex-zip",
		Version: "0.1.4",
	},
	{
		Name:    "eventmachine",
		Version: "1.2.7",
	},
	{
		Name:    "net-ssh",
		Version: "7.1.0",
	},
	{
		Name:    "rubyzip",
		Version: "2.3.2",
	},
	{
		Name:    "nexpose",
		Version: "7.3.0",
	},
	{
		Name:    "pdf-reader",
		Version: "2.11.0",
	},
	{
		Name:    "win32api",
		Version: "0.1.0",
	},
	{
		Name:    "jsobfu",
		Version: "0.4.2",
	},
	{
		Name:    "rex-powershell",
		Version: "0.1.97",
	},
	{
		Name:    "aws-sdk-s3",
		Version: "1.123.1",
	},
	{
		Name:    "jmespath",
		Version: "1.6.2",
	},
	{
		Name:    "websocket-extensions",
		Version: "0.1.5",
	},
	{
		Name:    "concurrent-ruby",
		Version: "1.2.2",
	},
	{
		Name:    "parser",
		Version: "3.2.2.3",
	},
	{
		Name:    "rex-exploitation",
		Version: "0.1.38",
	},
	{
		Name:    "em-http-request",
		Version: "1.1.7",
	},
	{
		Name:    "rubocop-ast",
		Version: "1.29.0",
	},
	{
		Name:    "ttfunk",
		Version: "1.7.0",
	},
	{
		Name:    "timeout",
		Version: "0.3.2",
	},
	{
		Name:    "rspec-rerun",
		Version: "1.1.0",
	},
	{
		Name:    "rex-random_identifier",
		Version: "0.1.10",
	},
	{
		Name:    "gssapi",
		Version: "1.3.1",
	},
	{
		Name:    "rb-readline",
		Version: "0.5.5",
	},
	{
		Name:    "debug",
		Version: "1.8.0",
	},
	{
		Name:    "racc",
		Version: "1.7.0",
	},
	{
		Name:    "aws-sdk-core",
		Version: "3.174.0",
	},
	{
		Name:    "rex-registry",
		Version: "0.1.4",
	},
	{
		Name:    "winrm",
		Version: "2.3.6",
	},
	{
		Name:    "daemons",
		Version: "1.4.1",
	},
	{
		Name:    "cookiejar",
		Version: "0.3.3",
	},
	{
		Name:    "aws-eventstream",
		Version: "1.2.0",
	},
	{
		Name:    "ruby_smb",
		Version: "3.2.5",
	},
	{
		Name:    "pry-byebug",
		Version: "3.10.1",
	},
	{
		Name:    "strptime",
		Version: "0.2.5",
	},
	{
		Name:    "redcarpet",
		Version: "3.6.0",
	},
	{
		Name:    "rex-socket",
		Version: "0.1.52",
	},
	{
		Name:    "rex-ole",
		Version: "0.1.7",
	},
	{
		Name:    "nokogiri",
		Version: "1.14.5",
	},
	{
		Name:    "openssl-ccm",
		Version: "1.2.3",
	},
	{
		Name:    "rex-core",
		Version: "0.1.31",
	},
	{
		Name:    "rex-bin_tools",
		Version: "0.1.8",
	},
	{
		Name:    "bootsnap",
		Version: "1.16.0",
	},
	{
		Name:    "rspec-rails",
		Version: "6.0.3",
	},
	{
		Name:    "activemodel",
		Version: "7.0.5",
	},
	{
		Name:    "rspec-core",
		Version: "3.12.2",
	},
	{
		Name:    "i18n",
		Version: "1.14.1",
	},
	{
		Name:    "factory_bot_rails",
		Version: "6.2.0",
	},
	{
		Name:    "rails-dom-testing",
		Version: "2.0.3",
	},
	{
		Name:    "ruby-rc4",
		Version: "0.1.5",
	},
	{
		Name:    "pry",
		Version: "0.14.2",
	},
	{
		Name:    "faker",
		Version: "3.2.0",
	},
	{
		Name:    "rspec-support",
		Version: "3.12.0",
	},
	{
		Name:    "http_parser.rb",
		Version: "0.8.0",
	},
	{
		Name:    "rex-java",
		Version: "0.1.6",
	},
	{
		Name:    "multi_json",
		Version: "1.15.0",
	},
	{
		Name:    "aws-sigv4",
		Version: "1.5.2",
	},
	{
		Name:    "xmlrpc",
		Version: "0.3.2",
	},
	{
		Name:    "simplecov-html",
		Version: "0.12.3",
	},
	{
		Name:    "hashery",
		Version: "2.1.2",
	},
	{
		Name:    "rack-protection",
		Version: "3.0.6",
	},
	{
		Name:    "activerecord",
		Version: "7.0.5",
	},
	{
		Name:    "webrick",
		Version: "1.8.1",
	},
	{
		Name:    "rake",
		Version: "13.0.6",
	},
	{
		Name:    "warden",
		Version: "1.2.9",
	},
	{
		Name:    "sshkey",
		Version: "2.0.0",
	},
	{
		Name:    "hrr_rb_ssh",
		Version: "0.4.2",
	},
	{
		Name:    "rspec-mocks",
		Version: "3.12.5",
	},
	{
		Name:    "actionpack",
		Version: "7.0.5",
	},
	{
		Name:    "zeitwerk",
		Version: "2.6.8",
	},
	{
		Name:    "mini_portile2",
		Version: "2.8.2",
	},
	{
		Name:    "crass",
		Version: "1.0.6",
	},
	{
		Name:    "docile",
		Version: "1.4.0",
	},
	{
		Name:    "filesize",
		Version: "0.2.0",
	},
	{
		Name:    "ruby-macho",
		Version: "3.0.0",
	},
	{
		Name:    "rex-struct2",
		Version: "0.1.3",
	},
	{
		Name:    "io-console",
		Version: "0.6.0",
	},
	{
		Name:    "faraday",
		Version: "2.7.6",
	},
	{
		Name:    "net-protocol",
		Version: "0.2.1",
	},
	{
		Name:    "metasploit-concern",
		Version: "5.0.1",
	},
	{
		Name:    "reline",
		Version: "0.3.5",
	},
	{
		Name:    "websocket-driver",
		Version: "0.7.5",
	},
	{
		Name:    "bcrypt",
		Version: "3.1.18",
	},
	{
		Name:    "memory_profiler",
		Version: "1.0.1",
	},
	{
		Name:    "regexp_parser",
		Version: "2.8.0",
	},
	{
		Name:    "mustermann",
		Version: "3.0.0",
	},
	{
		Name:    "tzinfo",
		Version: "2.0.6",
	},
	{
		Name:    "http-cookie",
		Version: "1.0.5",
	},
	{
		Name:    "metasploit-framework",
		Version: "6.3.26",
	},
	{
		Name:    "nio4r",
		Version: "2.5.9",
	},
	{
		Name:    "pcaprub",
		Version: "0.13.1",
	},
	{
		Name:    "minitest",
		Version: "5.18.0",
	},
	{
		Name:    "metasploit-payloads",
		Version: "2.0.148",
	},
	{
		Name:    "coderay",
		Version: "1.1.3",
	},
	{
		Name:    "bcrypt_pbkdf",
		Version: "1.1.0",
	},
	{
		Name:    "em-socksify",
		Version: "0.3.2",
	},
	{
		Name:    "railties",
		Version: "7.0.5",
	},
	{
		Name:    "dnsruby",
		Version: "1.70.0",
	},
	{
		Name:    "tzinfo-data",
		Version: "1.2023.3",
	},
	{
		Name:    "timecop",
		Version: "0.9.6",
	},
	{
		Name:    "arel-helpers",
		Version: "2.14.0",
	},
	{
		Name:    "rex-rop_builder",
		Version: "0.1.4",
	},
	{
		Name:    "pg",
		Version: "1.5.3",
	},
	{
		Name:    "aws-sdk-iam",
		Version: "1.79.0",
	},
	{
		Name:    "rails-html-sanitizer",
		Version: "1.6.0",
	},
	{
		Name:    "rubyntlm",
		Version: "0.6.3",
	},
	{
		Name:    "thor",
		Version: "1.2.2",
	},
	{
		Name:    "rex-mime",
		Version: "0.1.7",
	},
	{
		Name:    "irb",
		Version: "1.7.0",
	},
	{
		Name:    "little-plugger",
		Version: "1.1.4",
	},
	{
		Name:    "ast",
		Version: "2.4.2",
	},
	{
		Name:    "loofah",
		Version: "2.21.3",
	},
	{
		Name:    "hrr_rb_ssh-ed25519",
		Version: "0.4.2",
	},
	{
		Name:    "unf",
		Version: "0.1.4",
	},
	{
		Name:    "rspec-expectations",
		Version: "3.12.3",
	},
	{
		Name:    "sqlite3",
		Version: "1.6.3",
	},
	{
		Name:    "parallel",
		Version: "1.23.0",
	},
	{
		Name:    "aws-partitions",
		Version: "1.776.0",
	},
	{
		Name:    "faraday-net_http",
		Version: "3.0.2",
	},
	{
		Name:    "diff-lcs",
		Version: "1.5.0",
	},
	{
		Name:    "bson",
		Version: "4.15.0",
	},
	{
		Name:    "puma",
		Version: "6.3.0",
	},
	{
		Name:    "nori",
		Version: "2.6.0",
	},
	{
		Name:    "nessus_rest",
		Version: "0.1.6",
	},
	{
		Name:    "simplecov",
		Version: "0.18.2",
	},
	{
		Name:    "mqtt",
		Version: "0.6.0",
	},
	{
		Name:    "rex-arch",
		Version: "0.1.14",
	},
	{
		Name:    "afm",
		Version: "0.2.2",
	},
	{
		Name:    "byebug",
		Version: "11.1.3",
	},
	{
		Name:    "faye-websocket",
		Version: "0.11.2",
	},
	{
		Name:    "bindata",
		Version: "2.4.15",
	},
	{
		Name:    "metasploit_payloads-mettle",
		Version: "1.0.20",
	},
	{
		Name:    "public_suffix",
		Version: "5.0.1",
	},
	{
		Name:    "fivemat",
		Version: "1.3.7",
	},
	{
		Name:    "erubi",
		Version: "1.12.0",
	},
	{
		Name:    "rspec",
		Version: "3.12.0",
	},
	{
		Name:    "aws-sdk-ec2",
		Version: "1.382.0",
	},
	{
		Name:    "ruby-progressbar",
		Version: "1.13.0",
	},
	{
		Name:    "actionview",
		Version: "7.0.5",
	},
	{
		Name:    "activesupport",
		Version: "7.0.5",
	},
	{
		Name:    "rex-nop",
		Version: "0.1.2",
	},
	{
		Name:    "Ascii85",
		Version: "1.1.0",
	},
	{
		Name:    "openvas-omp",
		Version: "0.0.4",
	},
	{
		Name:    "factory_bot",
		Version: "6.2.1",
	},
}

func check(t *testing.T, tag string, target []*dxtypes.Package) {
	seen := make(map[string]*dxtypes.Package, len(target))

	for _, item := range target {
		key := item.Name + item.Version

		if p, ok := seen[key]; ok {
			// same
			// fmt.Println(name, item.Name, item.Version)
			// fmt.Printf("%s: [%s]%s and [%s]%s\n", tag, item.Name, item.Version, p.Name, p.Version)
			t.Fatalf("%s: [%s]%s and [%s]%s\n", tag, item.Name, item.Version, p.Name, p.Version)
		}

		seen[key] = item
	}
}

func TestData(t *testing.T) {
	check(t, "apk", APKWantPkgs)
	check(t, "apk-negative", APKNegativePkgs)
	check(t, "dpkg", DPKGWantPkgs)
	check(t, "rpm", RPMWantPkgs)
	check(t, "conan", ConanWantPkgs)
	check(t, "go-bianary", GOBianryWantPkgs)
	check(t, "go-mod", GoModWantPkgs)
	check(t, "go-modless", GoModLess117Pkgs)
	check(t, "php-composer", PHPComposerPkgs)
	check(t, "php-composer-wrong-json", PHPComposerWrongJsonPkgs)
	check(t, "php-composer-no-json", PHPComposerNoJsonPkgs)
	check(t, "python-packaging", PythonPackagingPkgs)
	check(t, "python-packaging-egg", PythonPackagingEggPkg)
	check(t, "python-packageing-whell", PythonPackagingWheel)
	check(t, "python-pip", PythonPIPPkgs)
	check(t, "python-pip-env", PythonPIPEnvPkgs)
	check(t, "python-poetry", PythonPoetryPkgs)
	check(t, "python-poetry-no-project", PythonPoetryNoProjectPkgs)
	check(t, "python-poetry-wrong-project", PythonPoetryWrongProjectPkgs)
	check(t, "java-gradle", JavaGradlePkgs)
	check(t, "java-pom", JavaPomPkgs)
	check(t, "java-pom-requirement", JavaPomRequirementPkgs)
	check(t, "node-npm", NodeNpmPkgs)
	check(t, "node-npm-folder", NodeNpmPkgsFolder)
}
