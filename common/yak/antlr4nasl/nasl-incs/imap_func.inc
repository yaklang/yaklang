###############################################################################
# OpenVAS Include File
# $Id: imap_func.inc 13819 2019-02-21 17:16:13Z cfischer $
#
# Support methods for IMAP
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
# $Revision: 13819 $
#
###############################################################################

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var _imap_is_broken_array, __imap_kb_login, __imap_kb_pass;

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if broken or not
_imap_is_broken_array = make_array();

# @brief Opens a connection to an IMAP server and receives the returned banner. If a banner was received it
#        is saved into the knowledge base. If a banner for a port was requested which was requested
#        previously the cached banner will be returned instead of doing a new connection. This function
#        is also sending additional "ID", "CAPABILITY", "NOOP", "UNKNOWNCMD" and "LOGOUT" commands to the
#        IMAP server and and saving the responses to the KB for later use in the fingerprinting of the
#        remote IAMP server by other VTs.
#
# @param port The port of the remote IMAP server.
#
# @return The banner of the remote host, NULL if no response was received or the remote port wasn't scanned
#         and FALSE if no port parameter was passed.
#
function get_imap_banner( port ) {

  local_var port;
  local_var banner, soc, tag, fpbanner, id, is_tls, capas, capa;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_imap_banner" );
    return FALSE;
  }

  # nb: nasl_builtin_find_service.c is setting imap/banner but we want to catch responses of
  # various requests below as well so we're using the uppercase IMAP here.
  banner = get_kb_item( "IMAP/banner/" + port );
  if( banner )
    return( banner );

  if( ! get_port_state( port ) )
    return NULL;

  if( imap_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc ) {
    imap_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = recv_line( socket:soc, length:4096 );
  if( ! imap_verify_banner( data:banner ) ) {
    close( soc );
    imap_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = chomp( banner );
  tag = 1; # nb: The start tag which needs to be increased after each sent command.

  # https://tools.ietf.org/html/rfc2971
  # Request to get a version for e.g. Cyrus-IMAP:
  # 01 ID ("name" "MailMate" "version" "r3255" "vendor" "Freron Software" "contact" "imap-id@freron.com")
  # Responses are e.g.
  # ID ("name" "dbmail" "version" "0.9.9" "os" "Linux" "os-version" "3.2.12-gentoo")
  # ID ("name", "Bigfoot", "version", "1.0", "os", "Linux", "os-version", "2.6",
  # nb: A few systems / implementation are replying with a "NO Only one Id allowed in non-authenticated state"
  # until an authenticated request is done in between. So this is not absolutely reliable.
  send( socket:soc, data:'A0' + tag + ' ID ("name" "OpenVAS" "version" "' + OPENVAS_VERSION + '" "vendor" "OpenVAS" "contact" "openvas@example.com")\r\n' );
  fpbanner = recv( socket:soc, length:4096 );
  fpbanner = chomp( fpbanner );

  # nb: Some IMAP servers (e.g. Microsoft Exchange 2013) are responding with a "A01 BAD ID failed" or "A01 BAD Invalid command (ID)"
  # but answering with a ID NIL (RFC 2971 says client can send both) so we're trying both to make sure that catch both variants.
  if( "A0" + tag + " BAD" >< fpbanner ) {
    tag++;
    send( socket:soc, data:'A0' + tag + ' ID NIL\r\n' );
    fpbanner = recv( socket:soc, length:4096 );
    fpbanner = chomp( fpbanner );
  }

  tag++;

  if( fpbanner )
    set_kb_item( name:"imap/fingerprints/" + port + "/id_banner", value:fpbanner );

  id = egrep( string:fpbanner, pattern:"\* ID .+", icase:TRUE );
  if( id && "ID NIL" >!< id )
    banner += '\n' + chomp( id );

  send( socket:soc, data:'A0' + tag + ' CAPABILITY\r\n' );
  fpbanner = recv( socket:soc, length:4096 );
  fpbanner = chomp( fpbanner );
  tag++;
  if( fpbanner ) {

    set_kb_item( name:"imap/fingerprints/" + port + "/capa_banner", value:fpbanner );

    if( get_port_transport( port ) > ENCAPS_IP ) {
      set_kb_item( name:"imap/fingerprints/" + port + "/tls_capa_banner", value:fpbanner );
      is_tls = TRUE;
    } else {
      set_kb_item( name:"imap/fingerprints/" + port + "/nontls_capa_banner", value:fpbanner );
      is_tls = FALSE;
    }


    # https://tools.ietf.org/html/rfc3501#page-24
    capas = egrep( string:fpbanner, pattern:"\* CAPABILITY.+IMAP4rev1", icase:TRUE );
    capas = chomp( capas );
    if( capas ) {
      capas = split( capas, sep:" ", keep:FALSE );
      foreach capa( capas ) {

        if( capa == "*" || capa == "CAPABILITY" || capa == "IMAP4rev1" )
          continue;

        set_kb_item( name:"imap/fingerprints/" + port + "/capalist", value:capa );

        if( is_tls )
          set_kb_item( name:"imap/fingerprints/" + port + "/tls_capalist", value:capa );
        else
          set_kb_item( name:"imap/fingerprints/" + port + "/nontls_capalist", value:capa );
      }
    }
  }

  send( socket:soc, data:'A0' + tag + ' NOOP\r\n' );
  fpbanner = recv( socket:soc, length:4096 );
  fpbanner = chomp( fpbanner );
  tag++;
  if( fpbanner )
    set_kb_item( name:"imap/fingerprints/" + port + "/noop_banner", value:fpbanner );

  send( socket:soc, data:'A0' + tag + ' UNKNOWNCMD\r\n' );
  fpbanner = recv( socket:soc, length:4096 );
  fpbanner = chomp( fpbanner );
  tag++;
  if( fpbanner )
    set_kb_item( name:"imap/fingerprints/" + port + "/unknowncmd_banner", value:fpbanner );

  # nb: Don't use imap_close_socket() as we want to catch the LOGOUT banner as well.
  send( socket:soc, data:'A0' + tag + ' LOGOUT\r\n' );
  fpbanner = recv( socket:soc, length:4096 );
  fpbanner = chomp( fpbanner);
  if( fpbanner )
    set_kb_item( name:"imap/fingerprints/" + port + "/logout_banner", value:fpbanner );

  close( soc );
  replace_kb_item( name:"IMAP/banner/" + port, value:banner );
  return banner;
}

# @brief Returns all IMAP ports of the target system which are saved
#        in the KB entry "Services/imap" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @note This function will fork on multiple open ports.
#
# @param default          The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault        Don't exit if no "default" parameter is passed, it still will return if no IMAP servers are detected at all.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found IMAP ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_imap_port( default, nodefault, ignore_broken, ignore_unscanned ) {

  local_var default, nodefault, ignore_broken, ignore_unscanned;
  local_var port;

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_imap_port" );
    exit( 0 );
  }

  port = get_kb_item( "Services/imap" );
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
  if( imap_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! ignore_broken && imap_get_is_marked_broken( port:default ) )
    exit( 0 );

  return default;
}

