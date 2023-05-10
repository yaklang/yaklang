###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for User Accounts
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
###############################################################################

##############################################################################
# CLASS: Win32_SystemAccount / Win32_UserAccount / Win32_Group / Win32_GroupUser Functions
#
# - wmi_user_sysaccount              - Queries System Account Information.
# - wmi_user_useraccount             - Lists the information of User Account.
# - wmi_user_group                   - Returns the Group of target system.
# - wmi_user_groupuser               - Gets associated Groups for all users.
# - wmi_user_is_useraccount_disabled - Returns Disabled=True, Enabled=False.
# - wmi_user_is_userpass_changeable  - Returns Cangeable=True, else False.
# - wmi_user_is_userpass_expires     -
# - wmi_user_is_user_locked          -
#
##############################################################################

# Ex: usrName = "Administrator"
function wmi_user_sysaccount(handle, usrName)
{
  if(!usrName){
    query = "Select SID from Win32_SystemAccount";
  }
  else {
    query = 'Select * from Win32_SystemAccount Where Name = ' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrList) || !usrList){
    return(0);
  }

  return usrList;
}

# Ex: usrName = "Administrator"
function wmi_user_useraccount(handle, usrName)
{
  if(!usrName){
    query = "Select SID from Win32_UserAccount";
  }
  else {
    query = 'Select * from Win32_UserAccount Where Name = ' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrList) || !usrList){
    return(0);
  }

  return usrList;
}

# Ex:  grpName = "Administrators"
function wmi_user_group(handle, grpName)
{
  if(!usrName){
    query = "Select SID from Win32_Group";
  }
  else {
    query = 'Select * from Win32_Group Where Name = ' +
             raw_string(0x22) + grpName + raw_string(0x22);
  }

  grpList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< grpList) || !grpList){
    return(0);
  }

  return grpList;
}

function wmi_user_groupuser(handle)
{
  query = 'Select * from Win32_GroupUser';

  grpList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< grpList) || !grpList){
    return(0);
  }

  return grpList;
}

# Ex: usrName = "Administrator"
function wmi_user_is_useraccount_disabled(handle, usrName)
{
  if(!usrName){
    query = 'Select Disabled from Win32_UserAccount';
  }else{
    query = 'Select Disabled from Win32_UserAccount Where Name = ' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrName = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrName) || !usrName){
    return(0);
  }

  return usrName;
}

function wmi_user_is_userpass_changeable(handle, usrName)
{
  if(!usrName){
    query = 'Select PasswordChangeable from Win32_UserAccount';
  }
  else {
    query = 'Select PasswordChangeable from Win32_UserAccount Where Name =' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrName = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrName) || !usrName){
    return(0);
  }

  return usrName;
}

function wmi_user_is_userpass_expires(handle, usrName)
{
  if(!usrName){
    query = 'Select PasswordExpires from Win32_UserAccount';
  }
  else {
    query = 'Select PasswordExpires from Win32_UserAccount Where Name =' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrName = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrName) || !usrName){
    return(0);
  }

  return usrName;
}

function wmi_user_is_userpass_required(handle, usrName)
{
  if(!usrName){
    query = 'Select PasswordRequired from Win32_UserAccount';
  }
  else {
    query = 'Select PasswordRequired from Win32_UserAccount Where Name =' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrName = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrName) || !usrName){
    return(0);
  }

  return usrName;
}

function wmi_user_is_user_locked(handle, usrName)
{
  if(!usrName){
    query = 'Select Lockout from Win32_UserAccount';
  }
  else {
    query = 'Select Lockout from Win32_UserAccount Where Name =' +
             raw_string(0x22) + usrName + raw_string(0x22);
  }

  usrName = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrName) || !usrName){
    return(0);
  }

  return usrName;
}

function wmi_local_users(handle)
{
  query = 'Select * from Win32_UserAccount Where LocalAccount=true';

  LocalUserList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< LocalUserList) || !LocalUserList){
    return(0);
  }

  return LocalUserList;
}

function wmi_useraccounts_active(handle)
{
  query = "Select SID from Win32_UserAccount WHERE Status='Ok'";
  usrList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< usrList) || !usrList){
    set_kb_item(name:"WMI/UserAccount/Active", value:"Error");
    return(0);
  }
  set_kb_item(name:"WMI/UserAccount/Active", value:usrList);
  return usrList;
}