###############################################################################
# OpenVAS Include File
# $Id$
#
# WMI Misc functions
#
# Authors:
# Chandrashekhar B <bchandra@secpod.com>
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
# $Revision$
#
###############################################################################

#
# An example SDDL (Security Descriptor Description Lanugage) looks like,
# "O:BAG:SYD:PAI(A;;0x001200a9;;;S-1-5-32-545)(A;OICIIO;GRGX;;;S-1-5-32-545)(A;;0x001301bf;;;S-1-5-32-547)(A;OICIIO;SDGRGWGX;;;S-1-5-32-547)(A;;0x001f01ff;;;BA)(A;OICIIO;GA;;;BA)(A;;0x001f01ff;;;SY)(A;OICIIO;GA;;;SY)(A;;0x001f01ff;;;BA)(A;OICIIO;GA;;;CO)"
#
# The below codes will help construct regular expressions like the above string to
# check access rights
#

sid_codes = ["AO", "SID_BUILTIN_ACCOUNT_OPERATORS",
             "BA", "SID_BUILTIN_ADMINISTRATORS",
             "RU", "SID_BUILTIN_PREW2K",
             "PO", "SID_BUILTIN_PRINT_OPERATORS", 
             "RS", "SID_BUILTIN_RAS_SERVERS",
             "AU", "SID_NT_AUTHENTICATED_USERS",
             "SY", "SID_NT_SYSTEM",
             "PS", "SID_NT_SELF",
             "WD", "SID_WORLD",
             "ED", "SID_NT_ENTERPRISE_DCS",
             "CO", "SID_CREATOR_OWNER",
             "CG", "SID_CREATOR_GROUP",
             "DA", "NULL, DOMAIN_RID_ADMINS",
             "EA", "DOMAIN_RID_ENTERPRISE_ADMINS",
             "DD", "NULL, DOMAIN_RID_DCS",
             "DU", "NULL, DOMAIN_RID_USERS",
             "CA", "NULL, DOMAIN_RID_CERT_ADMINS", 
             "TI", "SID_TRUSTED_INSTALLER",
             "S-1-0-0", "SID_NULL",
             "WORLD", "NAME_WORLD",
             "S-1-1", "SID_WORLD_DOMAIN",
             "S-1-1-0", "SID_WORLD",
             "S-1-3", "SID_CREATOR_OWNER_DOMAIN",
             "S-1-3-0", "SID_CREATOR_OWNER",
             "S-1-3-1", "SID_CREATOR_GROUP",
             "NT AUTHORITY", "NAME_NT_AUTHORITY",
             "SID_NT_AUTHORITY", "S-1-5",
             "S-1-5-1", "SID_NT_DIALUP",
             "S-1-5-2", "SID_NT_NETWORK",
             "S-1-5-3", "SID_NT_BATCH",
             "S-1-5-4", "SID_NT_INTERACTIVE",
             "S-1-5-6", "SID_NT_SERVICE",
             "S-1-5-7", "SID_NT_ANONYMOUS",
             "S-1-5-8", "SID_NT_PROXY",
             "S-1-5-9", "SID_NT_ENTERPRISE_DCS",
             "S-1-5-10", "SID_NT_SELF",
             "S-1-5-11", "SID_NT_AUTHENTICATED_USERS",
             "S-1-5-12", "SID_NT_RESTRICTED",
             "S-1-5-13", "SID_NT_TERMINAL_SERVER_USERS",
             "S-1-5-14", "SID_NT_REMOTE_INTERACTIVE",
             "S-1-5-15", "SID_NT_THIS_ORGANISATION",
             "S-1-5-18", "SID_NT_SYSTEM",
             "S-1-5-19", "SID_NT_LOCAL_SERVICE",
             "S-1-5-20", "SID_NT_NETWORK_SERVICE",
             "BUILTIN", "NAME_BUILTIN",
             "S-1-5-32", "SID_BUILTIN",
             "S-1-5-32-544", "SID_BUILTIN_ADMINISTRATORS",
             "S-1-5-32-545", "SID_BUILTIN_USERS",
             "S-1-5-32-546", "SID_BUILTIN_GUESTS",
             "S-1-5-32-547", "SID_BUILTIN_POWER_USERS",
             "S-1-5-32-548", "SID_BUILTIN_ACCOUNT_OPERATORS",
             "S-1-5-32-549", "SID_BUILTIN_SERVER_OPERATORS",
             "S-1-5-32-550", "SID_BUILTIN_PRINT_OPERATORS",
             "S-1-5-32-551", "SID_BUILTIN_BACKUP_OPERATORS",
             "S-1-5-32-552", "SID_BUILTIN_REPLICATOR",
             "S-1-5-32-553", "SID_BUILTIN_RAS_SERVERS",
             "S-1-5-32-554", "SID_BUILTIN_PREW2K",
             "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464", "SID_TRUSTED_INSTALLER"];

