###############################################################################
# OpenVAS Include File
# $Id: telnet_func.inc 14176 2019-03-14 11:29:33Z tpassfeld $
#
# Methods to handle Telnet
#
# Authors:
# Michel Arboi <arboi@alussinan.org>
#
# Copyright:
# Copyright (C) 2002 Michel Arboi <arboi@alussinan.org>
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2,
# as published by the Free Software Foundation
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#
# $Revision: 14176 $
#
###############################################################################

OPT_WILL      = 0xfb;
OPT_WONT      = 0xfc;
OPT_DO        = 0xfd;
OPT_DONT      = 0xfe;

OPT_SUBOPT    = 0xfa;
OPT_ENDSUBOPT = 0xf0;

# Global var to avoid that we're repeatedly asking redis for something we already know.
global_var _telnet_is_broken_array;

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if broken or not
_telnet_is_broken_array = make_array();

# nb: Those are various services which are negotiating a connection with us. These are used in telnet_verify_banner later.
__no_telnet = make_list( "<<<check_mk>>>", "\\check_mk\.ini", "<<<uptime>>>", "<<<services>>>", "<<<mem>>>", "Check_MK_Agent",
                         " stopped/demand ", " running/auto ", " stopped/disabled ", # Check_MK Agent
                         "NOTIC: iSCSI:", "INFOR: iSCSI:", "ERROR: iSCSI:", # DELL TL2000/TL4000 iSCSI-SAS Bridge on 1234/tcp
                         "Press Enter for Setup Mode", "^ATQ", "HSQLDB JDBC Network Listener",
                         "^OK MPD", "^IOR:", "Host.*is not allowed to connect to this (MySQL|MariaDB) server",
                         "Host.*is blocked.*mysqladmin flush-hosts",
                         "mysql_native_password",
                         "Where are you?", #rexecd
                         "DOCTYPE GANGLIA_XML", # Ganglia gmetad daemon
                         "^Asterisk Call Manager",
                         "^w0256", # Unknown service on 10003/tcp
                         "java\.rmi\.MarshalledObject",
                         "<\?xml version=", # Unknown service on 5547/tcp
                         "\-nthreads", "NServer:", # Unknown service on 34903/tcp
                         "^ERROR :Closing Link:.*Throttled: Reconnecting too fast", # unlrealircd
                         "^:.*NOTICE (Auth|AUTH).*Looking up your hostname", # unlrealircd
                         "^TDMM", # LANDesk Targeted Multicast Service, 33354/tcp
                         "^UDMM", # Unknown LANDesk Service, 33354/tcp
                         "\+HELLO v([0-9.]+) \$Name:", # e.g. +HELLO v1.1 $Name:  $, unknown service on 5600/tcp
                         "^ getnameinfo: Temporary failure in name resolution $", # rsh on 514/tcp, spaces at the begin and end are expected
                         "Welcome to the TeamSpeak 3 ServerQuery interface",
                         "500 OOPS: could not bind listening IPv4 socket", # Probably PureFTPd
                         "^ncacn_http/1\.0",
                         "^220 .*FTP [Ss]erver .*ready",
                         "^220 .*Ready for user login\.", # VIBNODE FTP
                         "^220 Service ready",
                         "^RFB 00[0-9]\.00[0-9]", # VNC
                         "\(Eggdrop v.* Eggheads\)" ); # Eggdrop Bot

# @brief Make a connection to a remote Telnet service and receives the banner from it.
#        If a banner was received it is saved into the knowledge base.
#        If a banner was requested which was requested previously the cached
#        banner will be returned instead of doing a new connection.
#
# @param port    The port of the remote Telnet service to get the banner from.
# @param timeout An optional timeout for the connection which is passed to the used
#                @ref telnet_negotiate function, defaults to 3 seconds if this parameter
#                is not specified.
#
# @return A string containing the received banner, NULL if no banner was received or
#         no connection could be made to the remote service and FALSE if not all required
#         parameters where passed.
#
function get_telnet_banner( port, timeout ) {

  local_var port, timeout;
  local_var banner, soc;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_telnet_banner" );
    return FALSE;
  }

  banner = get_kb_item( "telnet/banner/" + port );
  if( banner )
    return( banner );

  if( ! get_port_state( port ) )
    return NULL;

  if( telnet_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc ) {
    telnet_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = telnet_negotiate( socket:soc, timeout:timeout );
  if( ! banner ) {
    close( soc );
    telnet_set_is_marked_broken( port:port );
    return FALSE;
  }

  telnet_set_banner( port:port, banner:banner );
  telnet_close_socket( socket:soc, data:banner );

  return banner;
}

