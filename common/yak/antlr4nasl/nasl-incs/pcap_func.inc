# Copyright (C) 2021 Greenbone Networks GmbH
#
# SPDX-License-Identifier: GPL-2.0-or-later
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.

# @brief Sends a standard TCP / UDP packet with the given data and returns the "data" element part
#        of the response / the received packet.
#
# @param port        The (mandatory) port for the connection to the remote service.
# @param srcport     The (optional) source port to be used to send the data from. Defaults to a
#                    random generated port > 1024 if not given.
# @param data        The (mandatory) data to sent to the remote service.
# @param proto       The IP protocol (tcp or udp) for the connection to the remote service. Defaults
#                    to "udp" if not given.
# @param debug       If set to TRUE additional debug output is printed like e.g. the crafted and
#                    received IP and UDP / TCP packets and similar info.
# @param pcap_filter The (optional) PCAP filter which the received packets needs to be matching
#                    against. If not given a default filter is used which consists of:
#                     - The source port
#                     - The destination port
#                     - The source IP
#                     - The target IP
#                     - The chosen protocol (tcp or udp)
#
# @note This functions is explicitly setting the default values for each IP / UDP / TCP header as
#       used by GVM 21.04 / 21.10 to keep backward compatibility (in VTs which are using this
#       function) and to avoid problems if these defaults are ever changed in GVM in the future.
#
# @return The received "data" element of the received UDP / TCP packet or NULL if not all mandatory
#         parameter are filled or no data / packet was received (e.g. none matching the pcap filter,
#         pcap timeout reached, ...).
#
function pcap_tcp_udp_send_recv( port, srcport, data, proto, debug, pcap_filter ) {

  local_var port, srcport, data, proto, debug, pcap_filter;
  local_var PCAP_TIMEOUT, ownip, targetip, dstport, IPPROTO, IPV6_VERSION;
  local_var ip_packet, tcp_or_udp_packet, res;

  if( ! port ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#pcap_tcp_udp_send_recv" );
    return NULL;
  }

  # nb: Allow an empty string
  if( isnull( data ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#pcap_tcp_udp_send_recv" );
    return NULL;
  }

  if( ! proto )
    proto = "udp";

  if( ! srcport )
    srcport = rand() % (65536 - 1024) + 1024;

  # TBD: If islocalhost() and port == srcport also generate a random port and warn about it?

  PCAP_TIMEOUT = 5; # nb: The current default of send_packet / send_v6_packet. TBD: Make this configurable (e.g. via an additional function parameter)?
  ownip = this_host();
  targetip = get_host_ip();
  dstport = port; # nb: Just for easier distinction with the srcport later.

  if( proto == "tcp" ) {

    # nb: We shouldn't allow an empty PCAP filter so don't check with isnull() here...
    if( ! pcap_filter )
      pcap_filter = "src host " + targetip + " and dst host " + ownip + " and tcp and tcp src port " + dstport + " and tcp dst port " + srcport;

    IPPROTO = IPPROTO_TCP;
  } else {
    # nb: We shouldn't allow an empty PCAP filter so don't check with isnull() here...
    if( ! pcap_filter )
      pcap_filter = "src host " + targetip + " and dst host " + ownip + " and udp and udp src port " + dstport + " and udp dst port " + srcport;

    IPPROTO = IPPROTO_UDP;
  }

  if( debug )
    display( "---[ Chosen / passed PCAP filter ]---", "\n", pcap_filter );

  if( TARGET_IS_IPV6() ) {

    # nb: Previous versions had calculated a wrong IPv6 flow label
    # (see https://github.com/greenbone/openvas/pull/545) which
    # requires this workaround here.
    # TODO: Remove once all GOS/GVM versions < 20.8.0 are EOL.
    if( version_is_less( version:OPENVAS_VERSION, test_version:"20.8.0" ) )
      IPV6_VERSION = 0x60;
    else
      IPV6_VERSION = 6;

    # TODO: Use forge_ip_v6_packet once all GOS/GVM versions < 20.8.0 are EOL.
    ip_packet = forge_ipv6_packet( ip6_v:IPV6_VERSION,
                                   ip6_p:IPPROTO,
                                   ip6_src:ownip,
                                   ip6_dst:targetip,
                                   # nb: Those are the defaults from (see note in function description on the reason):
                                   # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery_v6.c#L112-L126
                                   ip6_tc:0,
                                   ip6_fl:0,
                                   ip6_hlim:64 );

    if( ! ip_packet ) {
      if( debug )
        display( "---[ Failed to craft IPv6 packet ]---" );

      return NULL;
    }

    if( debug ) {
      display( "---[ Crafted IPv6 packet ]---" );
      # TODO: Use dump_ip_v6_packet once all GOS/GVM versions < 20.8.0 are EOL.
      dump_ipv6_packet( ip_packet );
    }

    if( proto == "tcp" ) {

      tcp_or_udp_packet = forge_tcp_v6_packet( ip6:ip_packet,
                                               data:data,
                                               th_sport:srcport,
                                               th_dport:dstport,
                                               # nb: Those are the defaults from (see note in function description on the reason):
                                               # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery_v6.c#L520-L538
                                               th_seq:rand(),
                                               th_ack:0,
                                               th_x2:0,
                                               th_off:5,
                                               th_flags:0,
                                               th_win:0,
                                               th_urp:0,
                                               update_ip_len:TRUE );

      if( ! tcp_or_udp_packet ) {
        if( debug )
          display( "---[ Failed to craft TCP packet ]---" );

        return NULL;
      }

      if( debug ) {
        display( "---[ Crafted TCP packet ]---" );
        dump_tcp_v6_packet( tcp_or_udp_packet );
      }
    } else {

      tcp_or_udp_packet = forge_udp_v6_packet( ip6:ip_packet,
                                               data:data,
                                               uh_sport:srcport,
                                               uh_dport:dstport,
                                               # nb: Those are the defaults from (see note in function description on the reason):
                                               # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery_v6.c#L1390-L1403
                                               uh_ulen:strlen( data ) + 8,
                                               update_ip6_len:TRUE );

      if( ! tcp_or_udp_packet ) {
        if( debug )
          display( "---[ Failed to craft UDP packet ]---" );

        return NULL;
      }

      if( debug ) {
        display( "---[ Crafted UDP packet ]---" );
        dump_udp_v6_packet( tcp_or_udp_packet );
      }
    }

    res = send_v6packet( tcp_or_udp_packet, pcap_active:TRUE, pcap_filter:pcap_filter, pcap_timeout:PCAP_TIMEOUT );
    if( ! res ) {
      if( debug )
        display( "---[ No IPv6 packet received (Reasons e.g.: none matching the pcap filter, pcap timeout reached, ...) ]---" );

      return NULL;
    }

    if( debug ) {
      display( "---[ Received IPv6 packet ]---" );
      # TODO: Use dump_ip_v6_packet once all GOS/GVM versions < 20.8.0 are EOL.
      dump_ipv6_packet( res );
    }

    if( proto == "tcp" ) {
      if( debug ) {
        display( "---[ Received TCP packet ]---" );
        dump_tcp_v6_packet( res );
      }

      res = get_tcp_v6_element( tcp:res, element:"data" );
    } else {
      if( debug ) {
        display( "---[ Received UDP packet ]---" );
        dump_udp_v6_packet( res );
      }

      res = get_udp_v6_element( udp:res, element:"data" );
    }
  } else {

    ip_packet = forge_ip_packet( ip_v:4,
                                 ip_p:IPPROTO,
                                 ip_off:IP_DF,
                                 ip_src:ownip,
                                 ip_dst:targetip,
                                 # nb: Those are the defaults from (see note in function description on the reason):
                                 # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery.c#L89-L111
                                 ip_hl:5,
                                 ip_id:rand(),
                                 ip_len:strlen( data ) + 20,
                                 ip_tos:0,
                                 ip_ttl:64 );

    if( ! ip_packet ) {
      if( debug )
        display( "---[ Failed to craft IPv4 packet ]---" );

      return NULL;
    }

    if( debug ) {
      display( "---[ Crafted IPv4 packet ]---" );
      dump_ip_packet( ip_packet );
    }

    if( proto == "tcp" ) {

      tcp_or_udp_packet = forge_tcp_packet( ip:ip_packet,
                                            data:data,
                                            th_sport:srcport,
                                            th_dport:dstport,
                                            # nb: Those are the defaults from (see note in function description on the reason):
                                            # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery.c#L536-L562
                                            th_ack:0,
                                            th_flags:0,
                                            th_off:5,
                                            th_seq:rand(),
                                            th_urp:0,
                                            th_win:0,
                                            th_x2:0,
                                            update_ip_len:TRUE );

      if( ! tcp_or_udp_packet ) {
        if( debug )
          display( "---[ Failed to craft TCP packet ]---" );

        return NULL;
      }

      if( debug ) {
        display( "---[ Crafted TCP packet ]---" );
        dump_tcp_packet( tcp_or_udp_packet );
      }
    } else {

      tcp_or_udp_packet = forge_udp_packet( ip:ip_packet,
                                            data:data,
                                            uh_sport:srcport,
                                            uh_dport:dstport,
                                            # nb: Those are the defaults from (see note in function description on the reason):
                                            # https://github.com/greenbone/openvas-scanner/blob/v21.4.2/nasl/nasl_packet_forgery.c#L1440-L1458
                                            uh_ulen:strlen( data ) + 8,
                                            update_ip_len:TRUE );

      if( ! tcp_or_udp_packet ) {
        if( debug )
          display( "---[ Failed to craft UDP packet ]---" );

        return NULL;
      }

      if( debug ) {
        display( "---[ Crafted UDP packet ]---" );
        dump_udp_packet( tcp_or_udp_packet );
      }
    }

    res = send_packet( tcp_or_udp_packet, pcap_active:TRUE, pcap_filter:pcap_filter, pcap_timeout:PCAP_TIMEOUT );
    if( ! res ) {
      if( debug )
        display( "---[ No IPv4 packet received (Reasons e.g.: none matching the pcap filter, pcap timeout reached, ...) ]---" );

      return NULL;
    }

    if( debug ) {
      display( "---[ Received IPv4 packet ]---" );
      dump_ip_packet( res );
    }

    if( proto == "tcp" ) {
      if( debug ) {
        display( "---[ Received TCP packet ]---" );
        dump_tcp_packet( res );
      }

      res = get_tcp_element( tcp:res, element:"data" );
    } else {
      if( debug ) {
        display( "---[ Received UDP packet ]---" );
        dump_udp_v6_packet( res );
      }

      res = get_udp_element( udp:res, element:"data" );
    }
  }

  if( ! res ) {
    if( debug )
      display( "---[ Failed to extract 'data' element from received " + toupper( proto ) + " packet ]---" );

    return NULL;
  }

  if( debug )
    display( "---[ Extracted 'data' element from received " + toupper( proto ) + " packet ]---", "\n", hexdump( ddata:res ) );


  return res;
}