ace_types = [ "AU", "SEC_ACE_TYPE_SYSTEM_AUDIT",
              "AL", "SEC_ACE_TYPE_SYSTEM_ALARM",
              "OA", "SEC_ACE_TYPE_ACCESS_ALLOWED_OBJECT",
              "OD", "SEC_ACE_TYPE_ACCESS_DENIED_OBJECT",
              "OU", "SEC_ACE_TYPE_SYSTEM_AUDIT_OBJECT",
              "OL", "SEC_ACE_TYPE_SYSTEM_ALARM_OBJECT",
              "A",  "SEC_ACE_TYPE_ACCESS_ALLOWED",
              "D",  "SEC_ACE_TYPE_ACCESS_DENIED"];

ace_flags = [ "OI", "SEC_ACE_FLAG_OBJECT_INHERIT",
              "CI", "SEC_ACE_FLAG_CONTAINER_INHERIT",
              "NP", "SEC_ACE_FLAG_NO_PROPAGATE_INHERIT",
              "IO", "SEC_ACE_FLAG_INHERIT_ONLY",
              "ID", "SEC_ACE_FLAG_INHERITED_ACE",
              "SA", "SEC_ACE_FLAG_SUCCESSFUL_ACCESS",
              "FA", "SEC_ACE_FLAG_FAILED_ACCESS"];

ace_access_mask = [
              "RP", "SEC_ADS_READ_PROP",
              "WP", "SEC_ADS_WRITE_PROP",
              "CR", "SEC_ADS_CONTROL_ACCESS",
              "CC", "SEC_ADS_CREATE_CHILD",
              "DC", "SEC_ADS_DELETE_CHILD",
              "LC", "SEC_ADS_LIST",
              "LO", "SEC_ADS_LIST_OBJECT",
              "RC", "SEC_STD_READ_CONTROL",
              "WO", "SEC_STD_WRITE_OWNER",
              "WD", "SEC_STD_WRITE_DAC",
              "SD", "SEC_STD_DELETE",
              "DT", "SEC_ADS_DELETE_TREE",
              "SW", "SEC_ADS_SELF_WRITE",
              "GA", "SEC_GENERIC_ALL",
              "GR", "SEC_GENERIC_READ",
              "GW", "SEC_GENERIC_WRITE",
              "GX", "SEC_GENERIC_EXECUTE"];

