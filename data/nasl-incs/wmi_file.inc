###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id: wmi_file.inc 11781 2018-10-08 12:34:17Z cfischer $
#
# WMI Client Functions for Directories and Files
#
# Authors:
# Chandan S <schandan@secpod.com>
#
# Copyright:
# Copyright (c) 2009 Greenbone Networks GmbH, http://www.greenbone.net
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
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#
# $Revision: 11781 $
#
###############################################################################

##############################################################################
# CLASS: win32_Directory Functions
# https://docs.microsoft.com/en-us/windows/desktop/cimwin32prov/win32-directory
##############################################################################

# @brief Returns a list of all sub directories
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to query, for the folder c:\windows\system32\wbem, the path is \\windows\\system32\\.
#                      For the folder c:\scripts, the path is \\. (Both with or as the trailing double backslashes)
# @param includeHeader if set to TRUE the header "Name" will be returned in the list
#
# @return A list of all sub directories within the passed path, FALSE if the path doesn't exist and
#         NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_subdir( handle, dirPath, includeHeader ) {

  local_var handle, dirPath, includeHeader, query, queryres, splitList, _list, item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_subdir" );
    return NULL;
  }

  if( ! dirPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath#-#wmi_file_subdir" );
    return NULL;
  }

  query    = 'SELECT Name FROM Win32_Directory WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  if( includeHeader )
    return splitList;

  _list = make_list();
  foreach item( splitList ) {
    if( item == "Name" ) continue;
    _list = make_list( _list, item );
  }
  return _list;
}

# @brief Checks if a given path exists
#
# @param handle  the WMI handle for the request
# @param dirPath the path to query, Example: "C:\\windows" (With the drive letter but without the trailing backslashe(s))
#
# @return TRUE if the path exists, FALSE otherwise or NULL if not all required
#         information are passed or the target system has denied the access.
#
function wmi_file_check_dir_exists( handle, dirPath ) {

  local_var handle, dirPath, query, queryres;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_check_dir_exists" );
    return NULL;
  }

  if( ! dirPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath#-#wmi_file_check_dir_exists" );
    return NULL;
  }

  query    = 'SELECT Caption FROM Win32_Directory WHERE Name = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;
  else
    return TRUE;
}

# @brief Returns readable rights of sub directories.
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to query, Example: "\\windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param includeHeader if set to TRUE the headers "Name" and "Readable" will be returned in the array
#
# @return An array of readable rights of sub directories, FALSE if the path doesn't exist and
#         NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_is_subdir_readable( handle, dirPath, includeHeader ) {

  local_var handle, dirPath, includeHeader, query, queryres, splitList, _array, item, _item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_is_subdir_readable" );
    return NULL;
  }

  if( ! dirPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath#-#wmi_file_is_subdir_readable" );
    return NULL;
  }

  # nb: Name is always returned so include it in the query as well.
  query    = 'SELECT Name, Readable FROM Win32_Directory WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  _array = make_array();
  foreach item( splitList ) {
    if( item == "Name|Readable" && ! includeHeader ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    _array[_item[0]] = _item[1];
  }
  return _array;
}

# @brief Returns writeable rights of sub directories.
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to query, Example: "\\windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param includeHeader if set to TRUE the headers "Name" and "Writeable" will be returned in the array
#
# @return An array of writeable rights of sub directories, FALSE if the path doesn't exist and
#         NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_is_subdir_writeable( handle, dirPath, includeHeader ) {

  local_var handle, dirPath, includeHeader, query, queryres, splitList, _array, item, _item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_is_subdir_writeable" );
    return NULL;
  }

  if( ! dirPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath#-#wmi_file_is_subdir_writeable" );
    return NULL;
  }

  # nb: Name is always returned so include it in the query as well.
  query    = 'SELECT NName, Writeable FROM Win32_Directory WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  _array = make_array();
  foreach item( splitList ) {
    if( item == "Name|Writeable" && ! includeHeader ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    _array[_item[0]] = _item[1];
  }
  return _array;
}

#############################################################################
# CLASS: CIM_DataFile/Win32_ShortcutFile Functions
# https://docs.microsoft.com/en-us/windows/desktop/CIMWin32Prov/cim-datafile
##############################################################################

