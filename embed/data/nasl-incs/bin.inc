#########################################################################
# OpenVAS Include File
# $Id$
#
# Developed by LSS Security Team <http://security.lss.hr>
#
# Copyright (C) 2010 LSS <http://www.lss.hr>
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public
# License along with this program. If not, see
# <http://www.gnu.org/licenses/>.
#
# $Revision$
#
#########################################################################

#########################################################################
#
#  This small library implements three binary data handling methods:
#    bin_pack(format, anon_arg1, anon_arg2...)
#    bin_unpack(format, blob, pos, anon_arg1, anon_arg2...)
#    align(blob, boundary, al)
#  Very userfull tool when implementing data (de)serialization, (un)marshalling
#  for various network protocols (like RPC). Could be used as a base include for
#  machine independent data encoding schemes like NDR, XDR.
#  Similar methods are implemented in other scripting languages such as Python,
#  Ruby and Lua. NMAPs NSE scripts and Metasploit modules find extensive use of
#  these methods in handling binary network protocols. NASL lacks such a utility,
#  so the idea and design is ported to NASL.
#
#  References:
#  [1] Python Struct lib  http://docs.python.org/library/struct.html
#  [2] Ruby  http://ruby-doc.org/doxygen/1.8.4/pack_8c.html
#  [3] Lua BINlib NMAPs NSE reference  http://nmap.org/nsedoc/lib/bin.html
#
#  Examples of binary protocols: ethernet, tcp/udp, ip, netbios, smb, dce/rpc...
#  Examples of non-binary/plain-text/human-readable  protocols: http, irc, smtp, pop3...
#  
#  Read/Write operators for bin_unpack and bin_pack methods:
#    1) B - uint8 <-> 1B blob
#    2) S - uint16 <-> 2B blob
#    3) L - uint32 <-> 4B blob
#    4) H - uint64 <-> 8B blob
#    5) A - string <-> strlen(string)B blob
#    6) z - string <-> (strlen(string)+1)B blob (null terminated)
#    7) X - hex string <-> varB blob
#  Endianness operators for bin_unpack and bin_pack methods:
#    1) < - little endian
#    2) > - big endian
#    3) ! - network endian (big endian)
#    4) = - native endian (platform dependent)
#
#  TERMS and DEFINITIONS:
#    2) data - The term data means groups of information that represent the
#      qualitative or quantitative attributes of a variable or set of variables.
#      It differs from raw data or a blob because data is processed and
#      interpreted.
#    1) blob - a binary representation of an object. An object could  be a
#      simple data type such as an integer or a string. For example a two
#      byte blob representing number 7 would be "0x0007". NASL blob is of
#      string type. Blob is neither processed nor interpreted, it's raw data.
#    2) endianness - often simply referred to as byte order. Two types of
#      endianness are BIG and LITTLE endianness.
#      Wikipedia  endianness: "Most versus least significant byte first,
#      called big-endian and little-endian respectively."
#      (NOTE: endianness is actually spelled wrong all over that NVT.)
#  TODO:
#  -- add read/write for:
#    1)|strlen(string)|string| operators such that length
#    prefixes come in different sizes (byte, short, long)
#    2)fake unicode
#  -- add dealign method
#  EXAMPLE:
#    blob = bin_pack(format: "!BSLHxAzX", 0x12, 0x1234, 0x12345678, 0x12, "AAAA", "BBBB", "0x1234");
#    unpack_struct = bin_unpack(format: "!BSLHxAzX", blob: blob, 4, 2);
#    data = unpack_struct['data'];
#    blob_len = strlen(unpack_struct['blob']);
#    display("Unpacked " + blob_len + "B of data" + '\n');
#    dump_arr(arr: data, title: "Unpacked data");
#########################################################################

