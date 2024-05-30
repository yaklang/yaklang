###############################################################################
# OpenVAS Include File
#
# Miscellaneous support methods
#
# Authors:
# Michel Arboi <arboi@alussinan.org>
#
# Veerendra G.G <veerendragg@secpod.com>
# Added new construct_rpc_packet() function to construct rpc packet
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
###############################################################################

# Enables debugging output for a few functions below
global_var _misc_func_debug;
_misc_func_debug = 0;

# Global var to avoid that we're repeatedly asking redis for something we already know.
global_var __3rdparty_domain;

# @brief This function is deprecated.
#
function get_unknown_svc() {

  local_var port;

  if( ! isnull( _FCT_ANON_ARGS[0] ) ) {
    port = _FCT_ANON_ARGS[0];
  } else {
    port = get_kb_item( "Services/unknown" );
  }

  if( ! port ) return 0;
  if( port == 139 ) return 0;

  if( service_is_unknown( port:port ) ) {
    return port;
  } else {
    return 0;
  }
}

function register_service( port, proto, ipproto, message ) {

  local_var k, port, proto, ipproto, message;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#register_service" );
  if( ! proto ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#register_service" );

  if( ! ipproto) ipproto = "tcp";
  if( ! service_is_unknown( port:port, ipproto:ipproto ) ) {
    if( _misc_func_debug ) display(get_host_ip(), ": service is already known on port ", ipproto, ":", port, "\n");
    #return(0);
  }

  if( ipproto != "unknown" ) {

   k = strcat( "Known/", ipproto, "/", port );
   replace_kb_item( name:k, value:proto );
   if( ipproto == "tcp" ) {
     k = strcat( "Services/", proto );
   } else {
     k = strcat( "Services/", ipproto, "/", proto );
   }
   set_kb_item( name:k, value:port );

   register_service_as_host_detail( port:port, proto:ipproto, service:proto, message:message );

  }
  if( _misc_func_debug ) display(get_host_ip(), ": register_service: port=", port, ", proto=", proto, "\n");
}

# @brief Internal function to be called by @register_service only
#
function register_service_as_host_detail( port, proto, service, message ) {

  local_var port, proto, service, message, hd;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#register_service_as_host_detail" );
    return;
  }

  if( ! service ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#service#-#register_service_as_host_detail" );
    return;
  }

  if( ! proto )
    proto = 'tcp';

  # "623, udp, ipmi,'An IPMI service is running at this port. Supported IPMI version(s): v1.5, v2.0\n'"
  hd = port + ',' + proto + ',' + service;

  if( message )
    hd += ',' + message;

  register_host_detail( name:'Services', value:hd, desc:"Service detection (" + get_script_oid() + ")");

  return;
}

# This function may fork!
function known_service( port, ipproto ) {

  local_var k, p, port, ipproto;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#known_service" );

  if( ! ipproto ) ipproto = "tcp";
  k = strcat( "Known/", ipproto, "/", port );
  p = get_kb_item( k );
  #if (p) { display("Known service on port ", port, "\n"); }
  #else { display("Unknown service on port ", port, "\n"); }
  return p;
}

# This function does not fork!
function service_is_unknown( port, ipproto ) {

  local_var k, p, port, ipproto;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#service_is_unknown" );

  if( ! ipproto ) ipproto = "tcp";
  k = strcat( "Known/", ipproto, "/", port );
  p = get_kb_list( k );
  if( isnull( p ) ) return TRUE;
  foreach k( p ) {
    if( k != "unknown" ) { # fool proof
      return FALSE;
    }
  }
  return TRUE;
}

function verify_service( port, ipproto, proto ) {

  local_var k, p, port, ipproto, proto;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#verify_service" );
  if( ! proto ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#verify_service" );

  if( ! ipproto ) ipproto = "tcp";
  k = strcat( "Known/", ipproto, "/", port );
  p = get_kb_list( k );
  foreach k( p ) {
    if( k == proto ) {
      return TRUE;
    }
  }
  return FALSE;
}

# @brief Returns all ports matching the specified IP and service protocol passed via the "ipproto" and "proto" parameters. The function will evaluate
#        the following KB keys for this information:
#
#        "Services/proto" or "Services/ipproto/proto" and "Known/ipproto/default"
#
#        If no ports where found for the specified IP and service protocol or no "default" parameter is passed (if "nodefault" isn't set to TRUE)
#        it will exit. In the case where the "unscanned_closed/unscanned_closed_udp" scanner setting is set to "no" it will return the port passed
#        via the "default" parameter.
#
# @param default   The port to return if "unscanned_closed" is set to "no" and no ports have been found for the specified IP and service protocol.
# @param nodefault Don't exit if no "default" parameter is passed, it is still exiting if no ports matching the passed info were detected.
# @param ipproto   An (optional) IP protocol (e.g. "tcp" or "udp") to evaluate. If this parameter isn't passed it will default to "tcp".
# @param proto     A (mandatory) service protocol (e.g. "www" for HTTP were Services/www is evaluated).
#
# @note This function may fork if multiple ports are found open for the same service.
#
# @return The open / found ports matching the specific IP and service protocol or the port passed via the default parameter
#         in the case of no found ports and "unscanned_closed/unscanned_closed_udp" is set to "no".
#
function get_port_for_service( default, nodefault, ipproto, proto ) {

  local_var default, nodefault, ipproto, proto;
  local_var key, port;

  if( ! proto ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#get_port_for_service" );
    exit( 0 );
  }

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_port_for_service" );
    exit( 0 );
  }

  if( ! ipproto )
    ipproto = "tcp";

  if( ipproto == "tcp" )
    key = strcat( "Services/", proto );
  else
    key = strcat( "Services/", ipproto, "/", proto );

  port = get_kb_item( key );
  if( port )
    return port;

  # nb: If this is the case we haven't found an open port, default wasn't passed
  # and nodefault was set to TRUE. We can't continue in this case.
  if( ! default )
    exit( 0 );

  port = get_kb_item( "Known/" + ipproto + "/" + default );
  if( port == proto )
    return default;

  if( ipproto == "tcp" && get_tcp_port_state( default ) )
    return default;

  if( ipproto == "udp" && get_udp_port_state( default ) )
    return default;

  exit( 0 );
}

