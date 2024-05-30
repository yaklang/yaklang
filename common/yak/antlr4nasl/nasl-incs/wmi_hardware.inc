###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for Windows Hardware
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
# CLASS: Win32 Hardware Classes.
##############################################################################

function wmi_hardware_logicaldisk(handle)
{
  query = 'Select * From Win32_LogicalDisk';

  ldList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< ldList) || !ldList){
    return(0);
  }

  return wmi_misc_split_res(List:ldList);
}

# Win32_DisplayConfiguration
function wmi_hardware_displayconf(handle)
{
  query = 'Select * From Win32_DisplayConfiguration';

  confList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< confList) || !confList){
    return(0);
  }

  return wmi_misc_split_res(List:confList);
}


# Win32_FloppyDrive
function wmi_hardware_floppydrive(handle)
{
  query = 'Select * From Win32_FloppyDrive';

  flpyList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< flpyList) || !flpyList){
    return(0);
  }

  return wmi_misc_split_res(List:flpyList);
}

# Win32_FloppyController
function wmi_hardware_floppycontroller(handle)
{
  query = 'Select * From Win32_FloppyController';

  flpyList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< flpyList) || !flpyList){
    return(0);
  }

  return wmi_misc_split_res(List:flpyList);
}

# Win32_IDEController
function wmi_hardware_idecontroller(handle)
{
  query = 'Select * From Win32_IDEController';

  ideList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< ideList) || !ideList){
    return(0);
  }

  return wmi_misc_split_res(List:ideList);
}

# Win32_IP4RouteTable
function wmi_hardware_ip4routetable(handle)
{
  query = 'Select * From Win32_IP4RouteTable';

  ip4List = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< ip4List) || !ip4List){
    return(0);
  }

  return wmi_misc_split_res(List:ip4List);
}

# Win32_Keyboard
function wmi_hardware_get_keyboard(handle)
{
  query = 'Select * From Win32_Keyboard';

  keyList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< keyList) || !keyList){
    return(0);
  }

  return wmi_misc_split_res(List:keyList);
}

# Win32_Processor
function wmi_hardware_processor(handle)
{
  query = 'Select * From Win32_Processor';

  procList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< procList) || !procList){
    return(0);
  }

  return wmi_misc_split_res(List:procList);
}

# Win32_SCSIController
function wmi_hardware_scsicontroller(handle)
{
  query = 'Select * From Win32_SCSIController';

  scsiList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< scsiList) || !scsiList){
    return(0);
  }

  return wmi_misc_split_res(List:scsiList);
}

# Win32_SCSIControllerDevice
function wmi_hardware_scsicontrollerdevice(handle)
{
  query = 'Select * From Win32_SCSIControllerDevice';

  scsiList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< scsiList) || !scsiList){
    return(0);
  }

  return wmi_misc_split_res(List:scsiList);
}

# Win32_SerialPort
function wmi_hardware_serialport(handle)
{
  query = 'Select * From Win32_SerialPort';

  portList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< portList) || !portList){
    return(0);
  }

  return wmi_misc_split_res(List:portList);
}

# Win32_SerialPortConfiguration
function wmi_hardware_serialportconf(handle)
{
  query = 'Select * From Win32_SerialPortConfiguration';

  portList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< portList) || !portList){
    return(0);
  }

  return wmi_misc_split_res(List:portList);
}

function wmi_misc_software(handle, filePath, instDate)
{
  if(!filePath && !instDate){
    query = 'Select Path from Win32_SoftwareElement';
  }
  else if(instDate){
    query = 'Select * from Win32_SoftwareElement Where InstallDate = ' +
             raw_string(0x22) + instDate + raw_string(0x22);
  }
  else if(filePath){
    query = 'Select Version from Win32_SoftwareElement Where Path = ' +
             raw_string(0x22) + filePath + raw_string(0x22);
  }

  winAll = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< winAll || !winAll)){
    return(0);
  }

  return wmi_misc_split_res(List:winAll);
}
