package utils

import (
	"fmt"
	"testing"
	"yaklang/common/log"
)

var (
	TestRaw = []byte(`
# TaskFunc-Secure/WRQ
match ssh m|^SSH-([\d.]+)-([\d.]+) TaskFunc-Secure SSH Windows NT Server\r?\n| p/TaskFunc-Secure WinNT sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\d.]+) dss TaskFunc-SECURE SSH\r?\n| p/TaskFunc-Secure sshd/ v/$2/ i/dss-only; protocol $1/
match ssh m|^SSH-([\d.]+)-([\d.]+) TaskFunc-SECURE SSH.*\r?\n| p/TaskFunc-Secure sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-ReflectionForSecureIT_([-\w_.]+) - Process Software MultiNet\r\n| p/WRQ Reflection for Secure IT sshd/ v/$2/ i/OpenVMS MultiNet; protocol $1/ o/OpenVMS/ cpe:/o:hp:openvms/a
match ssh m|^SSH-([\d.]+)-ReflectionForSecureIT_([-\w_.]+)\r?\n| p/WRQ Reflection for Secure IT sshd/ v/$2/ i/protocol $1/

# SCS
match ssh m|^SSH-(\d[\d.]+)-SSH Protocol Compatible Server SCS (\d[-.\w]+)\r?\n| p/SCS NetScreen sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-SSH Compatible Server\r?\n| p/SCS NetScreen sshd/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-([\d.]+) SSH Secure Shell Tru64 UNIX\r?\n| p/SCS sshd/ v/$2/ i/protocol $1/ o/Tru64 UNIX/ cpe:/o:compaq:tru64/a
match ssh m|^SSH-([\d.]+)-(\d+\.\d+\.\d+) SSH Secure Shell| p/SCS sshd/ v/$2/ i/protocol $1/
match ssh m|^sshd: SSH Secure Shell (\d[-.\w]+) on ([-.\w]+)\nSSH-(\d[\d.]+)-| p/SCS SSH Secure Shell/ v/$1/ i/on $2; protocol $3/
match ssh m|^sshd: SSH Secure Shell (\d[-.\w]+) \(([^\r\n\)]+)\) on ([-.\w]+)\nSSH-(\d[\d.]+)-| p/SCS sshd/ v/$1/ i/$2; on $3; protocol $4/
match ssh m|^sshd2\[\d+\]: .*\r\nSSH-([\d.]+)-(\d[-.\w]+) SSH Secure Shell \(([^\r\n\)]+)\)\r?\n| p/SCS sshd/ v/$2/ i/protocol $1; $3/
match ssh m|^SSH-([\d.]+)-(\d+\.\d+\.[-.\w]+)| p/SCS sshd/ v/$2/ i/protocol $1/

# OpenSSH
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) Debian-(\S*maemo\S*)\r?\n| p/OpenSSH/ v/$2 Debian $3/ i/Nokia Maemo tablet; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:debian:debian_linux/ cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)[ -]{1,2}Debian[ -_](.*ubuntu.*)\r\n| p/OpenSSH/ v/$2 Debian $3/ i/Ubuntu Linux; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:canonical:ubuntu_linux/ cpe:/o:linux:linux_kernel/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)[ -]{1,2}Ubuntu[ -_]([^\r\n]+)\r?\n| p/OpenSSH/ v/$2 Ubuntu $3/ i/Ubuntu Linux; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:canonical:ubuntu_linux/ cpe:/o:linux:linux_kernel/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)[ -]{1,2}Debian[ -_]([^\r\n]+)\r?\n| p/OpenSSH/ v/$2 Debian $3/ i/protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:debian:debian_linux/ cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-([\d.]+)-OpenSSH_[\w.]+-FC-([\w.-]+)\.fc(\d+)\r\n| p/OpenSSH/ v/$2 Fedora/ i/Fedora Core $3; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:fedoraproject:fedora_core:$3/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD-([\d]+)\r?\n| p/OpenSSH/ v/$2/ i/FreeBSD $3; protocol $1/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD localisations (\d+)\r?\n| p/OpenSSH/ v/$2/ i/FreeBSD $3; protocol $1/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m=^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD-openssh-portable-(?:base-|amd64-)?[\w.,]+\r?\n= p/OpenSSH/ v/$2/ i/protocol $1/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD-openssh-portable-overwrite-base| p/OpenSSH/ v/$2/ i/protocol $1; overwrite base SSH/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD-openssh-gssapi-| p/OpenSSH/ v/$2/ i/gssapi; protocol $1/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) FreeBSD\n| p/OpenSSH/ v/$2/ i/protocol $1/ o/FreeBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:freebsd:freebsd/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) miniBSD-([\d]+)\r?\n| p/OpenSSH/ v/$2/ i/MiniBSD $3; protocol $1/ o/MiniBSD/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) NetBSD_Secure_Shell-([\w._+-]+)\r?\n| p/OpenSSH/ v/$2/ i/NetBSD $3; protocol $1/ o/NetBSD/ cpe:/a:openbsd:openssh:$2/ cpe:/o:netbsd:netbsd/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)_Mikrotik_v([\d.]+)\r?\n| p/OpenSSH/ v/$2 mikrotik $3/ i/protocol $1/ d/router/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) in RemotelyAnywhere ([\d.]+)\r?\n| p/OpenSSH/ v/$2/ i/RemotelyAnywhere $3; protocol $1/ o/Windows/ cpe:/a:openbsd:openssh:$2/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)\+CAN-2004-0175\r?\n| p/OpenSSH/ v/$2+CAN-2004-0175/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) NCSA_GSSAPI_20040818 KRB5\r?\n| p/OpenSSH/ v/$2 NCSA_GSSAPI_20040818 KRB5/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
# http://www.psc.edu/index.php/hpn-ssh
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)[-_]hpn(\w+) *(?:\"\")?\r?\n| p/OpenSSH/ v/$2/ i/protocol $1; HPN-SSH patch $3/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+\+sftpfilecontrol-v[\d.]+-hpn\w+)\r?\n| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+-hpn) NCSA_GSSAPI_\d+ KRB5\r?\n| p/OpenSSH/ v/$2/ i/protocol $1; kerberos support/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_3\.4\+p1\+gssapi\+OpenSSH_3\.7\.1buf_fix\+2006100301\r?\n| p/OpenSSH/ v/3.4p1 with CMU Andrew patches/ i/protocol $1/ cpe:/a:openbsd:openssh:3.4p1/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+\.RL)\r?\n| p/OpenSSH/ v/$2 Allied Telesis/ i/protocol $1/ d/switch/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+-CERN\d+)\r?\n| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+\.cern-hpn)| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+-hpn)\r?\n| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+-pwexp\d+)\r?\n| p/OpenSSH/ v/$2/ i/protocol $1/ o/AIX/ cpe:/a:openbsd:openssh:$2/ cpe:/o:ibm:aix/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)-chrootssh\n| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-Nortel\r?\n| p/Nortel SSH/ i/protocol $1/ d/switch/ cpe:/a:openbsd:openssh/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w.]+)[-_]hpn(\w+) DragonFly-| p/OpenSSH/ v/$2/ i/protocol $1; HPN-SSH patch $3/ o/DragonFlyBSD/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w.]+) DragonFly-| p/OpenSSH/ v/$2/ i/protocol $1/ o/DragonFlyBSD/ cpe:/a:openbsd:openssh:$2/
# Not sure about the next 2 being these specific devices:
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w_.-]+) FIPS\n| p/OpenSSH/ v/$2/ i/protocol $1; Imperva SecureSphere firewall/ d/firewall/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w_.-]+) FIPS\r\n| p/OpenSSH/ v/$2/ i/protocol $1; Cisco NX-OS/ d/switch/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w_.-]+) NCSA_GSSAPI_GPT_([-\w_.]+) GSI\n| p/OpenSSH/ v/$2/ i/protocol $1; NCSA GSSAPI authentication patch $3/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) \.\n| p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) PKIX\r\n| p/OpenSSH/ v/$2/ i/protocol $1; X.509 v3 certificate support/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)-FIPS\(capable\)\r\n| p/OpenSSH/ v/$2/ i/protocol $1; FIPS capable/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)-sshjail\n| p/OpenSSH/ v/$2/ i/protocol $1; sshjail patch/ cpe:/a:openbsd:openssh:$2/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) Raspbian-([^\r\n]+)\r?\n| p/OpenSSH/ v/$2 Raspbian $3/ i/protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) OVH-rescue\r\n| p/OpenSSH/ v/$2/ i/protocol $1; OVH hosting rescue/ cpe:/a:openbsd:openssh:$2/a
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) Trisquel_GNU/linux_([\d.]+)(?:-\d+)?\r\n| p/OpenSSH/ v/$2/ i/protocol $1; Trisquel $3/ o/Linux/ cpe:/a:openbsd:openssh:$2/a cpe:/o:linux:linux_kernel/a cpe:/o:trisquel_project:trisquel_gnu%2flinux:$3/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+) \+ILOM\.2015-5600\r\n| p/OpenSSH/ v/$2/ i/protocol $1; ILOM patched CVE-2015-5600/ cpe:/a:openbsd:openssh:$2/a cpe:/h:oracle:integrated_lights-out/

# Choose your destiny:
# 1) Match all OpenSSHs:
#match ssh m/^SSH-([\d.]+)-OpenSSH[_-]([\S ]+)/i p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/
# 2) Don't match unknown SSHs (and generate fingerprints)
match ssh m|^SSH-([\d.]+)-OpenSSH[_-]([\w.]+)\s*\r?\n|i p/OpenSSH/ v/$2/ i/protocol $1/ cpe:/a:openbsd:openssh:$2/

# These are strange ones. These routers pretend to be OpenSSH, but don't do it that well (see the \r):
match ssh m|^SSH-2\.0-OpenSSH\r?\n| p/Linksys WRT45G modified dropbear sshd/ i/protocol 2.0/ d/router/
match ssh m|^SSH-2\.0-OpenSSH_3\.6p1\r?\n| p|D-Link/Netgear DSL router modified dropbear sshd| i/protocol 2.0/ d/router/

match ssh m|^\0\0\0\$\0\0\0\0\x01\0\0\0\x1bNo host key is configured!\n\r!\"v| p/Foundry Networks switch sshd/ i/broken: No host key configured/
match ssh m|^SSH-(\d[\d.]+)-SSF-(\d[-.\w]+)\r?\n| p/SSF French SSH/ v/$2/ i/protocol $1/
match ssh m|^SSH-(\d[\d.]+)-lshd_(\d[-.\w]+) lsh - a free ssh\r\n\0\0| p/lshd secure shell/ v/$2/ i/protocol $1/
match ssh m|^SSH-(\d[\d.]+)-lshd-(\d[-.\w]+) lsh - a GNU ssh\r\n\0\0| p/lshd secure shell/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-Sun_SSH_(\S+)| p/SunSSH/ v/$2/ i/protocol $1/ cpe:/a:sun:sunssh:$2/
match ssh m|^SSH-([\d.]+)-meow roototkt by rebel| p/meow SSH ROOTKIT/ i/protocol $1/
# Akamai hosted systems tend to run this - found on www.microsoft.com
match ssh m|^SSH-(\d[\d.]*)-(AKAMAI-I*)\r?\n$| p/Akamai SSH/ v/$2/ i/protocol $1/ cpe:/a:akamai:ssh:$2/
match ssh m|^SSH-(\d[\d.]*)-AKAMAI-([\d.]+)\r?\n$| p/Akamai SSH/ v/$2/ i/protocol $1/ cpe:/a:akamai:ssh:$2/
match ssh m|^SSH-(\d[\d.]*)-(Server-V)\r?\n$| p/Akamai SSH/ v/$2/ i/protocol $1/ cpe:/a:akamai:ssh:$2/
match ssh m|^SSH-(\d[\d.]*)-(Server-VI)\r?\n$| p/Akamai SSH/ v/$2/ i/protocol $1/ cpe:/a:akamai:ssh:$2/
match ssh m|^SSH-(\d[\d.]*)-(Server-VII)\r?\n| p/Akamai SSH/ v/$2/ i/protocol $1/ cpe:/a:akamai:ssh:$2/
match ssh m|^SSH-(\d[\d.]+)-Cisco-(\d[\d.]+)\r?\n$| p/Cisco SSH/ v/$2/ i/protocol $1/ o/IOS/ cpe:/a:cisco:ssh:$2/ cpe:/o:cisco:ios/a
match ssh m|^SSH-(\d[\d.]+)-CiscoIOS_([\d.]+)XA\r?\n| p/Cisco SSH/ v/$2/ i/protocol $1; IOS XA/ o/IOS/ cpe:/a:cisco:ssh:$2/ cpe:/o:cisco:ios/a
match ssh m|^\r\nDestination server does not have Ssh activated\.\r\nContact Cisco Systems, Inc to purchase a\r\nlicense key to activate Ssh\.\r\n| p/Cisco CSS SSH/ i/Unlicensed/ cpe:/a:cisco:ssh/
match ssh m|^SSH-(\d[\d.]+)-VShell_(\d[_\d.]+) VShell\r?\n$| p/VanDyke VShell sshd/ v/$SUBST(2,"_",".")/ i/protocol $1/ cpe:/a:vandyke:vshell:$SUBST(2,"_",".")/
match ssh m|^SSH-2\.0-0\.0 \r?\n| p/VanDyke VShell sshd/ i/version info hidden; protocol 2.0/ cpe:/a:vandyke:vshell/
match ssh m|^SSH-([\d.]+)-([\w.]+) VShell\r?\n| p/VanDyke VShell/ v/$2/ i/protocol $1/ cpe:/a:vandyke:vshell:$2/
match ssh m|^SSH-([\d.]+)-([\w.]+) \(beta\) VShell\r?\n| p/VanDyke VShell/ v/$2 beta/ i/protocol $1/ cpe:/a:vandyke:vshell:$2:beta/
match ssh m|^SSH-([\d.]+)-(\d[-.\w]+) sshlib: WinSSHD (\d[-.\w]+)\r?\n| p/Bitvise WinSSHD/ v/$3/ i/sshlib $2; protocol $1/ o/Windows/ cpe:/a:bitvise:winsshd:$3/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-(\d[-.\w]+) sshlib: WinSSHD\r?\n| p/Bitvise WinSSHD/ i/sshlib $2; protocol $1; server version hidden/ o/Windows/ cpe:/a:bitvise:winsshd/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) sshlib: sshlibSrSshServer ([\w._-]+)\r\n| p/SrSshServer/ v/$3/ i/sshlib $2; protocol $1/
match ssh m|^SSH-([\d.]+)-([\w._-]+) sshlib: GlobalScape\r?\n| p/GlobalScape CuteFTP sshd/ i/sshlib $2; protocol $1/ o/Windows/ cpe:/a:globalscape:cuteftp/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w.-]+)_sshlib GlobalSCAPE\r\n| p/GlobalScape CuteFTP sshd/ i/sshlib $2; protocol $1/ o/Windows/ cpe:/a:globalscape:cuteftp/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w.-]+)_sshlib Globalscape\r\n| p/GlobalScape EFT sshd/ i/sshlib $2; protocol $1/ o/Windows/ cpe:/a:globalscape:eft_server/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) sshlib: EdmzSshDaemon ([\w._-]+)\r\n| p/EdmzSshDaemon/ v/$3/ i/sshlib $2; protocol $1/
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: WinSSHD ([\w._-]+)\r\n| p/Bitvise WinSSHD/ v/$3/ i/FlowSsh $2; protocol $1/ o/Windows/ cpe:/a:bitvise:winsshd:$3/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: WinSSHD ([\w._-]+): free only for personal non-commercial use\r\n| p/Bitvise WinSSHD/ v/$3/ i/FlowSsh $2; protocol $1; non-commercial use/ o/Windows/ cpe:/a:bitvise:winsshd:$3/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: WinSSHD: free only for personal non-commercial use\r\n| p/Bitvise WinSSHD/ i/FlowSsh $2; protocol $1; non-commercial use/ o/Windows/ cpe:/a:bitvise:winsshd/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: Bitvise SSH Server \(WinSSHD\) ([\w._-]+): free only for personal non-commercial use\r\n| p/Bitvise WinSSHD/ v/$3/ i/FlowSsh $2; protocol $1; non-commercial use/ o/Windows/ cpe:/a:bitvise:winsshd:$3/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: Bitvise SSH Server \(WinSSHD\) ([\w._-]+)\r\n| p/Bitvise WinSSHD/ v/$3/ i/FlowSsh $2; protocol $1/ o/Windows/ cpe:/a:bitvise:winsshd:$3/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-([\w._-]+) FlowSsh: Bitvise SSH Server \(WinSSHD\) \r\n| p/Bitvise WinSSHD/ i/FlowSsh $2; protocol $1/ o/Windows/ cpe:/a:bitvise:winsshd/ cpe:/o:microsoft:windows/a
# Cisco VPN 3000 Concentrator
# Cisco VPN Concentrator 3005 - Cisco Systems, Inc./VPN 3000 Concentrator Version 4.0.1.B Jun 20 2003
match ssh m|^SSH-([\d.]+)-OpenSSH\r?\n$| p/OpenSSH/ i/protocol $1/ d/terminal server/ cpe:/a:openbsd:openssh/a
match ssh m|^SSH-1\.5-X\r?\n| p/Cisco VPN Concentrator SSHd/ i/protocol 1.5/ d/terminal server/ cpe:/o:cisco:vpn_3000_concentrator_series_software/
match ssh m|^SSH-([\d.]+)-NetScreen\r?\n| p/NetScreen sshd/ i/protocol $1/ d/firewall/ cpe:/o:juniper:netscreen_screenos/
match ssh m|^SSH-1\.5-FucKiT RootKit by Cyrax\r?\n| p/FucKiT RootKit sshd/ i/**BACKDOOR** protocol 1.5/ o/Linux/ cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-2\.0-dropbear_([-\w.]+)\r?\n| p/Dropbear sshd/ v/$1/ i/protocol 2.0/ o/Linux/ cpe:/a:matt_johnston:dropbear_ssh_server:$1/ cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-2\.0-dropbear\r\n| p/Dropbear sshd/ i/protocol 2.0/ o/Linux/ cpe:/a:matt_johnston:dropbear_ssh_server/ cpe:/o:linux:linux_kernel/a
match ssh m|^Access to service sshd from [-\w_.]+@[-\w_.]+ has been denied\.\r\n| p/libwrap'd OpenSSH/ i/Access denied/ cpe:/a:openbsd:openssh/
match ssh m|^SSH-([\d.]+)-FortiSSH_([\d.]+)\r?\n| p/FortiSSH/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-cryptlib\r?\n| p/APC AOS cryptlib sshd/ i/protocol $1/ o/AOS/ cpe:/o:apc:aos/a
match ssh m|^SSH-([\d.]+)-([\d.]+) Radware\r?\n$| p/Radware Linkproof SSH/ v/$2/ i/protocol $1/ d/terminal server/
match ssh m|^SSH-2\.0-1\.0 Radware SSH \r?\n| p/Radware sshd/ i/protocol 2.0/ d/firewall/
match ssh m|^SSH-([\d.]+)-Radware_([\d.]+)\r?\n| p/Radware sshd/ v/$2/ i/protocol $1/ d/firewall/
match ssh m|^SSH-1\.5-By-ICE_4_All \( Hackers Not Allowed! \)\r?\n| p/ICE_4_All backdoor sshd/ i/**BACKDOOR** protocol 1.5/
match ssh m|^SSH-2\.0-mpSSH_([\d.]+)\r?\n| p/HP Integrated Lights-Out mpSSH/ v/$1/ i/protocol 2.0/ cpe:/h:hp:integrated_lights-out/
match ssh m|^SSH-2\.0-Unknown\r?\n| p/Allot Netenforcer OpenSSH/ i/protocol 2.0/
match ssh m|^SSH-2\.0-FrSAR ([\d.]+) TRUEX COMPT 32/64\r?\n| p/FrSAR truex compt sshd/ v/$1/ i/protocol 2.0/
match ssh m|^SSH-2\.0-(\d{8,12})\r?\n| p/Netpilot config access/ v/$1/ i/protocol 2.0/
match ssh m|^SSH-([\d.]+)-RomCliSecure_([\d.]+)\r?\n| p/Adtran Netvanta RomCliSecure sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-2\.0-APSSH_([\w.]+)\r?\n| p/APSSHd/ v/$1/ i/protocol 2.0/
match ssh m|^SSH-2\.0-Twisted\r?\n| p/Kojoney SSH honeypot/ i/protocol 2.0/ cpe:/a:twistedmatrix:twisted/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w.]+)\r?\n.*aes256|s p/Kojoney SSH honeypot/ i/Pretending to be $2; protocol $1/
match ssh m|^SSH-2\.0-Mocana SSH\r\n| p/Mocana embedded SSH/ i/protocol 2.0/
match ssh m|^SSH-2\.0-Mocana SSH \r?\n| p/Mocana embedded SSH/ i/protocol 2.0/
match ssh m|^SSH-2\.0-Mocana SSH ([\d.]+)\r?\n| p/Mocana NanoSSH/ v/$1/ i/protocol 2.0/
match ssh m|^SSH-1\.99-InteropSecShell_([\d.]+)\r?\n| p/InteropSystems SSH/ v/$1/ i/protocol 1.99/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-WeOnlyDo(?:-wodFTPD)? ([\d.]+)\r?\n| p/WeOnlyDo sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-WeOnlyDo-([\d.]+)\r?\n| p/WeOnlyDo sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-2\.0-PGP\r?\n| p/PGP Universal sshd/ i/protocol 2.0/ cpe:/a:pgp:universal_server/
match ssh m|^SSH-([\d.]+)-libssh[_-]([-\w.]+)\r?\n| p/libssh/ v/$2/ i/protocol $1/ cpe:/a:libssh:libssh:$2/
match ssh m|^SSH-([\d.]+)-HUAWEI-VRP([\d.]+)\r?\n| p/Huawei VRP sshd/ i/protocol $1/ d/router/ o/VRP $2/ cpe:/o:huawei:vrp:$2/
match ssh m|^SSH-([\d.]+)-HUAWEI-UMG([\d.]+)\r?\n| p/Huawei Unified Media Gateway sshd/ i/model: $2; protocol $1/ cpe:/h:huawei:$2/
# Huawei 6050 WAP
match ssh m|^SSH-([\d.]+)-HUAWEI-([\d.]+)\r?\n| p/Huawei WAP sshd/ v/$2/ i/protocol $1/ d/WAP/
match ssh m|^SSH-([\d.]+)-VRP-([\d.]+)\r?\n| p/Huawei VRP sshd/ i/protocol $1/ d/router/ o/VRP $2/ cpe:/o:huawei:vrp:$2/
match ssh m|^SSH-([\d.]+)-lancom\r?\n| p/lancom sshd/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-xxxxxxx\r?\n| p|Fortinet VPN/firewall sshd| i/protocol $1/ d/firewall/
match ssh m|^SSH-([\d.]+)-AOS_SSH\r?\n| p/AOS sshd/ i/protocol $1/ o/AOS/ cpe:/o:apc:aos/a
match ssh m|^SSH-([\d.]+)-RedlineNetworksSSH_([\d.]+) Derived_From_OpenSSH-([\d.])+\r?\n| p/RedLineNetworks sshd/ v/$2/ i/Derived from OpenSSH $3; protocol $1/
match ssh m|^SSH-([\d.]+)-DLink Corp\. SSH server ver ([\d.]+)\r?\n| p/D-Link sshd/ v/$2/ i/protocol $1/ d/router/
match ssh m|^SSH-([\d.]+)-FreSSH\.([\d.]+)\r?\n| p/FreSSH/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-Neteyes-C-Series_([\d.]+)\r?\n| p/Neteyes C Series load balancer sshd/ v/$2/ i/protocol $1/ d/load balancer/
match ssh m|^SSH-([\d.]+)-IPSSH-([\d.]+)\r?\n| p|Cisco/3com IPSSHd| v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-DigiSSH_([\d.]+)\r?\n| p/Digi CM sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-0 Tasman Networks Inc\.\r?\n| p/Tasman router sshd/ i/protocol $1/ d/router/
match ssh m|^SSH-([\d.]+)-([\w.]+)rad\r?\n| p/Rad Java SFTPd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\d.]+) in DesktopAuthority ([\d.]+)\r?\n| p/DesktopAuthority OpenSSH/ v/$2/ i/DesktopAuthority $3; protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-NOS-SSH_([\d.]+)\r?\n| p/3Com WX2200 or WX4400 NOS sshd/ v/$2/ i/protocol $1/ d/WAP/
match ssh m|^SSH-1\.5-SSH\.0\.1\r?\n| p/Dell PowerConnect sshd/ i/protocol 1.5/ d/power-device/
match ssh m|^SSH-([\d.]+)-Ingrian_SSH\r?\n| p/Ingrian SSH/ i/protocol $1/ d/security-misc/
match ssh m|^SSH-([\d.]+)-PSFTPd PE\. Secure FTP Server ready\r?\n| p/PSFTPd sshd/ i/protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-BlueArcSSH_([\d.]+)\r?\n| p/BlueArc sshd/ v/$2/ i/protocol $1/ d/storage-misc/
match ssh m|^SSH-([\d.]+)-Zyxel SSH server\r?\n| p/ZyXEL ZyWALL sshd/ i/protocol $1/ d/security-misc/ o/ZyNOS/ cpe:/o:zyxel:zynos/
match ssh m|^SSH-([\d.]+)-paramiko_([\w._-]+)\r?\n| p/Paramiko Python sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-USHA SSHv([\w._-]+)\r?\n| p/USHA SSH/ v/$2/ i/protocol $1/ d/power-device/
match ssh m|^SSH-([\d.]+)-SSH_0\.2\r?\n$| p/3com sshd/ v/0.2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-CoreFTP-([\w._-]+)\r?\n| p/CoreFTP sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-RomSShell_([\w._-]+)\r\n| p/AllegroSoft RomSShell sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-IFT SSH server BUILD_VER\n| p/Sun StorEdge 3511 sshd/ i/protocol $1; IFT SSH/ d/storage-misc/
match ssh m|^Could not load hosy key\. Closing connection\.\.\.$| p/Cisco switch sshd/ i/misconfigured/ d/switch/ o/IOS/ cpe:/a:cisco:ssh/ cpe:/o:cisco:ios/a
match ssh m|^Could not load host key\. Closing connection\.\.\.$| p/Cisco switch sshd/ i/misconfigured/ d/switch/ o/IOS/ cpe:/a:cisco:ssh/ cpe:/o:cisco:ios/a
match ssh m|^SSH-([\d.]+)-WS_FTP-SSH_([\w._-]+)(?: FIPS)?\r\n| p/WS_FTP sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/a:ipswitch:ws_ftp:$2/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-http://www\.sshtools\.com J2SSH \[SERVER\]\r\n| p/SSHTools J2SSH/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-DraySSH_([\w._-]+)\n\n\rNo connection is available now\. Try again later!$| p/DrayTek Vigor 2820 ADSL router sshd/ v/$2/ i/protocol $1/ d/broadband router/ cpe:/h:draytek:vigor_2820/a
match ssh m|^SSH-([\d.]+)-DraySSH_([\w._-]+)\n| p/DrayTek Vigor ADSL router sshd/ v/$2/ i/protocol $1/ d/broadband router/
match ssh m|^SSH-([\d.]+)-Pragma FortressSSH ([\d.]+)\n| p/Pragma Fortress SSH Server/ v/$2/ i/protocol $1/ o/Windows/ cpe:/a:pragmasys:fortress_ssh_server:$2/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-SysaxSSH_([\d.]+)\r\n| p/Sysax Multi Server sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/a:sysax:multi_server:$2/ cpe:/o:microsoft:windows/a
# CP-7900G and 8961
match ssh m|^SSH-([\d.]+)-1\.00\r\n$| p/Cisco IP Phone sshd/ i/protocol $1/ d/VoIP phone/
match ssh m|^SSH-([\d.]+)-Foxit-WAC-Server-([\d.]+ Build \d+)\n| p/Foxit WAC Server sshd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-ROSSSH\r\n| p/MikroTik RouterOS sshd/ i/protocol $1/ d/router/ o/Linux/ cpe:/o:linux:linux_kernel/a cpe:/o:mikrotik:routeros/
match ssh m|^SSH-([\d.]+)-3Com OS-([\w._-]+ Release \w+)\n| p/3Com switch sshd/ v/$2/ i/protocol $1/ d/switch/ o/Comware/ cpe:/o:3com:comware/
match ssh m|^SSH-([\d.]+)-3Com OS-3Com OS V([\w._-]+)\n| p/3Com switch sshd/ v/$2/ i/protocol $1/ d/switch/ o/Comware/ cpe:/o:3com:comware/
match ssh m|^SSH-([\d.]+)-XXXX\r\n| p/Cyberoam firewall sshd/ i/protocol $1/ d/firewall/
match ssh m|^SSH-([\d.]+)-xxx\r\n| p/Cyberoam UTM firewall sshd/ i/protocol $1/ d/firewall/
match ssh m|^SSH-([\d.]+)-OpenSSH_([\w._-]+)-HipServ\n| p/Seagate GoFlex NAS device sshd/ v/$2/ i/protocol $1/ d/storage-misc/
match ssh m|^SSH-([\d.]+)-xlightftpd_release_([\w._-]+)\r\n| p/Xlight FTP Server sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-Serv-U_([\w._-]+)\r\n| p/Serv-U SSH Server/ v/$2/ i/protocol $1/ cpe:/a:serv-u:serv-u:$2/
match ssh m|^SSH-([\d.]+)-CerberusFTPServer_([\w._-]+)\r\n| p/Cerberus FTP Server sshd/ v/$2/ i/protocol $1/ cpe:/a:cerberusftp:ftp_server:$2/
match ssh m|^SSH-([\d.]+)-CerberusFTPServer_([\w._-]+) FIPS\r\n| p/Cerberus FTP Server sshd/ v/$2/ i/protocol $1; FIPS/ cpe:/a:cerberusftp:ftp_server:$2/
match ssh m|^SSH-([\d.]+)-SSH_v2\.0@force10networks\.com\r\n| p/Force10 switch sshd/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-Data ONTAP SSH ([\w._-]+)\n| p/NetApp Data ONTAP sshd/ v/$2/ i/protocol $1/ cpe:/a:netapp:data_ontap/
match ssh m|^SSH-([\d.]+)-SSHTroll| p/SSHTroll ssh honeypot/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-AudioCodes\n| p/AudioCodes MP-124 SIP gateway sshd/ i/protocol $1/ d/VoIP adapter/ cpe:/h:audiocodes:mp-124/
match ssh m|^SSH-([\d.]+)-WRQReflectionForSecureIT_([\w._-]+) Build ([\w._-]+)\r\n| p/WRQ Reflection for Secure IT sshd/ v/$2 build $3/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-Nand([\w._-]+)\r\n| p/Nand sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-SSHD-CORE-([\w._-]+)-ATLASSIAN([\w._-]*)\r\n| p/Apache Mina sshd/ v/$2-ATLASSIAN$3/ i/Atlassian Stash; protocol $1/ cpe:/a:apache:sshd:$2/
# Might not always be Atlassian
match ssh m|^SSH-([\d.]+)-SSHD-UNKNOWN\r\n| p/Apache Mina sshd/ i/Atlassian Bitbucket; protocol $1/ cpe:/a:apache:sshd/
match ssh m|^SSH-([\d.]+)-GerritCodeReview_([\w._-]+) \(SSHD-CORE-([\w._-]+)\)\r\n| p/Apache Mina sshd/ v/$3/ i/Gerrit Code Review $2; protocol $1/ cpe:/a:apache:sshd:$3/
match ssh m|^SSH-([\d.]+)-SSHD-CORE-([\w._-]+)\r\n| p/Apache Mina sshd/ v/$2/ i/protocol $1/ cpe:/a:apache:sshd:$2/
match ssh m|^SSH-([\d.]+)-Plan9\r?\n| p/Plan 9 sshd/ i/protocol $1/ o/Plan 9/ cpe:/o:belllabs:plan_9/a
match ssh m|^SSH-2\.0-CISCO_WLC\n| p/Cisco WLC sshd/ d/remote management/
match ssh m|^SSH-([\d.]+)-([\w._-]+) sshlib: ([78]\.\d+\.\d+\.\d+)\r\n| p/MoveIT DMZ sshd/ v/$3/ i/sshlib $2; protocol $1/
match ssh m|^SSH-([\d.]+)-Adtran_([\w._-]+)\r\n| p/Adtran sshd/ v/$2/ i/protocol $1/ o/AOS/ cpe:/o:adtran:aos/
# Axway SecureTransport 1.5 ssh (too generic? --ed.)
match ssh m|^SSH-([\d.]+)-SSHD\r\n| p/Axway SecureTransport sshd/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-DOPRA-([\w._-]+)\n| p/Dopra Linux sshd/ v/$2/ i/protocol $1/ o/Dopra Linux/ cpe:/o:huawei:dopra_linux/
match ssh m|^SSH-([\d.]+)-AtiSSH_([\w._-]+)\r\n| p/Allied Telesis sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-CrushFTPSSHD\r\n| p/CrushFTP sftpd/ i/protocol $1/ cpe:/a:crushftp:crushftp/
# Probably not version 5
match ssh m|^SSH-([\d.]+)-CrushFTPSSHD_5\r\n| p/CrushFTP sftpd/ i/protocol $1/ cpe:/a:crushftp:crushftp/
match ssh m|^SSH-([\d.]+)-srtSSHServer_([\w._-]+)\r\n| p/South River Titan sftpd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/a:southrivertech:titan_ftp_server:$2/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-WRQReflectionforSecureIT_([\w._-]+) Build (\d+)\r\n| p/Attachmate Reflection for Secure IT sshd/ v/$2/ i/Build $3; protocol $1/ cpe:/a:attachmate:reflection_for_secure_it:$2/
match ssh m|^SSH-([\d.]+)-Maverick_SSHD\r\n| p/Maverick sshd/ i/protocol $1/ cpe:/a:sshtools:maverick_sshd/
match ssh m|^SSH-([\d.]+)-WingFTPserver\r\n| p/Wing FTP Server sftpd/ i/protocol $1/ cpe:/a:wingftp:wing_ftp_server/
match ssh m|^SSH-([\d.]+)-mod_sftp/([\w._-]+)\r\n| p/ProFTPD mod_sftp/ v/$2/ i/protocol $1/ cpe:/a:proftpd:proftpd:$2/
match ssh m|^SSH-1\.99--\n| p/Huawei VRP sshd/ i/protocol 1.99/ o/VRP/ cpe:/o:huawei:vrp/
# name is not hostname, but configurable service name
match ssh m|^SSH-([\d.]+)-SSH Server - ([^\r\n]+)\r\n\0\0...\x14|s p/Ice Cold Apps SSH Server (com.icecoldapps.sshserver)/ i/protocol $1; name: $2/ o/Android/ cpe:/a:ice_cold_apps:ssh_server/ cpe:/o:google:android/a cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-([\d.]+)-SSH Server - sshd\r\n| p/SSHelper sshd (com.arachnoid.sshelper)/ i/protocol $1/ o/Android/ cpe:/a:arachnoid:sshelper/ cpe:/o:google:android/a cpe:/o:linux:linux_kernel/a
match ssh m|^SSH-([\d.]+)-ConfD-([\w._-]+)\r\n| p/ConfD sshd/ v/$2/ i/protocol $1/ cpe:/a:tail-f:confd:$2/
match ssh m|^SSH-([\d.]+)-SERVER_([\d.]+)\r\n| p/FoxGate switch sshd/ v/$2/ i/protocol $1/
match ssh m|^SSH-2\.0-Server\r\n| p/AirTight WIPS sensor sshd/ i/protocol 2.0/
match ssh m|^SSH-([\d.]+)-EchoSystem_Server_([\w._-]+)\r\n| p/EchoSystem sshd/ v/$2/ i/protocol $1/ cpe:/a:echo360:echosystem:$2/
match ssh m|^SSH-([\d.]+)-FileCOPA\r\n| p/FileCOPA sftpd/ i/protocol $1/ o/Windows/ cpe:/a:intervations:filecopa/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-PSFTPd\. Secure FTP Server ready\r\n| p/PSFTPd/ i/protocol $1/ o/Windows/ cpe:/a:pleis:psftpd/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-NA_([\d.]+)\r\n| p/HP Network Automation/ v/$2/ i/protocol $1/ cpe:/a:hp:network_automation:$2/
match ssh m|^SSH-([\d.]+)-Comware-([\d.]+)\r?\n| p/HP Comware switch sshd/ v/$2/ i/protocol $1/ o/Comware/ cpe:/o:hp:comware:$2/
match ssh m|^SSH-([\d.]+)-SecureLink SSH Server \(Version ([\d.]+)\)\r\n| p/SecureLink sshd/ v/$2/ i/protocol $1/ cpe:/a:securelink:securelink:$2/
match ssh m|^SSH-([\d.]+)-WeOnlyDo-WingFTP\r\n| p/WingFTP sftpd/ i/protocol $1/ cpe:/a:wftpserver:wing_ftp_server/
match ssh m|^SSH-([\d.]+)-MS_(\d+\.\d\d\d)\r\n| p/Microsoft Windows IoT sshd/ v/$2/ i/protocol $1/ o/Windows 10 IoT Core/ cpe:/o:microsoft:windows_10:::iot_core/
match ssh m|^SSH-([\d.]+)-elastic-sshd\n| p/Elastic Hosts emergency SSH console/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-ZTE_SSH\.([\d.]+)\n| p|ZTE router/switch sshd| v/$2/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-SilverSHielD\r\n| p/SilverSHielD sshd/ i/protocol $1/ o/Windows/ cpe:/a:extenua:silvershield/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-XFB\.Gateway ([UW]\w+)\n| p/Axway File Broker (XFB) sshd/ i/protocol $1/ o/$2/ cpe:/a:axway:file_broker/
match ssh m|^SSH-([\d.]+)-CompleteFTP[-_]([\d.]+)\r\n| p/CompleteFTP sftpd/ v/$2/ i/protocol $1/ o/Windows/ cpe:/a:enterprisedt:completeftp:$2/ cpe:/o:microsoft:windows/a
match ssh m|^SSH-([\d.]+)-moxa_([\d.]+)\r\n| p/Moxa sshd/ v/$2/ i/protocol $1/ d/specialized/
match ssh m|^SSH-([\d.]+)-OneSSH_([\w.]+)\n| p/OneAccess OneSSH/ v/$2/ i/protocol $1/ cpe:/a:oneaccess:onessh:$1/
match ssh m|^SSH-([\d.]+)-AsyncSSH_(\d[\w.-]+)\r\n| p/AsyncSSH sshd/ v/$2/ i/protocol $1/ cpe:/a:ron_frederick:asyncssh:$2/
match ssh m|^SSH-([\d.]+)-ipage FTP Server Ready\r\n| p/iPage Hosting sftpd/ i/protocol $1/
match ssh m|^SSH-([\d.]+)-ArrayOS\n| p/Array Networks sshd/ i/protocol $1/ o/ArrayOS/ cpe:/o:arraynetworks:arrayos/
match ssh m|^SSH-([\d.]+)-SC123/SC143 CHIP-RTOS V([\d.]+)\r\n| p/Dropbear sshd/ i/protocol $1/ o/IPC@CHIP-RTOS $2/ cpe:/o:beck-ipc:chip-rtos:$2/ cpe:/a:matt_johnston:dropbear_ssh_server/
match ssh m|^SSH-([\d.]+)-Syncplify\.me\r\n| p/Syncplify.me Server sftpd/ i/protocol $1/ cpe:/a:syncplify:syncplify.me_server/
# Always 0.48 with static key. Dropbear, maybe?
match ssh m|^SSH-([\d.]+)-SSH_(\d[\d.]+)\r\n| p/ZyXEL embedded sshd/ v/$2/ i/protocol $1/ d/broadband router/
match ssh m|^SSH-([\d.]+)-TECHNICOLOR_SW_([\d.]+)\n| p/Technicolor SA sshd/ v/$2/ i/protocol $1/ d/broadband router/

# FortiSSH uses random server name - match an appropriate length, then check for 3 dissimilar character classes in a row.
# Does not catch everything, but ought to be pretty good.
match ssh m%^SSH-([\d.]+)-(?=[\w._-]{5,15}\n$).*(?:[a-z](?:[A-Z]\d|\d[A-Z])|[A-Z](?:[a-z]\d|\d[a-z])|\d(?:[a-z][A-Z]|[A-Z][a-z]))% p/FortiSSH/ i/protocol $1/ cpe:/o:fortinet:fortios/
`)
)

