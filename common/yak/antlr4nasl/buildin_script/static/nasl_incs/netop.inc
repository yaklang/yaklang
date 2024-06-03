###############################################################################
# OpenVAS Include File
# $Id: netop.inc 13066 2019-01-14 14:25:36Z cfischer $
#
# Methods for handling netop
#
# Authors:
# Martin O'Neal of Corsaire (http://www.corsaire.com)
# Jakob Bohm of Danware (http://www.danware.dk)
#
# Copyright:
# Copyright (C) 2004 Martin O'Neal of Corsaire & Jakob Bohm of Danware
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2,
# as published by the Free Software Foundation
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#
# $Revision: 13066 $
#
###############################################################################

# The original script produced by Martin O'Neal has been heavily modified
# by Jakob Bohm to provide additional product detail, plus also to limit
# any unnecessary information leakage caused by the scanning process itself.
# Jakob has also rewritten the test set as a whole to use the minimum of
# network traffic and make the process as efficient as possible. All together
# a jolly good show.

############## Meta-information recorded in the name of the KB entry ################

local_var addr;
global_var proto_nam, port;

############## HELO packets in NetOps native format ################

global_var helo_pkt_udp;
global_var helo_pkt_gen;
global_var quit_pkt_stream;

############## Received reply in NetOps native format ##################

global_var banner_pkt;

############## General derived strings ##################

local_var netop_kb_key;
global_var netop_kb_val;

local_var netop_svcidstr;
local_var netop_prod_typ;
local_var netop_prod_nam;
local_var netop_prod_dsc;