#endianness operator constants
global_var OP_LITTLE_ENDIAN, OP_BIG_ENDIAN, OP_NATIVE_ENDIAN, OP_NETWORK_ENDIAN, OP_DEFAULT_ENDIAN;
OP_LITTLE_ENDIAN = '<';
OP_BIG_ENDIAN = '>';
OP_NATIVE_ENDIAN = '=';
OP_NETWORK_ENDIAN = '!'; #actually big endian
OP_DEFAULT_ENDIAN = OP_NATIVE_ENDIAN;

#write/read operator constants
global_var OP_UINT_8, OP_UINT_16, OP_UINT_32, OP_UINT_64, OP_PAD_BYTE, VAL_PAD_BYTE, OP_STRING, OP_STRING_NULL, OP_STRING_HEX;
OP_UINT_8 = 'B'; #unsigned char -> 1B blob
OP_UINT_16 = 'S'; #unsigned short -> 2B blob
OP_UINT_32 = 'L'; #unsigned long -> 4B blob
OP_UINT_64 = 'H'; #unsigned long long, aka hyper long -> 8B blob
OP_PAD_BYTE = 'x'; #pad byte - 1B blob
VAL_PAD_BYTE = '\0'; #pad byte value, default = NULL
OP_STRING = 'A'; #string -> strlen(string) B blob
OP_STRING_NULL = 'z'; #string -> strlen(string) + 1 (null terminator) B blob
OP_STRING_HEX = 'X'; #hex string -> raw string B blob

#  @brief Parameters/objects are packed into a blob according to a format string.
#  @param format Format string that describes how the following arguments will be packed.
#    @default If not explicitly stated endianness is OP_DEFAULT_ENDIAN.
#  @param _FCT_ANON_ARGS Implicit array of arguments. For each write operator in 'format'
#    a corresponding object in _FCT_ANON_ARGS array must exist.
#  @return Returns a blob.
function bin_pack(format){
  local_var args;
  local_var format_len, endianness, iOp;
  local_var result, iArg;

  args = _FCT_ANON_ARGS;
  format_len = strlen(format);
  if(format[0] == OP_LITTLE_ENDIAN || format[0] == OP_BIG_ENDIAN || format[0] == OP_NATIVE_ENDIAN || format[0] == OP_NETWORK_ENDIAN){
    endianness = format[0];
    iOp = 1;
  }
  else{
    endianness = OP_DEFAULT_ENDIAN;
    iOp = 0;
  }
  #Process format string
  result = '';
  iArg = 0;
  for(iOp = iOp; iOp < format_len; iOp++){
    if(format[iOp] == OP_UINT_8){
      result += pack_uint_8(data: args[iArg]);
      iArg++;
    }
    else if(format[iOp] == OP_UINT_16){
      result += pack_uint_16(data: args[iArg], endianness: endianness);
      iArg++;
    }
    else if(format[iOp] == OP_UINT_32){
      result += pack_uint_32(data: args[iArg], endianness: endianness);
      iArg++;
    }
    else if(format[iOp] == OP_UINT_64){
      result += pack_uint_64(data: args[iArg], endianness: endianness);
      iArg++;
    }
    else if(format[iOp] == OP_PAD_BYTE){
      result += pack_pad_byte();
    }
    else if(format[iOp] == OP_STRING){
      result += pack_string(data: args[iArg]);
      iArg++;
    }
    else if(format[iOp] == OP_STRING_NULL){
      result += pack_string_null(data: args[iArg]);
      iArg++;
    }
    else if(format[iOp] == OP_STRING_HEX){
      result += pack_string_hex(data: args[iArg]);
      iArg++;
    }
    else if(format[iOp] == OP_LITTLE_ENDIAN || format[iOp] == OP_BIG_ENDIAN || format[iOp] == OP_NATIVE_ENDIAN || format[iOp] == OP_NETWORK_ENDIAN){
      endianness = format[iOp];
      continue;
    }
  }
  return result;
}

