###############################################################################
# OpenVAS Include File
#
# SMB Functions
#
# Authors:
# bchandra <bchandra@secpod.com>
#
# Copyright:
# Copyright (C) 2008 SecPod, http://www.secpod.com
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
###############################################################################

###############################################################################
# GetVer(), bin_word(), bin_dword() are a derivative of smbcl_func.inc available
# in the VT repository.
# This script was written by Carsten Koch-Mauthe <c.koch-mauthe at dn-systems.de>
#
# This script is released under the GNU GPLv2
###############################################################################

# @brief Returns the version of a file (in the form of an array) saved within
#        the KB tree "SMB/lsc_file_version_cache/".
#
# @param file_name The file name to get the version for. Currently supported: edgehtml.dll or mshtml.dll
#
# @note You need to add gb_smb_lsc_fetch_file_ver.nasl as a dependency to your VT
#       if you want to use this function. Also take note of the current supported
#       file names in the file_name parameter.
#
# @return An array containing the full file path and the version of the file,
#         NULL in the case no file_name was passed to the function or the file name
#         isn't supported by this function / by gb_smb_lsc_fetch_file_ver.nasl and
#         FALSE if no file version is available. The array indices are the following:
#
#         ret_array["path"]    = the full path of the file (e.g. C:\WINDOWS\system32\edgehtml.dll)
#         ret_array["version"] = the version of the file (e.g. 11.0.16299.785)
#
function smb_get_fileversion_from_cache( file_name ) {

  local_var file_name;
  local_var infos, ret_array;

  if( ! file_name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file_name#-#smb_get_fileversion_from_cache" );
    return;
  }

  if( file_name != "edgehtml.dll" && file_name != "mshtml.dll" ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#smb_get_fileversion_from_cache: Unsupported file name '" + file_name + "' passed. Currently supported: edgehtml.dll or mshtml.dll." );
    return;
  }

  infos = get_kb_item( "SMB/lsc_file_version_cache/" + file_name + "/infos" );
  if( ! infos )
    return FALSE;

  info_list = split( infos, sep:"#--#", keep:FALSE );
  if( max_index( info_list ) != 2 ) # nb: Something wrong...
    return FALSE;

  ret_arr["path"]    = info_list[0];
  ret_arr["version"] = info_list[1];
  return ret_arr;
}

# Get File version or product version reading through a file. An
# offset can be sent indicating the start location. By default
# file version is read, if product version is required, we can
# indicate verstr="prod"

