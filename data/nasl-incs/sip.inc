###############################################################################
# OpenVAS Include File
# $Id: sip.inc 13744 2019-02-18 15:31:13Z cfischer $
#
# functions to support sip
#
# Authors:
# Ferdy Riphagen <f.riphagen@nsec.nl>
#
# Copyright:
# Copyright (C) 2007 Ferdy Riphagen <f.riphagen@nsec.nl>
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
# $Revision: 13744 $
#
###############################################################################

# @brief Creates a SIP OPTIONS request, sends the request to the target, receives the
#        response and extracts the SIP header (banner) from the response. If a header
#        was received it is saved into the knowledge base. If a header was requested
#        which was requested previously the cached header will be returned instead of
#        doing a new request.
#
# @param port  The SIP port of the target.
# @param proto The (optional) IP Protocol ("tcp" or "udp") to be used. Defaults to "udp"
#              if not passed.
#
# @return The SIP header of the remote host, FALSE if no response or header/banner was received and NULL if no port parameter was passed.
#
function sip_get_banner( port, proto ) {

  local_var port, proto;
  local_var banner, opt, res, found, _banner;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#sip_get_banner" );
    return NULL;
  }

  if( ! proto )
    proto = "udp";

  banner = get_kb_item( "sip/banner/" + proto + "/" + port );
  if( banner )
    return banner;

  if( proto == "tcp" ) {
    if( ! get_tcp_port_state( port ) )
      return FALSE;
  } else {
    if( ! get_udp_port_state( port ) )
      return FALSE;
  }

  soc = sip_open_socket( port:port, proto:proto );
  if( ! soc )
    return FALSE;

  opt = sip_construct_options_req( port:port, proto:proto );
  send( socket:soc, data:opt );
  res = recv( socket:soc, length:1024 );

  if( res && sip_verify_banner( data:res ) )
    found = TRUE;

  if( ! found ) {
    opt = sip_construct_options_req( port:port, proto:proto, add_branch_rport:TRUE );
    send( socket:soc, data:opt );
    res = recv( socket:soc, length:1024 );
    if( res && sip_verify_banner( data:res ) ) {
      found = TRUE;
    }
  }

  if( ! found ) {
    # Found on the IceWarp Suite (but there might be more similar products). This is a SIP service which isn't responding to our
    # SIP OPTIONS request above (and in find_service5.nasl for UDP) so try HTTP GET again (see find_service1.nasl) to get the full banner
    # (yes, this seems to be working for UDP as well...)
    #
    # Similar happens for recent firmware versions of the AVM FRITZ!OS which responses with something like (see find_service1.nasl):
    # SIP/2.0 400 Illegal request line
    # From: <sip:missing>
    # To: <sip:missing>;tag=badrequest
    # User-Agent: FRITZ!OS
    # Content-Length: 0
    send( socket:soc, data:'GET / HTTP/1.0\r\n\r\n' );
    res = recv( socket:soc, length:1024 );
    if( res && sip_verify_banner( data:res ) )
      found = TRUE;
  }

  close( soc );

  if( ! found )
    return FALSE;

  replace_kb_item( name:"sip/full_banner/" + proto + "/" + port, value:chomp( res ) );

  if( res =~ "Server: " ) {
    _banner = egrep( pattern:"^Server: ", string:res, icase:TRUE );
    _banner = substr( _banner, 8 );
    _banner = chomp( _banner );

    if( _banner ) {
      set_kb_item( name:"sip/server_banner/" + proto + "/" + port, value:_banner );
      banner = _banner;
    }
  }

  if( res =~ "User-Agent: " ) {
    _banner = egrep( pattern:"^User-Agent: ", string:res, icase:TRUE );
    _banner = substr( _banner, 12 );
    _banner = chomp( _banner );

    if( _banner ) {
      if( banner )
        banner += '\n';
      set_kb_item( name:"sip/useragent_banner/" + proto + "/" + port, value:_banner );
      banner = _banner;
    }
  }

  if( res =~ "Allow:.*OPTIONS.*" ) {
    _banner = egrep( pattern:"^Allow:.*OPTIONS.*", string:res, icase:TRUE );
    _banner = substr( _banner, 7 );
    _banner = chomp( _banner );

    if( _banner )
      set_kb_item( name:"sip/options_banner/" + proto + "/" + port, value:_banner );
  }

  if( banner ) {
    replace_kb_item( name:"sip/banner/" + proto + "/" + port, value:banner );
    return banner;
  } else {
    return FALSE;
  }
}