# @brief Negotiate a Telnet connection via the passed socket
#        and returns any received data from the connected service.
#
# @param socket  The socket of the remote system to connect to.
# @param timeout An optional timeout for the connection, defaults to 3 seconds if
#                this parameter is not specified.
#
# @return The received data or NULL if no data was received or no / a wrong
#         socket was passed to the function.
#
function telnet_negotiate( socket, timeout ) {

  local_var socket, timeout;
  local_var counter, s, buf, prev, counter2;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#telnet_negotiate" );
    return NULL;
  }

  counter = 0;
  if( ! timeout || timeout < 0 )
    timeout = 3; # default timeout

  while( TRUE ) {
    s = recv( socket:socket, length:1, timeout:timeout );
    if( ! strlen( s ) )
      break;

    if( ord( s[0] ) != 0xff ) {
      buf += s;
    } else {
      counter++;
      s = recv( socket:socket, length:2 );

      if( ord( s[0] ) == OPT_DO ) {
        send( socket:socket, data:raw_string( 0xff, OPT_WONT ) + s[1] );
      } else if( ord( s[0] ) == OPT_WILL ) {
        send( socket:socket, data:raw_string( 0xff, OPT_DONT ) + s[1] );
      } else if( ord( s[0] ) == OPT_SUBOPT ) {
        # The remote telnet server is autistic :/
        prev = recv( socket:socket, length:1 );
        counter2 = 0;
        while( ord( prev ) != 0xff && ord( s[0] ) != OPT_ENDSUBOPT ) {
          prev = s;
          # No timeout - the answer is supposed to be cached
          s = recv( socket:socket, length:1, timeout:0 );
          if ( ! strlen( s ) )
            return buf;

          counter2++;

	  if( counter2 >= 100 )
            return buf;
        }
      }
      # Not necessary and may introduce endless loops
      #if ( ord(s[0]) == OPT_DONT ) send( socket:socket, data:raw_string( 0xff, OPT_WONT ) + s[1] );
      #if ( ord(s[0]) == OPT_WONT ) send( socket:socket, data:raw_string( 0xff, OPT_DONT ) + s[1] );
    }
    if( counter >= 100 || strlen( buf ) >= 4096 )
      break;
  }
  return buf;
}

# @brief Saves a passed Telnet banner into the internal KB (at telnet/banner/port)
#
# @param port   The port to save the banner for.
# @param banner The banner to save.
#
# @return TRUE if the banner was successfully saved within the KB and
#         NULL if no port or banner parameter was passed.
#
function telnet_set_banner( port, banner ) {

  local_var port, banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#telnet_set_banner" );
    return NULL;
  }

  if( ! banner ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#banner#-#telnet_set_banner" );
    return NULL;
  }

  replace_kb_item( name:"telnet/banner/" + port, value:str_replace( find:raw_string(0), replace:'', string:banner ) );
  return TRUE;
}

# @brief Returns all Telnet ports of the target system which are saved
#        in the KB entry "Services/telnet" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @note This function will fork on multiple open ports.
#
# @param default          The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault        Don't exit if no "default" parameter is passed, it still will return if no Telnet servers are detected at all.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found Telnet ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_telnet_port( default, nodefault, ignore_broken, ignore_unscanned ) {

  local_var default, nodefault, ignore_broken, ignore_unscanned;
  local_var port;

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_telnet_port" );
    exit( 0 );
  }

  port = get_kb_item( "Services/telnet" );
  if( port )
    default = port;

  # nb: If this is the case we haven't found an open port, default wasn't passed
  # and nodefault was set to TRUE. We can't continue in this case.
  if( ! default )
    exit( 0 );

  if( ! ignore_unscanned && ! get_port_state( default ) )
    exit( 0 );

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( telnet_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! ignore_broken && telnet_get_is_marked_broken( port:default ) )
    exit( 0 );

  return default;
}