function GetVersion( socket, uid, tid, fid, offset, verstr, debug ) {

  local_var socket, uid, tid, fid, offset, verstr, debug;
  local_var fsize, data, i, offset, tmp, version, ver, vers, len, ver_len, retVal;

  if( isnull( socket ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#GetVersion" );
  if( isnull( uid ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#uid#-#GetVersion" );
  if( isnull( tid ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#tid#-#GetVersion" );
  if( isnull( fid ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fid#-#GetVersion" );

  pattern = "F?i?l?e?V?e?r?s?i?o?n";
  if( verstr == "prod" ) {
    pattern = "P?r?o?d?u?c?t?V?e?r?s?i?o?n";
  } else if( verstr ) {
    pattern = verstr;
  }

  patlen = strlen( pattern );

  fsize = smb_get_file_size( socket:socket, uid:uid, tid:tid, fid:fid );

  if( isnull( offset ) ) {
    if( fsize < 180224 ) {
      offset = 0;
    } else {
      offset = fsize - 180224;
    }
  }

  if( offset < 0 ) {
    offset = fsize + offset;
  }

  start = offset;

  if( start < 0 || start > fsize ) {
    start = fsize/2;
  }

  offset = start;
  chunk = 16384;

  for( i = 0; offset < fsize; i++ ) {
    tmp = ReadAndX( socket:socket, uid:uid, tid:tid, fid:fid, count:chunk, off:offset );
    if( tmp ) {
      if( pattern == "build" ) {
        tmp = str_replace( find:raw_string(0), replace:"", string:tmp );
      } else {
        tmp = str_replace( find:raw_string(0), replace:"?", string:tmp );
      }

      data += tmp;
      version = strstr( data, pattern );
      if( version ) {
        len = strlen( version );
        for( i = patlen; i < len; i++ ) {
          if( ! isnull( version[i] ) ) {
            if( ( ord( version[i] ) < ord( "0" ) || ord( version[i] ) > ord( "9" ) ) &&
                ( version[i] != "." && version[i] != "," && version[i] != " " &&
                version[i] != "?" ) ) {
              ver_len = strlen( ver );
              if( ver_len > 0 && ver[ver_len - 1] == "." ) {
                vers = split( ver, sep:".", keep:FALSE );
                foreach item( vers ) {
                  retVal += string( "." + item );
                }
                retVal -= string( "." );
                return retVal;
              }
              return ver;
            } else if( version[i] == "," || version[i] == "." ) {
              ver += ".";
            } else if( ver && version[i] == "?" && version[i+1] == "?" ) {
              return ver;
            } else if( version[i] == " "||version[i] == "?" ) {
            } else {
              ver += version[i];
            }
          }
        }
      }
      offset += chunk;
    }
  }
  return NULL;
}

# @brief Enumerates existing registry values for a given key via SMB. It will gather all required
#        info about the SMB registry from the knowledge base.
#
# @param key  The registry key to enumerate for values, e.g. "SOFTWARE\Microsoft\Windows\CurrentVersion\SharedDlls\"
# @param type The type of the registry tree to query. Valid values are "HKLM, HKU and HKCU". If no type was specified the function
#             defaults to "HKLM".
#
# @return A list containing the existing/enumerated values of the given registry key, an empty list otherwise.
#
function registry_enum_values( key, type ) {

  local_var key, type, list, soc, r, prot, uid, tid, pipe, handle;
  local_var kb_proxy_key, kb_proxy, item, founditem, name, login, pass, domain, port;

  # This function is mostly called in foreach loops. Make sure to always return
  # an empty list here instead of the previously behavior to return FALSE if any of
  # the calls between the last return failed. This old  behavior had caused a
  # nasl_array_iterator: # unhandled type 57 (0x39) if FALSE was returned when failed.
  list = make_list();

  if( ! key ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#key#-#registry_enum_values" );
    return list;
  }

  if( ! type )
    type = "HKLM";
  else
    type = toupper( type );

  # Query KB ("proxy") first.
  kb_proxy_key = "SMB//registry_enum_values//Registry//" + type + "//" + tolower( key );
  kb_proxy = get_kb_list( kb_proxy_key );
  if( ! isnull( kb_proxy ) && kb_proxy && typeof( kb_proxy ) == "array" )
    return kb_proxy;

  if( kb_smb_is_samba() ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#Windows SMB NVT was started against a Samba Server" );
    return list;
  }

  name = kb_smb_name();
  if( ! name ) return list;

  port = kb_smb_transport();
  if( ! port ) return list;
  if( ! get_port_state( port ) ) return list;
  soc = open_sock_tcp( port );
  if( ! soc ) return list;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return list;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close( soc );
    return list;
  }

  login  = kb_smb_login();
  pass   = kb_smb_password();
  domain = kb_smb_domain();
  if( ! login ) login = "";
  if( ! pass )  pass = "";

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return list;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return list;
  }

  r = smb_tconx( soc:soc, name:name, uid:uid, share:"IPC$" );
  if( ! r ) {
    close( soc );
    return list;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return list;
  }

  r = smbntcreatex( soc:soc, uid:uid, tid:tid, name:"\winreg" );
  if( ! r ) {
    close( soc );
    return list;
  }

  pipe = smbntcreatex_extract_pipe( reply:r );
  if( ! pipe ) {
    close( soc );
    return list;
  }

  r = pipe_accessible_registry( soc:soc, uid:uid, tid:tid, pipe:pipe );
  if( ! r ) {
    close( soc );
    return list;
  }

  if( type == "HKLM" ) {
    handle = registry_open_hklm( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else if( type == "HKU" ) {
    handle = registry_open_hku( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else if( type == "HKCU" ) {
    handle = registry_open_hkcu( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else {
    close( soc );
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#registry_enum_values: Unsupported '" + type + "' passed to type function parameter." );
    return list;
  }

  if( ! handle ) {
    close( soc );
    return list;
  }

  r = registry_get_key( soc:soc, uid:uid, tid:tid, pipe:pipe, key:key, reply:handle );
  if( ! r ) {

    if( ! isnull( r ) )
      registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );

    registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:handle );

    # TBD: If the key doesn't exist this will return here. Save this as an empty list in the KB cache as well?
    close( soc );
    return list;
  }

  # TODO: Evaluate if the handle here needs to be closed via registry_close as well...
  list = registry_enum_value( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );

  registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );
  registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:handle );

  close( soc );

  # Always return a list for the reasons explained above
  # and place the values into the KB to be cached for later calls.
  # Using string() as set_kb_item won't allow to set a value of 0 into the KB.
  if( typeof( list ) != "array" ) {
    list = make_list();
    set_kb_item( name:kb_proxy_key, value:string("") );
  } else {
    founditem = FALSE;
    foreach item( list ) {
      founditem = TRUE;
      set_kb_item( name:kb_proxy_key, value:string( item ) );
    }
    if( ! founditem )
      set_kb_item( name:kb_proxy_key, value:string("") );
  }
  return list;
}

# @brief Enumerates registry keys via SMB. It will gather all required info about the SMB registry
#        from the knowledge base.
#
# @param key  The registry key to enumerate, e.g. "SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\"
# @param type The type of the registry tree to query. Valid values are "HKLM, HKU and HKCU". If no type was specified the function
#             defaults to "HKLM".
#
# @return A list containing the existing/enumerated registry keys, an empty list otherwise.
#
function registry_enum_keys( key, type ) {

  local_var key, type, list, soc, r, prot, uid, tid, pipe, handle;
  local_var kb_proxy_key, kb_proxy, item, founditem, name, login, pass, domain, port;

  # This function is mostly called in foreach loops. Make sure to always return
  # an empty list here instead of the previously behavior to return FALSE if any of
  # the calls between the last return failed. This old  behavior had caused a
  # nasl_array_iterator: # unhandled type 57 (0x39) if FALSE was returned when failed.
  list = make_list();

  if( ! key ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#key#-#registry_enum_keys" );
    return list;
  }

  if( ! type )
    type = "HKLM";
  else
    type = toupper( type );

  # Query KB ("proxy") first.
  kb_proxy_key = "SMB//registry_enum_keys//Registry//" + type + "//" + tolower( key );
  kb_proxy = get_kb_list( kb_proxy_key );
  if( ! isnull( kb_proxy ) && kb_proxy && typeof( kb_proxy ) == "array" )
    return kb_proxy;

  if( kb_smb_is_samba() ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#Windows SMB NVT was started against a Samba Server" );
    return list;
  }

  name = kb_smb_name();
  if( ! name ) return list;

  port = kb_smb_transport();
  if( ! port ) return list;
  if( ! get_port_state( port ) ) return list;
  soc = open_sock_tcp( port );
  if( ! soc ) return list;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return list;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close( soc );
    return list;
  }

  login  = kb_smb_login();
  pass   = kb_smb_password();
  domain = kb_smb_domain();
  if( ! login ) login = "";
  if( ! pass )  pass = "";

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return list;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return list;
  }

  r = smb_tconx( soc:soc, name:name, uid:uid, share:"IPC$" );
  if( ! r ) {
    close( soc );
    return list;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return list;
  }

  r = smbntcreatex( soc:soc, uid:uid, tid:tid, name:"\winreg" );
  if( ! r ) {
    close( soc );
    return list;
  }

  pipe = smbntcreatex_extract_pipe( reply:r );
  if( ! pipe ) {
    close( soc );
    return list;
  }

  r = pipe_accessible_registry( soc:soc, uid:uid, tid:tid, pipe:pipe );
  if( ! r ) {
    close( soc );
    return list;
  }

  if( type == "HKLM" ) {
    handle = registry_open_hklm( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else if( type == "HKU" ) {
    handle = registry_open_hku( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else if( type == "HKCU" ) {
    handle = registry_open_hkcu( soc:soc, uid:uid, tid:tid, pipe:pipe );
  } else {
    close( soc );
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#registry_enum_keys: Unsupported '" + type + "' passed to type function parameter." );
    return list;
  }

  if( ! handle ) {
    close( soc );
    return list;
  }

  r = registry_get_key( soc:soc, uid:uid, tid:tid, pipe:pipe, key:key, reply:handle );
  if( ! r ) {

    if( ! isnull( r ) )
      registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );

    registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:handle );

    # TBD: If the key doesn't exist this will return here. Save this as an empty list in the KB cache as well?
    close( soc );
    return list;
  }

  # TODO: Evaluate if the handle here needs to be closed via registry_close as well...
  list = registry_enum_key( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );

  registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:r );
  registry_close( soc:soc, uid:uid, tid:tid, pipe:pipe, reply:handle );

  close( soc );

  # Always return a list for the reasons explained above
  # and place the values into the KB to be cached for later calls.
  # Using string() as set_kb_item won't allow to set a value of 0 into the KB.
  if( typeof( list ) != "array" ) {
    list = make_list();
    set_kb_item( name:kb_proxy_key, value:string("") );
  } else {
    founditem = FALSE;
    foreach item( list ) {
      founditem = TRUE;
      set_kb_item( name:kb_proxy_key, value:string( item ) );
    }
    if( ! founditem )
      set_kb_item( name:kb_proxy_key, value:string("") );
  }
  return list;
}


################################################################################
# Open the given file
# param:
#       share: IPC Share, ex: $C
#       file: File path
################################################################################

function open_file( share, file ) {

  local_var share, file, soc, r, prot, uid, tid;
  local_var name, login, pass, domain, port;

  if( isnull( share ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#share#-#open_file" );
  if( isnull( file ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#open_file" );

  name    =  kb_smb_name();
  login   =  kb_smb_login();
  pass    =  kb_smb_password();
  domain  =  kb_smb_domain();
  port    =  kb_smb_transport();

  if( ! port ) port = 445;

  soc = open_sock_tcp( port );
  if( ! soc ) return FALSE;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close( soc );
    return FALSE;
  }

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return FALSE;
  }

  r = smb_tconx( soc:soc, name:name, uid:uid, share:share );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return FALSE;
  }

  r = OpenAndX( socket:soc, uid:uid, tid:tid, file:file );
  if( r ) smb_close_request( soc:soc, uid:uid, tid:tid, fid:r );

  close( soc );
  return r;
}

##########################################################################
# Get the file size for a given file
# param:
#       share: Windows IPC Share
#       file: Path to a given file
##########################################################################

function get_file_size( share, file ) {

  local_var share, file, soc, r, prot, uid, tid, fid, fileSize;
  local_var name, login, pass, domain, port;

  if( isnull( share ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#share#-#get_file_size" );
  if( isnull( file ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#get_file_size" );

  name    =  kb_smb_name();
  login   =  kb_smb_login();
  pass    =  kb_smb_password();
  domain  =  kb_smb_domain();
  port    =  kb_smb_transport();

  if( ! port ) port = 445;

  soc = open_sock_tcp( port );
  if( ! soc ) return FALSE;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close( soc );
    return FALSE;
  }

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return FALSE;
  }

  r = smb_tconx( soc:soc, name:name, uid:uid, share:share );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return FALSE;
  }

  fid = OpenAndX( socket:soc, uid:uid, tid:tid, file:file );
  if( ! fid ) {
    close( soc );
    return FALSE;
  }

  fileSize = smb_get_file_size( socket:soc, uid:uid, tid:tid, fid:fid );
  smb_close_request( soc:soc, uid:uid, tid:tid, fid:fid );
  close( soc );

  if( fileSize ) {
    return fileSize;
  }
}

# @brief Reads the content of a given file via SMB.
#
# @param share    The Windows IPC Share to connect to (e.g. C$).
# @param file     The file path to read the content from (e.g. \path\to\file.txt).
# @param fullpath The full path to a file to read the content from (e.g. C:\path\to\file.txt).
#                 This parameter can be used as an alternative to the "share" and "file" parameter.
# @param offset   Position from which to start reading the file. Mandatory.
# @param count    Number of bytes to read from offset. Mandatory.
#
# @note You need to either pass the "share" AND "file" parameter OR only the full path (e.g. C:\path\to\file.tx)
#       via the "fullpath" parameter.
#
# @return The content of the read file as a string, FALSE if no connection to the remote SMB server could be
#         opened or NULL if not all required parameters where passed.
#
function smb_read_file( share, file, fullpath, offset, count ) {

  local_var share, file, fullpath, offset, count;
  local_var name, login, pass, domain, port;
  local_var soc, r, prot, uid, tid, fid, content;

  if( ! share && ! file && ! fullpath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#share, file and fullpath#-#smb_read_file" );
    return NULL;
  }

  if( fullpath && ( share || file ) ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#smb_read_file: fullpath and share/file passed but only one of both can be used" );
    return NULL;
  }

  if( share && ! file ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#share passed but not file#-#smb_read_file" );
    return NULL;
  }

  if( file && ! share ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file passed but not share#-#smb_read_file" );
    return;
  }

  if( isnull( offset ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#offset#-#read_file" );
    return;
  }

  if( ! count ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#count#-#read_file" );
    return;
  }

  if( fullpath ) {
    share = ereg_replace( pattern:"([a-zA-Z]):.*", replace:"\1$", string:fullpath );
    share = toupper( share ); # nb: Just to be sure to pass the IPC share as uppercase if a small drive letter was passed.
    file  = ereg_replace( pattern:"[a-zA-Z]:(.*)", replace:"\1", string:fullpath );
  }

  name   =  kb_smb_name();
  login  =  kb_smb_login();
  pass   =  kb_smb_password();
  domain =  kb_smb_domain();
  port   =  kb_smb_transport();

  if( ! port )
    port = 445;

  soc = open_sock_tcp( port );
  if( ! soc )
    return FALSE;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close( soc );
    return FALSE;
  }

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return FALSE;
  }

  r = smb_tconx( soc:soc, name:name, uid:uid, share:share );
  if( ! r ) {
    close( soc );
    return FALSE;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return FALSE;
  }

  fid = OpenAndX( socket:soc, uid:uid, tid:tid, file:file );
  if( ! fid ) {
    close( soc );
    return FALSE;
  }

  content = ReadAndX( socket:soc, uid:uid, tid:tid, fid:fid, count:count, off:offset );
  smb_close_request( soc:soc, uid:uid, tid:tid, fid:fid );
  close( soc );
  return content;
}

function GetVer( file, share, prodvers ) {

  local_var pe_offset, sections_cnt, sections_offset, sections_data, i, rsrc_start, rsrc_virtstart;
  local_var dir_data, dir_offset, dir_entries, sub_dir, dir_tmp, vs_data, vs_length, reply, recurs, max_recurs, vs_offset;
  local_var itr, it, file, share, prodvers;

  if( isnull( share ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#share#-#GetVer" );
  if( isnull( file ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#GetVer" );

  if( isnull( prodvers ) ) prodvers = 0;

  pe_offset = bin_word( data:smb_read_file( share:share, file:file, offset:60, count:2 ) );

  if( smb_read_file( share:share, file:file, offset:pe_offset, count:2 ) != "PE" ) {
    return NULL;
  }

  sections_cnt = bin_word( data:smb_read_file( share:share, file:file, offset:pe_offset + 6, count:2 ) );
  section_offset = pe_offset + bin_word( data:smb_read_file( share:share, file:file, offset:pe_offset + 20, count:2 ) ) + 24;
  rsrc_start = NULL;
  max_recurs = 20;

  for( itr = 0; itr < sections_cnt; itr++ ) {
    sections_data = smb_read_file( share:share, file:file, offset:section_offset + itr * 40, count:40 );
    if( sections_data && substr( sections_data, 0, 4 ) == ".rsrc" ) {
      rsrc_start = bin_dword( data:substr( sections_data, 20 ) );
      rsrc_virtstart = bin_dword( data:substr( sections_data, 12 ) );
      break;
    }
    if( itr > max_recurs ) {
      break;
    }
  }
  if( isnull( rsrc_start ) ) {
    return NULL;
  }

  dir_data = smb_read_file( share:share, file:file, offset:rsrc_start, count:16 );
  dir_entries = bin_word( data:substr( dir_data, 12 ) ) + bin_word( data:substr( dir_data, 14 ) );
  dir_offset = NULL;
  sub_dir = 0;
  max_recurs = 100;
  recurs = 0;

  for( it = 0; it < dir_entries; it++ ) {
    dir_data = smb_read_file( share:share, file:file, offset:rsrc_start + 16 + it * 8, count:8 );
    if( bin_dword( data:substr( dir_data, 0 ) ) == 0x10 ) {

      repeat {
        dir_tmp = bin_dword( data:substr( dir_data, 4 ) );
        if( ( dir_tmp & 0x80000000 ) == 0x80000000 ) {
          dir_offset = dir_tmp - 0x80000000;
          sub_dir = 1;
        } else {
          dir_offset = dir_tmp;
          sub_dir = 0;
        }
        dir_data = smb_read_file( share:share, file:file, offset:rsrc_start + dir_offset + 16, count:8 );
        if( strlen( dir_data ) != 8 ) {
          return NULL;
        }
        if( ++recurs > max_recurs ) {
          return NULL;
        }
      }
      until sub_dir == 0;
      break;
    }
    if( it > max_recurs ) {
      break;
    }
  }

  if( isnull( dir_offset ) ) {
    return NULL;
  }

  dir_data = smb_read_file( share:share, file:file, offset:rsrc_start + dir_offset, count:4 );
  dir_offset = bin_dword( data:substr( dir_data, 0 ) );
  rsrc_start = rsrc_start + ( dir_offset - rsrc_virtstart );
  vs_data = smb_read_file( share:share, file:file, offset:rsrc_start, count:2 );
  if( ! vs_data ) {
    return NULL;
  }

  vs_length = bin_word( data:substr( vs_data, 0 ) );
  vs_data = smb_read_file( share:share, file:file, offset:rsrc_start, count:vs_length );

  if( bin_dword( data:substr( vs_data, 40 ) ) != 0xfeef04bd ) {
    return NULL;
  }

  if( prodvers == 0 ) {
    vs_offset = 48;
  } else {
    vs_offset = 56;
  }

  reply = NULL;
  reply = string( ( bin_dword( data:substr( vs_data, vs_offset ) ) >>> 16) + "." );
  reply = reply + string( ( bin_dword( data:substr( vs_data, vs_offset ) ) & 0xffff) + "." );
  reply = reply + string( ( bin_dword( data:substr( vs_data, vs_offset + 4 ) ) >>> 16) + "." );
  reply = reply + string( ( bin_dword( data:substr( vs_data, vs_offset + 4 ) ) & 0xffff ) );
  return reply;
}

function bin_word( data ) {

  local_var data;

  if( isnull( data ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#bin_word" );
    return 0;
  }

  #nb: Make sure type is not int
  if( typeof( data ) == "int" ) {
    return 0;
  }

  if( strlen( data ) > 1 ) {
    return( ord( data[0] ) + ( ord( data[1] ) << 8 ) );
  }
  return 0;
}

function bin_dword( data ) {

  local_var data;

  if( isnull( data ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#bin_dword" );
    return 0;
  }

  #nb: Make sure type is not int
  if( typeof( data ) == "int" ) {
    return 0;
  }

  if( strlen( data ) > 3 ) {
    return( ord( data[0] ) + ( ord( data[1] ) << 8 ) + ( ord( data[2] ) << 16 ) + ( ord( data[3] ) << 24 ) );
  }
  return 0;
}

function GetVersionFromFile( file, verstr, offset ) {

  local_var file, verstr, offset;
  local_var name, login, pass, domain, port;
  local_var mshare, soc, r, prot, uid, tid, ver;

  if( ! file ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#GetVersionFromFile" );
    return NULL;
  }

  name   =  kb_smb_name();
  login  =  kb_smb_login();
  pass   =  kb_smb_password();
  domain =  kb_smb_domain();
  port   =  kb_smb_transport();

  soc = open_sock_tcp( port );
  if( ! soc )
    return NULL;

  r = smb_session_request( soc:soc, remote:name );
  if( ! r ) {
    close( soc );
    return NULL;
  }

  prot = smb_neg_prot( soc:soc );
  if( ! prot ) {
    close(soc);
    return NULL;
  }

  r = smb_session_setup( soc:soc, login:login, password:pass, domain:domain, prot:prot );
  if( ! r ) {
    close( soc );
    return NULL;
  }

  uid = session_extract_uid( reply:r );
  if( ! uid ) {
    close( soc );
    return NULL;
  }

  mshare = ereg_replace( pattern:"([a-zA-Z]):.*", replace:"\1$", string:file );
  mshare = toupper( mshare ); # nb: Just to be sure to pass the IPC share as uppercase if a small drive letter was passed.

  r = smb_tconx( soc:soc, name:name, uid:uid, share:mshare );
  if( ! r ) {
    close( soc );
    return NULL;
  }

  tid = tconx_extract_tid( reply:r );
  if( ! tid ) {
    close( soc );
    return NULL;
  }

  file = ereg_replace( pattern:"[a-zA-Z]:(.*)", replace:"\1", string:file );

  fid = OpenAndX( socket:soc, uid:uid, tid:tid, file:file );
  if( ! fid ) {
    close( soc );
    return NULL;
  }

  if( isnull( verstr ) ) {
    ver = GetVersion( socket:soc, uid:uid, tid:tid, fid:fid, offset:offset );
  } else {
    ver = GetVersion( socket:soc, uid:uid, tid:tid, fid:fid, verstr:verstr, offset:offset );
  }

  close( soc );
  return ver;
}
