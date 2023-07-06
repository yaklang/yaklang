package analyzer

import "github.com/yaklang/yaklang/common/sca/types"

var ApkWantPkgs = []types.Package{
	{
		Name:    "alpine-baselayout",
		Version: "3.4.3-r1",
	},
	{
		Name:    "alpine-baselayout-data",
		Version: "3.4.3-r1",
	},
	{
		Name:    "alpine-keys",
		Version: "2.4-r1",
	},
	{
		Name:    "apk-tools",
		Version: "2.14.0-r2",
	},
	{
		Name:    "brotli-libs",
		Version: "1.0.9-r14",
	},
	{
		Name:    "busybox",
		Version: "1.36.1-r0",
	},
	{
		Name:    "busybox-binsh",
		Version: "1.36.1-r0",
	},
	{
		Name:    "ca-certificates",
		Version: "20230506-r0",
	},
	{
		Name:    "ca-certificates-bundle",
		Version: "20230506-r0",
	},
	{
		Name:    "curl",
		Version: "8.1.2-r0",
	},
	{
		Name:    "libc-utils",
		Version: "0.7.2-r5",
	},
	{
		Name:    "libcrypto3",
		Version: "3.1.1-r1",
	},
	{
		Name:    "libcurl",
		Version: "8.1.2-r0",
	},
	{
		Name:    "libidn2",
		Version: "2.3.4-r1",
	},
	{
		Name:    "libssl3",
		Version: "3.1.1-r1",
	},
	{
		Name:    "libunistring",
		Version: "1.1-r1",
	},
	{
		Name:    "musl",
		Version: "1.2.4-r0",
	},
	{
		Name:    "musl-utils",
		Version: "1.2.4-r0",
	},
	{
		Name:    "nghttp2-libs",
		Version: "1.53.0-r0",
	},
	{
		Name:    "scanelf",
		Version: "1.3.7-r1",
	},
	{
		Name:    "ssl_client",
		Version: "1.36.1-r0",
	},
	{
		Name:    "zlib",
		Version: "1.2.13-r1",
	},
}

var DpkgWantPkgs = []types.Package{
	{
		Name:    "adduser",
		Version: "3.118ubuntu5",
	},
	{
		Name:    "apt",
		Version: "2.4.9",
	},
	{
		Name:    "base-files",
		Version: "12ubuntu4.3",
	},
	{
		Name:    "base-passwd",
		Version: "3.5.52build1",
	},
	{
		Name:    "bash",
		Version: "5.1-6ubuntu1",
	},
	{
		Name:    "bsdutils",
		Version: "1:2.37.2-4ubuntu3",
	},
	{
		Name:    "ca-certificates",
		Version: "20230311ubuntu0.22.04.1",
	},
	{
		Name:    "coreutils",
		Version: "8.32-4.1ubuntu1",
	},
	{
		Name:    "curl",
		Version: "7.81.0-1ubuntu1.10",
	},
	{
		Name:    "dash",
		Version: "0.5.11+git20210903+057cd650a4ed-3build1",
	},
}

