###############################################################################
# OpenVAS Include File
# $Id: gather-package-list-docker-container.inc 14252 2019-03-18 09:01:25Z cfischer $
#
# Functions for OS Detection in Docker Containers
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2017 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
#
# $Revision: 14252 $
#
###############################################################################

OS_CPE = make_array(

    # OpenSUSE
    "openSUSELeap15.0", "cpe:/o:opensuse:leap:15.0",
    "openSUSELeap42.3", "cpe:/o:opensuse:leap:42.3",
    "openSUSELeap42.2", "cpe:/o:opensuse:leap:42.2",
    "openSUSELeap42.1", "cpe:/o:opensuse:leap:42.1",
    "openSUSELeap42.0", "cpe:/o:opensuse:leap:42.0",
    "openSUSE13.2", "cpe:/o:novell:opensuse:13.2",
    "openSUSE13.1", "cpe:/o:novell:opensuse:13.1",
    "openSUSE12.3", "cpe:/o:novell:opensuse:12.3",
    "openSUSE12.2", "cpe:/o:novell:opensuse:12.2",
    "openSUSE12.1", "cpe:/o:novell:opensuse:12.1",
    "openSUSE11.4", "cpe:/o:novell:opensuse:11.4",
    "openSUSE11.3", "cpe:/o:novell:opensuse:11.3",
    "openSUSE11.2", "cpe:/o:novell:opensuse:11.2",
    "openSUSE11.1", "cpe:/o:novell:opensuse:11.1",
    "openSUSE11.0", "cpe:/o:novell:opensuse:11.0",
    "openSUSE10.3", "cpe:/o:novell:opensuse:10.3",
    "openSUSE10.2", "cpe:/o:novell:opensuse:10.2",

    # SUSE Linux
    "SUSE10.1", "cpe:/o:novell:suse_linux:10.1",
    "SUSE10.0", "cpe:/o:novell:suse_linux:10.0",
    "SUSE9.3",  "cpe:/o:novell:suse_linux:9.3",
    "SUSE9.2",  "cpe:/o:novell:suse_linux:9.2",
    "SUSE9.1",  "cpe:/o:novell:suse_linux:9.1",
    "SUSE9.0",  "cpe:/o:novell:suse_linux:9.0",
    "SUSE8.2",  "cpe:/o:novell:suse_linux:8.2",
    "SUSE8.1",  "cpe:/o:novell:suse_linux:8.1",
    "SUSE8.0",  "cpe:/o:novell:suse_linux:8.0",
    "SUSE7.3",  "cpe:/o:novell:suse_linux:7.3",

    # Univention Corporate Server (http://wiki.univention.de/index.php?title=Maintenance_Cycle_for_UCS)
    "UCS4.2", "cpe:/o:univention:univention_corporate_server:4.2",
    "UCS4.1", "cpe:/o:univention:univention_corporate_server:4.1",
    "UCS4.0", "cpe:/o:univention:univention_corporate_server:4.0",
    "UCS3.3", "cpe:/o:univention:univention_corporate_server:3.3",
    "UCS3.2", "cpe:/o:univention:univention_corporate_server:3.2",
    "UCS3.1", "cpe:/o:univention:univention_corporate_server:3.1",
    "UCS3.0", "cpe:/o:univention:univention_corporate_server:3.0",
    "UCS2.4", "cpe:/o:univention:univention_corporate_server:2.4",
    "UCS2.3", "cpe:/o:univention:univention_corporate_server:2.3",
    "UCS2.2", "cpe:/o:univention:univention_corporate_server:2.2",
    "UCS2.1", "cpe:/o:univention:univention_corporate_server:2.1",
    "UCS2.0", "cpe:/o:univention:univention_corporate_server:2.0",

    # Ubuntu
    "UBUNTU19.04",    "cpe:/o:canonical:ubuntu_linux:19.04",
    "UBUNTU18.10",    "cpe:/o:canonical:ubuntu_linux:18.10",
    "UBUNTU18.04 LTS","cpe:/o:canonical:ubuntu_linux:18.04:-:lts",
    "UBUNTU17.10",    "cpe:/o:canonical:ubuntu_linux:17.10",
    "UBUNTU17.04",    "cpe:/o:canonical:ubuntu_linux:17.04",
    "UBUNTU16.10",    "cpe:/o:canonical:ubuntu_linux:16.10",
    "UBUNTU16.04 LTS","cpe:/o:canonical:ubuntu_linux:16.04:-:lts",
    "UBUNTU15.10",    "cpe:/o:canonical:ubuntu_linux:15.10",
    "UBUNTU15.04",    "cpe:/o:canonical:ubuntu_linux:15.04",
    "UBUNTU14.10",    "cpe:/o:canonical:ubuntu_linux:14.10",
    "UBUNTU14.04 LTS","cpe:/o:canonical:ubuntu_linux:14.04:-:lts",
    "UBUNTU13.10",    "cpe:/o:canonical:ubuntu_linux:13.10",
    "UBUNTU13.04",    "cpe:/o:canonical:ubuntu_linux:13.04",
    "UBUNTU12.10",    "cpe:/o:canonical:ubuntu_linux:12.10",
    "UBUNTU12.04 LTS","cpe:/o:canonical:ubuntu_linux:12.04",
    "UBUNTU11.10",    "cpe:/o:canonical:ubuntu_linux:11.10",
    "UBUNTU11.04",    "cpe:/o:canonical:ubuntu_linux:11.04",
    "UBUNTU10.10",    "cpe:/o:canonical:ubuntu_linux:10.10",
    "UBUNTU10.04 LTS","cpe:/o:canonical:ubuntu_linux:10.04:-:lts",
    "UBUNTU9.10",     "cpe:/o:canonical:ubuntu_linux:9.10",
    "UBUNTU9.04",     "cpe:/o:canonical:ubuntu_linux:9.04",
    "UBUNTU8.10",     "cpe:/o:canonical:ubuntu_linux:8.10",
    "UBUNTU8.04 LTS", "cpe:/o:canonical:ubuntu_linux:8.04:-:lts",
    "UBUNTU7.10",     "cpe:/o:canonical:ubuntu_linux:7.10",
    "UBUNTU7.04",     "cpe:/o:canonical:ubuntu_linux:7.04",
    "UBUNTU6.10",     "cpe:/o:canonical:ubuntu_linux:6.10",
    "UBUNTU6.06 LTS", "cpe:/o:canonical:ubuntu_linux:6.06:-:lts",
    "UBUNTU5.10",     "cpe:/o:canonical:ubuntu_linux:5.10",
    "UBUNTU5.04",     "cpe:/o:canonical:ubuntu_linux:5.04",
    "UBUNTU4.1",      "cpe:/o:canonical:ubuntu_linux:4.10",

    # Redhat Linux
    "RH9",    "cpe:/o:redhat:linux:9",
    "RH8.0",  "cpe:/o:redhat:linux:8.0",
    "RH7.3",  "cpe:/o:redhat:linux:7.3",

    # CentOS
    "CentOS7", "cpe:/o:centos:centos:7",
    "CentOS6", "cpe:/o:centos:centos:6",
    "CentOS5", "cpe:/o:centos:centos:5",
    "CentOS4", "cpe:/o:centos:centos:4",
    "CentOS3", "cpe:/o:centos:centos:3",
    "CentOS2", "cpe:/o:centos:centos:2",

    # Fedora / Fedora Core
    "FC28", "cpe:/o:fedoraproject:fedora:28",
    "FC27", "cpe:/o:fedoraproject:fedora:27",
    "FC26", "cpe:/o:fedoraproject:fedora:26",
    "FC25", "cpe:/o:fedoraproject:fedora:25",
    "FC24", "cpe:/o:fedoraproject:fedora:24",
    "FC23", "cpe:/o:fedoraproject:fedora:23",
    "FC22", "cpe:/o:fedoraproject:fedora:22",
    "FC21", "cpe:/o:fedoraproject:fedora:21",
    "FC20", "cpe:/o:fedoraproject:fedora:20",
    "FC19", "cpe:/o:fedoraproject:fedora:19",
    "FC18", "cpe:/o:fedoraproject:fedora:18",
    "FC17", "cpe:/o:fedoraproject:fedora:17",
    "FC16", "cpe:/o:fedoraproject:fedora:16",
    "FC15", "cpe:/o:fedoraproject:fedora:15",
    "FC14", "cpe:/o:fedoraproject:fedora:14",
    "FC13", "cpe:/o:fedoraproject:fedora:13",
    "FC12", "cpe:/o:fedoraproject:fedora:12",
    "FC11", "cpe:/o:fedoraproject:fedora:11",
    "FC10", "cpe:/o:fedoraproject:fedora:10",
    "FC9",  "cpe:/o:fedoraproject:fedora:9",
    "FC8",  "cpe:/o:fedoraproject:fedora:8",
    "FC7",  "cpe:/o:fedoraproject:fedora:7",
    "FC6",  "cpe:/o:fedoraproject:fedora_core:6",
    "FC5",  "cpe:/o:fedoraproject:fedora_core:5",
    "FC4",  "cpe:/o:fedoraproject:fedora_core:4",
    "FC3",  "cpe:/o:fedoraproject:fedora_core:3",
    "FC2",  "cpe:/o:fedoraproject:fedora_core:2",
    "FC1",  "cpe:/o:fedoraproject:fedora_core:1",

    # Mandriva
    "MNDK_2011.0",  "cpe:/o:mandriva:linux:2011.0",
    "MNDK_2010.2",  "cpe:/o:mandriva:linux:2010.2",
    "MNDK_2010.1",  "cpe:/o:mandriva:linux:2010.1",
    "MNDK_2010.0",  "cpe:/o:mandriva:linux:2010.0",
    "MNDK_2009.1",  "cpe:/o:mandriva:linux:2009.1",
    "MNDK_2009.0",  "cpe:/o:mandriva:linux:2009.0",
    "MNDK_2008.1",  "cpe:/o:mandriva:linux:2008.1",
    "MNDK_2008.0",  "cpe:/o:mandriva:linux:2008.0",
    "MNDK_2007.1",  "cpe:/o:mandriva:linux:2007.1",
    "MNDK_2007.0",  "cpe:/o:mandriva:linux:2007.0",
    "MNDK_2006.0",  "cpe:/o:mandriva:linux:2006.0",
    "MNDK_mes5.2",  "cpe:/o:mandriva:enterprise_server:5.2",
    "MNDK_mes5.1",  "cpe:/o:mandriva:enterprise_server:5.1",
    "MNDK_mes5.0",  "cpe:/o:mandriva:enterprise_server:5.0", # Keep both for backward compatibility
    "MNDK_mes5",    "cpe:/o:mandriva:enterprise_server:5", # Keep both for backward compatibility

    # Mandrake
    "MNDK_10.1",    "cpe:/o:mandrakesoft:mandrake_linux:10.1",
    "MNDK_10.0",    "cpe:/o:mandrakesoft:mandrake_linux:10.0",
    "MNDK_9.2",     "cpe:/o:mandrakesoft:mandrake_linux:9.2",
    "MNDK_9.1",     "cpe:/o:mandrakesoft:mandrake_linux:9.1",
    "MNDK_9.0",     "cpe:/o:mandrakesoft:mandrake_linux:9.0",
    "MNDK_8.2",     "cpe:/o:mandrakesoft:mandrake_linux:8.2",
    "MNDK_8.1",     "cpe:/o:mandrakesoft:mandrake_linux:8.1",
    "MNDK_8.0",     "cpe:/o:mandrakesoft:mandrake_linux:8.0",
    "MNDK_7.2",     "cpe:/o:mandrakesoft:mandrake_linux:7.2",

    #Mageia
    "MAGEIA6",     "cpe:/o:mageia:linux:6",
    "MAGEIA5",     "cpe:/o:mageia:linux:5",
    "MAGEIA4.1",   "cpe:/o:mageia:linux:4.1",
    "MAGEIA4",     "cpe:/o:mageia:linux:4",
    "MAGEIA3",     "cpe:/o:mageia:linux:3",
    "MAGEIA2",     "cpe:/o:mageia:linux:2",
    "MAGEIA1",     "cpe:/o:mageia:linux:1",

    # Slackware
    "SLK14.2", "cpe:/o:slackware:slackware_linux:14.2",
    "SLK14.1", "cpe:/o:slackware:slackware_linux:14.1",
    "SLK14.0", "cpe:/o:slackware:slackware_linux:14.0",
    "SLK13.37", "cpe:/o:slackware:slackware_linux:13.37",
    "SLK13.1", "cpe:/o:slackware:slackware_linux:13.1",
    "SLK13.0", "cpe:/o:slackware:slackware_linux:13.0",
    "SLK12.2", "cpe:/o:slackware:slackware_linux:12.2",
    "SLK12.1", "cpe:/o:slackware:slackware_linux:12.1",
    "SLK12.0", "cpe:/o:slackware:slackware_linux:12.0",
    "SLK11.0", "cpe:/o:slackware:slackware_linux:11.0",
    "SLK10.2", "cpe:/o:slackware:slackware_linux:10.2",
    "SLK10.1", "cpe:/o:slackware:slackware_linux:10.1",
    "SLK10.0", "cpe:/o:slackware:slackware_linux:10.0",
    "SLK9.1",  "cpe:/o:slackware:slackware_linux:9.1",
    "SLK9.0",  "cpe:/o:slackware:slackware_linux:9.0",
    "SLK8.1",  "cpe:/o:slackware:slackware_linux:8.1",
    "SLK8.0",  "cpe:/o:slackware:slackware_linux:8.0",
    "SLK7.1",  "cpe:/o:slackware:slackware_linux:7.1",
    "SLK7.0",  "cpe:/o:slackware:slackware_linux:7.0",
    "SLK4.0",  "cpe:/o:slackware:slackware_linux:4.0",
    "SLK3.9",  "cpe:/o:slackware:slackware_linux:3.9",
    "SLK3.6",  "cpe:/o:slackware:slackware_linux:3.6",
    "SLK3.5",  "cpe:/o:slackware:slackware_linux:3.5",
    "SLK3.4",  "cpe:/o:slackware:slackware_linux:3.4",
    "SLK3.3",  "cpe:/o:slackware:slackware_linux:3.3",
    "SLK3.2",  "cpe:/o:slackware:slackware_linux:3.2",
    "SLK3.1",  "cpe:/o:slackware:slackware_linux:3.1",
    "SLK3.0",  "cpe:/o:slackware:slackware_linux:3.0",
    "SLK2.3",  "cpe:/o:slackware:slackware_linux:2.3",
    "SLK2.2",  "cpe:/o:slackware:slackware_linux:2.2",
    "SLK2.1",  "cpe:/o:slackware:slackware_linux:2.1",
    "SLK2.0",  "cpe:/o:slackware:slackware_linux:2.0",
    "SLK1.1",  "cpe:/o:slackware:slackware_linux:1.1",
    "SLK1.00",  "cpe:/o:slackware:slackware_linux:1.00",

    # Amazon Linux
    "AMAZON",     "cpe:/o:amazon:linux",

    # Oracle Linux
    "OracleLinux7.4",  "cpe:/o:oracle:linux:7.4",
    "OracleLinux7.3",  "cpe:/o:oracle:linux:7.3",
    "OracleLinux7.2",  "cpe:/o:oracle:linux:7.2",
    "OracleLinux7.1",  "cpe:/o:oracle:linux:7.1",
    "OracleLinux7.0",  "cpe:/o:oracle:linux:7.0",
    "OracleLinux7",    "cpe:/o:oracle:linux:7",
    "OracleLinux6.9",  "cpe:/o:oracle:linux:6.9",
    "OracleLinux6.8",  "cpe:/o:oracle:linux:6.8",
    "OracleLinux6.7",  "cpe:/o:oracle:linux:6.7",
    "OracleLinux6.6",  "cpe:/o:oracle:linux:6.6",
    "OracleLinux6.5",  "cpe:/o:oracle:linux:6.5",
    "OracleLinux6.4",  "cpe:/o:oracle:linux:6.4",
    "OracleLinux6.3",  "cpe:/o:oracle:linux:6.3",
    "OracleLinux6.2",  "cpe:/o:oracle:linux:6.2",
    "OracleLinux6.1",  "cpe:/o:oracle:linux:6.1",
    "OracleLinux6.0",  "cpe:/o:oracle:linux:6.0",
    "OracleLinux6",    "cpe:/o:oracle:linux:6",
    "OracleLinux5.11", "cpe:/o:oracle:linux:5.11",
    "OracleLinux5.10", "cpe:/o:oracle:linux:5.10",
    "OracleLinux5.9",  "cpe:/o:oracle:linux:5.9",
    "OracleLinux5.8",  "cpe:/o:oracle:linux:5.8",
    "OracleLinux5.7",  "cpe:/o:oracle:linux:5.7",
    "OracleLinux5.6",  "cpe:/o:oracle:linux:5.6",
    "OracleLinux5.5",  "cpe:/o:oracle:linux:5.5",
    "OracleLinux5.4",  "cpe:/o:oracle:linux:5.4",
    "OracleLinux5.3",  "cpe:/o:oracle:linux:5.3",
    "OracleLinux5.2",  "cpe:/o:oracle:linux:5.2",
    "OracleLinux5.1",  "cpe:/o:oracle:linux:5.1",
    "OracleLinux5.0",  "cpe:/o:oracle:linux:5.0",
    "OracleLinux5",    "cpe:/o:oracle:linux:5",
    "OracleLinux4.9",  "cpe:/o:oracle:linux:4.9",
    "OracleLinux4.8",  "cpe:/o:oracle:linux:4.8",
    "OracleLinux4.7",  "cpe:/o:oracle:linux:4.7",
    "OracleLinux4.6",  "cpe:/o:oracle:linux:4.6",
    "OracleLinux4.5",  "cpe:/o:oracle:linux:4.5",
    "OracleLinux4",    "cpe:/o:oracle:linux:4",

    # Gentoo
    "GENTOO", "cpe:/o:gentoo:linux",

    # Arch Linux
    # nb: Arch Linux is a rolling release so there is no "real" version
    "ArchLinux", "cpe:/o:archlinux:archlinux",

    # NixOS
    # nb: NixOS has version strings like the following:
    # VERSION_ID="18.09pre145524.2a8a5533d18"
    # For now just register the CPE without the version...
    "NixOS", "cpe:/o:nixos_project:nixos"
);