#  @brief Blob is unpacked according to the unpacking format.
#  @param format Format string that describes how the blob will be unpacked.
#    @default If not explicitly stated endianness is OP_DEFAULT_ENDIAN.
#  @param blob A blob which is parsed and unpacked according to the 'format'.
#  @param pos  Position within 'blob' from where the parsing starts.
#    @default 0
#  @param _FCT_ANON_ARGS Implicit array of length values for each OP_STRING or
#    OP_STRING_HEX in 'format'.
#  @return Returns an array structured as:
#    return['blob'] - a part of 'blob' which has been processed to produce return['data']
#    return['data'] - array containing unpacked data. For every write/read
#      operator found in 'format' there exists an array element indexed by operators placement in 'format' string.
#    @note When naming the return struct I suggest 'unpack_struct'.
function bin_unpack(format, blob, pos){
  local_var lens, pos_old;
  local_var format_len, endianness, iOp;
  local_var result, iLen, iField, unpack_struct, data;
  if(isnull(pos))
    pos = 0;
  lens = _FCT_ANON_ARGS;
  pos_old = pos;
  format_len = strlen(format);
  if(format[0] == OP_LITTLE_ENDIAN || format[0] == OP_BIG_ENDIAN || format[0] == OP_NATIVE_ENDIAN || format[0] == OP_NETWORK_ENDIAN){
    endianness = format[0];
    iOp = 1;
  }
  else{
    endianness = OP_DEFAULT_ENDIAN;
    iOp = 0;
  }
  #Process format string
  iField = 0;
  iLen = 0;
  for(iOp = iOp; iOp < format_len; iOp++){
    if(format[iOp] == OP_UINT_8){
      unpack_struct = unpack_uint_8(blob: blob, pos: pos);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
    }
    else if(format[iOp] == OP_UINT_16){
      unpack_struct = unpack_uint_16(blob: blob, pos: pos, endianness: endianness);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
    }
    else if(format[iOp] == OP_UINT_32){
      unpack_struct = unpack_uint_32(blob: blob, pos: pos, endianness: endianness);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
    }
    else if(format[iOp] == OP_UINT_64){
      unpack_struct = unpack_uint_64(blob: blob, pos: pos, endianness: endianness);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
    }
    else if(format[iOp] == OP_PAD_BYTE){
      pos += unpack_pad_byte(blob: blob, pos: pos); #drop pad bytes...not interested in pad bytes. Kill all them padders, muahaaa!
    }
    else if(format[iOp] == OP_STRING){
      unpack_struct = unpack_string(blob: blob, pos: pos, len: lens[iLen]);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
      iLen++;
    }
    else if(format[iOp] == OP_STRING_NULL){
      unpack_struct = unpack_string_null(blob: blob, pos: pos);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
    }
    else if(format[iOp] == OP_STRING_HEX){
      unpack_struct = unpack_string_hex(blob: blob, pos: pos, len: lens[iLen]);
      data[iField] = unpack_struct['data'];
      pos += strlen(unpack_struct['blob']);
      iField++;
      iLen++;
    }
    else if(format[iOp] == OP_LITTLE_ENDIAN || format[iOp] == OP_BIG_ENDIAN || format[iOp] == OP_NATIVE_ENDIAN || format[iOp] == OP_NETWORK_ENDIAN){
      endianness = format[iOp];
      continue;
    }
  }
  result['blob'] = substr(blob, pos_old, pos - 1);
  result['data'] = data;
  return result;
}

