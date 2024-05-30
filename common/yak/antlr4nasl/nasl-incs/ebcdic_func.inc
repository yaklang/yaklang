# Copyright (C) 2020 Greenbone Networks GmbH
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

global_var EBCDIC_TABLE;
EBCDIC_TABLE = raw_string(
  0000, 0001, 0002, 0003, 0234, 0011, 0206, 0177, 0227, 0215, 0216, 0013, 0014, 0015, 0016, 0017,
  0020, 0021, 0022, 0023, 0235, 0205, 0010, 0207, 0030, 0031, 0222, 0217, 0034, 0035, 0036, 0037,
  0200, 0201, 0202, 0203, 0204, 0012, 0027, 0033, 0210, 0211, 0212, 0213, 0214, 0005, 0006, 0007,
  0220, 0221, 0026, 0223, 0224, 0225, 0226, 0004, 0230, 0231, 0232, 0233, 0024, 0025, 0236, 0032,
  0040, 0240, 0342, 0344, 0340, 0341, 0343, 0345, 0347, 0361, 0242, 0056, 0074, 0050, 0053, 0174,
  0046, 0351, 0352, 0353, 0350, 0355, 0356, 0357, 0354, 0337, 0041, 0044, 0052, 0051, 0073, 0254,
  0055, 0057, 0302, 0304, 0300, 0301, 0303, 0305, 0307, 0321, 0246, 0054, 0045, 0137, 0076, 0077,
  0370, 0311, 0312, 0313, 0310, 0315, 0316, 0317, 0314, 0140, 0072, 0043, 0100, 0047, 0075, 0042,
  0330, 0141, 0142, 0143, 0144, 0145, 0146, 0147, 0150, 0151, 0253, 0273, 0360, 0375, 0376, 0261,
  0260, 0152, 0153, 0154, 0155, 0156, 0157, 0160, 0161, 0162, 0252, 0272, 0346, 0270, 0306, 0244,
  0265, 0176, 0163, 0164, 0165, 0166, 0167, 0170, 0171, 0172, 0241, 0277, 0320, 0335, 0336, 0256,
  0136, 0243, 0245, 0267, 0251, 0247, 0266, 0274, 0275, 0276, 0133, 0135, 0257, 0250, 0264, 0327,
  0173, 0101, 0102, 0103, 0104, 0105, 0106, 0107, 0110, 0111, 0255, 0364, 0366, 0362, 0363, 0365,
  0175, 0112, 0113, 0114, 0115, 0116, 0117, 0120, 0121, 0122, 0271, 0373, 0374, 0371, 0372, 0377,
  0134, 0367, 0123, 0124, 0125, 0126, 0127, 0130, 0131, 0132, 0262, 0324, 0326, 0322, 0323, 0325,
  0060, 0061, 0062, 0063, 0064, 0065, 0066, 0067, 0070, 0071, 0263, 0333, 0334, 0331, 0332, 0237);

# @brief Converts a EBCDIC string into an ASCII string using the EBCDIC_TABLE conversion table.
#
# @param ebcdic The EBCDIC string as a raw string.
#
# @return The ASCII string representing the EBCDIC string or NULL in case of no ebcdic parameter is provided.
#
function ebcdic2ascii(ebcdic) {
  local_var ebcdic;
  local_var len, ascii, i;

  if( isnull( ebcdic ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ebcdic#-#ebcdic2ascii" );
    return NULL;
  }

  len = strlen( ebcdic );
  ascii = "";

  for( i = 0; i < len; i++ ) {
    char = ord( ebcdic[i] );
    ascii += EBCDIC_TABLE[char];
  } 

  return ascii;
}
