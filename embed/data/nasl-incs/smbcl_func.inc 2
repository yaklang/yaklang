###############################################################################
# OpenVAS Include File
# $Id: smbcl_func.inc 13332 2019-01-28 15:16:13Z cfischer $
# Description: functions for smb client tool
#
# Authors:
# Carsten Koch-Mauthe <c.koch-mauthe at dn-systems.de>
#
# Copyright:
# Copyright (C) 2008 Greenbone Networks GmbH
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
# $Revision: 13332 $
#
###############################################################################

function smbversion() {

  local_var arg, r, domain, os, server;
  local_var username, password;

  if(!get_kb_item("Tools/Present/smbclient")) {
    log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> smbclient not available for the scanner.");
    return NULL;
  }

  username = kb_smb_login();
  password = kb_smb_password();

  arg[0] = "smbclient";
  arg[1] = "-d0";
  arg[2] = string( "-U" + username + "%" + password );
  arg[3] = "-cq";
  arg[4] = "//" + get_host_ip() + "/IPC$";
  arg[5] = is_domain();
  r = toupper(pread(cmd:"smbclient", argv:arg));
  if(!ereg(pattern:"NT_STATUS", string:r)) {
    domain = ereg_replace(pattern:"(.*DOMAIN=\[)*(\].*)*", string:r, replace:"");
    os = ereg_replace(pattern:"(.*OS=\[)*(\].*)*", string:r, replace:"");
    server = ereg_replace(pattern:"(.*SERVER=\[)*(\].*)*", string:r, replace:"");
    replace_kb_item(name:string("SMB/OS"), value:os);
    replace_kb_item(name:string("SMB/DOMAIN"), value:domain);
    replace_kb_item(name:string("SMB/SERVER"), value:server);
    return TRUE;
  } else {
    if(r)
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> " + chomp(r));
    else
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> No data received from smbclient");
    return FALSE;
  }
}

function is_domain() {

  local_var domain;

  domain = kb_smb_domain();

  if(domain) {
    return("-W" + domain);
  } else {
    return NULL;
  }
}

function smbgetfile(share, filename, tmp_filename) {

  local_var share, filename, tmp_filename;
  local_var username, password, arg, r;

  if(!get_kb_item("Tools/Present/smbclient")) {
    log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> smbclient not available for the scanner.");
    return NULL;
  }

  username = kb_smb_login();
  password = kb_smb_password();

  arg[0] = "smbclient";
  arg[1] = "-d1";
  arg[2] = string( "-U" + username + "%" + password );
  arg[3] = "//" + get_host_ip() + "/" + share;
  arg[4] = "-cget \"+raw_string(0x22)+filename+"\"+raw_string(0x22)+" "+raw_string(0x22)+tmp_filename+raw_string(0x22);
  arg[5] = is_domain();
  r = pread(cmd:"smbclient", argv:arg);
  if(egrep(pattern:"getting file", string:r)) {
    return TRUE;
  } else {
    if(r)
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> " + chomp(r));
    else
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> No data received from smbclient");
    return FALSE;
  }
}

# Get Directories from SMB Source. typ 0 = all entries. typ 1 = only file entries. typ 2 = only directory entries.
function smbgetdir(share, dir, typ) {

  local_var share, dir, typ;
  local_var username, password, arg, ret, r, i, dir_att;

  if(!get_kb_item("Tools/Present/smbclient")) {
    log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> smbclient not available for the scanner.");
    return NULL;
  }

  username = kb_smb_login();
  password = kb_smb_password();

  arg[0] = "smbclient";
  arg[1] = "-d1";
  arg[2] = string( "-U" + username + "%" + password );
  arg[3] = "//" + get_host_ip() + "/" + share;
  arg[4] = "-cdir \"+raw_string(0x22)+dir+"\"+raw_string(0x22);
  arg[5] = is_domain();
  if( (typ < 0) || (typ > 2) || isnull(typ) ) {
    typ = 0;
  }
  ret = NULL;
  r = pread(cmd:"smbclient", argv:arg );
  if( !egrep(pattern: "NT_STATUS", string:r) ) {
    r = split(r, keep:FALSE);
    for( i = 0; i < (max_index(r)-2); i++ ) {
      if( (substr(r[i], 0, 4) == "  .. ") || (substr(r[i], 0, 3) == "  . ") || (!ereg(pattern:" [0-9]{4}$", string:r[i])) ) {
        continue;
      }
      r[i] = ereg_replace(pattern:"[ ]*[0-9]*.{26}$", string:r[i], replace:"");
      dir_att = ereg(pattern: "D[A-Z]*$", string:r[i]);
      r[i] = ereg_replace(pattern:" [A-Za-z]*$", string:r[i], replace:"");
      r[i] = ereg_replace(pattern:" *$", string:r[i], replace:"");
      r[i] = ereg_replace(pattern:"^ *", string:r[i], replace:"");
      if( ((typ == 0) || (typ == 1)) && (dir_att == 0) ) {
        ret[max_index(ret)] = r[i];
        continue;
      }
      if( ((typ == 0) || (typ == 2)) && (dir_att == 1) ) {
        ret[max_index(ret)] = r[i];
      }
    }
    return(ret);
  } else {
    if(r)
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> " + chomp(r));
    else
      log_message(port:0, proto:"SMBClient", data:"Error getting SMB-Data -> No data received from smbclient");
    return(ret);
  }
}