#  @brief Aligns a blob to a boundary.
#  @rationale As many network protocols define some kind of alignment when considering
#    protocol packet fields and corresponding values, this method comes
#    as a very useful function. For example dce/rpc or ms-rpc RPC protocol for defining
#    data relies on NDR (Network Data Representation) which aligns all integer data types
#    to a 4 byte boundary.
#  @param blob A blob that needs to be aligned to a 'boundary' byte boundary
#    and a 'al' alignment.
#  @param boundary Integer that represents the bytes alignment boundary.
#  @param al String that represents the type of alignment (left or right).
#  @return Returns an aligned blob.
#  @example A raw string representing short integer 0x1234 right aligned to a 4 byte
#    boundary:
#    short int: 0x1234
#     blob: raw_string(0x12, 0x34);
#    blob_aligned: raw_string(0x00,0x00,0x12,0x34);
#default constants
global_var ALIGN_PAD_BYTE, ALIGN_LEFT, ALIGN_RIGHT, ALIGN_DEAFULT;
ALIGN_PAD_BYTE = raw_string(0x00);#Data alignment padding value.
ALIGN_LEFT = 'left';#alignment
ALIGN_RIGHT = 'right';
ALIGN_DEFAULT = ALIGN_RIGHT;
function align(blob, boundary, al){
  local_var padding_size, padding, i;
  if(isnull(al))
    al = ALIGN_DEFAULT;
  padding_size = boundary - (strlen(blob) % boundary);
  padding = '';
  for(i = 0; i < padding_size; i++)
    padding += ALIGN_PAD_BYTE;
  if(al == ALIGN_LEFT){
    return blob + padding;
  }
  else{
    return padding + blob;
  }
}

#  @brief Make a string indexed array(dictionary) from an regular array.
#  @param arr Regular number indexed array.
#  @param _FCT_ANON_ARGS Implicit list of string indexes
#  @return Array indexed by strings.
function populate(arr){
  local_var result;
  for(i = 0; i < max_index(arr); i++){
    result[_FCT_ANON_ARGS[i]] = arr[i];
  }
  return result;
}
#  @brief Display an array
function dump_arr(arr, title){
  display('---[ ' + title + ' ]---\n' );
  local_var k, e, key, elem, keyz, elemz;
  k = 0;
  foreach key (keys(arr)) keyz[k++] = key;
  for(e = 0; e < k; e++) display(keyz[e] + ':' + arr[keyz[e]] + '\n');
  display('--------------\n' );
}
###########################
######### INIT ############
###########################
#  @brief Initialize native endianness.
global_var NATIVE_BYTE_ORDER;
#Check endianness in run-time -- NASL developers, please give us get_byte_order() :D
NATIVE_BYTE_ORDER = OP_BIG_ENDIAN;#TODO: get_byte_order()

#############################
#### PACK/UNPACK methods ####
#############################

#  @brief Pack an unsigned char into a 1B blob.
#  @param data  Unsigned char to pack.
#  @return Returns a 1B blob representing a unsigned char object.
function pack_uint_8(data){
  return raw_string(data % 256);
}

#  @brief Unpack a 1B blob into an unsigned char object.
#  @param blob  Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = unsigned char object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_uint_8(blob, pos){
  local_var result;
  result['data'] = ord(blob[pos]);
  result['blob'] = substr(blob, pos, pos);
  return result;
}

#  @brief Pack an unsigned short into a 2B blob.
#  @param data  Unsigned short to pack.
#  @param endianness Byte order for packing.
#  @return Returns a 2B blob representing a unsigned short object.
function pack_uint_16(data, endianness){
  local_var uint_16, uint_16_lo, uint_16_hi;
  local_var result;
  uint_16 = data % (256*256); #modulo normalization - if data is bigger than short
  uint_16_lo = uint_16 % 256;
  uint_16_hi = uint_16 / 256;

  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }

  if(endianness == OP_LITTLE_ENDIAN){
    result = raw_string(uint_16_lo, uint_16_hi);
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    result = raw_string(uint_16_hi, uint_16_lo);
  }
  return result;
}

#  @brief Unpack a 2B blob into an unsigned short object.
#  @param blob  Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @param endianness Byte order for unpacking.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = unsigned short object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_uint_16(blob, pos, endianness){
  local_var result;
  if(isnull(pos))
    pos = 0;
  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }
  if(endianness == OP_LITTLE_ENDIAN){
    result['data'] = ord(blob[pos]) + ord(blob[pos+1])*256;
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    result['data'] = ord(blob[pos])*256 + ord(blob[pos+1]);
  }
  result['blob'] = substr(blob, pos, pos + 1);
  return result;
}