# @brief Queries List of all files under directory.
#
# @param handle  the WMI handle for the request
# @param dirPath the path to query, Example: "\\Windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param includeHeader if set to TRUE the header "Name" will be returned in the list
#
# @return A list of all files within the passed path, FALSE if the path doesn't exist and
#         NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_filelist( handle, dirPath, includeHeader ) {

  local_var handle, dirPath, includeHeader, query, queryres, splitList, _list, item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_filelist" );
    return NULL;
  }

  if( ! dirPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath#-#wmi_file_filelist" );
    return NULL;
  }

  query    = 'SELECT Name FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  if( includeHeader )
    return splitList;

  _list = make_list();
  foreach item( splitList ) {
    if( item == "Name" ) continue;
    _list = make_list( _list, item );
  }
  return _list;
}

# @brief Gets the size of a file.
#
# @param handle        the WMI handle for the request
# @param filePath      the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
# @param includeHeader if set to TRUE the headers "Name" and "Writeable" will be returned in the array
#
# @return An array including the file name and its size, FALSE if the file doesn't exist and NULL
#         if not all required information are passed or the target system has denied the access.
#
function wmi_file_filesize( handle, filePath, includeHeader ) {

  local_var handle, filePath, query, includeHeader, query, queryres, splitList, _array, item, _item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_filesize" );
    return NULL;
  }

  if( ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath#-#wmi_file_filesize" );
    return NULL;
  }

  # nb: Name is always returned so include it in the query as well.
  query    = 'SELECT FileSize, Name FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  _array = make_array();
  foreach item( splitList ) {
    if( item == "FileSize|Name" && ! includeHeader ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    _array[_item[0]] = _item[1];
  }
  return _array;
}

# @brief Returns a list of files matching a passed file extension.
#
# @param handle        the WMI handle for the request
# @param fileExtn      the file extension for which files should be searched for, Example: "exe" (without a dot)
# @param dirPath       the path to limit the query to (optional),
#                      Example: "\\Windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param includeHeader if set to TRUE the header "Name" will be returned in the list
#
# @return A list of all files matching the passed file extension, FALSE if no files for this extension exist
#         and NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_get_extnfile( handle, dirPath, fileExtn, includeHeader ) {

  local_var handle, dirPath, fileExtn, includeHeader, query, queryres, splitList, _list, item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_get_extnfile" );
    return NULL;
  }

  if( ! dirPath && ! fileExtn ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath and fileExtn#-#wmi_file_get_extnfile" );
    return NULL;
  }

  if( dirPath && ! fileExtn ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath passed but not fileExtn#-#wmi_file_get_extnfile" );
    return NULL;
  }

  if( dirPath && fileExtn ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( fileExtn ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  }
  queryres = wmi_query( wmi_handle:handle, query:query );

  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  if( includeHeader )
    return splitList;

  _list = make_list();
  foreach item( splitList ) {
    if( item == "Name" ) continue;
    _list = make_list( _list, item );
  }
  return _list;
}

# @brief Checks if a given file exists
#
# @param handle   the WMI handle for the request
# @param filePath the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
#
# @return TRUE if the path exists, FALSE otherwise or NULL if not all required
#         information are passed or the target system has denied the access.
#
function wmi_file_check_file_exists( handle, filePath ) {

  local_var handle, filePath, query, queryres;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_check_file_exists" );
    return NULL;
  }

  if( ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath#-#wmi_file_check_file_exists" );
    return NULL;
  }

  query    = 'SELECT Name FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;
  else
    return TRUE;
}