var RpmWantPkgs = []types.Package{

	{
		Name:    "mariner-release",
		Version: "2.0",
	},
	{
		Name:    "filesystem",
		Version: "1.1",
	},
	{
		Name:    "glibc",
		Version: "2.34",
	},
	{
		Name:    "zlib",
		Version: "1.2.11",
	},
	{
		Name:    "openssl-libs",
		Version: "1.1.1k",
	},
	{
		Name:    "xz-libs",
		Version: "5.2.5",
	},
	{
		Name:    "bzip2-libs",
		Version: "1.0.8",
	},
	{
		Name:    "zstd-libs",
		Version: "1.5.0",
	},
	{
		Name:    "sqlite-libs",
		Version: "3.34.1",
	},
	{
		Name:    "elfutils-libelf",
		Version: "0.185",
	},
	{
		Name:    "popt",
		Version: "1.16",
	},
	{
		Name:    "libgcc",
		Version: "11.2.0",
	},
	{
		Name:    "openssl",
		Version: "1.1.1k",
	},
	{
		Name:    "libcap",
		Version: "2.26",
	},
	{
		Name:    "lua-libs",
		Version: "5.3.5",
	},
	{
		Name:    "pcre-libs",
		Version: "8.44",
	},
	{
		Name:    "expat-libs",
		Version: "2.4.3",
	},
	{
		Name:    "libstdc++",
		Version: "11.2.0",
	},
	{
		Name:    "ncurses-libs",
		Version: "6.2",
	},
	{
		Name:    "readline",
		Version: "8.1",
	},
	{
		Name:    "libffi",
		Version: "3.4.2",
	},
	{
		Name:    "gmp",
		Version: "6.2.1",
	},
	{
		Name:    "e2fsprogs-libs",
		Version: "1.46.4",
	},
	{
		Name:    "p11-kit",
		Version: "0.23.22",
	},
	{
		Name:    "pcre",
		Version: "8.44",
	},
	{
		Name:    "libselinux",
		Version: "3.2",
	},
	{
		Name:    "coreutils",
		Version: "8.32",
	},
	{
		Name:    "grep",
		Version: "3.7",
	},
	{
		Name:    "bash",
		Version: "5.1.8",
	},
	{
		Name:    "libsepol",
		Version: "3.2",
	},
	{
		Name:    "libgpg-error",
		Version: "1.43",
	},
	{
		Name:    "xz",
		Version: "5.2.5",
	},
	{
		Name:    "libassuan",
		Version: "2.5.5",
	},
	{
		Name:    "krb5",
		Version: "1.18",
	},
	{
		Name:    "libgcrypt",
		Version: "1.9.4",
	},
	{
		Name:    "cracklib",
		Version: "2.9.7",
	},
	{
		Name:    "pam",
		Version: "1.5.1",
	},
	{
		Name:    "nspr",
		Version: "4.21",
	},
	{
		Name:    "mariner-rpm-macros",
		Version: "2.0",
	},
	{
		Name:    "rpm-libs",
		Version: "4.17.0",
	},
	{
		Name:    "gzip",
		Version: "1.11",
	},
	{
		Name:    "bzip2",
		Version: "1.0.8",
	},
	{
		Name:    "slang",
		Version: "2.3.2",
	},
	{
		Name:    "ncurses",
		Version: "6.2",
	},
	{
		Name:    "expat",
		Version: "2.4.3",
	},
	{
		Name:    "libssh2",
		Version: "1.9.0",
	},
	{
		Name:    "curl-libs",
		Version: "7.76.0",
	},
	{
		Name:    "curl",
		Version: "7.76.0",
	},
	{
		Name:    "file-libs",
		Version: "5.40",
	},
	{
		Name:    "libcap-ng",
		Version: "0.7.9",
	},
	{
		Name:    "tar",
		Version: "1.34",
	},
	{
		Name:    "lz4",
		Version: "1.9.2",
	},
	{
		Name:    "newt",
		Version: "0.52.21",
	},
	{
		Name:    "chkconfig",
		Version: "1.20",
	},
	{
		Name:    "libsolv",
		Version: "0.7.19",
	},
	{
		Name:    "nss-libs",
		Version: "3.44",
	},
	{
		Name:    "pinentry",
		Version: "1.2.0",
	},
	{
		Name:    "kmod",
		Version: "29",
	},
	{
		Name:    "libksba",
		Version: "1.6.0",
	},
	{
		Name:    "zstd",
		Version: "1.5.0",
	},
	{
		Name:    "unzip",
		Version: "6.0",
	},
	{
		Name:    "npth",
		Version: "1.6",
	},
	{
		Name:    "gnupg2",
		Version: "2.3.3",
	},
	{
		Name:    "gpgme",
		Version: "1.16.0",
	},
	{
		Name:    "vim",
		Version: "8.2.4081",
	},
	{
		Name:    "libtool",
		Version: "2.4.6",
	},
	{
		Name:    "findutils",
		Version: "4.8.0",
	},
	{
		Name:    "mpfr",
		Version: "4.1.0",
	},
	{
		Name:    "gawk",
		Version: "5.1.0",
	},
	{
		Name:    "gdbm",
		Version: "1.21",
	},
	{
		Name:    "lua",
		Version: "5.3.5",
	},
	{
		Name:    "libarchive",
		Version: "3.4.2",
	},
	{
		Name:    "openldap",
		Version: "2.4.57",
	},
	{
		Name:    "elfutils-libelf-devel",
		Version: "0.185",
	},
	{
		Name:    "libgomp",
		Version: "11.2.0",
	},
	{
		Name:    "cpio",
		Version: "2.13",
	},
	{
		Name:    "libtasn1",
		Version: "4.14",
	},
	{
		Name:    "p11-kit-trust",
		Version: "0.23.22",
	},
	{
		Name:    "ca-certificates-tools",
		Version: "2.0.0",
	},
	{
		Name:    "json-c",
		Version: "0.14",
	},
	{
		Name:    "libpkgconf",
		Version: "1.8.0",
	},
	{
		Name:    "pkgconf",
		Version: "1.8.0",
	},
	{
		Name:    "tdnf-cli-libs",
		Version: "2.1.0",
	},
	{
		Name:    "tdnf",
		Version: "2.1.0",
	},
	{
		Name:    "tdnf-plugin-repogpgcheck",
		Version: "2.1.0",
	},
	{
		Name:    "sed",
		Version: "4.8",
	},
	{
		Name:    "net-tools",
		Version: "1.60",
	},
	{
		Name:    "ca-certificates-shared",
		Version: "2.0.0",
	},
	{
		Name:    "ca-certificates-base",
		Version: "2.0.0",
	},
	{
		Name:    "pkgconf-m4",
		Version: "1.8.0",
	},
	{
		Name:    "pkgconf-pkg-config",
		Version: "1.8.0",
	},
	{
		Name:    "zstd-devel",
		Version: "1.5.0",
	},
	{
		Name:    "popt-devel",
		Version: "1.16",
	},
	{
		Name:    "zlib-devel",
		Version: "1.2.11",
	},
	{
		Name:    "xz-devel",
		Version: "5.2.5",
	},
	{
		Name:    "pcre-devel",
		Version: "8.44",
	},
	{
		Name:    "libsepol-devel",
		Version: "3.2",
	},
	{
		Name:    "libselinux-devel",
		Version: "3.2",
	},
	{
		Name:    "util-linux",
		Version: "2.37.2",
	},
	{
		Name:    "util-linux-devel",
		Version: "2.37.2",
	},
	{
		Name:    "python3",
		Version: "3.9.9",
	},
	{
		Name:    "python3-libs",
		Version: "3.9.9",
	},
	{
		Name:    "glib",
		Version: "2.60.1",
	},
	{
		Name:    "wget",
		Version: "1.20.3",
	},
	{
		Name:    "e2fsprogs",
		Version: "1.46.4",
	},
	{
		Name:    "systemd-rpm-macros",
		Version: "249.7",
	},
	{
		Name:    "elfutils-default-yama-scope",
		Version: "0.185",
	},
	{
		Name:    "cryptsetup-libs",
		Version: "2.3.3",
	},
	{
		Name:    "device-mapper-libs",
		Version: "2.03.05",
	},
	{
		Name:    "systemd",
		Version: "249.7",
	},
	{
		Name:    "elfutils",
		Version: "0.185",
	},
	{
		Name:    "elfutils-devel",
		Version: "0.185",
	},
	{
		Name:    "rpm-build-libs",
		Version: "4.17.0",
	},
	{
		Name:    "debugedit",
		Version: "5.0",
	},
	{
		Name:    "rpm",
		Version: "4.17.0",
	},
	{
		Name:    "rpm-devel",
		Version: "4.17.0",
	},
	{
		Name:    "rpm-build",
		Version: "4.17.0",
	},
	{
		Name:    "mariner-repos-shared",
		Version: "2.0",
	},
	{
		Name:    "mariner-repos-preview",
		Version: "2.0",
	},
	{
		Name:    "mariner-repos-microsoft-preview",
		Version: "2.0",
	},
	{
		Name:    "mariner-repos-extras-preview",
		Version: "2.0",
	},
	{
		Name:    "mariner-repos-extended-preview",
		Version: "2.0",
	},
	{
		Name:    "audit-libs",
		Version: "3.0.6",
	},
	{
		Name:    "audit",
		Version: "3.0.6",
	},
	{
		Name:    "libsemanage",
		Version: "3.2",
	},
	{
		Name:    "shadow-utils",
		Version: "4.9",
	},
	{
		Name:    "sudo",
		Version: "1.9.5p2",
	},
	{
		Name:    "core-packages-container",
		Version: "2.0",
	},
	{
		Name:    "sqlite",
		Version: "3.34.1",
	},
}
