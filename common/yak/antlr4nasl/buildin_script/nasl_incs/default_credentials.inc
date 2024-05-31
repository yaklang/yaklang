###############################################################################
# OpenVAS Include File
# $Id: default_credentials.inc 12710 2018-12-07 19:40:26Z cfischer $
#
# Default Credentials
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2009 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
#
# $Revision: 12710 $
#
###############################################################################

# user:pass:vendor(multiple vendors separated by ","):protocol(multiple protocols separated by ",")
# Current available options protocol: all,ssh,http,telnet
credentials = make_list(
"root:pass:axis:all",
"root:calvin:dell:all",
"root:password:korenix:all",
"MGR:REGO:hp:all",
"MAIL:TELESUP:hp:all",
"storwatch:specialist:ibm:all",
"admin:hp.com:hp:all",
"user:public:compaq:all",
"MGR:HPP196:hp:all",
"at4400:at4400:alcatel:all",
"FIELD:HPWORD PUB:hp:all",
"root:letacla:alcatel:all",
"HELLO:MGR.SYS:hp:all",
"mtch:mtch:alcatel:all",
"User:none:bay networks,dlink:all",
"device:device:apc:all",
"cisco:cisco:cisco:all",
"Administrator:admin:cisco,conexant,corecess,gvc:all",
"MANAGER:COGNOS:hp:all",
"MAIL:HPOFFICE:hp:all",
"admin:diamond:cisco:all",
"patrol:patrol:bmc:all",
"MAIL:MAIL:hp:all",
"admin:changeme:cisco:all",
"admin:default:cisco:all",
"dhs3pms:dhs3pms:alcatel:all",
"root:Mau'dib:celerity:all",
"login:password:efficient:all",
"PFCUser:240653C9467E45:compaq:all",
"Administrator:letmein:integral:all",
"davox:davox:davox:all",
"debug:synnet:3com:all",
"MANAGER:SECURITY:hp:all",
"FIELD:SERVICE:hp:all",
"sa:none:cisco,davox:all",
"Cisco:Cisco:cisco:all",
"root:attack:cisco:all",
"guest:none:cisco,edimax,intel:all",
"MGR:CONV:hp:all",
"MANAGER:TELESUP:hp:all",
"volition:volition:3m:all",
"administrator:administrator:compaq:all",
"FIELD:HPP187 SYS:hp:all",
"public:none:ericsson:all",
"cmaker:cmaker:cisco:all",
"OPERATOR:DISC:hp:all",
"OPERATOR:SUPPORT:hp:all",
"admin:synnet:3com:all",
"SYSDBA:masterkey:interbase:all",
"PBX:PBX:dictaphone:all",
"apc:apc:apc:all",
"acc:acc:andover controls:all",
"root:tslinux:cyclades:all",
"tech:tech:3com:all",
"root:ascend:ascend:all",
"operator:none:3com:all",
"FIELD:MANAGER:hp:all",
"MGR:WORD:hp:all",
"root:root:ambit,avaya:all",
"OPERATOR:COGNOS:hp:all",
"super:surt:cyclades:all",
"netrangr:attack:cisco:all",
"install:llatsni:alcatel:all",
"Manager:none:bay networks:all",
"admin:hello:intel,shiva:all",
"admin:bintec:bintec:all",
"craft:none:avaya:all",
"MGR:TELESUP:hp:all",
"manager:manager:3com:all",
"MANAGER:TCH:hp:all",
"cgadmin:cgadmin:cyberguard:all",
"adm:none:3com:all",
"monitor:monitor:3com:all",
"MGR:VESOFT:hp:all",
"admin:my_DEMARC:demarc:all",
"WP:HPOFFICE:hp:all",
"manager:friend:Allied:all",
"diag:switch:alcatel:all",
"Anonymous:none:compaq,hp:all",
"netman:netman:ericsson:all",
"root:davox:davox:all",
"MANAGER:HPOFFICE:hp:all",
"MGR:HPOFFICE:hp:all",
"Guest:none:intel:all",
"admin:radius:funk software:all",
"security:security:3com,bay networks:all",
"admin:epicrouter:e-tech:all",
"supervisor:supervisor:everfocus:all",
"MGR:RJE:hp:all",
"MAIL:MPE:hp:all",
"root:none:3com,allied,buffalo,intel:all",
"DTA:TJM:aspect:all",
"admin:cisco:cisco:all",
"NICONEX:NICONEX:intel:all",
"MGR:ROBELLE:hp:all",
"FIELD:SUPPORT:hp:all",
"FIELD:HPONLY:hp:all",
"MGR:CNAS:hp:all",
"RSBCMON:SYS:hp:all",
"HELLO:OP.OPERATOR:hp:all",
"NETWORK:NETWORK:dictaphone:all",
"admin:linga:alteon:all",
"admin:switch:alcatel:all",
"hscroot:abc123:ibm:all",
"admin:none:3com,aceex,alteon,telekom,dlink,enterasys,extreme networks,fortinet,hp,ibm,intel:all",
"Administrator:the same all over:bmc:all",
"MGR:XLSERVER:hp:all",
"HELLO:MANAGER.SYS:hp:all",
"kermit:kermit:alcatel:all",
"MGR:CAROLIAN:hp:all",
"ADVMAIL:HP:hp:all",
"D-Link:D-Link:dlink:all",
"MDaemon:MServer:deerfield:all",
"MGR:ITF3000:hp:all",
"admin:netadmin:enterasys:all",
"admin:secure:ADIC:all",
"admin:system:cisco:all",
"tech:none:3com:all",
"customer:none:aspect:all",
"MGR:SYS:hp:all",
"MGR:NETBASE:hp:all",
"root:fivranne:brocade:all",
"bbsd-client:changeme2:cisco:all",
"user:user:compaq:all",
"vt100:public:ibm:all",
"root:ROOT500:avaya:all",
"cellit:cellit:cellit:all",
"anonymous:none:compaq:all",
"netman:none:cabletron:all",
"Administrator:none:3com:all",
"MAIL:REMOTE:hp:all",
"manager:admin:allied:all",
"intel:intel:intel:all",
"MGR:SECURITY:hp:all",
"MGR:HPP189:hp:all",
"operator:operator:compaq,everfocus:all",
"mediator:mediator:celerity:all",
"MGR:HPDESK:hp:all",
"adminttd:adminttd:3com:all",
"sysadm:anicust:accelerated networks:all",
"setup:setup:intel:all",
"HELLO:FIELD.SUPPORT:hp:all",
"mtcl:mtcl:alcatel:all",
"MGR:CCC:hp:all",
"bbsd-client:null:cisco:all",
"root:cms500:avaya:all",
"admin:comcomcom:3com:all",
"MANAGER:ITF3000:hp:all",
"admin:password:3com,adic,brocade,ciphertrust,ibm:all",
"OPERATOR:SYSTEM:hp:all",
"IntraStack:Asante:asante:all",
"MGR:INTX3:hp:all",
"Root:none:3com,ambit:all",
"admin:1234:edimax:all",
"root:tini:dallas semiconductors:all",
"FIELD:MGR:hp:all",
"anonymous:any@:datawizard.net:all",
"Administrator:changeme:cisco:all",
"FIELD:LOTUS:hp:all",
"root:permit:alcatel:all",
"adfexc:adfexc:alcatel:all",
"root:default:f5:all",
"halt:tlah:alcatel:all",
"MGR:HPP187:hp:all",
"PCUSER:SYS:hp:all",
"readonly:lucenttech2:ascend:all",
"SPOOLMAN:HPOFFICE:hp:all",
"MGR:HPONLY:hp:all",
"MANAGER:SYS:hp:all",
"diag:danger:avaya:all",
"user:none:dlink:all",
"craft:crftpw:avaya:all",
"login:admin:efficient:all",
"admin:admin:huawei,3com,allied,alteon,amitech,billon,checkpoint,cisco,com3,davox,dlink,draytek,dynalink,everfocus,flowpoint,foundry networks,hp:all",
"client:client:alcatel:all",
"OPERATOR:SYS:hp:all",
"MGR:COGNOS:hp:all",
"manuf:xxyyzz:avaya:all",
"3comcso:RIP000:3com:all",
"dhs3mt:dhs3mt:alcatel:all",
"ADVMAIL:HPOFFICE DATA:hp:all",
"superadmin:secret:ibm:all",
"superuser:none:asante:all",
"recovery:recovery:3com:all",
"NETOP:none:dictaphone:all",
"IntraSwitch:Asante:asante:all",
"cisco:none:cisco:all",
"rwa:rwa:accelar:all",
"root:alpine:iOS:all",
"mobile:dottie:iOS:all",
"admin:PASS:Lantronix:all",
"TANDBERG:none:Tandberg:all",
"skyboxview:skyboxview:SkyBOX:all",
"pi:raspberry:raspberry:ssh",
"root:toor:kali linux,backtrack linux:all",
"msfadmin:msfadmin:metasploitable:ssh",
"root:abcd1234:trango:all",
"root:nutanix/4u:nutanix:ssh",
"foo:bar:wazuh:http" # foo:$apr1$qdSLixxe$4syV8RRHWSSsvo8Gu6rZ.1 from user file of wazuh-api
);

uploaded_credentials = get_kb_list( "default_credentials/credentials" );
if( uploaded_credentials ) {

  uploaded_credentials_only = get_kb_item( "default_credentials/uploaded_credentials_only" );

  if( uploaded_credentials_only == "yes" ) {
    credentials = uploaded_credentials;
  } else {
    credentials = make_list( uploaded_credentials, credentials );
  }
}

function try( vendor ) {

  local_var vendor, clist, credential;

  if( ! vendor || isnull( vendor ) || vendor == "" ) return credentials;

  foreach credential( credentials ) {

    if( vendor >!< credential ) continue;

    if( typeof( clist ) == "undef") {
       clist = make_list( credential );
    } else {
       clist = make_list( clist, credential );
    }
  }
  return clist;
}