# @brief Returns all IMAP ports of the target system which are saved
#        in the KB entry "Services/imap" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of IMAP ports (namely 143 and 993)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note  In contrast to @ref get_imap_port this function will not fork
#        and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting
#        blocked by the remote service with to many concurrent connections.
#
# @param default_list     The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                         is set to "no" and no ports have been found open.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found IMAP ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function imap_get_ports( default_list, ignore_broken, ignore_unscanned ) {

  local_var default_list, ignore_broken, ignore_unscanned;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 143, 993 );
  num_ports = 0;

  ports = get_kb_list( "Services/imap" );
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
    if( imap_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken && imap_get_is_marked_broken( port:port ) )
      continue;

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Tries to open a TCP connection to a IMAP server. If the remote server is a IMAP server and the expected
#        response was received the opened socket is returned.
#
# @param port The port of the remote IMAP server.
#
# @return The opened socket or NULL if no connection could be opened, no or unexpected data was
#         received or no port parameter was passed.
#
function imap_open_socket( port ) {

  local_var port;
  local_var soc, banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#imap_open_socket" );
    return NULL;
  }

  if( imap_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc )
    return NULL;

  banner = recv_line( socket:soc, length:4096 );
  if( ! imap_verify_banner( data:banner ) ) {
    close( soc );
    return NULL;
  }
  return soc;
}

