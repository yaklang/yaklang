###############################################################################
# OpenVAS Vulnerability Test
#
# Ping Host
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2009 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
###############################################################################

if(description)
{
  script_oid("1.3.6.1.4.1.25623.1.0.100315");
  script_version("2019-05-24T11:20:30+0000");
  script_tag(name:"last_modification", value:"2019-05-24 11:20:30 +0000 (Fri, 24 May 2019)");
  script_tag(name:"creation_date", value:"2009-10-26 10:02:32 +0100 (Mon, 26 Oct 2009)");
  script_tag(name:"cvss_base", value:"0.0");
  script_tag(name:"cvss_base_vector", value:"AV:N/AC:L/Au:N/C:N/I:N/A:N");
  script_name("Ping Host");
  script_category(ACT_SCANNER);
  script_family("Port scanners");
  script_copyright("This script is Copyright (C) 2009 Greenbone Networks GmbH");

  script_add_preference(name:"Use fp", type:"checkbox", value:"no");

  # Don't change the preference names, those names are hardcoded within some manager functions...
  # nb: Same goes for id: parameter, those numbers are hardcoded in the manager as well.

  ### In the following two lines, unreachable is spelled incorrectly.
  ### Unfortunately, this must stay in order to keep compatibility with existing scan configs.
  script_add_preference(name:"Report about unrechable Hosts", type:"checkbox", value:"no");
  script_add_preference(name:"Mark unrechable Hosts as dead (not scanning)", type:"checkbox", value:"yes", id:5);
  script_add_preference(name:"Report about reachable Hosts", type:"checkbox", value:"no");
  script_add_preference(name:"Use ARP", type:"checkbox", value:"no", id:4);
  script_add_preference(name:"Do a TCP ping", type:"checkbox", value:"yes", id:1);
  script_add_preference(name:"TCP ping tries also TCP-SYN ping", type:"checkbox", value:"yes", id:2);
  script_add_preference(name:"TCP ping tries only TCP-SYN ping", type:"checkbox", value:"yes", id:7);
  script_add_preference(name:"Do an ICMP ping", type:"checkbox", value:"yes", id:3);
  script_add_preference(name:"nmap additional ports for -PA", type:"entry", value:"137,587,3128,8081");
  script_add_preference(name:"nmap: try also with only -sP", type:"checkbox", value:"no");
  script_add_preference(name:"Log nmap output", type:"checkbox", value:"no");
  script_add_preference(name:"Log failed nmap calls", type:"checkbox", value:"no");

  script_tag(name:"summary", value:"This check tries to determine whether a remote host is up (alive).

  Several methods are used for this depending on configuration of this check. Whether a host is up can
  be detected in 3 different ways:

  - A ICMP message is sent to the host and a response is taken as alive sign.

  - An ARP request is sent and a response is taken as alive sign.

  - A number of typical TCP services (namely the 20 top ports of nmap)
  are tried and their presence is taken as alive sign.

  None of the methods is failsafe. It depends on network and/or host configurations
  whether they succeed or not. Both, false positives and false negatives can occur.
  Therefore the methods are configurable.

  If you select to not mark unreachable hosts as dead, no alive detections are
  executed and the host is assumed to be available for scanning.

  In case it is configured that hosts are never marked as dead, this can cause
  considerable timeouts and therefore a long scan duration in case the hosts
  are in fact not available.

  The available methods might fail for the following reasons:

  - ICMP: This might be disabled for a environment and would then cause false
  negatives as hosts are believed to be dead that actually are alive. In contrast
  it is also possible that a Firewall between the scanner and the target host is answering
  to the ICMP message and thus hosts are believed to be alive that actually are dead.

  - TCP ping: Similar to the ICMP case a Firewall between the scanner and the target might
  answer to the sent probes and thus hosts are believed to be alive that actually are dead.");

  script_tag(name:"qod_type", value:"remote_banner");

  exit(0);
}

include("misc_func.inc");
include("host_details.inc");
include("network_func.inc");

global_var report_dead_methods, failed_nmap_report;
report_dead_methods = ""; # nb: To make openvas-nasl-lint happy...
failed_nmap_report = "";


use_fp = script_get_preference("Use fp");
if( isnull( use_fp ) )
  use_fp = "yes";

report_up = script_get_preference("Report about reachable Hosts");
if( isnull( report_up ) )
  report_up = "no";

### In the following two lines, unreachable is spelled incorrectly.
### Unfortunately, this must stay in order to keep compatibility with existing scan configs.
report_dead = script_get_preference("Report about unrechable Hosts");
mark_dead   = script_get_preference("Mark unrechable Hosts as dead (not scanning)");
if( isnull( report_dead ) )
  report_dead = "no";

if( isnull( mark_dead ) )
  mark_dead = "yes";

icmp_ping = script_get_preference("Do an ICMP ping", id:3);
if( isnull( icmp_ping ) )
  icmp_ping = "yes";

tcp_ping = script_get_preference("Do a TCP ping", id:1);
if( isnull( tcp_ping ) )
  tcp_ping = "no";

tcp_syn_ping = script_get_preference("TCP ping tries also TCP-SYN ping", id:2);
if( isnull( tcp_syn_ping ) )
  tcp_syn_ping = "no";

tcp_syn_ping_only = script_get_preference("TCP ping tries only TCP-SYN ping", id:7);
if( isnull( tcp_syn_ping_only ) )
  tcp_syn_ping_only = "no";

arp_ping = script_get_preference("Use ARP", id:4);
if( isnull( arp_ping ) )
  arp_ping = "no";

sp_only = script_get_preference("nmap: try also with only -sP");
if( isnull( sp_only ) )
  sp_only = "no";

log_nmap_output = script_get_preference("Log nmap output");
if( isnull( log_nmap_output ) )
  log_nmap_output = "no";

log_failed_nmap = script_get_preference("Log failed nmap calls");
if( isnull( log_failed_nmap ) )
  log_failed_nmap = "no";

set_kb_item( name:"/ping_host/mark_dead", value:mark_dead );
set_kb_item( name:"/tmp/start_time", value:unixtime() );

if( "no" >< icmp_ping && "no" >< tcp_ping && "no" >< arp_ping && "no" >< sp_only ) {
  log_message( data:"The alive test was not launched because no method was selected." );
  exit( 0 );
}

if( "no" >< mark_dead && "no" >< report_dead ) {
  if( "yes" >< log_nmap_output )
    log_message( data:"'Log nmap output' was set to 'yes' but 'Report about unrechable Hosts' and 'Mark unrechable Hosts as dead (not scanning)' to no. Plugin will exit without logging." );
  exit( 0 );
}

if (pingHost()){
  set_kb_item( name:"/tmp/ping/ICMP", value:1 );
  exit( 0 );
}

# Host seems to be dead.
register_host_detail( name:"dead", value:1 );

if( "yes" >< report_dead ) {
  if( "yes" >< use_fp && report_dead_methods == "" ) {
    report_dead_methods += '\n\nMethod: nmap chosen but invocation of nmap failed due to unknown reasons.';
    if( "yes" >!< log_nmap_output || "yes" >!< log_failed_nmap )
      report_dead_methods += " Please set 'Log nmap output' and 'Log failed nmap calls' to 'yes' and re-run this test to get additional output.";
    else
      report_dead_methods += ' Please see the output below for some hints on the failed nmap calls.\n\n' + failed_nmap_report;
  } else if( report_dead_methods != "" ) {
    report_dead_methods = ' Used/configured checks:' + report_dead_methods;
  }
  log_message( data:"The remote host " + get_host_ip() + ' was considered as dead.' + report_dead_methods, port:0 );
}

if( "yes" >< mark_dead )
  set_kb_item( name:"Host/dead", value:TRUE );

exit( 0 );