# @brief Sends a SIP packet to the remote service and returns the received response.
#
# @param port  The SIP port of the target.
# @param data  The request to sent (e.g. created via @ref sip_construct_options_req).
# @param proto The (optional) IP Protocol ("tcp" or "udp") to be used. Defaults to "udp"
#              if not passed.
#
# @return The received response, FALSE if no response was received and NULL if no port parameter was passed.
#
function sip_send_recv( port, data, proto ) {

  local_var port, data, proto;
  local_var soc, res;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#sip_send_recv" );
    return NULL;
  }

  if( ! data )
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#sip_send_recv" );

  if( ! proto )
    proto = "udp";

  soc = sip_open_socket( port:port, proto:proto );
  if( ! soc )
    return FALSE;

  send( socket:soc, data:data );
  res = recv( socket:soc, length:1024 );
  close( soc );

  res = chomp( res );

  if( res )
    return res;
  else
    return FALSE;
}

# @brief Sends a SIP packet to the remote service and tries to check if the service is still alive.
#
# @param port  The SIP port of the target.
# @param proto The (optional) IP Protocol ("tcp" or "udp") to be used. Defaults to "udp"
#              if not passed.
# @param retry The mount of retries to use. Defaults to 2 if not passed.
#
# @return TRUE if the service is still alive, FALSE otherwise and NULL if no port parameter was passed.
#
function sip_alive( port, proto, retry ) {

  local_var port, proto, retry;
  local_var i, soc, res, opt;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#sip_alive" );
    return NULL;
  }

  if( ! proto ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#sip_alive" );
    proto = "udp";
  }

  if( ! retry )
    retry = 2;

  i = 0;

  soc = sip_open_socket( port:port, proto:proto );

  while( ! soc && i++ < retry ) {
    sleep( 1 );
    soc = sip_open_socket( port:port, proto:proto );
  }

  if( ! soc )
    return FALSE;

  opt = sip_construct_options_req( port:port, proto:proto );

  send( socket:soc, data:opt );
  res = recv( socket:soc, length:1024 );
  close( soc );

  if( ! res )
    return FALSE;

  if( "SIP/2.0" >< res )
    return TRUE;
  else
    return FALSE;
}

# @brief Creates a SIP OPTIONS requests.
#
# @param port             The SIP port of the target. Defaults to "5060" if not passed.
# @param proto            The (optional) IP Protocol ("tcp" or "udp") to be used. Defaults to "udp"
#                         if not passed.
# @param add_branch_rport If set to TRUE the rport (RFC3581) and branch variables are passed to the OPTIONS request.
#
# @return The created SIP OPTIONS request as a string.
#
function sip_construct_options_req( port, proto, add_branch_rport ) {

  local_var port, proto, add_branch_rport;
  local_var ext, opt;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#sip_construct_options_req" );
    port = "5060";
  }

  if( ! proto ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#sip_construct_options_req" );
    proto = "udp";
  }

  # https://tools.ietf.org/html/rfc3581
  if( add_branch_rport )
    ext = ";branch=openvas;rport";

  opt = "OPTIONS sip:openvas@" + get_host_name() + ":" + port + " SIP/2.0" + '\r\n' +
        "Via: SIP/2.0/" + toupper( proto ) + " " + this_host() + ":" + port + ext + '\r\n' +
        "Max-Forwards: 70" + '\r\n' +
        "To: <sip:openvas@" + get_host_name() + ":" + port + ">" + '\r\n' +
        "From: OpenVAS <sip:openvas@" + this_host() + ":" + port + ">;tag=" + rand() + '\r\n' +
        "Call-ID: " + rand() + '\r\n' +
        "CSeq: 63104 OPTIONS" + '\r\n' +
        "Contact: <sip:openvas@" + this_host() + ":" + port + ">" + '\r\n' +
        "Accept: application/sdp" + '\r\n' +
        "Content-Length: 0" + '\r\n\r\n';

  return opt;
}