function GetPEFileVersion(tmp_filename, orig_filename) {

  local_var tmp_filename, orig_filename;
  local_var r, fp;

  r = NULL;
  fp = file_open(mode:"r", name:tmp_filename);
  if( ! isnull(fp) ) {
    r = PEVersion(fp:fp);
    file_close(fp);
  }
  if( isnull(r) ) {
    return NULL;
  }
  replace_kb_item(name:"SMB/FILEVERSION/"+orig_filename, value:r );
  return r;
}

# PEVersion fp = file to check, prodvers = 0/NULL give fileversion, prodvers <> 0/NULL give Productversion
function PEVersion(fp, prodvers) {

  local_var fp, prodvers;
  local_var pe_offset, sections_cnt, sections_offset, rsrc_start, max_recurs, i, sections_data, rsrc_virtstart;
  local_var dir_data, dir_entries, dir_offset, sub_dir, recurs, dir_tmp, vs_data, vs_length, vs_offset, r;

  if( isnull(prodvers) ) {
    prodvers = 0;
  }

  pe_offset = bin_word(data: fileread(fp:fp, offset:60, length:2));

  if( fileread(fp:fp, offset:pe_offset, length:2) != "PE" ) {
    return NULL;
  }

  sections_cnt = bin_word(data: fileread(fp:fp, offset:pe_offset + 6, length:2));
  section_offset = pe_offset + bin_word(data:fileread(fp:fp, offset:pe_offset + 20, length:2)) + 24;
  rsrc_start = NULL;
  max_recurs = 20;
  for( i = 0; i < sections_cnt; i++ ) {
    sections_data = fileread(fp:fp, offset:section_offset + i * 40, length:40);
    if( substr(sections_data, 0, 4) == ".rsrc" ) {
      rsrc_start = bin_dword(data:substr(sections_data, 20));
      rsrc_virtstart = bin_dword(data:substr(sections_data, 12));
      break;
    }
    if( i > max_recurs ) {
      break;
    }
  }
  if( isnull(rsrc_start) ) {
    return NULL;
  }

  dir_data = fileread(fp:fp, offset:rsrc_start, length:16);
  dir_entries = bin_word(data:substr(dir_data, 12)) + bin_word(data:substr(dir_data, 14));
  dir_offset = NULL;
  sub_dir = 0;
  max_recurs = 100;
  recurs = 0;
  for( i = 0; i < dir_entries; i++ ) {
    dir_data = fileread(fp:fp, offset:rsrc_start + 16 + i * 8, length:8);
    if( bin_dword(data:substr(dir_data, 0)) == 0x10 ) {
      repeat {
        dir_tmp = bin_dword(data:substr(dir_data, 4));
        if( (dir_tmp & 0x80000000) == 0x80000000 ) {
          dir_offset = dir_tmp - 0x80000000;
          sub_dir = 1;
        } else {
          dir_offset = dir_tmp;
          sub_dir = 0;
        }
        dir_data = fileread(fp:fp, offset:rsrc_start + dir_offset + 16, length:8);
        if( strlen(dir_data) != 8 ) {
          return NULL;
        }
        if( ++recurs > max_recurs ) {
          return NULL;
        }
      }
      until sub_dir == 0;
      break;
    }
    if( i > max_recurs ) {
      break;
    }
  }
  if( isnull(dir_offset) ) {
    return NULL;
  }

  dir_data = fileread(fp:fp, offset:rsrc_start + dir_offset, length:4);
  dir_offset = bin_dword(data:substr(dir_data, 0));
  rsrc_start = rsrc_start + (dir_offset - rsrc_virtstart);
  vs_data = fileread(fp:fp, offset:rsrc_start, length:2);
  vs_length = bin_word(data:substr(vs_data, 0));
  vs_data = fileread(fp:fp, offset:rsrc_start, length:vs_length);
  if( bin_dword(data:substr(vs_data, 40)) != 0xfeef04bd ) {
    return NULL;
  }
  if( prodvers == 0 ) {
    vs_offset = 48;
  } else {
    vs_offset = 56;
  }
  r = NULL;
  r = string((bin_dword(data:substr(vs_data, vs_offset)) >>> 16) + ".");
  r = r + string((bin_dword(data:substr(vs_data, vs_offset)) & 0xffff) + ".");
  r = r + string((bin_dword(data:substr(vs_data, vs_offset + 4)) >>> 16) + ".");
  r = r + string((bin_dword(data:substr(vs_data, vs_offset + 4)) & 0xffff) + ".");
  return r;
}

function fileread(fp, offset, length) {

  local_var fp, offset, length;
  local_var data;

  file_seek(fp:fp, offset:offset);
  data = file_read(fp:fp, length:length);
  if( isnull(data) ){
    return(NULL);
  } else {
   return(data);
  }
}

function bin_word(data) {
  local_var data;
  return( ord(data[0]) + (ord(data[1]) << 8) );
}

function bin_dword(data) {
  local_var data;
  return( ord(data[0]) +
         (ord(data[1]) << 8) +
         (ord(data[2]) << 16) +
         (ord(data[3]) << 24) );
}

function get_windir() {

  local_var os;

  os = get_kb_item("SMB/OS");
  if( (os == "WINDOWS 5.0") || (os == "WINDOWS NT 4.0") ) {
    return("WINNT\");
  } else {
    if( ereg(pattern:"WINDOWS", string:os) ) {
      return("WINDOWS\");
    } else {
      return(NULL);
    }
  }
}