# copied from misc_func.inc as we don't want to taint netop.inc with
# a third-party include
# TODO: Remove and use hex2raw() from misc_func.inc once openvas-nasl-lint
# can handle .inc includes using a function from another include.
_netop_func_debug = FALSE;
function netop_hex2raw( s ) {

  local_var i, j, ret, l, s;

  if( isnull( s ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#s#-#netop_hex2raw" );

  s = chomp( s );  # remove trailing blanks, CR, LF...
  l = strlen( s );
  if( l % 2 ) {
    if( _netop_func_debug ) display( "hex2raw: odd string: ", s, "\n" );
    l --;
  }
  s = tolower( s );
  for( i = 0; i < l; i += 2) {
    if( ord( s[i] ) >= ord( "0" ) && ord( s[i] ) <= ord( "9" ) )
      j = int( s[i] );
    else
      j = int( ( ord( s[i] ) - ord( "a" ) ) + 10 );

    j *= 16;
    if( ord( s[i + 1] ) >= ord( "0" ) && ord( s[ i + 1] ) <= ord( "9" ) )
      j += int( s[i+1] );
    else
      j += int( ( ord( s[ i + 1 ] ) - ord( "a" ) ) + 10 );
    ret += raw_string( j );
  }
  return ret;
}

################### HELO packet for UDP only ##################
helo_pkt_udp = netop_hex2raw(s:'d6818106010000f70e5301044e657373757301436f7273616972ff0100008701') + # 0-31
               crap(length:32, data:raw_string(0x20)) + # 32-63
               crap(length:224, data:raw_string(0x00)); # 64-288

################### Generic HELO packet ##################
helo_pkt_gen = netop_hex2raw(s:'d6818106010000f73e4e010400000000000001436f7273616972ff0100008701') + # 0-31
               crap(length:32, data:raw_string(0x20)) + # 32-63
               crap(length:224, data:raw_string(0x00)); # 64-288

################### QUIT packet for stream transports (tcp etc.)  ##################
quit_pkt_stream = netop_hex2raw(s:'d681810000f900f9eee3') + # 0-9
                  crap(length:16, data:raw_string(0x00));   # 10-25

################### utility functions ##################

function netop_zeropad(str, len) {
  local_var res;

  res = crap(length:len, data:raw_string(0));
  if (!isnull(str)) {
    res = substr(str + res, 0, len - 1);
  }
  return res;
}

function netop_spacepad(str, len) {
  local_var res;

  res = crap(length:len, data:' ');
  if (!isnull(str)) {
    res = substr(str + res, 0, len - 1);
  }
  return res;
}

function ip_dot2raw(ip_dot) {
  local_var int32, octet;

  ip_dot = split(ip_dot, sep:".", keep:FALSE);
  int32 = '';

  for(octet=0;octet<4;octet++) {
    int32 = raw_string(int(ip_dot[octet]), int32);
  }
  return(int32);
}

function ip_raw2dot(int32) {
  local_var ip_dot;

  ip_dot = int(ord(int32[3]))+"."+int(ord(int32[2]))+"."+int(ord(int32[1]))+"."+int(ord(int32[0]));
  return(ip_dot);
}

################### extract info items from banner ###################
function netop_banner_items(typ) {
  local_var res, i, v1;

  res = '';
  for (i = 64; i < 320; i += 32) {
    v1 = substr(netop_kb_val, i, i + 31);
    if (typ == 0 && ord(v1) >= 32 || typ != 0 && typ == ord(v1)) {
      if (typ > 2)
        v1 = substr(v1, 1, 31);
      else if (typ == 2)
        v1 = ip_raw2dot(int32: substr(v1, 1, 4));
      else if (typ == 1)
        v1 = hexstr(substr(v1, 1, 6));
      if (res != '')
        res += ', ';
      res += '"' + chomp(v1) + '"';
    }
  }
  return res;
}

################### derive product type from banner etc. ###################

function netop_product_ident() {
  local_var school_phrase1, school_stud_dsc, rc_also_host, rc_host_dsc;

  school_phrase1 = 'During the session, the Teacher' +
                   ' has extensive control over the users computer,' +
                   ' with very few restrictions, as is appropriate' +
                   ' when the teacher can be assumed to be a higher' +
                   ' authority than the student.';

  school_stud_dsc = 'NetOp School Student (client) allows its user to' +
                    ' participate in live online education or training' +
                    ' sessions broadcast from a computer running NetOp' +
                    ' School Teacher.\n\n' +
                    school_phrase1;

  rc_also_host = ' is also a fully' +
                 ' featured NetOp Remote Control Host, which allows' +
                 ' its computer to be remotely controlled and/or' +
                 ' managed from any NetOp Remote Control Guest' +
                 ' subject to a separate set of configurable' +
                 ' security restrictions.';

  rc_host_dsc = 'NetOp Remote Control Host is a service / agent,' +
                ' which allows the computer on which it is running' +
                ' to be remotely controlled and/or managed from any' +
                ' NetOp Remote Control Guest program, subject to a' +
                ' wide selection of configurable security and' +
                ' authentication restrictions.';

  if ( (ord(netop_kb_val[63]) & 0x08) != 0 || netop_banner_items(typ:17) != '' ) {
    netop_svcidstr = 'netop-sch';
    netop_prod_typ = 'SSTD';
    netop_prod_nam = 'NetOp School Student';
    netop_prod_dsc = school_stud_dsc;
  } else if ((ord(netop_kb_val[63]) & 0x10) != 0 || netop_banner_items(typ:8) != '' ) {
    netop_svcidstr = 'netop-teacher';
    netop_prod_typ = 'STCH';
    netop_prod_nam = 'NetOp School Teacher';
    netop_prod_dsc = 'NetOp School Teacher (console) allows its user to' +
                     ' conduct live online education or training' +
                     ' sessions broadcast to computers running NetOp' +
                     ' School Student.\n\n' +
    school_phrase1;
  } else if ((ord(netop_kb_val[63]) & 0x04) != 0 || netop_banner_items(typ:6) != '' || netop_banner_items(typ:7) != '' ) {
    netop_svcidstr = 'netop-guest';
    netop_prod_typ = 'RGST';
    netop_prod_nam = 'NetOp Remote Control Guest';
    netop_prod_dsc = 'NetOp Remote Control Guest (client) allows its' +
                     ' user to remotely control and/or manage any' +
                     ' computer running NetOp Remote Control Host' +
                     ' modules on a variety of operating systems,' +
                     ' subject of cause to the security restrictions' +
                     ' configured on that Host.';
  } else if ((ord(netop_kb_val[62]) & 0x01) != 0) {
    netop_svcidstr = 'netop-rc';
    netop_prod_typ = 'RGWS';
    netop_prod_nam = 'NetOp Remote Control Gateway';
    netop_prod_dsc = 'NetOp Remote Control Gateway is an application' +
                     ' layer proxy allowing programs from the NetOp' +
                     ' Remote Control and NetOp School families to' +
                     ' communicate across proxy-style firewalls,' +
                     ' disjoint networks, dissimilar network protocols' +
                     ' (e.g. modems, shared memory, TCP and UDP) etc.' +
                     ' subject to configurable access restrictions.' +
                     '\n\n' +
    netop_prod_nam + rc_also_host;
  } else if ((ord(netop_kb_val[62]) & 0x08) != 0) {
    netop_svcidstr = 'netop-rc';
    netop_prod_typ = 'RNMS';
    netop_prod_nam = 'NetOp Remote Control Name Server';
    netop_prod_dsc = 'NetOp Name Server is a dynamic' +
                     ' naming service allowing programs from the NetOp' +
                     ' Remote Control and NetOp School families to' +
                     ' locate each other even when general facilities' +
                     ' such as dynamic DNS are not available to all' +
                     ' participating computers, or when ports etc. need' +
                     ' to be included in the naming information' +
                     ' published.\n\n' +
    netop_prod_nam + rc_also_host;
  } else if (netop_banner_items(typ:5) != '' || netop_banner_items(typ:14) != '' ) {
    netop_svcidstr = 'netop-rc';
    netop_prod_typ = 'RSES';
    netop_prod_nam = 'NetOp Remote Control Security Server';
    netop_prod_dsc = 'NetOp Remote Control Security Server is a central' +
                     ' authentication and authorization server allowing' +
                     ' centralized login validation, permission' +
                     ' management and security event logging for' +
                     ' programs from the NetOp Remote Control family.' +
                     '\n\n' +
    netop_prod_nam + rc_also_host;
  } else if ((ord(netop_kb_val[63]) & 0x02) != 0) {
    netop_svcidstr = 'netop-rc';
    netop_prod_typ = 'RHST';
    netop_prod_nam = 'NetOp Remote Control Host';
    netop_prod_dsc = rc_host_dsc;
  } else if (port == 1971) {
    netop_svcidstr = 'netop-sch';
    netop_prod_typ = 'SCH?';
    netop_prod_nam = 'NetOp School Student or Teacher';
    netop_prod_dsc = school_stud_dsc;
  } else {
    netop_svcidstr = 'netop-rc';
    netop_prod_typ = 'NRC?';
    netop_prod_nam = 'NetOp Remote Control Host or Guest';
    netop_prod_dsc = rc_host_dsc;
  }
  netop_prod_dsc += '\n\nSee http://www.netop.com for more info.\n';
}

################ Log that a NetOp program has been detected ################
function netop_log_detected() {
  local_var msg, info_only;

  info_only = 0;

  msg = '\nDanware ' + netop_prod_nam + ' is listening on this port\n\n' + netop_prod_dsc + '\nSolution:  ';

  if (netop_prod_typ == 'RGST') {
    info_only = 1;
    msg += 'Make sure the user of this machine is' +
           ' authorized to remotely manage other computers' +
           ' or has been permitted to use this computer as' +
           ' a terminal to access other computers.\n';
  } else if (netop_prod_typ == 'STCH') {
    info_only = 1;
    msg += 'Make sure the user of this machine is a teacher' +
           ' or is acting as a group leader for some' +
           ' teamwork\n';
  } else if (netop_prod_typ == 'SSTD' || netop_prod_typ == 'SCH?') {
    info_only = 1;
    msg += 'Make sure the user of this machine is currently' +
           ' participating in online training using NetOp' +
           ' School, and that a teacher password has been' +
           ' set on the Student.\n\n' +
           'Outside dedicated teaching environments, NetOp' +
           ' School should not be running when the user is' +
           ' not actively participating in a class\n';
  } else if (netop_prod_typ == 'RHST' || netop_prod_typ == 'NRC?') {
    msg += 'If this program is required, make sure' +
           ' appropriate security settings are used (on the' +
           ' Options menu), including strong passwords on' +
           ' permitted accounts and an effective action on' +
           ' too many bad password attempts\n\n' +
           'If this program is unused, disable load at' +
           ' system startup from the programs option menu or' +
           ' uninstall the software.\n';
  } else {
    msg += 'If this service is running deliberately, make' +
           ' sure it is configured with strong security' +
           ' settings on the options menu, including strong' +
           ' passwords on any enabled accounts and an' +
           ' effective action on too many failed logins\n\n' +
           'If this service is not supposed to be running,' +
           ' uninstall it and investigate why and how it was' +
           ' installed.\n';
  }
  security_message(proto:proto_nam, port:port, data:msg);
}

################ Extract information from KB value and key ################
function netop_kb_derive() {
  netop_kb_val = netop_zeropad(str:netop_kb_val, len:320);
  if(ord(netop_kb_val[0]) >  32 ) {
    netop_kb_key = substr(netop_kb_val, 0, 31);
  }
  netop_kb_key = netop_zeropad(str:netop_kb_key, len:32);
  insstr(netop_kb_val, netop_kb_key, 0, 31);

  proto_nam = chomp(substr(netop_kb_key, 0, 7));
  port      = ord(netop_kb_key[9]) * 256 + ord(netop_kb_key[8]);
  addr      = ip_raw2dot(int32: substr(netop_kb_key, 16, 31));

  netop_product_ident();
}

########### Check if any NetOp programs found and fork for each ###########
function netop_each_found() {
  netop_kb_val = get_kb_item('NetOp/allbanners');
  if(isnull(netop_kb_val)) {
    return 0;
  } else {
    netop_kb_val = netop_hex2raw(s:netop_kb_val);
    netop_kb_derive();
    return 1;
  }
}

######### validate banner to see if it is from NetOp, then add to KB ########
function netop_check_and_add_banner() {
  local_var blen, s1, s2;

  netop_kb_key = netop_spacepad(str:proto_nam, len:8) +
                 netop_zeropad(str:raw_string((port & 255), ((port & 65280) >> 8)), len:8) +
                 netop_zeropad(str:ip_dot2raw(ip_dot:addr), len:16);

  # check response contains correct contents
  if (isnull(banner_pkt)) {
    banner_pkt = '';
  }

  blen = strlen(banner_pkt);
  if(blen > 23 && ord(banner_pkt[0]) == 0xd6 && banner_pkt[1]==banner_pkt[2] &&
     ((ord(banner_pkt[7])==0xf8 && blen >= 32 && substr(banner_pkt, 11, 17) == 'Corsair') ||
      (ord(banner_pkt[7])==0xf9 && ord(banner_pkt[5])==0xf9))) {
    if (ord(banner_pkt[7])==0xf9 && blen > 27) {
      banner_pkt = substr(banner_pkt, 0, 26);
    }

    # Force packet size before adding to knowledge base
    netop_kb_val = netop_zeropad(str:(netop_kb_key + banner_pkt), len:320);

    netop_kb_derive();

    # update knowledge base
    s2 = 'NetOp/' + hexstr(netop_kb_key) + '/banner';
    set_kb_item(name:s2, value:hexstr(netop_kb_val));
    set_kb_item(name:'NetOp/allbanners', value:hexstr(netop_kb_val));

    s1 = proto_nam + '/';
    if (s1 == 'tcp/') s1 ='';

    s2 = 'Known/' + proto_nam + '/' + port;
    set_kb_item(name:s2, value:netop_svcidstr);
    s2 = 'Services/' + s1 + netop_svcidstr;
    set_kb_item(name:s2, value:port);
    s2 = 'Services/' + s1 + 'netop-any';
    set_kb_item(name:s2, value:port);

    netop_log_detected();
  } else {
    netop_kb_val = crap(length: 320, data: raw_string(0x00));
  }
}

################### end of netop.inc ##################