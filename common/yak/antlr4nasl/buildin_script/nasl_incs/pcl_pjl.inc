# Copyright (C) 2019 Greenbone Networks GmbH
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

# @brief Returns an array containing various PCL/PJL requests as the array
#        index and the expected result/response to this request as the array
#        value.
#
# @return An array with the information as described in the function summary.
#
function pcl_pjl_get_detect_requests() {

  local_var reqs, final_reqs;

  final_reqs = make_array();

  # PJL5:
  # http://h10032.www1.hp.com/ctg/Manual/bpl13208.pdf
  # http://h10032.www1.hp.com/ctg/Manual/bpl13207.pdf
  # https://developers.hp.com/system/files/PJL_Technical_Reference_Manual.pdf
  reqs = make_list( "@PJL INFO ID", "@PJL INFO STATUS", "@PJL INFO LOG", "@PJL INFO SUPPLIES", "@PJL INFO PRODINFO", "@PJL ECHO" + vt_strings["default"] );
  foreach req( reqs )
    final_reqs['\x1b%-12345X' + req + ' \r\n\x1b%-12345X\r\n'] = req + '\r\n';

  # PCL5:
  # https://web.archive.org/web/20151122184353/http://download.brother.com/welcome/doc002907/Tech_Manual_Y.pdf
  # nb: While the document says the response should look like e.g. "PCL<CR><LF>ECHO" or "PCL<CR><LF>INFO MEMORY"
  # some devices are returning e.g. "PCL<LF><NUL>ECHO".
  final_reqs[raw_string( 0x1B ) + "*s1M"] = 'PCL';
  final_reqs[raw_string( 0x1B ) + "*s#X"] = 'PCL';

  # PCL3:
  # https://web.archive.org/web/20160313083904/http://www.service.belhard.com/documentation/PCL3_developers_guide.pdf
  # https://web.archive.org/web/20090304014343/http://home.vrweb.de:80/martin.lottermoser/pcl3/index.html
  # TODO, but it seems there are no commands available which are returning us some data.

  return final_reqs;
}

# @brief Register the PCL/PJL ports of a system in the KB.
#
# @param ports The (optional) list of ports to use/register in the KB.
#
# @return NULL
#
function pcl_pjl_register_all_ports( ports ) {

  local_var ports, port;

  if( isnull( ports ) || typeof( ports ) != "array" ) {
    ports = pcl_pjl_get_default_ports();
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

# @brief Creates a list of default PCL/PJL ports.
#        https://www.ibm.com/support/pages/recommended-pjl-port-number-values-lan-3812-pjl-device-descriptions
#
# @return The created list of default PCL/PJL ports.
#
function pcl_pjl_get_default_ports() {

  local_var ports;

  ports = make_list( 2000, 2501, 9100, 9101, 9102, 9103, 9104, 9105, 9106, 9107, 9112, 9113, 9114, 9115, 9116, 10001 );

  return ports;
}

# @brief Creates a comma separated string of default PCL/PJL ports.
#
# @return The comma separated string of default PCL/PJL ports.
#
function pcl_pjl_get_default_ports_string() {

  local_var ports;

  ports = "2000,2501,9100,9101,9102,9103,9104,9105,9106,9107,9112,9113,9114,9115,9116,10001";

  return ports;
}
