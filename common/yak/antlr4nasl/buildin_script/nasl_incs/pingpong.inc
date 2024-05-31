# OpenVAS Vulnerability Test
# $Id: pingpong.inc 10964 2011-05-18 20:50:52Z jan $
# Description: Method for a udp pingpong
#
# Authors:
# Michel Arboi
#
# Copyright:
# Copyright (C) 2003 Michel Arboi
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

# "ping pong" is possible with any service. However, it is really dangerous
# against chargen or echo. Some "simple services" do not answer if the
# source port is privileged, thus preventing the exploitation.

function udp_ping_pong(port, data, answer)
{
  local_var	s, r1, r2;

  if (!port || ! get_udp_port_state(port)) return 0;

  if (! answer)  # or isnull(answer)?
  {
    s = open_sock_udp(port);
    if (!s)  return 0;
    send(socket: s, data: data);
    r1 = recv(socket: s, length: 1024);
    close(s);
  }
  else
    r1 = answer;

  if (! r1) return 0;	# Cannot test

  s = open_priv_sock_udp(dport: port);
  if (!s)  return 0;
  send(socket: s, data: data);
  r2 = recv(socket: s, length: 1024);
  close(s);
  if (! r2) return 0;	# Not vulnerable
  #display("udp_ping_pong: port=", port, " - the service answers only to non privileged sockets\n");
  return 1;    
}