#  @brief Pack an unsigned long into a 4B blob.
#  @param data  Unsigned long to pack.
#  @param pos Position within 'data' from where the parsing starts.
#  @param endianness Byte order for packing.
#  @return Returns a 4B blob representing a unsigned long object.
function pack_uint_32(data, endianness){
  local_var uint_32, uint_32_lo, uint_32_hi, uint_32_lo_lo, uint_32_lo_hi, uint_32_hi_lo, uint_32_hi_hi;
  local_var result;
  uint_32 = data;
  uint_32_lo = uint_32 % (256*256);
  uint_32_hi = uint_32 / (256*256);
  uint_32_lo_lo = uint_32_lo % 256;
  uint_32_lo_hi = uint_32_lo / 256;
  uint_32_hi_lo = uint_32_hi % 256;
  uint_32_hi_hi = uint_32_hi / 256;

  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }

  if(endianness == OP_LITTLE_ENDIAN){
    result = raw_string(uint_32_lo_lo, uint_32_lo_hi, uint_32_hi_lo, uint_32_hi_hi);
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    result = raw_string(uint_32_hi_hi, uint_32_hi_lo, uint_32_lo_hi, uint_32_lo_lo);
  }
  return result;
}


#  @brief Unpack a 4B blob into an unsigned long object.
#  @param blob  Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @param endianness Byte order for unpacking.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = unsigned long object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_uint_32(blob, pos, endianness){
  local_var result;
  if(isnull(pos))
    pos = 0;
  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }
  if(endianness == OP_LITTLE_ENDIAN){
    result['data'] = ord(blob[pos]) + ord(blob[pos+1])*256+ ord(blob[pos+2])*256*256 + ord(blob[pos+3])*256*256*256;
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    result['data'] = ord(blob[pos+3]) + ord(blob[pos+2])*256+ ord(blob[pos+1])*256*256 + ord(blob[pos])*256*256*256;
  }
  result['blob'] = substr(blob, pos, pos + 3);
  return result;
}

#  @brief Pack an unsigned long long into a 8B blob.
#  @param data  Unsigned long long to pack.
#  @param endianness Byte order for packing.
#  @return Returns a 8B blob representing a unsigned long long object.
function pack_uint_64(data, endianness){
  local_var result;
  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }

  if(endianness == OP_LITTLE_ENDIAN){
    result = pack_uint_32(data: data, endianness: endianness) + raw_string(0x00, 0x00, 0x00, 0x00);
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    result = raw_string(0x00, 0x00, 0x00, 0x00) + pack_uint_32(data: data, endianness: endianness);
  }
  return result;
}


#  @brief Unpack a 8B blob into an unsigned long long object.
#  @param blob  Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @param endianness Byte order for unpacking.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = unsigned long long object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_uint_64(blob, pos, endianness){
  local_var unpack_struct, result;
  if(endianness == OP_NATIVE_ENDIAN){
    endianness = NATIVE_BYTE_ORDER; #retrieved from INIT
  }
  if(endianness == OP_LITTLE_ENDIAN){
    unpack_struct = unpack_uint_32(blob: blob, endianness: endianness);
    result['data'] = unpack_struct['data'];
  }
  else if(endianness == OP_BIG_ENDIAN || endianness == OP_NETWORK_ENDIAN){
    unpack_struct = unpack_uint_32(blob: blob, pos: pos + 4, endianness: endianness);
    result['data'] = unpack_struct['data'];
  }
  result['blob'] = substr(blob, pos, pos + 7);
  return result;
}

#  @brief Pack VAL_PAD_BYTE into a 1B blob.
#  @glob_param VAL_PAD_BYTE
#  @return Returns a 1B blob representing a VAL_PAD_BYTE.
function pack_pad_byte(){
  return VAL_PAD_BYTE;
}