# @brief Closes a passed TCP socket of a IMAP server by first sending a QUIT message to the service.
#
# @param socket The socket to close.
# @param id     The current ID (required) used by the calling VT which is prepended to the LOGOUT message
#               example: "05" is getting "05 LOGOUT"
#
# @return NULL
#
function imap_close_socket( socket, id ) {

  local_var socket;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#imap_close_socket" );
    return;
  }

  if( ! id ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#imap_close_socket" );
    return;
  }

  send( socket:socket, data:id + ' LOGOUT\r\n' );
  recv_line( socket:socket, length:4096 );
  close( socket );
}

# @brief Returns the info if a remote IMAP server was internally (in the KB)
#        marked as "broken". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote IMAP server.
#
# @return TRUE if the remote IMAP server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function imap_get_is_marked_broken( port ) {

  local_var port;
  local_var marked_broken_list, marked_broken;
  # nb: _imap_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#imap_get_is_marked_broken" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _imap_is_broken_array[port] ) ) {
    if( _imap_is_broken_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_broken = get_kb_item( "imap/" + port + "/is_broken" );
  if( marked_broken ) {
    _imap_is_broken_array[port] = TRUE;
  } else {
    _imap_is_broken_array[port] = FALSE;
    marked_broken = FALSE;
  }
  return marked_broken;
}

# @brief Sets if a remote IMAP server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entries:
#        "imap/" + port + "/is_broken" and "imap/is_broken".
#
# @param port The port of the remote IMAP server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function imap_set_is_marked_broken( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#imap_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"imap/is_broken", value:TRUE );
  set_kb_item( name:"imap/" + port + "/is_broken", value:TRUE );
  _imap_is_broken_array[port] = TRUE;
  return TRUE;
}

# @brief Verify if the received/passed data is related to a IMAP connection / server.
#
# @param data The data to be verified.
#
# @return TRUE if the passed data is related to a IMAP connection / server, FALSE otherwise.
#
function imap_verify_banner( data ) {

  local_var data;

  data = chomp( data );
  if( ! data )
    return FALSE;

  if( eregmatch( pattern:"^\* OK", string:data, icase:FALSE ) || "IMAP4rev1" >< data )
    return TRUE;

  return FALSE;
}

# @brief Returns an array containing the username/password credentials pair for a IMAP login.
#        To avoid querying the KB for the same info again it will be cached within an internal variable.
#
# @return An array containing the following array indices:
#
#         ret_array["login"] = The login defined via the "IMAP account" preference of logins.nasl,
#                              an empty string in the case the VT wasn't included in the scan config or
#                              the preference was unset.
#         ret_array["pass"]  = The password defined via the "IMAP password (sent in clear)" preference of
#                              logins.nasl, an empty string in the case the VT wasn't included in the
#                              scan config or the preference has been cleared.
#
# @note Please include logins.nasl (or any VT pulling this dependency in) into your
#       script_dependencies so that a user configured user account is taken into account.
#
function imap_get_kb_creds() {

  local_var login, pass, ret_array;
  # nb: __imap_kb_login and __imap_kb_pass are global vars

  if( ! isnull( __imap_kb_login ) ) {
    login = string( __imap_kb_login );
  } else {
    login = get_kb_item( "imap/login" );
    if( isnull( login ) )
      login = "";
    __imap_kb_login = string( login );
  }

  if( ! isnull( __imap_kb_pass ) ) {
    pass = string( __imap_kb_pass );
  } else {
    pass = get_kb_item( "imap/password" );
    if( isnull( pass ) )
      pass = "";
    __imap_kb_pass = string( pass );
  }

  ret_array["login"] = login;
  ret_array["pass"]  = pass;
  return ret_array;
}

# Duplicated from misc_func.inc
function imap_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#imap_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}