access_mask_hex = ["0xF0000000", "SEC_MASK_GENERIC",
                   "0x0F000000", "SEC_MASK_FLAGS",
                   "0x00FF0000", "SEC_MASK_STANDARD",
                   "0x0000FFFF", "SEC_MASK_SPECIFIC",
                   "0x10000000", "SEC_GENERIC_ALL",
                   "0x20000000", "SEC_GENERIC_EXECUTE",
                   "0x40000000", "SEC_GENERIC_WRITE",
                   "0x80000000", "SEC_GENERIC_READ",
                   "0x01000000", "SEC_FLAG_SYSTEM_SECURITY",
                   "0x02000000", "SEC_FLAG_MAXIMUM_ALLOWED",
                   "0x00010000", "SEC_STD_DELETE",
                   "0x00020000", "SEC_STD_READ_CONTROL",
                   "0x00040000", "SEC_STD_WRITE_DAC",
                   "0x00080000", "SEC_STD_WRITE_OWNER",
                   "0x00100000", "SEC_STD_SYNCHRONIZE",
                   "0x000F0000", "SEC_STD_REQUIRED",
                   "0x001F0000", "SEC_STD_ALL",
                   "0x00000001", "SEC_FILE_READ_DATA",
                   "0x00000002", "SEC_FILE_WRITE_DATA",
                   "0x00000004", "SEC_FILE_APPEND_DATA",
                   "0x00000008", "SEC_FILE_READ_EA",
                   "0x00000010", "SEC_FILE_WRITE_EA",
                   "0x00000020", "SEC_FILE_EXECUTE",
                   "0x00000080", "SEC_FILE_READ_ATTRIBUTE",
                   "0x00000100", "SEC_FILE_WRITE_ATTRIBUTE",
                   "0x000001ff", "SEC_FILE_ALL",
                   "0x00000001", "SEC_DIR_LIST",
                   "0x00000002", "SEC_DIR_ADD_FILE",
                   "0x00000004", "SEC_DIR_ADD_SUBDIR", 
                   "0x00000008", "SEC_DIR_READ_EA",
                   "0x00000010", "SEC_DIR_WRITE_EA",
                   "0x00000020", "SEC_DIR_TRAVERSE",
                   "0x00000040", "SEC_DIR_DELETE_CHILD",
                   "0x00000080", "SEC_DIR_READ_ATTRIBUTE",
                   "0x00000100", "SEC_DIR_WRITE_ATTRIBUTE",
                   "0x00000001", "SEC_REG_QUERY_VALUE",
                   "0x00000002", "SEC_REG_SET_VALUE",
                   "0x00000004", "SEC_REG_CREATE_SUBKEY",
                   "0x00000008", "SEC_REG_ENUM_SUBKEYS",
                   "0x00000010", "SEC_REG_NOTIFY",
                   "0x00000020", "SEC_REG_CREATE_LINK",
                   "0x00000001", "SEC_ADS_CREATE_CHILD",
                   "0x00000002", "SEC_ADS_DELETE_CHILD",
                   "0x00000004", "SEC_ADS_LIST",
                   "0x00000008", "SEC_ADS_SELF_WRITE",
                   "0x00000010", "SEC_ADS_READ_PROP",
                   "0x00000020", "SEC_ADS_WRITE_PROP",
                   "0x00000040", "SEC_ADS_DELETE_TREE",
                   "0x00000080", "SEC_ADS_LIST_OBJECT",
                   "0x00000100", "SEC_ADS_CONTROL_ACCESS"];

access_mask_or = [
 "SEC_RIGHTS_FILE_READ",
 "SEC_STD_READ_CONTROL|SEC_STD_SYNCHRONIZE|SEC_FILE_READ_DATA|SEC_FILE_READ_ATTRIBUTE|SEC_FILE_READ_EA",
 "SEC_RIGHTS_FILE_WRITE",
 "SEC_STD_READ_CONTROL|SEC_STD_SYNCHRONIZE|SEC_FILE_WRITE_DATA|SEC_FILE_WRITE_ATTRIBUTE|SEC_FILE_WRITE_EA|SEC_FILE_APPEND_DATA",
 "SEC_RIGHTS_FILE_EXECUTE",
 "SEC_STD_SYNCHRONIZE|SEC_STD_READ_CONTROL|SEC_FILE_READ_ATTRIBUTE|SEC_FILE_EXECUTE",
 "SEC_RIGHTS_FILE_ALL", "SEC_STD_ALL|SEC_FILE_ALL",
 "SEC_RIGHTS_DIR_READ", "SEC_RIGHTS_FILE_READ",
 "SEC_RIGHTS_DIR_WRITE", "SEC_RIGHTS_FILE_WRITE",
 "SEC_RIGHTS_DIR_EXECUTE", "SEC_RIGHTS_FILE_EXECUTE",
 "SEC_RIGHTS_DIR_ALL", "SEC_RIGHTS_FILE_ALL"];

function get_wmi_misc_sid_codes() {
  return sid_codes;
}

function get_wmi_misc_ace_types() {
  return ace_types;
}

function get_wmi_misc_ace_flags() {
  return ace_flags;
}

