###############################################################################
# Functions for Moxa
# $Id: moxa.inc 12369 2018-11-16 07:00:32Z cfischer $
#
# Authors:
# Christian Kuersteiner <christian.kuersteiner@greenbone.net>
#
# Copyright:
# Copyright (c) 2017 Greenbone Networks GmbH
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
# $Revision: 12369 $
#
###############################################################################

# from dsci.dll in NPort Administration Suite
moxa_models = make_array("105100801051", "NPort 5110",
                         "005101801a51", "NPort 5110A",
                         "005100803051", "NPort 5130",
                         "005101803a51", "NPort 5130A",
                         "005100805051", "NPort 5150",
                         "120300802203", "NPort 5210",
                         "005201801a52", "NPort 5210A",
                         "120300801203", "NPort 5230",
                         "120300803203", "NPort 5232",
                         "120300803213", "NPort 5232I",
                         "005000800405", "NPort 5410",
                         "005400800154", "NPort 5410 V3",
                         "005000803405", "NPort 5430",
                         "005400800254", "NPort 5430 V3",
                         "005000803415", "NPort 5430I",
                         "005400800354", "NPort 5430I V3",
                         "005400800454", "NPort 5450",
                         "005400800554", "NPort 5450I",
                         "005400800654", "NPort 5450",
                         "005400800754", "NPort 5450I",
                         "105600801856", "NPort 5610-8",
                         "105600801356", "NPort 5610-16",
                         "105600801256", "NPort 5612-16",
                         "105600803856", "NPort 5630-8",
                         "105600803356", "NPort 5630-16",
                         "105600805856", "NPort 5650-8",
                         "105600805356", "NPort 5650-16",
                         "105600805956", "NPort 5650-8-M-SC",
                         "105600805456", "NPort 5650-16-M-SC",
                         "105600805a56", "NPort 5650-8-S-SC",
                         "105600805556", "NPort 5650-16-S-SC",
                         "105601800057", "NPort 5610-8-DT",
                         "105601800457", "NPort 5610-8-DT-J",
                         "105601800257", "NPort 5650-8-DT",
                         "105601800357", "NPort 5650I-8-DT",
                         "105601800657", "NPort 5650-8-DT-J",
                         "045000800420", "NPort W2004-US",
                         "045000801420", "NPort W2004-EU",
                         "045000802420", "NPort W2004-JP",
                         "502200805121", "NPort W2150-US",
                         "502200805221", "NPort W2150-EU",
                         "502200805321", "NPort W2150-JP",
                         "502200805122", "NPort W2250-US",
                         "502200805222", "NPort W2250-EU",
                         "502200805322", "NPort W2250-JP",
                         "004100800941", "NE-4100T",
                         "004100801841", "NE-4110A",
                         "104100801841", "NE-4110A-CMD",
                         "004100801941", "NE-4110S",
                         "104100801941", "NE-4110S-CMD",
                         "004100802841", "NE-4120A",
                         "104100802841", "NE-4120A-CMD",
                         "004100802941", "NE-4120S",
                         "104100802941", "NE-4120S-CMD",
                         "002100800021", "WE-2100T-US",
                         "002100800121", "WE-2100T-EU",
                         "002100800221", "WE-2100T-JP",
                         "110200000300", "DE-211",
                         "010300000100", "DE-301",
                         "020300000100", "DE-302",
                         "040300000100", "DE-304",
                         "110300000300", "DE-311",
                         "118300000300", "DE-311M",
                         "340300000200", "DE-334",
                         "182500000100", "CN2510-8",
                         "132500000100", "CN2510-16",
                         "182600000100", "CN2610-8",
                         "132600000100", "CN2610-16",
                         "002600805326", "CN2650-8",
                         "002600805426", "CN2650-16",
                         "002600805726", "CN2650I-8",
                         "002600805826", "CN2650I-16",
                         "002600805926", "CN2610-8-2AC",
                         "002600805a26", "CN2610-16-2AC",
                         "002600805126", "CN2650-8-2AC",
                         "002600805226", "CN2650-16-2AC",
                         "002600805526", "CN2650I-8-2AC",
                         "002600805626", "CN2650I-16-2AC",
                         "505200805151", "NPort IA-5150",
                         "505200805251", "NPort IA-5150I",
                         "505200805351", "NPort IA-5150-S-SC",
                         "505200805451", "NPort IA-5150I-S-SC",
                         "505200805551", "NPort IA-5150-M-SC",
                         "505200805651", "NPort IA-5150I-M-SC",
                         "505200805152", "NPort IA-5250",
                         "505200805052", "NPort IA-5250I",
                         "006000805061", "NPort 6150",
                         "006000805062", "NPort 6250",
                         "006000805162", "NPort 6250-S-SC",
                         "006000805262", "NPort 6250-M-SC",
                         "006000805064", "NPort 6450",
                         "006000805866", "NPort 6650-8",
                         "006000805366", "NPort 6650-16",
                         "006000805466", "Port 6610-8",
                         "006000805566", "NPort 6610-16",
                         "006000805666", "NPort 6610-32",
                         "006000805766", "NPort 6650-32",
                         "502300805123", "NPort W2150Plus-US",
                         "502300805223", "NPort W2150Plus-EU",
                         "502300805323", "NPort W2150Plus-JP",
                         "502300805423", "NPort W2250Plus-US",
                         "502300805523", "NPort W2250Plus-EU",
                         "502300805623", "NPort W2250Plus-JP");

# Return the Moxa model based on the device ID received on 4800/udp
function moxa_get_model(data)
{
  foreach mo (keys(moxa_models)) {
    if (hexstr(data) == mo)
      return moxa_models[mo];
  }
  return "unknown";
}


# Send and receive for Moxa management protocol on 4800/udp
function moxa_send_recv(socket, data)
{
  send(socket: socket, data: data);
  recv = recv(socket: socket, length: 4);
  if (strlen(recv) < 4)
    return;
 
  len = getword(blob: recv, pos: 2);
 
  return recv(socket: socket, length: len);
}