function register_detected_docker_os(container_id, os, oskey, oscpe) {

  local_var container_id, os, oskey, oscpe;
  local_var cpe;

  if(!container_id || !os || !oskey || (!OS_CPE[oskey] && !oscpe))
    return;

  if(oscpe)
    cpe = oscpe;
  else
    cpe = OS_CPE[oskey];

  set_kb_item(name:"docker/lsc/container/" + container_id + "/OS", value:os);
  set_kb_item(name:"docker/lsc/container/" + container_id + "/OS-CPE", value:cpe);

  return make_array("OS", chomp(os), "OS-CPE", cpe);
}

function get_container_os(container_id, soc) {

  local_var container_id, soc;
  local_var bf, base_cmd, rls;

  if(! soc || ! container_id)
    return;

  base_cmd = "docker exec " + container_id + " ";

  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/system-release");

  if("Amazon Linux AMI release" >< rls) {
    buf = ssh_cmd( socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'" );
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id,  os:"Amazon Linux", oskey:"AMAZON");
    return ret;
  }

  # oraclelinux is almost like rhel .. but ..
  rls = ssh_cmd(socket:soc, cmd:base_cmd + "rpm -qf /etc/redhat-release");

  if("oraclelinux-release-4" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    if(match = eregmatch(pattern:"oraclelinux-release-4.([0-9]+)", string:rls)) {
      version = "4." + match[1];
    } else {
      version = "4";
    }
    ret = register_detected_docker_os(container_id:container_id, os:"Oracle Linux " + version, oskey:"OracleLinux" + version);
    return ret;
  }
  if("oraclelinux-release-5" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    if(match = eregmatch(pattern:"oraclelinux-release-5.([0-9]+)", string:rls)) {
      version = "5." + match[1];
    } else {
      version = "5";
    }
    ret = register_detected_docker_os(container_id:container_id, os:"Oracle Linux " + version, oskey:"OracleLinux" + version);
    return ret;
  }
  if("oraclelinux-release-6" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    if(match = eregmatch(pattern:"oraclelinux-release-6.([0-9]+)", string:rls)) {
      version = "6." + match[1];
    } else {
      version = "6";
    }
    ret = register_detected_docker_os(container_id:container_id, os:"Oracle Linux " + version, oskey:"OracleLinux" + version);
    return ret;
  }
  if("oraclelinux-release-7" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    if(match = eregmatch(pattern:"oraclelinux-release-7.([0-9]+)", string:rls)) {
      version = "7." + match[1];
    } else {
      version = "7";
    }
    ret = register_detected_docker_os(container_id:container_id, os:"Oracle Linux " + version, oskey:"OracleLinux" + version);
    return ret;
  }

  # Ok...let's first check if this is a RedHat/Fedora Core/Mandrake release
  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/redhat-release");

  if("Red Hat Linux release 7.3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"RH7.3");
    return ret;
  }
  if("Red Hat Linux release 8.0 (Psyche)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"RH8.0");
    return ret;
  }
  if("Red Hat Linux release 9 (Shrike)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"RH9");
    return ret;
  }

  if("Fedora Core release 1 (Yarrow)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
     if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC1");
    return ret;
  }
  if("Fedora Core release 2 (Tettnang)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC2");
    return ret;
  }
  if("Fedora Core release 3 (Heidelberg)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC3");
    return ret;
  }
  if("Fedora Core release 4 (Stentz)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC4");
    return ret;
  }
  if("Fedora Core release 5 (Bordeaux)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC5");
    return ret;
  }
  if("Fedora Core release 6 (Zod)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC6");
    return ret;
  }
  if("Fedora release 7 (Moonshine)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC7");
    return ret;
  }
  if("Fedora release 8 (Werewolf)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC8");
    return ret;
  }
  if("Fedora release 9 (Sulphur)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC9");
    return ret;
  }
  if("Fedora release 10 (Cambridge)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC10");
    return ret;
  }
  if("Fedora release 11 (Leonidas)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC11");
    return ret;
  }
  if("Fedora release 12 (Constantine)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC12");
    return ret;
  }
  if("Fedora release 13 (Goddard)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC13");
    return ret;
  }
  if("Fedora release 14 (Laughlin)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC14");
    return ret;
  }
  if("Fedora release 15 (Lovelock)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC15");
    return ret;
  }
  if("Fedora release 16 (Verne)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC16");
    return ret;
  }
  if("Fedora release 17 (Beefy Miracle)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC17");
    return ret;
  }
  if("Fedora release 18 (Spherical Cow)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC18");
    return ret;
  }
  if("Fedora release 19" >< rls && "Cat" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC19");
    return ret;
  }
  if("Fedora release 20" >< rls && "(Heisenbug)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC20");
    return ret;
  }
  if("Fedora release 21" >< rls && "(Twenty One)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC21");
    return ret;
  }
  if("Fedora release 22" >< rls && "(Twenty Two)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC22");
    return ret;
  }
  if("Fedora release 23" >< rls && "(Twenty Three)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC23");
    return ret;
  }
  if("Fedora release 24" >< rls && "(Twenty Four)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC24");
    return ret;
  }
  if("Fedora release 25" >< rls && "(Twenty Five)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC25");
    return ret;
  }
  if("Fedora release 26" >< rls && "(Twenty Six)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC26");
    return ret;
  }
  if("Fedora release 27" >< rls && "(Twenty Seven)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC27");
    return ret;
  }
  if("Fedora release 28" >< rls && "(Twenty Eight)" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"FC28");
    return ret;
  }

  if("Mandriva Linux Enterprise Server release 5.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_mes5");
    return ret;
  }
  if("Mandriva Linux Enterprise Server release 5.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_mes5.1");
    return ret;
  }
  if("Mandriva Linux Enterprise Server release 5.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_mes5.2");
    return ret;
  }

  if("Mandriva Linux release 2011.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2011.0");
    return ret;
  }
  if("Mandriva Linux release 2010.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2010.2");
    return ret;
  }
  if("Mandriva Linux release 2010.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2010.1");
    return ret;
  }
  if("Mandriva Linux release 2010.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2010.0");
    return ret;
  }
  if("Mandriva Linux release 2009.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2009.1");
    return ret;
  }
  if("Mandriva Linux release 2009.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2009.0");
    return ret;
  }
  if("Mandriva Linux release 2008.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2008.1");
    return ret;
  }
  if("Mandriva Linux release 2008.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2008.0");
    return ret;
  }
  if("Mandriva Linux release 2007.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2007.1");
  }
  if("Mandriva Linux release 2007.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2007.0");
    return ret;
  }
  if("Mandriva Linux release 2006.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_2006.0");
    return ret;
  }

  if("Mandrakelinux release 10.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_10.1");
    return ret;
  }
  if("Mandrake Linux release 10.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_10.0");
    return ret;
  }
  if("Mandrake Linux release 9.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_9.2");
    return ret;
  }
  if("Mandrake Linux release 9.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_9.1");
    return ret;
  }
  if("Mandrake Linux release 9.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_9.0");
    return ret;
  }
  if("Mandrake Linux release 8.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_8.2");
    return ret;
  }
  if("Mandrake Linux release 8.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_8.1");
    return ret;
  }
  if("Mandrake Linux release 8.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_8.0");
    return ret;
  }
  if("Mandrake Linux release 7.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:"MNDK_7.2");
    return ret;
  }

  if("Mageia release 6" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 6", oskey:"MAGEIA6");
    return ret;
  }
  if("Mageia release 5" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 5", oskey:"MAGEIA5");
    return ret;
  }
  if("Mageia release 4.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 4.1", oskey:"MAGEIA4.1");
    return ret;
  }
  if("Mageia release 4" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 4", oskey:"MAGEIA4");
    return ret;
  }
  if("Mageia release 3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 3", oskey:"MAGEIA3");
    return ret;
  }
  if("Mageia release 2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 2", oskey:"MAGEIA2");
    return ret;
  }
  if("Mageia release 1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Mageia release 1", oskey:"MAGEIA1");
    return ret;
  }

  # Ok...also using /etc/redhat-release is CentOS...let's try them now
  # We'll stay with major release # checking unless we find out we need to do
  # otherwise.
  #CentOS release 4.0 (Final)
  #CentOS release 4.1 (Final)
  #CentOS release 3.4 (final)

  if("CentOS Linux release 7" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 7", oskey:"CentOS7");
    return ret;
  }
  if("CentOS release 6" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 6", oskey:"CentOS6");
    return ret;
  }
  if("CentOS release 5" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 5", oskey:"CentOS5");
    return ret;
  }
  if("CentOS release 4" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 4", oskey:"CentOS4");
    return ret;
  }
  if("CentOS release 3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 3", oskey:"CentOS3");
    return ret;
  }
  if("CentOS release 2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};\n'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"CentOS release 2", oskey:"CentOS2");
    return ret;
  }

  # Hmmm...is it Ubuntu?
  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/lsb-release");

  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=4.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 4.10", oskey:"UBUNTU4.1");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=5.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 5.04", oskey:"UBUNTU5.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=5.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 5.10", oskey:"UBUNTU5.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=6.06" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 6.06", oskey:"UBUNTU6.06 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=6.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 6.10", oskey:"UBUNTU6.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=7.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 7.04", oskey:"UBUNTU7.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=7.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 7.10", oskey:"UBUNTU7.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=8.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 8.04 LTS", oskey:"UBUNTU8.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=8.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if( buf )
      set_kb_item( name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 8.10", oskey:"UBUNTU8.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=9.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 9.04", oskey:"UBUNTU9.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=9.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 9.10", oskey:"UBUNTU9.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=10.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 10.04 LTS", oskey:"UBUNTU10.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=10.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 10.10", oskey:"UBUNTU10.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=11.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 11.04", oskey:"UBUNTU11.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=11.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 11.10", oskey:"UBUNTU11.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=12.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 12.04 LTS", oskey:"UBUNTU12.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=12.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 12.10", oskey:"UBUNTU12.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=13.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 13.04", oskey:"UBUNTU13.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=13.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 13.10", oskey:"UBUNTU13.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=14.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 14.04 LTS", oskey:"UBUNTU14.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=14.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 14.10", oskey:"UBUNTU14.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=15.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 15.04", oskey:"UBUNTU15.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=15.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 15.10", oskey:"UBUNTU15.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=16.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 16.04 LTS", oskey:"UBUNTU16.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=16.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 16.10", oskey:"UBUNTU16.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=17.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 17.04", oskey:"UBUNTU17.04");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=17.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 17.10", oskey:"UBUNTU17.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=18.04" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 18.04 LTS", oskey:"UBUNTU18.04 LTS");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=18.10" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 18.10", oskey:"UBUNTU18.10");
    return ret;
  }
  if("DISTRIB_ID=Ubuntu"><rls && "DISTRIB_RELEASE=19.04" >< rls) {
    ## For Ubuntu19 dpkg -l lists first few lines and user needs to scroll for rest of data
    ## Packages collected in 'buf' variable include only these first few packages.
    ## Currently using dpkg -l|head -10000 to display 10000 lines
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l|head -10000'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Ubuntu 19.04", oskey:"UBUNTU19.04");
    return ret;
  }

  # Check for Univention Corporate Server (UCS)
  if(rls =~ 'DISTRIB_ID=("|\')?Univention("|\')?') {
    ucs_release = eregmatch(string:rls, pattern:'DISTRIB_RELEASE="([1-9][0-9]*[.][0-9]+)-([0-9]+) errata([0-9]+)[^"]*"');

    if(!isnull(ucs_release[1]))
      set_kb_item(name:"docker/lsc/container/" + container_id + "/ucs/version", value:ucs_release[1]);

    if(!isnull(ucs_release[2]))
      set_kb_item(name:"docker/lsc/container/" + container_id + "/ucs/patch", value:ucs_release[2]);

    if(!isnull(ucs_release[3]))
      set_kb_item(name:"docker/lsc/container/" + container_id + "/ucs/errata", value:ucs_release[3]);

    ucs_desciption = eregmatch(string:rls, pattern:'DISTRIB_DESCRIPTION="([^"]*)"');

    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'");

    if(!isnull(buf) && !isnull(ucs_release[1]) && !isnull(ucs_desciption[1])) {
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);
      ret = register_detected_docker_os(container_id:container_id, os:ucs_desciption[1], oskey:"UCS"+ucs_release[1]);
      return ret;
    }
  }

  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/debian_version");

  # nb: At least Ubuntu 18.10 has "buster/sid" in debian_version so keep this in mind
  # if Ubuntu is wrongly detected and keep the Ubuntu pattern above the Debian ones.
  if(rls =~ "^[0-9]+[0-9.]+" || "buster/sid" >< rls) {

    rls   = chomp(rls);
    cpe   = "cpe:/o:debian:debian_linux";
    oskey = "DEB";

    buf = ssh_cmd(socket:soc, cmd:base_cmd + "bash -c 'COLUMNS=400 dpkg -l'", nosh:TRUE);
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/packages", value:";" + buf);

    vers = eregmatch(pattern:"^([0-9]+)([0-9.]+)", string:rls, icase:FALSE);
    if(vers[1]) {
      cpe   += ":" + vers[1];
      oskey += vers[1]; # nb: We only want to save the "major" release like 6, 7 and so on in ssh/login/release...
    }

    if(vers[2]) {
      cpe += vers[2];
      if(vers[1] =~ "^[1-3]$")
        oskey += "." + vers[2]; # nb: but the older releases needs the second digit as well...
    }

    if(!vers && "buster/sid" >< rls) {
      cpe   += ":10.0";
      oskey += "10";
    }

    ret = register_detected_docker_os(container_id:container_id, os:"Debian " + rls, oskey:oskey, oscpe:cpe);
    return ret;
  }

  # How about Slackware?
  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/slackware-version");

  if("Slackware 14.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 14.2", oskey:"SLK14.2");
    return ret;
  }
  if("Slackware 14.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 14.1", oskey:"SLK14.1");
    return ret;
  }
  if("Slackware 14.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 14.0", oskey:"SLK14.0");
    return ret;
  }
  if("Slackware 13.37" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 13.37", oskey:"SLK13.37");
    return ret;
  }
  if("Slackware 13.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 13.1", oskey:"SLK13.1");
    return ret;
  }
  if("Slackware 13.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 13.0", oskey:"SLK13.0");
    return ret;
  }
  if("Slackware 12.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 12.2", oskey:"SLK12.2");
    return ret;
  }
  if("Slackware 12.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 12.1", oskey:"SLK12.1");
    return ret;
  }
  if("Slackware 12.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 12.0", oskey:"SLK12.0");
    return ret;
  }
  if("Slackware 11.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 11.0", oskey:"SLK11.0");
    return ret;
  }
  if("Slackware 10.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 10.2", oskey:"SLK10.2");
    return ret;
  }
  if("Slackware 10.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 10.1", oskey:"SLK10.1");
    return ret;
  }
  if("Slackware 10.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 10.0", oskey:"SLK10.0");
    return ret;
  }
  if("Slackware 9.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 9.1", oskey:"SLK9.1");
    return ret;
  }
  if("Slackware 9.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 9.0", oskey:"SLK9.0");
    return ret;
  }
  if("Slackware 8.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 8.1", oskey:"SLK8.1");
    return ret;
  }
  if("Slackware 8.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 8.0", oskey:"SLK8.0");
    return ret;
  }
  if("Slackware 7.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 7.1", oskey:"SLK7.1");
    return ret;
  }
  if("Slackware 7.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 7.0", oskey:"SLK7.0");
    return ret;
  }
  if("Slackware 4.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 4.0", oskey:"SLK4.0");
    return ret;
  }
  if("Slackware 3.9" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.9", oskey:"SLK3.9");
    return ret;
  }
  if("Slackware 3.6" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.6", oskey:"SLK3.6");
    return ret;
  }
  if("Slackware 3.5" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.5", oskey:"SLK3.5");
    return ret;
  }
  if("Slackware 3.4" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.4", oskey:"SLK3.4");
    return ret;
  }
  if("Slackware 3.3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.3", oskey:"SLK3.3");
    return ret;
  }
  if("Slackware 3.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.2", oskey:"SLK3.2");
    return ret;
  }
  if("Slackware 3.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.1", oskey:"SLK3.1");
    return ret;
  }
  if("Slackware 3.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 3.0", oskey:"SLK3.0");
    return ret;
  }
  if("Slackware 2.3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 2.3", oskey:"SLK2.3");
    return ret;
  }
  if("Slackware 2.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 2.2", oskey:"SLK2.2");
    return ret;
  }
  if("Slackware 2.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 2.1", oskey:"SLK2.1");
    return ret;
  }
  if("Slackware 2.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 2.0", oskey:"SLK2.0");
    return ret;
  }
  if("Slackware 1.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 1.1", oskey:"SLK1.1");
    return ret;
  }
  if("Slackware 1.00" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "ls /var/log/packages");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/slackpack", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Slackware 1.00", oskey:"SLK1.00");
    return ret;
  }

  # How about SuSe? and openSUSE?
  # https://en.wikipedia.org/wiki/SUSE_Linux_distributions
  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/os-release");

  if("openSUSE Leap 15.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE Leap 15.0", oskey:"openSUSELeap15.0");
    return ret;
  }
  if("openSUSE Leap 42.3" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE Leap 42.3", oskey:"openSUSELeap42.3");
    return ret;
  }
  if("openSUSE Leap 42.2" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE Leap 42.2", oskey:"openSUSELeap42.2");
    return ret;
  }
  if("openSUSE Leap 42.1" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE Leap 42.1", oskey:"openSUSELeap42.1");
    return ret;
  }
  if("openSUSE Leap 42.0" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE Leap 42.0", oskey:"openSUSELeap42.0");
    return ret;
  }

  # nb: Arch Linux is a rolling release so there is no "real" version
  if('NAME="Arch Linux"' >< rls) {
    ret = register_detected_docker_os(container_id:container_id, os:"Arch Linux", oskey:"ArchLinux");
    return ret;
  }

  if("NAME=NixOS" >< rls || "ID=nixos" >< rls) {
    ret = register_detected_docker_os(container_id:container_id, os:"NixOS", oskey:"NixOS");
    return ret;
  }

  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/SuSE-release");
  if("openSUSE 13.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 13.2", oskey:"openSUSE13.2");
    return ret;
  }
  if("openSUSE 13.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 13.1", oskey:"openSUSE13.1");
    return ret;
  }
  if("openSUSE 12.3 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 12.3", oskey:"openSUSE12.3");
    return ret;
  }
  if("openSUSE 12.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 12.2", oskey:"openSUSE12.2");
    return ret;
  }
  if("openSUSE 12.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 12.1", oskey:"openSUSE12.1");
    return ret;
  }
  if("openSUSE 11.4 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 11.4", oskey:"openSUSE11.4");
    return ret;
  }
  if("openSUSE 11.3 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 11.3", oskey:"openSUSE11.3");
    return ret;
  }
  if("openSUSE 11.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 11.2", oskey:"openSUSE11.2");
    return ret;
  }
  if("openSUSE 11.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 11.1", oskey:"openSUSE11.1");
    return ret;
  }
  if("openSUSE 11.0 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 11.0", oskey:"openSUSE11.0");
    return ret;
  }
  if("openSUSE 10.3 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 10.3", oskey:"openSUSE10.3");
    return ret;
  }
  if("openSUSE 10.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"openSUSE 10.2", oskey:"openSUSE10.2");
    return ret;
  }

  if("SUSE LINUX 10.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 10.1", oskey:"SUSE10.1");
    return ret;
  }
  if("SUSE LINUX 10.0 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 10.0", oskey:"SUSE10.0");
    return ret;
  }
  if("SuSE Linux 9.3 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 9.3", oskey:"SUSE9.3");
    return ret;
  }
  if("SuSE Linux 9.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 9.2", oskey:"SUSE9.2");
    return ret;
  }
  if("SuSE Linux 9.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 9.1", oskey:"SUSE9.1");
    return ret;
  }
  if("SuSE Linux 9.0 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 9.0", oskey:"SUSE9.0");
    return ret;
  }
  if("SuSE Linux 8.2 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 8.2", oskey:"SUSE8.2");
    return ret;
  }
  if("SuSE Linux 8.1 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 8.1", oskey:"SUSE8.1");
    return ret;
  }
  if("SuSE Linux 8.0 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 8.0", oskey:"SUSE8.0");
    return ret;
  }
  if("SuSE Linux 7.3 " >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);
    ret = register_detected_docker_os(container_id:container_id, os:"SuSE Linux 7.3", oskey:"SUSE7.3");
    return ret;
  }

  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/euleros-release");

  # EulerOS release 2.0
  # EulerOS release 2.0 (SP2)
  if(rls =~ "EulerOS release") {

    rls   = chomp(rls);
    cpe   = "cpe:/o:huawei:euleros";
    oskey = "EULEROS";

    buf = ssh_cmd(socket:soc, cmd:base_cmd + "/bin/rpm -qa --qf '%{NAME}~%{VERSION}~%{RELEASE};'");
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/rpms", value:";" + buf);

    vers_nd_sp = eregmatch(pattern:"^EulerOS release ([0-9]+\.[0-9]+)( \(SP([0-9]+)\))?", string:rls, icase:TRUE);
    if( vers_nd_sp[1] ) {

      cpe   += vers_nd_sp[1];
      oskey += vers_nd_sp[1];

      if(vers_nd_sp[3]) {
        cpe   += ":sp" + vers_nd_sp[3];
        oskey += "SP" + vers_nd_sp[3];
      } else {
        oskey += "SP0";
      }
    }

    ret = register_detected_docker_os(container_id:container_id, os:rls, oskey:oskey, oscpe:cpe);
    return ret;
  }

  rls = ssh_cmd(socket:soc, cmd:base_cmd + "cat /etc/gentoo-release");
  if("Gentoo" >< rls) {
    buf = ssh_cmd(socket:soc, cmd:base_cmd + 'find /var/db/pkg -mindepth 2 -maxdepth 2 -printf "%P\\n"');
    if(buf)
      set_kb_item(name:"docker/lsc/container/" + container_id + "/pkg", value:";" + buf);
    # Determine the list of maintained packages
    buf = ssh_cmd(socket:soc, cmd:base_cmd +  "find /usr/portage/ -wholename '/usr/portage/*-*/*.ebuild' | sed 's,/usr/portage/\([^/]*\)/.*/\([^/]*\)\.ebuild$,\1/\2,'");
    if(strlen(buf) == 0) { # Earlier find used 'path' in place of 'wholename'
      buf = ssh_cmd(socket:soc, cmd:base_cmd +  "find /usr/portage/ -path '/usr/portage/*-*/*.ebuild' | sed 's,/usr/portage/\([^/]*\)/.*/\([^/]*\)\.ebuild$,\1/\2,'");
    }
    set_kb_item( name:"docker/lsc/container/" + container_id + "/gentoo_maintained", value:buf);
    ret = register_detected_docker_os(container_id:container_id, os:"Gentoo", oskey:"GENTOO");
    return ret;
  }
}