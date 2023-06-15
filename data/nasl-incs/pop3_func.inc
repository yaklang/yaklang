###############################################################################
# OpenVAS Include File
# $Id: pop3_func.inc 13818 2019-02-21 16:26:49Z cfischer $
#
# Methods for handling POP3
#
# Authors:
# Anonymous
#
# Copyright:
# Copyright (C) 2004 Anonymous
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
# $Revision: 13818 $
#
###############################################################################

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var _pop3_is_broken_array, __pop3_kb_login, __pop3_kb_pass;

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if broken or not
_pop3_is_broken_array = make_array();

# @brief Opens a connection to a POP3 server and receives the returned banner. If a banner was received it
#        is saved into the knowledge base. If a banner for a port was requested which was requested
#        previously the cached banner will be returned instead of doing a new connection. This function
#        is also sending additional "CAPA" and "QUIT" commands to the POP3 server and saving the responses
#        to the KB for later use in the fingerprinting of the remote POP3 server by other VTs.
#
# @param port The port of the remote POP3 server.
#
# @return The banner of the remote host, NULL if no response was received or the remote port wasn't scanned
#         and FALSE if no port parameter was passed.
#
function get_pop3_banner( port ) {

  local_var port;
  local_var banner, soc, is_tls, capabanner, n, implbanner, quitbanner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_pop3_banner" );
    return FALSE;
  }

  # nb: nasl_builtin_find_service.c is setting pop3/banner but we want to catch the CAPA response
  # as well so we're using the uppercase POP3 here.
  banner = get_kb_item( "POP3/banner/" + port );
  if( banner )
    return banner;

  if( ! get_port_state( port ) )
    return NULL;

  if( pop3_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc ) {
    pop3_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = recv_line( socket:soc, length:4096 );
  if( ! pop3_verify_banner( data:banner ) ) {
    close( soc );
    pop3_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = chomp( banner );

  if( get_port_transport( port ) > ENCAPS_IP )
    is_tls = TRUE;
  else
    is_tls = FALSE;

  # nb: This part is used to catch the software from the "CAPA" request.
  # e.g. the initial banner is only: POP3 banner is +OK POP3 server ready <example@example.com>
  # but the CAPA request is exposing the software version: IMPLEMENTATION Smartertools_SmarterMail_1.5
  # RFC: https://tools.ietf.org/html/rfc2449#page-13
  send( socket:soc, data:'CAPA\r\n' );
  capabanner = recv_line( socket:soc, length:4096 );
  capabanner = chomp( capabanner );
  if( capabanner ) {
    set_kb_item( name:"pop3/fingerprints/" + port + "/capa_banner", value:capabanner );
    if( is_tls )
      set_kb_item( name:"pop3/fingerprints/" + port + "/tls_capa_banner", value:capabanner );
    else
      set_kb_item( name:"pop3/fingerprints/" + port + "/nontls_capa_banner", value:capabanner );
  }

  # nb: Keep in sync with gb_starttls_pop3.nasl
  if( capabanner == "+OK" || "capability list follows" >< tolower( capabanner ) || "List of capabilities follows" >< tolower( capabanner ) ||
      "capa list follows" >< tolower( capabanner ) || "list follows" >< capabanner || "Here's what I can do" >< capabanner ) {
    while( capabanner = recv_line( socket:soc, length:4096 ) ) {
      n++;
      implbanner = egrep( pattern:"IMPLEMENTATION .*", string:capabanner );
      implbanner = chomp( implbanner );
      if( implbanner ) {
        set_kb_item( name:"pop3/fingerprints/" + port + "/impl_banner", value:implbanner );
        banner += '\n' + implbanner;
      }
      capabanner = chomp( capabanner );
      if( capabanner && capabanner != "." ) {

        set_kb_item( name:"pop3/fingerprints/" + port + "/capalist", value:capabanner );

        if( is_tls )
          set_kb_item( name:"pop3/fingerprints/" + port + "/tls_capalist", value:capabanner );
        else
          set_kb_item( name:"pop3/fingerprints/" + port + "/nontls_capalist", value:capabanner );
      }

      if( n > 256 ) # nb: Too much data...
        break;
    }
  }

  # nb: Don't use pop3_close_socket as we want to catch the QUIT banner as well.
  send( socket:soc, data:'QUIT\r\n' );
  quitbanner = recv_line( socket:soc, length:4096 );
  quitbanner = chomp( quitbanner );
  if( quitbanner )
    set_kb_item( name:"pop3/fingerprints/" + port + "/quit_banner", value:quitbanner );

  close( soc );
  replace_kb_item( name:"POP3/banner/" + port, value:banner );
  return banner;
}

# @brief Returns all POP3 ports of the target system which are saved
#        in the KB entry "Services/pop3" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @note This function will fork on multiple open ports.
#
# @param default          The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault        Don't exit if no "default" parameter is passed, it still will return if no POP3 servers are detected at all.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found POP3 ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_pop3_port( default, nodefault, ignore_broken, ignore_unscanned ) {

  local_var default, nodefault, ignore_broken, ignore_unscanned;
  local_var port;

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_pop3_port" );
    exit( 0 );
  }

  port = get_kb_item( "Services/pop3" );
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
  if( pop3_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! ignore_broken && pop3_get_is_marked_broken( port:default ) )
    exit( 0 );

  return default;
}