# @brief Search for a file on the target system with the search scope specified by the given parameters. The function queries the knowledge base first to check
#        if a file matching the specified search pattern was already found and cached and if not it tries to establish a WMI connection to the remote host.
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to limit the query to (optional), Example: "\\Windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param dirPathLike   the path to limit the query to (optional) which is used in a LIKE statement, Example: "%windowssearchengine%"
# @param fileName      the file name to search for (mandatory, without the file extension), Example: "Safari"
# @param fileExtn      the file extension to limit the query to (optional), Example: "exe" (without a dot)
# @param includeHeader if set to TRUE the header "Name" will be returned in the list
#
# @return A list of files matching the query, FALSE if no files exists matching the query or NULL
#         if not all required information are passed or the target system has denied the access.
#
function wmi_file_file_search( handle, dirPath, dirPathLike, fileName, fileExtn, includeHeader ) {

  local_var handle, dirPath, fileName, fileExtn, includeHeader, query, queryres, splitList, _list, item;
  local_var kb_proxy_key, kb_proxy_key_list;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_file_search" );
    return NULL;
  }

  if( ! dirPath && ! dirPathLike && ! fileName && ! fileExtn ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath, dirPathLike, fileName and fileExtn#-#wmi_file_file_search" );
    return NULL;
  }

  if( dirPath && dirPathLike ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#wmi_file_file_search: dirPath and dirPathLike passed but only one of both can be used" );
    return NULL;
  }

  if( dirPath && ( ! fileName && ! fileExtn ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath passed but not fileName and fileExtn#-#wmi_file_file_search" );
    return NULL;
  }

  if( dirPathLike && ( ! fileName && ! fileExtn ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPathLike passed but not fileName and fileExtn#-#wmi_file_file_search" );
    return NULL;
  }

  if( fileExtn && ( ! fileName && ! dirPath && ! dirPathLike ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fileExtn passed but not fileName and dirPath/dirPathLike#-#wmi_file_file_search" );
    return NULL;
  }

  if( fileExtn && ! fileName ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fileExtn passed but not fileName#-#wmi_file_file_search" );
    return NULL;
  }

  if( dirPath && fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_file_search_cache//by_dirpath_filename_fileextn//" + tolower( dirPath ) + "//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( dirPathLike && fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_file_search_cache//by_dirpathlike_filename_fileextn//" + tolower( dirPathLike ) + "//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_file_search_cache//by_filename_fileextn//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( fileName ) {
    kb_proxy_key = "WMI//wmi_file_file_search_cache//by_filename//" + tolower( fileName );
  }

  if( includeHeader )
    kb_proxy_list = get_kb_list( kb_proxy_key + "//withheader" );
  else
    kb_proxy_list = get_kb_list( kb_proxy_key + "//withoutheader" );
  if( ! isnull( kb_proxy_list ) || kb_proxy_list )
    return kb_proxy_list;

  if( dirPath && fileName && fileExtn ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 ) + ' AND FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( dirPathLike && fileName && fileExtn ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE Path LIKE ' + raw_string( 0x22 ) + dirPathLike + raw_string( 0x22 ) + ' AND FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( fileName && fileExtn ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( fileName ) {
    query = 'SELECT Name FROM CIM_DataFile WHERE FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 );
  }
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE; # TBD: Also set an empty list into the cache?

  _list = make_list();

  splitList = split( queryres, keep:FALSE );
  foreach item( splitList ) {
    if( item == "Name" ) {
      set_kb_item( name:kb_proxy_key + "//withheader", value:item );
      if( ! includeHeader ) continue;
    } else {
      set_kb_item( name:kb_proxy_key + "//withheader", value:item );
      set_kb_item( name:kb_proxy_key + "//withoutheader", value:item );
    }
    _list = make_list( _list, item );
  }
  return _list;
}

# @brief Extracts all properties and description of a given file.
#
# @param handle   the WMI handle for the request
# @param filePath the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
#
# @return An array of all properties and description of the given file, FALSE if the file doesn't exist and
#         NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_fileinfo( handle, filePath ) {

  local_var handle, filePath, query, queryres, splitList, propList, returnArray, i, desc, j;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_fileinfo" );
    return NULL;
  }

  if( ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath#-#wmi_file_fileinfo" );
    return NULL;
  }

  query    = 'SELECT * FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  # TODO: Maybe move into a separate function in wmi_misc.inc?
  splitList   = split( queryres, keep:FALSE );
  propList    = split( splitList[0], sep:"|", keep:FALSE );
  returnArray = make_array();

  for( i = 1; i < max_index( splitList ); i++ ) {
    desc = split( splitList[i], sep:"|", keep:FALSE );
    for( j = 0; j < max_index( propList ); j++ ) {
      returnArray[propList[j]] = desc[j];
    }
  }
  return returnArray;
}