# @brief Returns all ports matching the specific IP and service protocol. The function will evaluate the following KB keys for this information:
#        "Services/proto", "Services/ipproto/proto" and "Known/ipproto/default".
#
# @param default_list The port list to return if "unscanned_closed" is set to "no" and no ports have been found for the specified IP and service protocol.
# @param ipproto      An (optional) IP protocol (e.g. "tcp" or "udp") to evaluate. If this parameter isn't passed it will default to "tcp".
# @param proto        A (mandatory) service protocol (e.g. "www" for HTTP were Services/www is evaluated).
#
# @note  In contrast to @ref get_port_for_service this function will not fork and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting blocked by the remote service with to many concurrent connections.
#
# @return A list containing the open / found ports matching the specific IP and service protocol, if none where found an empty list. In the case of
#         no found ports and "unscanned_closed/unscanned_closed_udp" is set to "no" the list passed to the default_ports parameter is returned.
#
function get_ports_for_service( default_list, ipproto, proto ) {

  local_var default_list, ipproto, proto;
  local_var port_list, num_ports, key, ports, port, default;

  port_list = make_list();

  if( ! proto ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#proto#-#get_port_for_service" );
    return port_list;
  }

  num_ports = 0;

  if( ! ipproto )
    ipproto = "tcp";

  if( ipproto == "tcp" )
    key = strcat( "Services/", proto );
  else
    key = strcat( "Services/", ipproto, "/", proto );

  ports = get_kb_list( key );
  if( ports && typeof( ports ) == "array" ) {
    foreach port( ports ) {
      num_ports++;
      port_list = make_list( port_list, port );
    }
  }

  if( num_ports > 0 )
    return port_list;

  # nb: If this is the case we haven't found an open port and the default_list
  # parameter wasn't passed. We can't continue in this case.
  if( ! default_list || typeof( default_list ) != "array" )
    return port_list;

  foreach default( default_list ) {
    ports = get_kb_list( "Known/" + ipproto + "/" + default );
    if( ports && typeof( ports ) == "array" ) {
      foreach port( keys( ports ) ) {
        if( ports[port] == proto ) {
          num_ports++;
          port_list = make_list( port_list, default );
        }
      }
    }
  }

  if( num_ports > 0 )
    return port_list;

  foreach default( default_list ) {

    if( ipproto == "tcp" && get_tcp_port_state( default ) )
      port_list = make_list( port_list, default );

    if( ipproto == "udp" && get_udp_port_state( default ) )
      port_list = make_list( port_list, default );
  }

  return port_list;
}

function get_unknown_banner( port, ipproto, dontfetch ) {

  local_var sb, sbH, banner, soc, req, tcp, p, bannerHex, port, ipproto, dontfetch;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_unknown_banner" );
    return;
  }

  if( ! ipproto ) ipproto = "tcp";
  if( ipproto == "tcp" )
    tcp = TRUE;
  else
    tcp = FALSE;

  if( tcp ) {
   sb  = strcat( "unknown/banner/", port );
   sbH = strcat( "unknown/bannerHex/", port );
  } else {
   sb  = strcat( "unknown/banner/", ipproto, "/", port );
   sbH = strcat( "unknown/bannerHex/", ipproto, "/", port );
  }

  banner = get_kb_item( sbH );
  if( banner ) return hex2raw( s:banner );
  banner = get_kb_item( sb );
  if( banner ) return banner;

  banner = get_kb_item( "BannerHex/" + port );
  if( banner ) return( hex2raw( s:banner ) );
  banner = get_kb_item( "Banner/" + port );
  if( banner ) return( banner );

  banner = get_kb_item( "Amap/" + ipproto + "/" + port + "/FullBanner" );
  if( banner ) return( banner );

  foreach p( make_list( "spontaneous", "get_http", "help", "xml", "json", "sip", "bin" ) ) {
    banner = get_kb_item( "FindService/" + ipproto + "/" + port + "/" + p );
    bannerHex = get_kb_item( "FindService/" + ipproto + "/" + port + "/" + p + "Hex" );
    if( banner || bannerHex ) {
      if( strlen( bannerHex ) > 2 * strlen( banner ) )
       return hex2raw( s:bannerHex );
      else
       return( banner );
    }
  }
  if( dontfetch ) return( NULL );
  if( ! get_port_state( port ) ) return( NULL );
  if( ! tcp ) return( NULL );

  soc = open_sock_tcp( port );
  if( ! soc ) return( NULL );
  # I don't think that it makes sense to send an HTTP request
  #req = http_head(item:"/", port:port);
  #send(socket:soc, data:req);
  banner = recv( socket:soc, length:2048 );
  close( soc );
  if( banner ) {
    replace_kb_item( name:sb, value:banner );
    if( '\0' >< sb )
     replace_kb_item( name:sbH, value:hexstr( banner ) );
  }
  return( banner );
}

function set_unknown_banner( port, banner, ipproto ) {

  local_var sb, port, banner, ipproto;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#set_unknown_banner" );

  if( ! ipproto || ipproto == 'tcp' )
    sb = string( "unknown/banner/", port );
  else
    sb = strcat( 'unknown/banner/', ipproto, '/', port );
  set_kb_item( name:sb, value:banner );
  if( '\0' >< banner ) {
    if( ! ipproto || ipproto == 'tcp' )
      sb = string( "unknown/bannerHex/", port );
    else
      sb = strcat( 'unknown/bannerHex/', ipproto, '/', port );
    set_kb_item( name:sb, value:hexstr( banner ) );
  }
}