func TestParseNmapServiceProbes(t *testing.T) {
	testRaw := []byte(`SSH-1.1-TECHNICOLOR_SW_1.1
`)
	testPassed := false
	for _, rule := range ParseNmapServiceMatchedRule(TestRaw) {
		if rule.Matched.Match(testRaw) {
			testPassed = true
		}
	}

	if !testPassed {
		t.Fail()
		return
	}

	testProbe30 := []byte(`
##############################NEXT PROBE##############################
# Citrix MetaFrame application discovery service
# http://sh0dan.org/oldfiles/hackingcitrix.html
Probe UDP Citrix q|\x1e\0\x01\x30\x02\xfd\xa8\xe3\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0|
rarity 5
ports 1604

# Citrix MetaFrame
match icabrowser m|^\x30\0\x02\x31\x02\xfd\xa8\xe3\x02\0\x06\x44| p/Citrix MetaFrame/ cpe:/a:citrix:metaframe/

match ntp m|^\x1e\xc0\x010\x02\0\xa8\xe3\0\0\0\0$| p/Digium Switchvox PBX ntpd/ d/PBX/

match openvpn m|^\.\x83&SU\xe3_\xd5V\x01\0\0\0\0\0\x010\x02\xfd\xa8\xe3\0| p/SoftEther VPN OpenVPN Clone Function/
`)
	_f := false
	for _, rule := range ParseNmapServiceProbeRule(testProbe30) {
		_f = true
		fmt.Printf("Payload len: %d : %#v\n", len(rule.Payload), string(rule.Payload))
		if len(rule.Payload) != 30 {
			t.Fail()
			return
		}
	}

	if !_f {
		log.Error("parse Probes failed")
		t.Fail()
		return
	}

}
