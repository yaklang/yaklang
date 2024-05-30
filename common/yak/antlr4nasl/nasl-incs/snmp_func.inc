###############################################################################
# OpenVAS Include File
# $Id: snmp_func.inc 9909 2018-05-18 12:29:04Z cfischer $
#
# Functions for SNMP
#
# Authors:
# Christian Kuersteiner <christian.kuersteiner@greenbone.net>
#
# Copyright:
# Copyright (c) 2016 Greenbone Networks GmbH
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
# $Revision: 9909 $
#
###############################################################################

valid_snmpv3_errors = make_list( "Unknown user name",
                                 "Unsupported security level",
                                 "Generic error",
                                 "Session abort failure",
                                 "Unknown security model in message",
                                 "MIB not initialized",
                                 "Authentication failure (incorrect password, community or key)" );

# From nasl_snmpv3_get of openvas-scanner/nasl/nasl_snmp.c
invalid_snmpv3_creds_errors = make_list( "Missing function argument",
                                         "Invalid port value",
                                         "Invalid protocol value",
                                         "Missing privproto or privpass",
                                         "authproto should be md5 or sha1",
                                         "privproto should be des or aes" );

global_var snmp_error, v3_creds, valid_snmpv3_errors, invalid_snmpv3_creds_errors, last_snmp_error, _snmp_func_debug;
_snmp_func_debug = FALSE;

function check_snmpv1( port, community ) {

  local_var port, community, oid, protocol, ret;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#check_snmpv1" );
    return;
  }

  if( ! community ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#community#-#check_snmpv1" );
    return;
  }

  oid = '1.3.6.1.2.1.1.1.0';
  protocol = 'udp';

  ret = snmpv1_get( port:port, oid:oid, protocol:protocol, community:community );
  if( ret[0] < 0 ) return;

  if( ret[0] == 0 && ! isnull( ret[1] ) ) return TRUE;

  return;
}

function check_snmpv2( port, community ) {

  local_var port, community, oid, protocol, ret;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#check_snmpv2" );
    return;
  }

  if( ! community ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#community#-#check_snmpv2" );
    return;
  }

  oid = '1.3.6.1.2.1.1.1.0';
  protocol = 'udp';

  ret = snmpv2c_get( port:port, oid:oid, protocol:protocol, community:community );
  if( int( ret[0] ) < 0 ) return;

  if( ret[0] == 0 && ! isnull( ret[1] ) ) return TRUE;

  return;
}

function check_snmpv3( port ) {

  # nb: v3_creds is a global var
  local_var port, oid, protocol, snmpv3_username, snmpv3_password;
  local_var snmpv3_authalgo, snmpv3_privpass, snmpv3_privalgo, ret;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#check_snmpv3" );
    return;
  }

  oid = '1.3.6.1.2.1.1.1.0';
  protocol = 'udp';

  snmpv3_username = get_kb_item( "SNMP/v3/username" );
  if( ! snmpv3_username || ( strlen( snmpv3_username ) == 0 ) ) {
    # Set some credentials to test for SNMPv3
    snmpv3_username = 'openvas';
    snmpv3_password = 'openvaspass';
    snmpv3_authalgo = 'md5';
    snmpv3_privpass = 'openvaspass';
    snmpv3_privalgo = 'des';
  } else {
    snmpv3_password = get_kb_item( "SNMP/v3/password" );
    snmpv3_authalgo = get_kb_item( "SNMP/v3/auth_algorithm" );
    if( ! snmpv3_privpass = get_kb_item( "SNMP/v3/privacy_password" ) )
      snmpv3_privalgo = NULL;
    else
      snmpv3_privalgo = get_kb_item( "SNMP/v3/privacy_algorithm" );

    v3_creds = TRUE;
  }

  ret = snmpv3_get( port:port, protocol:protocol, username:snmpv3_username, oid:oid,
                    authpass:snmpv3_password, authproto:snmpv3_authalgo, privpass:snmpv3_privpass,
                    privproto:snmpv3_privalgo );

  if( ret[0] == -1 ) {
    snmp_error = ret[1];
    if( in_array( array:valid_snmpv3_errors, search:snmp_error ) )
      return 2;
    else
      return;
  }

  if( ret[0] == 0 && ! isnull( ret[1] ) ) return 1;

  if( ret[0] == -2 )
    snmp_error = ret[1];

  return;
}

