###############################################################################
# OpenVAS Include File
# $Id: kyocera_printers.inc 13658 2019-02-14 08:16:14Z cfischer $
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2013 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version
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
# $Revision: 13658 $
#
###############################################################################

global_var ky_last_user, ky_last_pass;

function get_ky_detect_urls() {

  local_var ky_detect_urls;

  ky_detect_urls = make_array();

  ky_detect_urls['var ModelName="([^"]+)"'] = '/';
  ky_detect_urls['HeaderStatusPC."([^"]+)"'] = '/startwlm/Start_Wlm.htm';
  ky_detect_urls['PtrVal\\[0\\] = "([^"]+)";'] = '/eng/start/about.htm';
  ky_detect_urls['PtrVal\\[0\\] = "([^"]+)"'] = '/start/about.htm';
  ky_detect_urls['sVal\\[0\\] = "([^"]+)";'] = '/eng/status/statprn.htm';
  ky_detect_urls['sVal\\[0\\] = "([^"]+)"'] = '/eng/status/statprn1.htm';

  return ky_detect_urls;
}

function ky_default_logins() {

  return make_list("none:admin00",
                   "Admin:admin00",
                   "admin00:admin00",
                   "Admin:Admin",
                   "admin:admin",
                   "5200:5200",
                   "root:root",
                   "2800:2800",
                   "3000:3000",
                   "admin:none",
                   "3500:3500",
                   "4000:4000",
                   "2500:2500",
                   "/:admin00",
                   "none:PASSWORD");
}

function check_ky_default_login( model, port ) {

  local_var model, port;
  local_var ky, logins, host, useragent, login, user_pass;
  local_var username, password, login_data, len, req, c_buf;

  ky = ky_login_details(model:model);
  if(!ky)
    return FALSE;

  logins = ky_default_logins();

  host = http_host_name(port:port);
  useragent = http_get_user_agent();

  foreach login (logins) {

    user_pass = split(login, sep:":", keep:FALSE);
    username = user_pass[0];
    password = user_pass[1];

    if(username == "none") username = "";
    if(password == "none") password = "";

    login_data = ky['login_data'];

    if(login_data) {
      login_data = str_replace(string:login_data, find:"%%%USERNAME%%%", replace:username);
      login_data = str_replace(string:login_data, find:"%%%PASSWORD%%%", replace:password);
    }

    len = strlen(login_data);

    if(ky['req_type'] == "POST") {

       req = string("POST " , ky['login_url'], " HTTP/1.1\r\n",
                    "Host: ", host,"\r\n",
                    "DNT: 1\r\n",
                    "User-Agent: ", useragent, "\r\n",
                    "Content-Type: application/x-www-form-urlencoded\r\n",
                    "Content-Length: ", len, "\r\n",
                    "Connection: close\r\n",
                    "\r\n",
                    login_data);
    }

    else if(ky['req_type'] == "GET") {

      if(ky['req_auth'] == "BASIC") {

        userpass = username + ':' + password;
        userpass64 = base64(str:userpass);

        req = string("GET ", ky['login_url'], " HTTP/1.1\r\n",
                     "Host: ", host, "\r\n");

        c_buf = http_send_recv(port:port, data:req + '\r\n', bodyonly:FALSE);
        ky_error_exit(buf:c_buf);

        if("HTTP/1.1 401" >!< c_buf && "HTTP/1.1 302" >!< c_buf)
          return 2; # setup without auth

        req += string("Authorization: Basic ", userpass64, "\r\n\r\n");
      }
    }

    buf = http_send_recv(port:port, data:req, bodyonly:FALSE);
    ky_error_exit(buf:buf);

    if(ereg(pattern:ky['http_status'], string:buf)) {
      if(eregmatch(pattern:ky['login_success'], string:buf)) {
        ky_last_user = username;
        ky_last_pass = password;
        return 1;
      }
    }
  }
  return 0;
}