# @brief Gets the version of the passed file. The function queries the knowledge base first to check if a version
#        was already found and cached and if not it tries to establish a WMI connection to the remote host.
#
# @param handle        the WMI handle for the request
# @param filePath      the full path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
# @param dirPath       the path to limit the query to (optional), Example: "\\Windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param dirPathLike   the path to limit the query to (optional) which is used in a LIKE statement, Example: "%windowssearchengine%"
# @param fileName      the file name to search for (mandatory, without the file extension), Example: "Safari"
# @param fileExtn      the file extension to limit the query to (optional), Example: "exe" (without a dot)
# @param includeHeader if set to TRUE the headers "Name" and "Version" will be returned in the array
#
# @return An array of files as the key and the version of the file as a value, FALSE if the file doesn't
#         exist and NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_fileversion( handle, filePath, dirPath, dirPathLike, fileName, fileExtn, includeHeader ) {

  local_var handle, filePath, dirPath, dirPathLike, fileName, fileExtn, includeHeader, query, queryres, splitList, _array, item, _item;
  local_var kb_proxy_key, kb_proxy_list;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_fileversion" );
    return NULL;
  }

  if( ! filePath && ! dirPath && ! dirPathLike && ! fileName && ! fileExtn ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath, dirPath, dirPathLike, fileName and fileExtn#-#wmi_file_fileversion" );
    return NULL;
  }

  if( filePath && ( dirPath || dirPathLike || fileName || fileExtn ) ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#wmi_file_fileversion: filePath can be used only alone without other parameters" );
    return NULL;
  }

  if( dirPath && dirPathLike ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#wmi_file_fileversion: dirPath and dirPathLike passed but only one of both can be used" );
    return NULL;
  }

  if( dirPath && ( ! fileName && ! fileExtn ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath passed but not fileName and fileExtn#-#wmi_file_fileversion" );
    return NULL;
  }

  if( dirPathLike && ( ! fileName && ! fileExtn ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPathLike passed but not fileName and fileExtn#-#wmi_file_fileversion" );
    return NULL;
  }

  if( fileExtn && ( ! fileName && ! dirPath && ! dirPathLike ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fileExtn passed but not fileName and dirPath/dirPathLike#-#wmi_file_fileversion" );
    return NULL;
  }

  if( fileExtn && ! fileName ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fileExtn passed but not fileName#-#wmi_file_fileversion" );
    return NULL;
  }

  if( filePath ) {
    kb_proxy_key = "WMI//wmi_file_fileversion_cache//by_filepath//" + tolower( filePath );
  } else if( dirPath && fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_fileversion_cache//by_dirpath_filename_fileextn//" + tolower( dirPath ) + "//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( dirPathLike && fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_fileversion_cache//by_dirpathlike_filename_fileextn//" + tolower( dirPathLike ) + "//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( fileName && fileExtn ) {
    kb_proxy_key = "WMI//wmi_file_fileversion_cache//by_filename_fileextn//" + tolower( fileName ) + "//" + tolower( fileExtn );
  } else if( fileName ) {
    kb_proxy_key = "WMI//wmi_file_fileversion_cache//by_filename//" + tolower( fileName );
  }

  if( includeHeader )
    kb_proxy_list = get_kb_list( kb_proxy_key + "//withheader" );
  else
    kb_proxy_list = get_kb_list( kb_proxy_key + "//withoutheader" );
  if( ! isnull( kb_proxy_list ) || kb_proxy_list ) {
    # nb: The function returns an array where we're saving the KB into in a form of "item#---#value"
    # because we can't save such an array into the KB.
    _array = make_array();
    foreach item( kb_proxy_list ) {
      _item = split( item, sep:"#---#", keep:FALSE );
      _array[_item[0]] = _item[1];
    }
    return _array;
  }

  # nb: Name is always returned so include it in the query as well.
  if( filePath ) {
    query = 'SELECT Name, Version FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  } else if( dirPath && fileName && fileExtn ) {
    query = 'SELECT Name, Version FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 ) + ' AND FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( dirPathLike && fileName && fileExtn ) {
    query = 'SELECT Name, Version FROM CIM_DataFile WHERE Path LIKE ' + raw_string( 0x22 ) + dirPathLike + raw_string( 0x22 ) + ' AND FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( fileName && fileExtn ) {
    query = 'SELECT Name, Version FROM CIM_DataFile WHERE FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 ) + ' AND Extension = ' + raw_string( 0x22 ) + fileExtn + raw_string( 0x22 );
  } else if( fileName ) {
    query = 'SELECT Name, Version FROM CIM_DataFile WHERE FileName = ' + raw_string( 0x22 ) + fileName + raw_string( 0x22 );
  }

  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE; # TBD: Also set an empty list into the cache?

  _array = make_array();

  splitList = split( queryres, keep:FALSE );
  foreach item( splitList ) {
    _item = split( item, sep:"|", keep:FALSE );
    if( _item[0] == "Name" && _item[1] == "Version" ) {
      set_kb_item( name:kb_proxy_key + "//withheader", value:_item[0] + "#---#" + _item[1] );
      if( ! includeHeader ) continue;
    } else {
      set_kb_item( name:kb_proxy_key + "//withheader", value:_item[0] + "#---#" + _item[1] );
      set_kb_item( name:kb_proxy_key + "//withoutheader", value:_item[0] + "#---#" + _item[1] );
    }
    _array[_item[0]] = _item[1];
  }
  return _array;
}

# @brief Returns the creation date of the passed file.
#
# @param handle   the WMI handle for the request
# @param filePath the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
#
# @return A string containing the creation date of the file, FALSE if the file doesn't exist and NULL
#         if not all required information are passed or the target system has denied the access.
#
function wmi_file_filecreationdate( handle, filePath ) {

  local_var handle, filePath, query, queryres, splitList, splitList, item, _item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_filecreationdate" );
    return NULL;
  }

  if( ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath#-#wmi_file_filecreationdate" );
    return NULL;
  }

  # nb: Name is always returned so include it in the query as well.
  query    = 'SELECT CreationDate, Name FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  foreach item( splitList ) {
    if( item == "CreationDate|Name" ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    return _item[0];
  }
  return string( "unknown" );
}

# @brief Returns the last modified date of the passed file.
#
# @param handle   the WMI handle for the request
# @param filePath the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
#
# @return A string containing the last modified date of the file, FALSE if the file doesn't exist and NULL
#         if not all required information are passed or the target system has denied the access.
#
function wmi_file_filemodifieddate( handle, filePath ) {

  local_var handle, filePath, query, queryres, splitList, splitList, item, _item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_filemodifieddate" );
    return NULL;
  }

  if( ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#filePath#-#wmi_file_filemodifieddate" );
    return NULL;
  }

  # nb: Name is always returned so include it in the query as well.
  query    = 'SELECT LastModified, Name FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  foreach item( splitList ) {
    if( item == "LastModified|Name" ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    return _item[0];
  }
  return string( "unknown" );
}

# @brief Returns an array of files containing the info if these are readable.
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to the directory to query (takes precedence over the filePath variable if both are passed),
#                      Example: "\\windows\\" (Without the drive letter and with the doubled trailing backslashes)
# @param filePath      the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
# @param includeHeader if set to TRUE the headers "Name" and "Readable" will be returned in the array
#
# @return An array of files as the key and the info if these are readable as a value, FALSE if the file/path doesn't exist
#         and NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_is_file_readable( handle, dirPath, filePath, includeHeader ) {

  local_var handle, dirPath, filePath, includeHeader, query, queryres, splitList, _array, item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_is_file_readable" );
    return NULL;
  }

  if( ! dirPath && ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath and filePath#-#wmi_file_is_file_readable" );
    return NULL;
  }

  # nb: Name is always returned so include it in the queries as well.
  if( dirPath )
    query = 'SELECT Name, Readable FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  else
    query = 'SELECT Name, Readable FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  _array = make_array();
  foreach item( splitList ) {
    if( item == "Name|Readable" && ! includeHeader ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    _array[_item[0]] = _item[1];
  }
  return _array;
}