#
# Get the banner for a given service
# You must also specify a default port, in case this is not in the kb
#
function get_service_banner_line( service, port, ipproto ) {

  local_var banner, soc, key, gport, tcp, service, port, ipproto;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_service_banner_line" );
  if( ! service ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#service#-#get_service_banner_line" );

  if( ! ipproto ) ipproto = "tcp";
  if( ipproto == "tcp" )
    tcp = TRUE;
  else
    tcp = FALSE;

  if( tcp )
    gport = get_kb_item( strcat( "Services/", service ) );
  else
    gport = get_kb_item( strcat( "Services/", ipproto, "/", service ) );
  if( ! gport ) gport = port;

  if( tcp )
    key = strcat( service, "/banner/", gport );
  else
    key = strcat( service, "/banner/", ipproto, "/", gport );

  banner = get_kb_item( key );

  if( ! banner ) {
    if( ! tcp ) return;
    if( get_port_state( gport ) ) {
      soc = open_sock_tcp( gport );
      if( soc ) {
        banner = recv_line( socket:soc, length:2048 );
        close( soc );
      }
    }
#   if( banner ) set_kb_item( name:key, value: banner );
  }
  return( banner );
}

#
# Fast replacement for getrpcport() which uses the libc
#
function get_rpc_port( program, protocol, portmap ) {

  local_var broken, req, soc, r, port, program, protocol, portmap, len;
  local_var a, b, c, d, p_a, p_b, p_c, p_d, pt_a, pt_b, pt_c, pt_d;

  if( isnull( program ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#program#-#get_rpc_port" );
  if( isnull( protocol ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#protocol#-#get_rpc_port" );

  a = rand() % 255;
  b = rand() % 255;
  c = rand() % 255;
  d = rand() % 255;

  p_a = program / 16777216;  p_a = p_a % 256;
  p_b = program / 65356;     p_b = p_b % 256;
  p_c = program / 256;       p_c = p_c % 256;
  p_d = program % 256;

  pt_a = protocol / 16777216; pt_a = pt_a % 256;
  pt_b = protocol / 65535   ; pt_b = pt_b % 256;
  pt_c = protocol / 256;    ; pt_c = pt_c % 256;
  pt_d = protocol % 256;

  req = raw_string( a, b, c, d,  # XID
                    0x00, 0x00, 0x00, 0x00,   # Msg type: call
                    0x00, 0x00, 0x00, 0x02,   # RPC Version
                    0x00, 0x01, 0x86, 0xA0,   # Program
                    0x00, 0x00, 0x00, 0x02,   # Program version
                    0x00, 0x00, 0x00, 0x03,   # Procedure
                    0x00, 0x00, 0x00, 0x00,   # Credentials - flavor
                    0x00, 0x00, 0x00, 0x00,   # Credentials - length
                    0x00, 0x00, 0x00, 0x00,   # Verifier - Flavor
                    0x00, 0x00, 0x00, 0x00,   # Verifier - Length

                    p_a,  p_b,  p_c,  p_d,    # Program
                    0xFF, 0xFF, 0xFF, 0xFF);  # Version (any)

  if( protocol == IPPROTO_TCP )
    req += raw_string( 0x00, 0x00, 0x00, 0x06 );
  else
    req += raw_string( pt_a, pt_b, pt_c, pt_d );

  req += raw_string( 0x00, 0x00, 0x00, 0x00 );    # Port

  if( protocol == IPPROTO_TCP )
  {
    req = mkdword( strlen( req ) ) + req;
    req = raw_string( 0x80 | ord( req[ 0 ]) ) + substr( req, 1, strlen( req ) );
  }

  if( isnull( portmap ) ) {
    port = int( get_kb_item( "rpc/portmap" ) );
    if( port == 0 ) port = 111;
  }
  else port = portmap;

  broken = get_kb_item( "/tmp/rpc/noportmap/" + port );
  if( broken ) return( 0 );

  if( protocol == IPPROTO_TCP )
  {
    len = 32;
    soc = open_sock_tcp( port );
  }
  else
  {
    len = 28;
    soc = open_sock_udp( port );
  }

  if( ! soc ) return;

  send( socket:soc, data:req );
  r = recv( socket:soc, length:len );

  close( soc );

  if( ! r ) {
    set_kb_item( name:"/tmp/rpc/noportmap/" + port, value:TRUE );
    return( 0 );
  }

  if( strlen( r ) != len ) {
    return( 0 );
  } else {
    port = getdword( blob:raw_string( r[ len -  4 ], r[ len -  3 ], r[ len - 2 ], r[ len - 1 ] ) );

    if( protocol == IPPROTO_TCP ) {
      if( get_tcp_port_state( port ) ) {
        return( port );
      } else {
        return( 0 );
      }
    } else {
      if( get_udp_port_state( port ) ) {
        return( port );
      } else {
        return( 0 );
      }
    }
  }
}

## This function will construct rpc packet
function construct_rpc_packet( program, prog_ver, procedure, data, udp, credentials, verifier ) {

  local_var program, prog_ver, procedure, data, udp, credentials, verifier;
  local_var xid, header, cred_data, verifier_data, rpc_packet, data_len, frag_header;

  if( isnull( program ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#program#-#construct_rpc_packet" );
  if( isnull( prog_ver ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#prog_ver#-#construct_rpc_packet" );
  if( isnull( procedure ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#procedure#-#construct_rpc_packet" );

  ## Add 0 If credentials are not passed
  if( isnull( credentials ) ) {
    credentials[0] = 0;
    credentials[1] = 0;
  }

  ## Add 0 If Verified are not passed
  if( isnull( verifier ) ) {
    verifier[0] = 0;
    verifier[1] = 0;
  }

  ## Random XID
  xid = rand();

  ## Construct complete RPC Rstat Request
  header  = mkdword( xid );                          ## XID
  header += mkdword( 0 );                            ## Message Type: Call (0)
  header += mkdword( 2 );                            ## RPC Version: 2
  header += mkdword( program );                      ## Program
  header += mkdword( prog_ver );                     ## Program Version
  header += mkdword( procedure );                    ## Procedure

  ## Credentials
  cred_data = mkdword( credentials[0] );             ## Flavor
  cred_data += mkdword( strlen( credentials[1] ) );  ## Length

  ## Verifier
  verifier_data = mkdword( verifier[0] );            ## Flavor
  verifier_data += mkdword( strlen( verifier[1] ) ); ## Length

  rpc_packet = header + cred_data + verifier_data + data;

  ## Add Fragment header if it's not UDP protocol
  if( udp != "udp" || udp == FALSE ) {
    ## Fragment Length
    data_len = strlen( header + cred_data + verifier_data + data );

    ## Fragment Header
    frag_header  = mkbyte( 0x80 );               ## Last Fragment
    frag_header  += mkbyte( 0 );                 ##
    frag_header  += mkdword( data_len );         ## Fragment Length
    rpc_packet = frag_header + rpc_packet;
  }

  return( rpc_packet );
}

function rand_str( length, charset ) {

  local_var l, i, s, n, length, charset;

  if( ! charset )
    charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_";
  if( isnull( length ) )
    length = 8;
  l = strlen( charset );
  s = "";
  for( i = 0; i < length; i++ ) {
    n = rand() % l;
    s += charset[n];
  }
  return s;
}

function add_port_in_list( list, port ) {

  local_var l, list, port;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#add_port_in_list" );
  if( ! list ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#list#-#add_port_in_list" );

  if( ! get_port_state( port ) ) {
    if( isnull( list ) ) {
      return make_list();
    } else {
      return list;
    }
 }

 if( isnull( list ) ) return make_list( port );

 foreach l( list ) {
  if( l == port )
   return list;
 }

 return make_list( list, port );
}

# hex2raw was written by Renaud?
function hex2raw( s ) {

  local_var i, j, ret, l, s;

  if( isnull( s ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#s#-#hex2raw" );

  s = chomp( s );  # remove trailing blanks, CR, LF...
  l = strlen( s );
  if( l % 2 ) {
    if( _misc_func_debug ) display( "hex2raw: odd string: ", s, "\n" );
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

function report_service( port, svc, banner, message ) {

  local_var k, name, a, port, svc, banner, message;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#report_service" );
  if( ! svc ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#svc#-#report_service" );

  svc = tolower( svc );
  if( ! isnull( banner ) ) {
    k = strcat( svc, "/banner/", port );
    set_kb_item( name:k, value:banner );
  }

  register_service( port:port, proto:svc, message:message );

  if( svc == 'www' ) name = 'web server';
  else if( svc == 'proxy' ) name = 'web proxy';
  else if( svc == 'hylafax-ftp' || svc == 'hylafax' ) name = 'HylaFAX server';
  else if( svc == 'agobot.fo' ) name = 'Agobot.fo backdoor';
  else if( svc == 'unknown_irc_bot' ) name = 'IRC bot';
  else if( svc == 'auth' ) name = 'identd';

  else name = toupper( svc ) + ' server';
  a = tolower( name[0] );
  if( a == 'a' || a == 'e' || a == 'i' || a == 'o' ) a = 'An ';
  else a = 'A ';
  log_message( port:port, data:a + name + ' is running on this port' );
}

function base64_decode( str, key_str ) {

  local_var len, i, j, k, ret, base64, b64, a, b, c, o, str, key_str;

  if( isnull( str ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#str#-#base64_decode" );

  len = strlen( str );
  ret = "";

  if( key_str )
    base64 = key_str;
  else
    base64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

  for( i = 0; i < 256; i++ )
    b64[i] = 0;
  for( i = 0; i < strlen( base64 ); i++ )
    b64[ord( base64[i] )] = i;

  for( j = 0; j < len; j += 4 ) {
    for( i = 0; i < 4; i++ ) {
      c = ord( str[j+i] );
      a[i] = c;
      b[i] = b64[c];
    }

    o[0] = ( b[0] << 2 ) | ( b[1] >> 4 );
    o[1] = ( b[1] << 4 ) | ( b[2] >> 2 );
    o[2] = ( b[2] << 6 ) | b[3];
    if( a[2] == ord( '=' ) ) {
     i = 1;
    } else if( a[3] == ord( '=' ) ) {
     i = 2;
    } else {
     i = 3;
    }
    for( k = 0; k < i; k++ )
      ret += raw_string( int( o[k] ) & 255 );

    if( i < 3 )
      break;
  }

  return ret;
}

function base64_code( c ) {

  local_var c, __base64_code;

  __base64_code = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

  if( isnull( c ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#c#-#base64_code" );

  return( __base64_code[c] );
}

function pow2( x ) {

  local_var __ret, x;

  if( isnull( x ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#x#-#pow2" );

  __ret = 1;
  while( x ) {
    __ret = __ret * 2;
    x = x  - 1;
  }
  return( __ret );
}

function base64( str ) {

  local_var len, i, ret, char_count, _bits, val, cnt, mul, str;

  if( isnull( str ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#str#-#base64" );

  len = strlen( str );
  i = 0;
  ret = "";
  char_count = 0;
  _bits = 0;

  while( i < len ) {
    _bits = _bits + ord( str[i] );
    char_count = char_count + 1;
    if( char_count == 3 ) {
      val = _bits / 262144;
      ret = string( ret, base64_code( c:val ) );
      val = _bits / 4096;
      val = val & 0x3F;
      ret = string( ret, base64_code( c:val ) );
      val = _bits / 64;
      val = val & 0x3F;
      ret = string( ret, base64_code( c:val ) );
      val = _bits & 0x3F;
      ret = string( ret, base64_code( c:val ) );
      char_count = 0;
      _bits = 0;
    } else {
      _bits = _bits * 256;
    }
    i = i + 1;
  }

  if( ! ( char_count == 0 ) ) {
    cnt = char_count * 8;
    mul = 16;
    mul = mul - cnt;
    mul = pow2( x:mul );
    _bits = _bits * mul;
    val = _bits / 262144;
    ret = string( ret, base64_code( c:val ) );
    val = _bits / 4096;
    val = val & 0x3F;
    ret = string( ret, base64_code( c:val ) );
    if( char_count == 1 ) {
      ret = string(ret, "==");
    } else {
      val = _bits / 64;
      val = val & 0x3F;
      ret = string( ret, base64_code( c:val ), "=" );
    }
  }
  return( ret );
}


# This function converts a string representing a decimal number to
# to hexadecimal; eg, dec2hex(1098757090) == "417db3e2".
#
# Args:
#   o num, decimal number.
#
# Return:
#   hex number represented as a raw string.
#
# updated: 16-Nov-2004, George A. Theall
#
function dec2hex( num ) {

  local_var digits, hex, rem, num;

  if( isnull ( num ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#num#-#dec2hex" );

  hex = "";

  num = int( num );
  while( num > 0 ) {
    rem = num % 256;
    hex = raw_string( rem, hex );
    num = num / 256;
    if( num > 0 && num < 255 ) {
      hex = raw_string( num, hex );
      num = 0;
    }
  }
  if( ! hex ) hex = raw_string( 0x00 );

  return hex;
}

# Convert a Date CVS field to Unix time
# Michel Arboi
function cvsdate2unixtime( date ) {

  local_var v, u, date;

  if( ! date ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#date#-#cvsdate2unixtime" );

  v = eregmatch( string:date, pattern:".Date: ([0-9]+)/([01][0-9])/([0-3][0-9]) ([0-2][0-9]):([0-6][0-9]):([0-6][0-9]) \$" );
  if( isnull( v ) ) return;
  u = mktime( year:v[1], mon:v[2], mday:v[3], hour:v[3], min:v[5], sec:v[6] );
  return u;
}

function in_array( search, array, part_match ) {

  local_var search, array, part_match, val;

  if( typeof( array ) != "array" ) {
    set_kb_item( name:"nvt_debug_no_array/" + get_script_oid(), value:get_script_oid() + "#-#array#-#in_array" );
    return;
  }

  if( ! search || isnull( search ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#search#-#in_array" );
    return;
  }

  foreach val( array ) {
    if( part_match ) {
      if( search >< val ) return TRUE;
    } else {
      if( val == search ) return TRUE;
    }
  }
  return;
}

function array_key_exist( key, array, part_match ) {

  local_var key, array, part_match, a;

  if( typeof( array ) != "array" ) {
    set_kb_item( name:"nvt_debug_no_array/" + get_script_oid(), value:get_script_oid() + "#-#array#-#array_key_exist" );
    return;
  }

  if( ! key || isnull( key ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#key#-#array_key_exist" );
    return;
  }

  foreach a( keys( array ) ) {
    if( part_match ) {
      if( key >< a ) return TRUE;
    } else {
      if( a == key ) return TRUE;
    }
  }

  return;
}

function is_array() {

  local_var array, a;

  array = _FCT_ANON_ARGS[0];

  if( ! array ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#_FCT_ANON_ARGS[0]#-#is_array" );
    return;
  }

  if( typeof( array ) != "array" )
    return FALSE;

  if( max_index( array ) == 0 ) {
    foreach a( array )
      return TRUE;

    return;
  }
  return TRUE;
}

# from packeteer_web_login.nasl
function hex2str() {

  local_var xlat, hs, s, i, j;

  hs = _FCT_ANON_ARGS[0];

  if( isnull( hs ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#_FCT_ANON_ARGS[0]#-#hex2str" );
    return;
  }

  s = "";
  for( i = 0; i < 256; ++i )
    xlat[ tolower( substr( hex( i ), 2 ) ) ] = raw_string( i );

  for( j = 0; j < strlen( hs ) / 2; ++j )
    s += xlat[ tolower( substr( hs, 2 * j, 2 * j + 1 ) ) ];

  return s;
}

function is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }
  return;
}

function register_all_pjl_ports( ports ) {

  local_var ports, port;

  if( isnull( ports ) || typeof( ports ) != "array" ) {
    ports = make_list( 9100, 9101, 9102, 9103, 9104, 9105, 9106, 9107, 9112, 9113, 9114, 9115, 9116 );
  }

  foreach port( ports ) {
    if( get_port_state( port ) ) {
      register_service( port:port, proto:"hp-pjl" );
      register_service( port:port, proto:"fragile_port" );
      replace_kb_item( name:"BannerHex/" + port, value:"aeaeaeaeae" );
      replace_kb_item( name:"Banner/" + port, value:"ignore-this-banner" );
    }
  }
  return;
}

function get_unknown_port( default, nodefault, ipproto ) {

  local_var default, nodefault, ipproto, udp, _port, port;

  if( ! nodefault && ! default ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_unknown_port" );

  if( ! ipproto ) ipproto = "tcp";
  if( ipproto == "tcp" ) {
    udp = FALSE;
    port = get_kb_item( "Services/unknown" );
  } else {
    if( ipproto == "udp" ) {
      udp = TRUE;
      # nb: We only want to return all unknown UDP ports if this is explicitly enabled within global_settings.nasl
      if( get_kb_item( "global_settings/non-default_udp_service_discovery" ) ) {
        port = get_kb_item( "Services/udp/unknown" );
      } else {
        port = default;
      }
    } else {
      udp = FALSE;
      port = get_kb_item( "Services/" + ipproto + "/unknown" );
    }
  }

  if( port ) {
    _port = port;
  } else {
    _port = default;
  }

  if( ! _port ) exit( 0 );

  if( ! udp ) {
    # Ignore NetBIOS port as previously done in get_unknown_svc()
    if( _port == 139 ) exit( 0 );

    # Includes e.g. PJL ports which are printing everything
    # sent to them so exit for such a port here
    if( is_fragile_port( port:_port ) ) exit( 0 );
  }

  # Don't check port passed in default variable. Other detections
  # (like telnet.nasl) might have falsely marked this as known.
  if( _port != default ) {
    if( ! service_is_unknown( port:_port, ipproto:ipproto ) ) exit( 0 );
  }

  if( ! udp ) {
    if( ! get_port_state( _port ) ) exit( 0 );
  } else {
    if( ! get_udp_port_state( _port ) ) exit( 0 );
  }
  return _port;
}

function get_unknown_port_list( default, nodefault, ipproto ) {

  local_var default, nodefault, ipproto, udp, port, port_list, _port_list, __port_list;

  __port_list = make_list();

  if( ! nodefault && ! default ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_unknown_port_list" );

  if( ! ipproto ) ipproto = "tcp";
  if( ipproto == "tcp" ) {
    udp = FALSE;
    port_list = get_kb_list( "Services/unknown" );
  } else {
    if( ipproto == "udp" ) {
      udp = TRUE;
      # nb: We only want to return all unknown UDP ports if this is explicitly enabled within global_settings.nasl
      if( get_kb_item( "global_settings/non-default_udp_service_discovery" ) ) {
        port_list = get_kb_list( "Services/udp/unknown" );
      } else {
        port_list = make_list( default );
      }
    } else {
      udp = FALSE;
      port_list = get_kb_list( "Services/" + ipproto + "/unknown" );
    }
  }

  if( port_list ) {
    if( ! in_array( search:default, array:port_list ) ) { # The passed default might be already in the Services/unknown list
      _port_list = make_list( default, port_list );
    } else {
      _port_list = port_list;
    }
  } else {
    _port_list = make_list( default );
  }

  foreach port( _port_list ) {

    if( ! udp ) {
      # Ignore NetBIOS port as previously done in get_unknown_svc()
      if( port == 139 ) continue;

      # Includes e.g. PJL ports which are printing everything
      # sent to them so continue for such a port here
      if( is_fragile_port( port:port ) ) continue;
    }

    # Don't check port passed in default variable. Other detections
    # (like telnet.nasl) might have falsely marked this as known.
    if( port != default ) {
      if( ! service_is_unknown( port:port, ipproto:ipproto ) ) continue;
    }

    if( ! udp ) {
      if( ! get_port_state( port ) ) continue;
    } else {
      if( ! get_udp_port_state( port ) ) continue;
    }

    __port_list = make_list( __port_list, port );
  }
  return __port_list;
}

# Add a dependency to secpod_open_tcp_ports.nasl if you want to use this
function get_all_tcp_ports( ) {

  local_var port;

  port = get_kb_item( "TCP/PORTS" );
  if( ! port ) exit( 0 );

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( is_fragile_port( port:port ) ) exit( 0 );

  if( ! get_port_state( port ) ) exit( 0 );

  return port;
}

# Add a dependency to secpod_open_tcp_ports.nasl if you want to use this
function get_all_tcp_ports_list( ) {

  local_var _ports, ports, port;

  ports = make_list();

  _ports = get_kb_list( "TCP/PORTS" );

  foreach port( _ports ) {

    # Includes e.g. PJL ports which are printing everything
    # sent to them so continue for such a port here
    if( is_fragile_port( port:port ) ) continue;

    if( ! get_port_state( port ) ) continue;

    ports = make_list( ports, port );
  }
  return ports;
}

# "if( is_printer_mac( mac:mac ) ) do something..."
# include mac_prefix.inc if you want to use this function
function is_printer_mac( mac ) {

  local_var mac, mac_s, p_vendors, max_prefix, mac_vendor, pv;

  if( ! mac ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#mac#-#is_printer_mac" );
    return;
  }

  mac_s = split( mac, sep:":", keep:FALSE );
  if( max_index( mac_s ) != 6 ) return;

  p_vendors = make_list( "xerox", "canon", "kyocera", "lexmark", "hewlettp", "samsung" );

  mac_prefix = toupper( mac_s[0] + ':' + mac_s[1] + ':' + mac_s[2] );

  mac_vendor = tolower( mac_prefixes[ mac_prefix ] );
  if( ! mac_vendor ) return;

  foreach pv( p_vendors )
    if( mac_vendor == pv )
      return TRUE;

  return;
}

# @brief Returns the first found known to be open TCP port on the remote system. This function
#        replaces the nasl built-in get_host_open_port where you can't control that it doesn't
#        return a "fragile" port.
#
# @note Add a dependency to secpod_open_tcp_ports.nasl if you want to use this
#
# @return The first found known to be open TCP Port. If none has been found the function will exit.
#
function get_host_open_tcp_port( ) {

  local_var ports;

  ports = get_all_tcp_ports_list();
  if( isnull( ports[0] ) )
    exit( 0 );
  else
    return ports[0]; # TBD: Bring some randomness into this?
}

function bin2string( ddata, noprint_replacement ) {

  local_var ddata, noprint_replacement, tmp, i, j, line, linenumber, len, data, c;

  if( isnull( ddata ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ddata#-#bin2string" );

  len = strlen ( ddata );
  linenumber = len / 16;

  for ( i = 0; i <= linenumber; i++ ) {

    line = line2string ( line:i, linenumber:len ); #TBD: This seems to be unused in all bin2string functions moved here
    data = "";

    for ( j = 0; j < 16; j++ ) {
      if ( ( i*16+j ) < len ) {
        c = ddata[i*16+j];

        if( isprint( c:c ) ) {
          data += c;
        } else {
          if( ! isnull( noprint_replacement ) ) {
            data += noprint_replacement;
          }
        }
      }
   }
   tmp += string( data );
  }
  return tmp;
}

# modified unicode4 from smb_nt.inc
function ascii2unicode( data ) {

  local_var data, len, ret, i;

  if( isnull( data ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#ascii2unicode" );
    return;
  }

  len = strlen( data );

  for( i = 0; i < len; i++ ) {
    ret += data[i] + raw_string( 0x00 );
  }
  return ret;
}

function join( list, sep ) {

  local_var list, sep, l, ret;

  if( ! list )
    return;

  if( ! sep )
    sep = " ";

  foreach l ( list )
    ret += l + sep;

  return ereg_replace( string:ret, pattern:sep + '$', replace:'' );
}

# @brief Sends a specific data string (if passed) and receives the answer
#        of the remote service.
#
# @param port   The port of the remote service. Either the port OR the soc parameter
#               needs to be passed, not both. If the port parameter was passed the
#               function will close the opened socket on its own.
# @param soc    An already opened socket to the remote service. Either the soc OR the
#               soc parameter needs to be passed, not both.
# @param data   An (optional) data string which should be send to the remote service.
# @param proto  The protocol used to connect to the service if the port parameter is passed.
#               Defaults to 'tcp' if not passed.
# @param length The amount of bytes which should be received from the remote service.
#               Defaults to '1024' if not passed.
#
# @return The received data or NULL if no connection to the remote service could be opened
#         or not all parameters where passed correctly.
#
function socket_send_recv( port, soc, data, proto, length ) {

  local_var port, soc, data, proto, length;
  local_var nosock, recv;

  if( ! port && ! soc ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port/soc#-#socket_send_recv" );
    return;
  }

  if( proto && proto != "udp" && proto != "tcp" ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#socket_send_recv: proto parameter passed but doesn't contain match 'tcp' or 'udp'" );
    return;
  }

  if( port && soc ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#socket_send_recv: port and soc parameter passed, please chose only one" );
    return;
  }

  if( ! soc )
    nosock = TRUE;

  if( port && nosock ) {
    if( ! proto )
      proto = "tcp";

    if( proto == "udp" )
      soc = open_sock_udp( port );
    else
      soc = open_sock_tcp( port );

    if( ! soc )
      return;
  }

  if( data )
    send( socket:soc, data:data );

  if( ! length )
    length = 1024;

  recv = recv( socket:soc, length:length );

  # close socket only if it was opened by this function
  if( nosock )
    close( soc );

  return chomp( recv );
}

function text_format_table( array, sep, columnheader ) {

  local_var array, sep, columnheader, len, a, report, maxlen, flen, padding_len, reportheader;

  if( ! array || ! is_array( array ) ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#text_format_table: No array or empty variable passed" );
    return;
  }

  if( isnull( sep ) )
    sep = " : ";

  foreach a( sort( keys( array ) ) ) {
    len = strlen( a );

    if( ! maxlen ) {
      maxlen = len;
      continue;
    }

    if( maxlen < len )
      maxlen = len;
  }

  foreach a( sort( keys( array ) ) ) {
    flen = strlen( a );
    padding_len = ( maxlen - flen ) + 1;
    report += a + crap( data:" ", length:padding_len ) + sep + array[a] + '\n';
  }

  if( strlen( report ) ) {
    if( columnheader && is_array( columnheader ) ) {
      reportheader = columnheader[0] + crap( data:" ", length:maxlen - strlen( columnheader[0] ) + strlen( sep ) + 1 ) + columnheader[1] + '\n';
      reportheader += crap( data:"-", length:strlen( columnheader[0] ) ) + crap( data:" ", length:maxlen - strlen( columnheader[0] ) + strlen( sep ) + 1 ) + crap( data:"-", length:strlen( columnheader[1] ) ) + '\n';
      report = reportheader + report;
    }
    return report;
  }
}

# @brief Closes an passed socket and exit afterwards
#
# @param _FCT_ANON_ARGS[0] The socket to close as an unnamed argument.
#
function close_sock_and_exit() {

  local_var soc;

  soc = _FCT_ANON_ARGS[0];

  if( soc )
    close( soc );

  exit( 0 );
}

function eol_date_reached( eol_date ) {

  local_var eol_date, local_time;

  if( ! eol_date ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#eol_date#-#eol_date_reached" );
    return;
  }

  # Special case to mark an OS/Product as EOL if the date is unknown.
  # This avoids that we need to guess a date for e.g. older products.
  if( eol_date == "unknown" ) return TRUE;

  eol_date = str_replace( string:eol_date, find:"-", keep:FALSE );
  local_time = make_date_str( date:localtime( unixtime() ) );

  if( int( local_time ) >= int( eol_date ) )
    return TRUE;

  return;
}

function build_eol_message( eol_type, name, cpe, version, location, skip_version, eol_version, eol_date, eol_url ) {

  local_var eol_type, name, cpe, version, location, skip_version, eol_version, eol_date, eol_url, report;

  if( eol_type != "prod" && eol_type != "os" ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#build_prod_eol_message: Wrong value passed to eol_type. Valid values are: prod, os" );
    return "";
  }

  if( eol_type == "prod" ) {

    if( isnull( name ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#name#-#build_eol_message" );
    if( isnull( cpe ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#build_eol_message" );
    if( isnull( version ) && ! skip_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#build_eol_message" );
    if( isnull( eol_version ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#eol_version#-#build_eol_message" );
    if( isnull( eol_date ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#eol_date#-#build_eol_message" );

    if( ! skip_version ) {
      report += 'CPE:               ' + cpe + ':' + version + '\n';
      report += 'Installed version: ' + version + '\n';
    } else {
      report += 'CPE:               ' + cpe + '\n';
    }

    if( location )
      report += 'Location/URL:      ' + location + '\n';

    report +=   'EOL version:       ' + eol_version + '\n';
    report +=   'EOL date:          ' + eol_date + '\n';

    if( eol_url )
      report += 'EOL info:          ' + eol_url + '\n';

  } else if( eol_type == "os" ) {

    if( isnull( name ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#name#-#build_eol_message" );
    if( isnull( cpe ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#build_eol_message" );
    if( isnull( eol_date ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#eol_date#-#build_eol_message" );
    if( isnull( eol_url ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#eol_url#-#build_eol_message" );

    report += 'CPE:               ' + cpe + '\n';

    if( version && version != "unknown" ) {
      report += 'Installed version,' + '\n';
      report += 'build or SP:       ' + version + '\n';
    }

    if( eol_version )
      report += 'EOL version:       ' + eol_version + '\n';

    if( eol_date )
      report += 'EOL date:          ' + eol_date + '\n';

    if( eol_url )
      report += 'EOL info:          ' + eol_url + '\n';
  }
  return report;
}

# @brief Converts a passed date string generated by the unixtime() function
#        into a form like 20180813
#
# @param date a date string generated by the unixtime() function
#
# @todo Merge the functionality of make_date_str() in 2013/gb_host_scanned_wmi.nasl
#       and 2013/gb_host_scanned_ssh.nasl into this function.
#
# @return A string containing the current date in the form of 20180813 or NULL
#         if no date parameter was passed.
#
function make_date_str( date ) {

  local_var date, time, month, day;

  if( isnull( date ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#date#-#make_date_str" );
    return;
  }

  time  = localtime( date );
  month = fancy_date( datestr:time["mon"]  );
  day   = fancy_date( datestr:time["mday"] );

  # Using string to make sure we return a string and don't do an addition of ints
  return string( time["year"], month, day );
}

# @brief Adds a leading 0/zero to a passed numeric string if the value is < 10
#        and returns the string back.
#
# @todo Merge the functionality of fancy_date() in 2013/gb_host_scanned_wmi.nasl,
#       2013/gb_host_scanned_ssh.nasl, gb_nist_win_oval_sys_char_generator.nasl
#       and kb_2_sc.nasl into this function.
#
# @param datastr the numeric string to evaluate
#
# @return a string with a leading 0 if the passed string was < 10, the same string
#         otherwise and NULL if no datestr parameter was passed.
#
function fancy_date( datestr ) {

  local_var datestr;

  if( isnull( datestr ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#datestr#-#fancy_date" );
    return;
  }

  if( int( datestr ) < 10 )
    return string( "0", datestr );
  else
    return datestr;
}

# @brief Creates an array of the commands 'ipconfig' and 'id' as the array key and the regex to match
#        against the response of these commands as the array value. The function will chose the
#        returned commands if the remote system is running on Windows, Linux or if the OS is unknown.
#
# @param _FCT_ANON_ARGS[0] Allows (optional) to pass either "windows" or "linux" as an anonymous function parameter
#                          to define directly which kind of files should be returned.
#
# @note Add a dependency to os_detection.nasl if you want to use this function. The VT calling this
#       function needs to also include host_details.inc (for host_runs());
#
# @return An array as described in the function summary.
#
function exploit_commands() {

  local_var res;

  if( _FCT_ANON_ARGS[0] ) {

    if( tolower( _FCT_ANON_ARGS[0] ) == "windows" ) {
      res = "yes";
    } else if( tolower( _FCT_ANON_ARGS[0] ) == "linux" ) {
      res = "no";
    }
  } else {
    res = host_runs( "windows" );
  }

  if( res == "yes" ) {
    return make_array( "Windows.IP..onfiguration", "ipconfig" );
  } else if( res == "no" ) {
    return make_array( "uid=[0-9]+.*gid=[0-9]+", "id" );
  }

  # unknown
  return make_array( "uid=[0-9]+.*gid=[0-9]+", "id",
                     "Windows.IP..onfiguration", "ipconfig" );
}

# @brief Creates an array of common files to check during "Path traversal" attacks. The array contains the
#        the regex to match against the expected content of these files as the array key and the file to check
#        as the array value. The function will chose the returned files if the remote system is running on
#        Windows, Linux or if the OS is unknown.
#
# @param _FCT_ANON_ARGS[0] Allows (optional) to pass either "windows" or "linux" as an anonymous function parameter
#                          to define directly which kind of files should be returned.
#
# @note Add a dependency to os_detection.nasl if you want to use this function.
#
# @return An array as described in the function summary.
#
function traversal_files() {

  local_var res;

  if( _FCT_ANON_ARGS[0] ) {

    if( tolower( _FCT_ANON_ARGS[0] ) == "windows" ) {
      res = "yes";
    } else if( tolower( _FCT_ANON_ARGS[0] ) == "linux" ) {
      res = "no";
    }
  } else {
    res = host_runs( "windows" );
  }

  if( res == "yes" ) {
    return make_array( "\[boot loader\]", "boot.ini",
                       "; for 16-bit app supporT", "winnt/win.ini",
                       "; for 16-bit app support", "windows/win.ini" );
  } else if( res == "no" ) {
    return make_array( "root:.*:0:[01]:", "etc/passwd" ); # TBD: Also check e.g. etc/hosts for basic IDS?
  }

  # "unknown"
  return make_array( "root:.*:0:[01]:", "etc/passwd",
                     "\[boot loader\]", "boot.ini",
                     "; for 16-bit app supporT", "winnt/win.ini",
                     "; for 16-bit app support", "windows/win.ini" );
}

# Add a dependency to gb_open_udp_ports.nasl if you want to use this
function get_all_udp_ports( ) {

  local_var port;

  port = get_kb_item( "UDP/PORTS" );
  if( ! port ) exit( 0 );
  if( ! get_udp_port_state( port ) ) exit( 0 );
  return port;
}

# Add a dependency to gb_open_udp_ports.nasl if you want to use this
function get_all_udp_ports_list( ) {

  local_var _ports, ports, port;

  ports = make_list();
  _ports = get_kb_list( "UDP/PORTS" );

  foreach port( _ports ) {
    if( ! get_udp_port_state( port ) ) continue;
    ports = make_list( ports, port );
  }
  return ports;
}

# @brief Gets the hostname of the target via get_host_name() and
#        if no IPv4/IPv6 address was returned by this function it
#        returns a list containing the parts of the target hostname.
#
#        For example a hostname of www.example.com returns a list containing
#        the following:
#        1 -> www
#        2 -> example
#        3 -> com
#        4 -> www.example
#        5 -> www.example.com
#        6 -> example.com
#        7 -> com.example
#        8 -> com.example.www
#
# @todo We currently can't catch a example.com from www.sub.example.com
#
# @return a list containing the parts of the target hostname, an empty list
#         if get_host_name() returned an IPv4/IPv6 address.
#
function create_hostname_parts_list( ) {

  local_var list, hn, hnp, hnl, p, parts, i;

  list = make_list();
  hn   = get_host_name();

  # nb: We don't want to add an IPv4/IPv6 address here...
  if( ":" >!< hn && ! ereg( string:hn, pattern:"^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$" ) ) {

    hnp = split( hn, sep:".", keep:FALSE );
    hnl = max_index( hnp );

    parts = "";
    for( i = 0; i < hnl; i++ ) {
      # each single entry on its own
      list = make_list( list, hnp[i] );
      # concatenate each entry with the previous one
      parts += "." + hnp[i];
      parts = ereg_replace( pattern:"^\.", string:parts, replace:'' );
      if( ! in_array( search:parts, array:list ) )
        list = make_list( list, parts );
    }

    parts = "";
    for( i = 1; i < hnl; i++ ) {
      # This is a special case to skip the first part like www from www.example.com
      parts += "." + hnp[i];
      parts = ereg_replace( pattern:"^\.", string:parts, replace:'' );
      if( ! in_array( search:parts, array:list ) )
        list = make_list( list, parts );
    }

    parts = "";
    for( i = hnl - 1; i >= 0; i-- ) {
      # concatenate each entry with the previous one
      parts += "." + hnp[i];
      parts = ereg_replace( pattern:"^\.", string:parts, replace:'' );
      if( ! in_array( search:parts, array:list ) )
        list = make_list( list, parts );
    }
  }
  return list;
}

# @brief Checks if the NVT calling this function is executed on a Greenbone OS (GOS).
#
# @return TRUE if executed on a GOS, FALSE if not executed on a GOS and NULL if the
#              function vendor_version() needed for this check is missing (e.g. running
#              a GOS/GVM/OpenVAS installation not providing this function yet).
#
function executed_on_gos() {

  if( ! defined_func( "vendor_version" ) ) return NULL;

  if( vendor_version() =~ "^Greenbone OS" ) {
    return TRUE;
  } else {
    return FALSE;
  }
}

# @brief Tries to detect the local Greenbone OS (GOS) version of the NVT calling this function.
#
# @return The version number as a string if executed on a GOS and it was possible to gather the version,
#         FALSE if not executed on a GOS and NULL if it was not possible to gather the version.
#
function get_local_gos_version() {

  local_var result;

  if( isnull( executed_on_gos() ) ) {
    return NULL;
  } else if( ! executed_on_gos() ) {
    return FALSE;
  } else {
    result = eregmatch( pattern:"([0-9.]+)$", string:vendor_version() );
    return result[1];
  }
}

# @brief Returns an array containing various representations of a Vulnerability Test (VT) string which can
#        be used in VTs as e.g. a user name.
#
# @note The returned string depends on the platform (OpenVAS/GVM/GOS) the scanner is running at.
#
# @return An array containing various string representations of either "GBN-VT" or "OpenVAS-VT"
#         depending on the platform the scanner is running at. Currently the following array indices
#         are supported / available:
#
#         ret_array["default"] = either "GBN-VT" or "OpenVAS-VT"
#         ret_array["default_rand"] = "default" with random numbers appended
#         ret_array["default_hex"] = "default" converted to a hex string
#         ret_array["default_rand_hex"] = "default_rand" converted to a hex string
#
#         ret_array["ping_string"] = either "GBN-VT" or "OpenVAS-VT" in a form of e.g. "_GBN-VT1234_" filled up to 16 bytes of total length, for the use in the "-p" parameter of unixoide ping commands which allows a max. of 16 bytes in the passed string
#
#         ret_array["lowercase"] = either "gbn-vt" or "openvas-vt"
#         ret_array["lowercase_rand"] = "lowercase" with random numbers appended
#         ret_array["lowercase_hex"] = "lowercase" converted to a hex string
#         ret_array["lowercase_rand_hex"] = "lowercase_rand" converted to a hex string
#
#         ret_array["uppercase"] = either "GBN-VT" or "OPENVAS-VT"
#         ret_array["uppercase_rand"] = "uppercase" with random numbers appended
#         ret_array["uppercase_hex"] = "uppercase" converted to a hex string
#         ret_array["uppercase_rand_hex"] = "uppercase_rand" converted to a hex string
#
function get_vt_strings() {

  local_var ret_array, vt_string, vt_string_lo, vt_string_up, rand_numbers;

  ret_array = make_array();

  if( executed_on_gos() ) {
    vt_string = "GBN-VT";
  } else {
    vt_string = "MVULNSCAN";
  }

  vt_string_lo = tolower( vt_string );
  vt_string_up = toupper( vt_string );
  rand_numbers = string( rand() ); # nb: Returns an integer with a length between 9 and 10, converting it to string first

  ret_array["default"] = vt_string;
  ret_array["default_rand"] = vt_string + rand_numbers;
  ret_array["default_hex"] = hexstr( vt_string );
  ret_array["default_rand_hex"] = hexstr( vt_string + rand_numbers );

   # nb: e.g. for the use in the "-p" parameter of unixoide ping commands which allows a max. of 16 bytes in the passed string
  ret_array["ping_string"] = "_" + vt_string + substr( rand_numbers, 0, 16 - strlen( vt_string ) - 3 ) + "_";

  ret_array["lowercase"] = vt_string_lo;
  ret_array["lowercase_rand"] = vt_string_lo + rand_numbers;
  ret_array["lowercase_hex"] = hexstr( vt_string_lo );
  ret_array["lowercase_rand_hex"] = hexstr( vt_string_lo + rand_numbers );

  ret_array["uppercase"] = vt_string_up;
  ret_array["uppercase_rand"] = vt_string_up + rand_numbers;
  ret_array["uppercase_hex"] = hexstr( vt_string_up );
  ret_array["uppercase_rand_hex"] = hexstr( vt_string_up + rand_numbers );

  return ret_array;
}