function get_wmi_misc_ace_access_mask() {
  return ace_access_mask;
}

function get_wmi_misc_access_mask_hex() {
  return access_mask_hex;
}

function get_wmi_misc_get_access_mask_or() {
  return access_mask_or;
}

# Returns WMI Query by splitting its properties.
function wmi_misc_split_res(List)
{
  if(List == NULL){
    return(0);
  }

  List = split(List, "\n", keep:0);
  prop = split(List[0], sep:"|", keep:0);

  for(i=1; i<max_index(List); i++)
  {
    desc = split(List[i], sep:"|", keep:0);
    for(j=0; j<max_index(prop); j++){
      finalList = finalList + '\n' + prop[j] + ' = ' + desc[j];
    }
    finalList = finalList + '\n\n';
  }
  return finalList;
}



#
# Obtain Owner SID from the SDDL
#
function wmi_misc_decode_owner_sid(sddl)
{
  if (sddl == NULL)
    return NULL;

  sddl_owner_sid = eregmatch(string:sddl, pattern:"O:[a-zA-Z0-9-]{0,}:", icase:TRUE);
  sddl_owner_sid_code = split(sddl_owner_sid[0], sep:":", keep:0);
  sid_length = strlen(sddl_owner_sid_code[1]);
  sddl_owner_sid_code_cut = substr(sddl_owner_sid_code[1],0,cut=sid_length-2);

  for(i = 0; i >= 0; i++)
  {
    if(sid_codes[i] == NULL)
      break;

    if(sid_codes[i] == sddl_owner_sid_code_cut)
      val = sid_codes[i + 1]; 
    
  }
  if (val) return val;
  else return sddl_owner_sid_code_cut;
}


#
# Obtain Groups SID from SDDL
#
function wmi_misc_decode_group_sid(sddl)
{
  if (sddl == NULL)
    return NULL;

  sddl_owner_sid = eregmatch(string:sddl, pattern:"G:[a-zA-Z0-9-]{0,}:", icase:TRUE);
  sddl_owner_sid_code = split(sddl_owner_sid[0], sep:":", keep:0);
  sid_length = strlen(sddl_owner_sid_code[1]);
  sddl_owner_sid_code_cut = substr(sddl_owner_sid_code[1],0,cut=sid_length-2);

  for(i = 0; i >= 0; i++)
  {
    if(sid_codes[i] == NULL)
        break;

    if(sid_codes[i] == sddl_owner_sid_code_cut)
      val = sid_codes[i + 1];

  }
  if (val) return val;
  else return sddl_owner_sid_code_cut;
}

#
# Obtain DACL string from SDDL
#
function wmi_misc_decode_dacl(sddl)
{
  if (sddl == NULL)
    return NULL;

  dacl = eregmatch(string:sddl, pattern:"D:.+S:", icase:TRUE);
  dacl = dacl[0] - "S:";

  return dacl;
}


#
# Obtain SACL string from SDDL
#

function wmi_misc_decode_sacl(sddl)
{
  if (sddl == NULL)
    return NULL;

  return eregmatch(string:sddl, pattern:"S:.+", icase:TRUE);
}

# Win32_LocalTime
function wmi_misc_localtime(handle)
{
  query = 'Select * From Win32_LocalTime';

  timeList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< timeList) || !timeList){
    return(0);
  }

  return wmi_misc_split_res(List:timeList);
}

# Win32_LogicalDiskToPartition
function wmi_misc_logicaldiskpartition(handle)
{
  query = 'Select * From Win32_LogicalDiskToPartition';

  parList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< parList) || !parList){
    return(0);
  }

  return wmi_misc_split_res(List:parList);
}

# Win32_NetworkAdapter
function wmi_misc_networkadapter(handle)
{
  query = 'Select * From Win32_NetworkAdapter';

  adptList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< adptList) || !adptList){
    return(0);
  }

  return wmi_misc_split_res(List:adptList);
}

# Win32_NetworkProtocol
function wmi_misc_networkprotocol(handle)
{
  query = 'Select * From Win32_NetworkProtocol';

  protList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< protList) || !protList){
    return(0);
  }

  return wmi_misc_split_res(List:protList);
}