# @brief Returns an array of files containing the info if these are writeable.
#
# @param handle        the WMI handle for the request
# @param dirPath       the path to the directory to query (takes precedence over the filePath variable if both are passed),
#                      Example: "\\windows\\"  (Without the drive letter and with the doubled trailing backslashes)
# @param filePath      the path to the file to query (including the file extension), Example: "c:\\windows\\system32\\cmd.exe"
# @param includeHeader if set to TRUE the headers "Name" and "Writeable" will be returned in the array
#
# @return An array of files as the key and the info if these are writeable as a value, FALSE if the file/path doesn't exist
#         and NULL if not all required information are passed or the target system has denied the access.
#
function wmi_file_is_file_writeable( handle, dirPath, filePath, includeHeader ) {

  local_var handle, dirPath, filePath, includeHeader, query, queryres, splitList, _array, item;

  if( isnull( handle ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#handle#-#wmi_file_is_file_writeable" );
    return NULL;
  }

  if( ! dirPath && ! filePath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dirPath and filePath#-#wmi_file_is_file_writeable" );
    return NULL;
  }

  # nb: Name is always returned so include it in the queries as well.
  if( dirPath )
    query = 'SELECT Name, Writeable FROM CIM_DataFile WHERE Path = ' + raw_string( 0x22 ) + dirPath + raw_string( 0x22 );
  else
    query = 'SELECT Name, Writeable FROM CIM_DataFile WHERE Name = ' + raw_string( 0x22 ) + filePath + raw_string( 0x22 );
  queryres = wmi_query( wmi_handle:handle, query:query );
  if( "NTSTATUS" >< queryres )
    return NULL;
  else if( ! queryres )
    return FALSE;

  splitList = split( queryres, keep:FALSE );
  _array = make_array();
  foreach item( splitList ) {
    if( item == "Name|Writeable" && ! includeHeader ) continue;
    _item = split( item, sep:"|", keep:FALSE );
    _array[_item[0]] = _item[1];
  }
  return _array;
}