function snmp_get( port, oid, version, community ) {

  local_var port, oid, version, community, ret;
  local_var v3_username, v3_password, v3_authalgo, v3_privpass, v3_privalgo;

  # The SNMP functions needs this include but the installation might be lacking libsnmp support
  # Report this back to the caller so we at least know why snmp_get is failing
  if( ! defined_func( "snmpv3_get" ) ) {
    report  = "snmp_get: The scanner/libraries are not build with libsnmp support. Advanced SNMP checks will fail.";
    report += " Please rebuild with SNMP support enabled.";
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#" + report );
    return;
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#snmp_get" );
    return;
  }

  if( ! oid ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#oid#-#snmp_get" );
    return;
  }

  if( ! version ) version = get_kb_item( "SNMP/" + port + "/prefered_version" );
  if( ! version ) return;

  if( version == 3 ) {

    if( get_kb_item( "SNMP/" + port + "/v3/working" ) ) {

      v3_username = get_kb_item( "SNMP/v3/username" );
      v3_password = get_kb_item( "SNMP/v3/password" );
      v3_authalgo = get_kb_item( "SNMP/v3/auth_algorithm" );
      v3_privpass = get_kb_item( "SNMP/v3/privacy_password" );
      v3_privalgo = get_kb_item( "SNMP/v3/privacy_algorithm" );

      if( v3_username && v3_username != "" ) {

        if( ! v3_privpass ) v3_privalgo = NULL;

        ret = snmpv3_get( port:port, protocol:"udp", username:v3_username, oid:oid,
                          authpass:v3_password, authproto:v3_authalgo, privpass:v3_privpass,
                          privproto:v3_privalgo );

        if( ret[0] != 0 || snmp_is_error_message( ret[1] ) ) {
          last_snmp_error = ret[1];
          if( _snmp_func_debug ) display("SNMP debug: Received error in snmpv3_get " + last_snmp_error );
          return;
        }

        if( ret[0] == 0 && ! snmp_is_error_message( ret[1] ) )
          return snmp_clean_ret( ret[1] );
      }
    }
  }

  if( version == 2 ) {

    if( get_kb_item( "SNMP/" + port + "/v2c/working" ) ) {

      if( ! community ) community = snmp_get_community( port:port, version:2 );

      if( community && community != "" ) {

        ret = snmpv2c_get( port:port, oid:oid, protocol:"udp", community:community );

        if( ret[0] != 0 || snmp_is_error_message( ret[1] ) ) {
          last_snmp_error = ret[1];
          if( _snmp_func_debug ) display("SNMP debug: Received error in snmpv2c_get " + last_snmp_error );
          return;
        }

        if( ret[0] == 0 && ! snmp_is_error_message( ret[1] ) )
          return snmp_clean_ret( ret[1] );
      }
    }
  }

  if( version == 1 ) {

    if( get_kb_item( "SNMP/" + port + "/v1/working" ) ) {

      if( ! community ) community = snmp_get_community( port:port, version:1 );

      if( community && community != "" ) {

        ret = snmpv1_get( port:port, oid:oid, protocol:"udp", community:community ) ;

        if( ret[0] != 0 || snmp_is_error_message( ret[1] ) ) {
          last_snmp_error = ret[1];
          if( _snmp_func_debug ) display("SNMP debug: Received error in snmpv1_get " + last_snmp_error );
          return;
        }
        if( ret[0] == 0 && ! snmp_is_error_message( ret[1] ) )
          return snmp_clean_ret( ret[1] );
      }
    }
  }
  return;
}

function snmp_clean_ret() {

  local_var arg;

  arg = _FCT_ANON_ARGS[0];

  if( ! arg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#_FCT_ANON_ARGS[0]#-#snmp_clean_ret" );
    return;
  }

  return ereg_replace( pattern:'^"(.*)"$', string:arg, replace:"\1" );
}

function snmp_get_community( port, version ) {

  local_var port, version, ckey, clist;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#snmp_get_community" );
    return;
  }

  if( ! version ) version = get_kb_item( "SNMP/" + port + "/prefered_version" );
  if( ! version ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#snmp_get_community: couldn't determine SNMP version / no SNMP version available" );
    return;
  }

  if( version  == 1 )
    ckey = "SNMP/" + port + "/v1/community";
  else if( version  == 2 )
    ckey = "SNMP/" + port + "/v2c/community";
  else
    return;

  clist = get_kb_list( ckey );

  if( ! clist ) return;

  clist = make_list( clist );

  return clist[ 0 ];
}

function snmp_is_error_message() {

  local_var message, errors, error;

  message = _FCT_ANON_ARGS[0];

  if( ! message ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#_FCT_ANON_ARGS[0]#-#snmp_is_error_message" );
    return;
  }

  errors = make_list( "No Such Object available on this agent at this OID",
                      "No Such Instance" );

  foreach error( errors ) {
    if( error >< message )
      return TRUE;
  }
  return;
}

# Get the software and OID from a given pattern
function snmp_get_sw_oid( pattern, port ) {

  local_var pattern, port, list, split, i, result;

  if( ! pattern || pattern == "" ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#pattern#-#snmp_get_sw_oid" );
    return;
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#snmp_get_sw_oid" );
    return;
  }

  if( ! list = get_kb_item( "SNMP/" + port + "/installed_software" ) )
    return;

  split = split( list, sep:"|", keep:FALSE );

  i = 0;
  while( i < max_index( split ) ) {
    if( ereg( pattern:pattern, string:split[i+1] ) ) {
      result = make_array( "oid", split[i],
                           "package", split[i+1] );
      return result;
    }
    i += 2;
  }
}

function get_snmp_port( default ) {

  local_var default, port;

  port = get_kb_item( "Services/udp/snmp" );
  if( port ) default = port;

  if( ! get_udp_port_state( default ) ) exit( 0 );

  return default;
}

function get_snmp_sysdesc( port ) {

  local_var port, sysdesc;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_snmp_sysdesc" );
    return;
  }

  sysdesc = get_kb_item( "SNMP/" + port + "/sysdesc" );
  if( ! sysdesc ) return;

  return sysdesc;
}