function ky_login_details(model) {

  local_var model;
  local_var ky_printer;

  ky_printer = make_array();

  if(model == "FS-1030D") {

    ky_printer['req_type']  = "GET";
    ky_printer['login_url'] = '/eng/basic/SecAcct.htm';
    ky_printer['req_auth']  = "BASIC";
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'form0.NEWPASS.value';

    return ky_printer;
  }

  else if(model == "FS-9530DN"        ||
          model == "KM-2560"          ||
          model == "KM-2050"          ||
          model == "CD 1240_DC 2240"  ||
          model == "FS-2000D"         ||
          model == "FS-9130DN"        ||
          model == "KM-3060"          ||
          model == "KM-C4035E"        ||
          model == "FS-3820N"         ||
          model == "FS-3900DN"        ||
          model == "TASKalfa 221"     ||
          model == "2GR-50"           ||
          model == "KM-C2525E"        ||
          model == "KM-C2525E(J)"     ||
          model == "A3-3140_314G"     ||
          model == "A3-2640_264G"     ||
          model == "TASKalfa 181(J)"  ||
          model == "CHC-2650_265G-05" ||
          model == "KM-2550"          ||
          model == "KM-1650(J)"       ||
          model == "KM-1820"          ||
          model == "FS-C5030N"        ||
          model == "KM-4050"          ||
          model == "FS-C5015N"        ||
          model == "KM-5050") {

    ky_printer['req_type']  = "GET";
    ky_printer['login_url'] = '/basic/SecAcct.htm';
    ky_printer['req_auth']  = "BASIC";
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'form0.NEWPASS.value';

    return ky_printer;
  }

  else if(model == "FS-9500DN"              ||
          model == "FS-1920"                ||
          model == "FS-9100DN"              ||
          model == "FS-1900"                ||
          model == "FS-1000"                ||
          model == "FS-1010"                ||
          model == "Printing System (U) 30" ||
          model == "Printing System (N) 35" ||
          model == "Printing System (U) 50" ||
          model == "Printing System (Q) 45" ||
          model == "KM-2530"                ||
          model == "KM-3035(J)"             ||
          model == "KM-3035"                ||
          model == "KM-5035"                ||
          model == "KM-5035(J)"             ||
          model == "FS-3800"                ||
          model == "CHC-3035"               ||
          model == "KM-3530"                ||
          model == "A4_Cr-16PPM-04"         ||
          model == "KM-2030"                ||
          model == "FS-9120DN"              ||
          model == "FS-9520DN"              ||
          model == "FS-1300D"               ||
          model == "FS-6020"                ||
          model == "CHC-4035"               ||
          model == "KM-2531(J)"             ||
          model == "FS-1020D"               ||
          model == "FS-1120D") {

    ky_printer['req_type']  = "GET";
    ky_printer['login_url'] = '/eng/security/main.htm';
    ky_printer['req_auth']  = "BASIC";
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'The Admin password controls access';

    return ky_printer;
  }

  else if(model == "FS-6025MFP"     ||
          model == "TASKalfa 300i"  ||
          model == "FS-1128MFP"     ||
          model == "FS-C5250DN"     ||
          model == "FS-C5200DN"     ||
          model == "TASKalfa 400ci" ||
          model == "FS-1135MFP"     ||
          model == "FS-1370DN"      ||
          model == "FS-C5100DN"     ||
          model == "FS-C2126MFP"    ||
          model == "FS-1028MFP"     ||
          model == "FS-C5150DN"     ||
          model == "FS-4020DN"      ||
          model == "FS-3925DN"      ||
          model == "FS-2020D"       ||
          model == "TASKalfa 255"   ||
          model == "FS-3920DN"      ||
          model == "FS-1035MFP"     ||
          model == "FS-C5350DN"     ||
          model == "FS-C2126MFP+"   ||
          model == "FS-C5300DN"     ||
          model == "FS-C5400DN"     ||
          model == "FS-3540MFP"     ||
          model == "FS-C8020MFP"    ||
          model == "KM-2810"        ||
          model == "TASKalfa 420i"  ||
          model == "TASKalfa 300ci" ||
          model == "FS-C2626MFP"    ||
          model == "FS-3140MFP"     ||
          model == "FS-3640MFP"     ||
          model == "FS-C8025MFP"    ||
          model == "FS-C2026MFP+"   ||
          model == "FS-1130MFP"     ||
          model == "FS-2025D"       ||
          model == "KM-2820") {

    ky_printer['req_type']  = "POST";
    ky_printer['login_url'] = '/start/login.cgi';
    ky_printer['login_data'] = 'okhtmfile=%2Fopt1%2Findex.htm&failhtmfile=%2Fstart%2FStartAccessDenied.htm&func=authLogin&arg01_UserName=%%%USERNAME%%%&arg02_Password=%%%PASSWORD%%%&arg03_LoginType=&submit001=OK&arg01_UserName=&language=..%2Fopt1%2Findex.htm';
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'Set-Cookie: level=3';

    return ky_printer;
  }

  else if(model == "TASKalfa 4500i"  ||
          model == "TASKalfa 2550ci" ||
          model == "TASKalfa 3500i"  ||
          model == "TASKalfa 4550ci" ||
          model == "TASKalfa 5500i"  ||
          model == "FS-4300DN"       ||
          model == "FS-6525MFP"      ||
          model == "TASKalfa 3050ci") {

    ky_printer['req_type']  = "POST";
    ky_printer['login_url'] = '/startwlm/login.cgi';
    ky_printer['login_data'] = 'failhtmfile=%2Fstartwlm%2FStart_Wlm.htm&okhtmfile=%2Fstartwlm%2FStart_Wlm.htm&func=authLogin&arg03_LoginType=_mode_off&arg04_LoginFrom=_wlm_login&language=..%2Fwlmeng%2Findex.htm&hiddRefreshDevice=..%2Fstartwlm%2FHme_DvcSts.htm&hiddRefreshPanelUsed=..%2Fstartwlm%2FHme_PnlUsg.htm&hiddRefreshPaperid=..%2Fstartwlm%2FHme_Paper.htm&hiddRefreshTonerid=..%2Fstartwlm%2FHme_StplPnch.htm&hiddRefreshStapleid=..%2Fstartwlm%2FHme_Toner.htm&hiddnBackNavIndx=1&hiddRefreshDeviceBack=&hiddRefreshPanelUsedBack=&hiddRefreshPaperidBack=&hiddRefreshToneridBack=&hiddRefreshStapleidBack=&hiddCompatibility=&hiddPasswordToOpenChk=&hiddPasswordToOpen=&hiddRePasswordToOpen=&hiddPasswordToEditChk=&hiddPasswordToEdit=&hiddRePasswordToEdit=&hiddPrinting=&hiddChanges=&hiddCopyingOfText=&hiddEmaiSID=&hiddEmaiName=&hiddECM=&hiddDocID=&privid=&publicid=&attrtype=&attrname=&hiddFaxType=&hiddSMBNumber1=&hiddSMBNumber2=&hiddSMBNumber3=&hiddSMBNumber4=&hiddSMBNumber5=&hiddSMBNumber6=&hiddSMBNumber7=&hiddFTPNumber1=&hiddFTPNumber2=&hiddFTPNumber3=&hiddFTPNumber4=&hiddFTPNumber5=&hiddFTPNumber6=&hiddFTPNumber7=&hiddFAXAddress1=&hiddFAXAddress2=&hiddFAXAddress3=&hiddFAXAddress4=&hiddFAXAddress5=&hiddFAXAddress6=&hiddFAXAddress7=&hiddFAXAddress8=&hiddFAXAddress9=&hiddFAXAddress10=&hiddIFaxAdd=&hiddIFaxConnMode=&hiddIFaxResolution=&hiddIFaxComplession=&hiddIFaxPaperSize=&hiddImage=&hiddTest=&hiddDoc_Num=&hiddCopy=&hiddDocument=&hiddDocRec=&AddressNumberPersonal=&AddressNumberGroup=&hiddPersonAddressID=&hiddGroupAddressID=&hiddPrnBasic=&hiddPageName=&hiddDwnLoadType=&hiddPrintType=&hiddSend1=&hiddSend2=&hiddSend3=&hiddSend4=&hiddSend5=&hiddAddrBokBackUrl=&hiddAddrBokName=&hiddAddrBokFname=&hiddSendFileName=&hiddenAddressbook=&hiddenAddressbook1=&hiddSendDoc_Num=&hiddSendColor=&hiddSendAddInfo=&hiddSendFileFormat=&hiddRefreshDevice=..%2Fstartwlm%2FHme_DvcSts.htm&hiddRefreshPanelUsed=..%2Fstartwlm%2FHme_PnlUsg.htm&hiddRefreshPaperid=..%2Fstartwlm%2FHme_Paper.htm&hiddRefreshTonerid=..%2Fstartwlm%2FHme_StplPnch.htm&hiddRefreshStapleid=..%2Fstartwlm%2FHme_Toner.htm&hiddnBackNavIndx=0&hiddRefreshDeviceBack=&hiddRefreshPanelUsedBack=&hiddRefreshPaperidBack=&hiddRefreshToneridBack=&hiddRefreshStapleidBack=&hiddValue=&arg01_UserName=%%%USERNAME%%%&arg02_Password=%%%PASSWORD%%%&arg03_LoginType=&arg05_AccountId=&Login=Login&arg06_DomainName=&hndHeight=0';
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'Set-Cookie: level=1';

    return ky_printer;
  }

  else if(model == "TASKalfa 250ci") {

    ky_printer['req_type']  = "POST";
    ky_printer['login_url'] = '/start/login.cgi';
    ky_printer['login_data'] = 'okhtmfile=%2Feng%2Findex.htm&failhtmfile=%2Fstart%2FStartAccessDenied.htm&func=authLogin&arg01_UserName=%%%USERNAME%%%&arg02_Password=%%%PASSWORD%%%&arg03_LoginType=&submit001=OK&arg01_UserName=&language=..%2Feng%2Findex.htm';
    ky_printer['http_status'] = 'HTTP/1.. 200';
    ky_printer['login_success'] = 'Set-Cookie: level=3';

    return ky_printer;
  }
}

function ky_error_exit(buf) {

  local_var buf;

  if(!buf || ereg(pattern:"HTTP/1\.[01] (404|500)", string: buf))
    exit(0);

  return TRUE;
}