# @brief Opens a connection to a SIP server and returns the opened socket.
#
# @param port  The SIP port of the target.
# @param proto The (optional) IP Protocol ("tcp" or "udp") to be used. Defaults to "udp"
#              if not passed.
#
# @param The opened socket, FALSE in the case no socket could be opened or the port wasn't
#        scanned previously and NULL if no port parameter was passed.
#
function sip_open_socket( port, proto ) {

  local_var port, proto;
  local_var soc;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#sip_open_socket" );
    return NULL;
  }

  if( ! proto ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#sip_open_socket" );
    proto = "udp";
  }

  if( proto == "tcp" ) {
    if( ! get_port_state( port ) )
      return FALSE;

    soc = open_sock_tcp( port );
    if( ! soc )
      return FALSE;
  } else {
    if( ! get_udp_port_state( port ) )
      return FALSE;

    if( islocalhost() ) {
      soc = open_sock_udp( port );
    } else {
      soc = open_priv_sock_udp( sport:5060, dport:port );
    }
    if( ! soc )
      return FALSE;
  }
  return soc;
}

# @brief Returns all open/detected ports related to a SIP service. If none has been found the function will exit.
#
# @param default_port  The default port to fall back if no open/detected ports where found.
#                      Defaults to "5060" if not passed.
# @param default_proto The default protocol to fall back if no open/detected ports where found.
#                      Defaults to "udp" if not passed.
#
# @return An array containing the following array indices:
#
#         ret_array["port"]  = The port of the SIP service.
#         ret_array["proto"] = The protocol of the SIP service.
#
function sip_get_port_proto( default_port, default_proto ) {

  local_var default_port, default_proto;
  local_var port_and_proto, x, x_port, x_proto, ret_arr;

  if( ! default_port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default_port#-#sip_get_port_proto" );
    default_port = "5060";
  }

  if( ! default_proto ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default_proto#-#sip_get_port_proto" );
    default_proto = "udp";
  }

  port_and_proto = get_kb_item( "sip/port_and_proto" ); # this could fork, syntax e.g.: 5060#-#udp
  if( port_and_proto ) {
    x = split( port_and_proto, sep:"#-#", keep:FALSE );
    x_port  = x[0];
    x_proto = x[1];
  } else {
    x_port  = default_port;
    x_proto = default_proto;
  }

  if( x_proto == "udp" ) {
    if( ! get_udp_port_state( x_port ) )
      exit( 0 );
  } else {
    if( ! get_port_state( x_port ) )
      exit( 0 );

    # Includes e.g. PJL ports which are printing everything
    # sent to them so exit for such a port here
    if( sip_is_fragile_port( port:x_port ) )
      exit( 0 );
  }

  ret_arr['port']  = x_port;
  ret_arr['proto'] = x_proto;
  return ret_arr;
}

# @brief Verify if the received/passed data is related to a SIP connection / server.
#
# @param data The data to be verified.
#
# @note See find_service1.nasl, find_service5.nasl and @ref sip_get_banner for some examples and additional notes.
#
# @return TRUE if the passed data is related to a SIP connection / server, FALSE otherwise.
#
function sip_verify_banner( data ) {

  local_var data;

  data = chomp( data );
  if( ! data || strlen( data ) < 11 )
    return FALSE;

  # nb: Don't use the more detailed "^Via: SIP/2\.0/TCP" as there seems to be some services answering just with something like e.g. "Via: ;"
  # nb: Also don't use a space after the doubled point as some services are responding with something like:
  # User-Agent:Mitel-5304-SIP-Phone 06.05.00.06 08000FA9F751
  # From:<sip:nm@nm>;tag=root
  if( data =~ "^SIP/2\.0 [0-9]{3}" && egrep( string:data, pattern:"^(Via|From|To|User-Agent|Allow|Contact):", icase:TRUE ) )
    return TRUE;

  return FALSE;
}

# Duplicated from misc_func.inc
function sip_is_fragile_port( port ) {

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