# @brief Returns all POP3 ports of the target system which are saved
#        in the KB entry "Services/pop3" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of POP3 ports (namely 110 and 995)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note  In contrast to @ref get_pop3_port this function will not fork
#        and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting
#        blocked by the remote service with to many concurrent connections.
#
# @param default_list     The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                         is set to "no" and no ports have been found open.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found POP3 ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function pop3_get_ports( default_list, ignore_broken, ignore_unscanned ) {

  local_var default_list, ignore_broken, ignore_unscanned;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 110, 995 );
  num_ports = 0;

  ports = get_kb_list( "Services/pop3" );
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
    if( pop3_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken && pop3_get_is_marked_broken( port:port ) )
      continue;

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Tries to open a TCP connection to a POP3 server. If the remote server is a POP3 server and the expected
#        response was received the opened socket is returned.
#
# @param port The port of the remote POP3 server.
#
# @return The opened socket or NULL if no connection could be opened, no or unexpected data was
#         received or no port parameter was passed.
#
function pop3_open_socket( port ) {

  local_var port;
  local_var soc, banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#pop3_open_socket" );
    return NULL;
  }

  if( pop3_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc )
    return NULL;

  banner = recv_line( socket:soc, length:2048 );
  if( ! pop3_verify_banner( data:banner ) ) {
    close( soc );
    return NULL;
  }
  return soc;
}

# @brief Closes a passed TCP socket of a POP3 server by first sending a QUIT message to the service.
#
# @param socket The socket to close.
#
# @return NULL
#
function pop3_close_socket( socket ) {

  local_var socket;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#pop3_close_socket" );
    return;
  }

  send( socket:socket, data:'QUIT\r\n' );
  recv_line( socket:socket, length:4096 );
  close( socket );
}

# @brief Returns the info if a remote POP3 server was internally (in the KB)
#        marked as "broken". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote POP3 server.
#
# @return TRUE if the remote POP3 server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function pop3_get_is_marked_broken( port ) {

  local_var port;
  local_var marked_broken_list, marked_broken;
  # nb: _pop3_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#pop3_get_is_marked_broken" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _pop3_is_broken_array[port] ) ) {
    if( _pop3_is_broken_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_broken = get_kb_item( "pop3/" + port + "/is_broken" );
  if( marked_broken ) {
    _pop3_is_broken_array[port] = TRUE;
  } else {
    _pop3_is_broken_array[port] = FALSE;
    marked_broken = FALSE;
  }
  return marked_broken;
}

# @brief Sets if a remote POP3 server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entries:
#        "pop3/" + port + "/is_broken" and "pop3/is_broken".
#
# @param port The port of the remote POP3 server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function pop3_set_is_marked_broken( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#pop3_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"pop3/is_broken", value:TRUE );
  set_kb_item( name:"pop3/" + port + "/is_broken", value:TRUE );
  _pop3_is_broken_array[port] = TRUE;
  return TRUE;
}

# @brief Verify if the received/passed data is related to a POP3 connection / server.
#
# @param data The data to be verified.
#
# @return TRUE if the passed data is related to a POP3 connection / server, FALSE otherwise.
#
function pop3_verify_banner( data ) {

  local_var data;

  data = chomp( data );
  if( ! data )
    return FALSE;

  if( eregmatch( pattern:"^\+OK ", string:data, icase:FALSE ) || " POP3" >< data )
    return TRUE;

  return FALSE;
}

# @brief Returns an array containing the username/password credentials pair for a POP3 login.
#        To avoid querying the KB for the same info again it will be cached within an internal variable.
#
# @return An array containing the following array indices:
#
#         ret_array["login"] = The login defined via the "POP3 account" preference of logins.nasl,
#                              an empty string in the case the VT wasn't included in the scan config or
#                              the preference was unset.
#         ret_array["pass"]  = The password defined via the "POP3 password (sent in clear)" preference of
#                              logins.nasl, an empty string in the case the VT wasn't included in the
#                              scan config or the preference has been cleared.
#
# @note Please include logins.nasl (or any VT pulling this dependency in) into your
#       script_dependencies so that a user configured user account is taken into account.
#
function pop3_get_kb_creds() {

  local_var login, pass, ret_array;
  # nb: __pop3_kb_login and __pop3_kb_pass are global vars

  if( ! isnull( __pop3_kb_login ) ) {
    login = string( __pop3_kb_login );
  } else {
    login = get_kb_item( "pop3/login" );
    if( isnull( login ) )
      login = "";
    __pop3_kb_login = string( login );
  }

  if( ! isnull( __pop3_kb_pass ) ) {
    pass = string( __pop3_kb_pass );
  } else {
    pass = get_kb_item( "pop3/password" );
    if( isnull( pass ) )
      pass = "";
    __pop3_kb_pass = string( pass );
  }

  ret_array["login"] = login;
  ret_array["pass"]  = pass;
  return ret_array;
}

# Duplicated from misc_func.inc
function pop3_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#pop3_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }
  return;
}