#  @brief Unpack a 1B blob.
#  @param blob  Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @return Returns number of bytes unpacked = 1.
#  @note 'bin_unpack' method discards this data.
function unpack_pad_byte(blob, pos){
  return 1;
}

#  @brief Pack a string into a strlen(string)B blob.
#  @param data String to pack.
#  @return Returns strlen(data)B blob.
#  @note It's obvious that this is just a simple string copy.
function pack_string(data){
  return data;
}

#  @brief Unpack a blob of 'len' length into a string object.
#  @param blob Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @param len String length.
#    @default Blob length minus 'pos'.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = string object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_string(blob, pos, len){
  local_var result;
  if(isnull(len))
    len = strlen(blob) - pos;
  result['data'] = substr(blob, pos, pos + len - 1);
  result['blob'] = result['data'];
  return result;
}

#  @brief Pack a string into a strlen(string)B+1B NULL terminated blob.
#  @param data String to pack.
#  @return Returns strlen(data)B + 1B blob.
function pack_string_null(data){
  return data + raw_string(0x00);
}

#  @brief Unpack a null terminated blob.
#  @param blob Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = string object (without null terminator)
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_string_null(blob, pos){
  local_var result, i, blob_len;
  result['data'] = '';
  result['blob'] = '';
  blob_len = strlen(blob);
  for(i = pos; i < blob_len; i++){
    result['blob'] += blob[i];
    if(blob[i] == raw_string(0x00)) break;
    result['data'] += blob[i];
  }
  return result;
}

#  @brief Pack a hex ascii string into a raw string blob.
#  @param data Hex ascii string to pack.
#    @example "0x120x23" -> raw_string(0x12, 0x23)
#    @example "012345678" -> raw_string(0x12, 0x34, 0x56, 0x78)
#    @note Hex strings can contain whitespaces.
#    --- Wiresharks Copy->Bytes (Hex Stream) :D ...useful a?
#  @return Returns a blob.
function pack_string_hex(data){
  local_var hex, hex_len, result;
  local_var val;
  #Prep data
  hex = tolower(data);
  hex = ereg_replace(string: hex, pattern: "0x|[' ''\r''\n''\t']", replace: '');
  hex_len = strlen(hex);
  #Parse
  result = '';
  for(i = 0; i < hex_len; i = i+2){
    if(ord(hex[i]) >= ord('a') && ord(hex[i]) <= ord('f')){
      val = 16*(ord(hex[i]) - ord('a') + 10);
    }
    else if(ord(hex[i]) >= ord('0') && ord(hex[i]) <= ord('9')){
      val = 16*(ord(hex[i]) - ord('0'));
    }
    else continue;

    if(ord(hex[i+1]) >= ord('a') && ord(hex[i+1]) <= ord('f')){
      val += (ord(hex[i+1]) - ord('a') + 10);
    }
    else if(ord(hex[i+1]) >= ord('0') && ord(hex[i+1]) <= ord('9')){
      val += ord(hex[i+1]) - ord('0');;
    }
    else continue;
    result += raw_string(val);
  }
  return result;
}

#  @brief Unpack a blob of 'len' length into a hex string.
#  @param blob Blob to unpack.
#  @param pos Position within 'blob' from where the parsing starts.
#  @param len Blob length.
#    @default Blob length minus 'pos'.
#  @return Returns an 'unpack_struct':
#    unpack_struct['data'] = hex string object
#    unpack_struct['blob'] = a corresponding part of 'blob'
function unpack_string_hex(blob, pos, len){
  local_var hexstr;
  local_var result;
  if(isnull(len))
    len = strlen(blob) - pos;
  hexstr = '';
  for(i = 0; i < len; i++){
    hexstr += hex(ord(blob[pos + i])) + ',';
  }
  result['data'] = hexstr;
  result['blob'] = substr(blob, pos, pos + len - 1);
  return result;
}