# @brief Returns all Telnet ports of the target system which are saved
#        in the KB entry "Services/telnet" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of Telnet ports (namely 23 and 992)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note In contrast to @ref get_telnet_port this function will not fork
#       and return a list instead. This should be used e.g. for
#       Detection-VTs where we want to make sure that we're not getting
#       blocked by the remote service with to many concurrent connections.
#
# @param default_list     The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                         is set to "no" and no ports have been found open.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found Telnet ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function telnet_get_ports( default_list, ignore_broken, ignore_unscanned ) {

  local_var default_list, ignore_broken, ignore_unscanned;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 23, 992 );
  num_ports = 0;

  ports = get_kb_list( "Services/telnet" );
  if( ports && typeof( ports ) == "array" ) {
    foreach port( ports ) {
      num_ports++;
      check_port_list = make_list( check_port_list, port );
    }
  }

  if( num_ports == 0 ) {
    if( default_list && typeof( default_list ) == "array" )
      check_port_list = default_list;
    else
      check_port_list = default_ports;
  }

  foreach port( check_port_list ) {

    if( ! ignore_unscanned && ! get_port_state( port ) )
      continue;

    # Includes e.g. PJL ports which are printing everything
    # sent to them so don't include them here.
    if( telnet_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken && telnet_get_is_marked_broken( port:port ) )
      continue;

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Closes a passed TCP socket of a Telnet server. The function accepts an additional previously received
#        banner via the data parameter to decide if a final 'exit' needs to be sent before closing the socket.
#
# @param socket The socket to close.
# @param data   The data to check if a final 'exit' should be sent.
#
# @return NULL
#
function telnet_close_socket( socket, data ) {

  local_var socket, data;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#telnet_close_socket" );
    return;
  }

  # nb: Some devices need a separate exit to accept subsequent requests
  if( ( "Sollae Systems" >< data && ( data =~ "lsh>$" || data =~ "msh>$" || "password" >< data ) ) ||
    'Please type "menu" for the MENU system,' >< data || 'or "?" for help, or "/" for current settings' >< data || # nb: HP JetDirect
    "Welcome to NetLinx" >< data || "Local devices for system" >< data ) { # nb: Both for AMX NetLinx
    send( socket:socket, data:'exit\r\n' );
  }

  close( socket );
}

# @brief Returns the info if a remote Telnet server was internally (in the KB)
#        marked as "broken". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote Telnet server.
#
# @return TRUE if the remote Telnet server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function telnet_get_is_marked_broken( port ) {

  local_var port;
  local_var marked_broken_list, marked_broken;
  # nb: _telnet_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#telnet_get_is_marked_broken" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _telnet_is_broken_array[port] ) ) {
    if( _telnet_is_broken_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_broken = get_kb_item( "telnet/" + port + "/is_broken" );
  if( marked_broken ) {
    _telnet_is_broken_array[port] = TRUE;
  } else {
    _telnet_is_broken_array[port] = FALSE;
    marked_broken = FALSE;
  }
  return marked_broken;
}

# @brief Sets if a remote Telnet server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entries:
#        "telnet/" + port + "/is_broken" and "telnet/is_broken".
#
# @param port The port of the remote Telnet server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function telnet_set_is_marked_broken( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#telnet_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"telnet/is_broken", value:TRUE );
  set_kb_item( name:"telnet/" + port + "/is_broken", value:TRUE );
  _telnet_is_broken_array[port] = TRUE;
  return TRUE;
}

# @brief Verify if the received/passed data is related to a Telnet connection / server.
#
# @param data The data to be verified.
#
# @note You need to include misc_func.inc and dump.inc into your VT if you want to use this function.
#
# @return TRUE if the passed data is related to a Telnet connection / server, FALSE otherwise.
#
function telnet_verify_banner( data ) {

  local_var data;
  local_var nt;
  # nb: __no_telnet is a global list above

  data = chomp( data );
  if( ! data )
    return FALSE;

  if( strlen( data ) < 4 )
    return FALSE;

  data = bin2string( ddata:data, noprint_replacement:' ' );
  if( ! data || data =~ '^[ \r\n]*$' )
    return FALSE;

  foreach nt( __no_telnet ) {
    if( egrep( pattern:nt, string:data ) )
      return FALSE;
  }
  return TRUE;
}

# @brief Verify if the received/passed data contains a login/shell prompt of a Telnet connection / server.
#
# @param data The data to be verified.
#
# @note You need to include misc_func.inc and dump.inc (for @ref telnet_verify_banner) into your VT if you want to use this function.
# @note Pattern were extracted from e.g. w_apc_default_telnet_credentials.nasl, sw_zebra_telnet_default_password.nasl,
#       gb_zxv10_w300_hardcoded_credentials_2014.nasl, gb_audemat_fmb80_default_telnet_credentials.nasl,
#       gb_lantronix_unprotected_telnet.nasl, gb_windows_ce_unprotected_telnet.nasl, gb_netlinx_telnet_detect.nasl
#
# @return TRUE if the passed data contains a login/shell prompt of a Telnet connection / server, FALSE otherwise.
#
function telnet_has_login_prompt( data ) {

  local_var data;

  if( ! telnet_verify_banner( data:data ) )
    return FALSE;

  if( data !~ "(Pocket CMD.+\\>|Kernel|login|password|user ?name|user|press enter.+setup mode|polycom command shell|welcome to viewstation|hi, my name is.+here is what i know about myself|you are logged in|management console.+sollae systems|lsh>|Welcome\. Type <return>, enter password at # prompt|BusyBox|list of built-in commands|Welcome to NetLinx) ?:?" )
    return FALSE;

  return TRUE;
}

# Duplicated from misc_func.inc
function telnet_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#telnet_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}
