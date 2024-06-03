###############################################################################
# Functions for ESXi
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2012 Greenbone Networks GmbH
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
###############################################################################

global_var sess, esxi_error, port, installed_bulletins, response, esxi_version;

esxi_error = NULL;
sess = NULL;
installed_bulletins = NULL;
port = NULL;
response = NULL;
esxi_version = NULL;

function start_esxi_session(user,pass) {

 local_var user, pass, c, recv;

  recv = _esxi_soap_request(data:'
                                <RetrieveServiceContent xmlns="urn:vim25">
                                  <_this type="ServiceInstance">ServiceInstance</_this>
                                </RetrieveServiceContent>',
                                search:"VMware ESX"); 

  if(!recv) { 
    _esx_error(data:'ESXi: Initial connect failed\n');
    return FALSE;
  } 

  sM = eregmatch(pattern:'<sessionManager type="SessionManager">(.*)</sessionManager>', string:recv);
  
  if(!isnull(sM[1])) { 
    SessionManager = sM[1];
  } else {
    SessionManager = "ha-sessionmgr";
  }  

  recv = _esxi_soap_request(data:'
                                <Login xmlns="urn:vim25">
                                  <_this type="SessionManager">' + SessionManager  + '</_this>
                                  <userName>' + user + '</userName>
                                  <password>' + pass  + '</password>
                                </Login>',
                                search:"<key>");

  c = eregmatch(pattern:'Set-Cookie: vmware_soap_session="([^"]+)"',string:recv);

  if(isnull(c[1])) {
    _esx_error(data:'ESXi: Could not extract session\n');
    return FALSE;
  }  

  sess = c[1];

  if(!recv) {
    _esx_error(data:'ESXi: Login failed\n');
    return FALSE;
  } 

  return TRUE;

}  

function get_esxi5_0_vibs(port,user,pass) {

  local_var user, pass, recv;

  if(esxi_version !~ "^(5|6)\.") {
    _esx_error(data:'ESXi 5.x: Wrong ESXi version. Expected: 5.x/6.x. Received: ' + esxi_version  + '\n');
    return FALSE;
  }

  if(!user || !pass) {
    _esx_error(data:'ESXi 5.x: Could not find user or password\n');
    return FALSE;
  }

  if(!start_esxi_session(user:user,pass:pass)) {
    return FALSE;
  }

  recv = _esxi_soap_request(data:'
                            <VimEsxCLIsoftwareviblist xmlns="urn:vim25">
                              <_this type="VimEsxCLIsoftwarevib">ha-cli-handler-software-vib</_this>
                            </VimEsxCLIsoftwareviblist>',
                            search:"<VimEsxCLIsoftwareviblistResponse");


  if(!recv) {
    _esx_error(data:'ESXi 5.x: Could not get installed bulletins/patches\n');
    return FALSE;
  }  

  if(resp = parse_esxi_5_0_response(recv:recv)) return TRUE;

  return FALSE;

}

function get_esxi4_x_vibs(port,user,pass) {

  local_var user, pass, c, t, task, recv, resp;

  if(esxi_version !~ "^4\.") {
    _esx_error(data:'ESXi 4.x: Wrong ESXi version. Expected: 4.x. Received: ' + esxi_version  + '\n');
    return FALSE;
  }


  if(!user || !pass) {
    _esx_error(data:'ESXi 4.x: Could not find user or password\n');
    return FALSE;
  }  

  if(!start_esxi_session(user:user,pass:pass)) {
    return FALSE;
  }

  recv = _esxi_soap_request(data:'
                                <RetrieveProperties xmlns="urn:vim25">
                                <_this type="PropertyCollector">ha-property-collector</_this>
                                <specSet>
                                  <propSet>
                                    <type>HostSystem</type>
                                    <all>0</all>
                                  </propSet>
                                  <objectSet>
                                    <obj type="Folder">ha-folder-root</obj>
                                    <skip>0</skip>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>folderTraversalSpec</name>
                                      <type>Folder</type>
                                      <path>childEntity</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>datacenterHostTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>datacenterVmTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>datacenterDatastoreTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>datacenterNetworkTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>computeResourceRpTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>computeResourceHostTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>hostVmTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>resourcePoolVmTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>datacenterDatastoreTraversalSpec</name>
                                      <type>Datacenter</type>
                                      <path>datastoreFolder</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>datacenterNetworkTraversalSpec</name>
                                      <type>Datacenter</type>
                                      <path>networkFolder</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>datacenterVmTraversalSpec</name>
                                      <type>Datacenter</type>
                                      <path>vmFolder</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>datacenterHostTraversalSpec</name>
                                      <type>Datacenter</type>
                                      <path>hostFolder</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>computeResourceHostTraversalSpec</name>
                                      <type>ComputeResource</type>
                                      <path>host</path>
                                      <skip>0</skip>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>computeResourceRpTraversalSpec</name>
                                      <type>ComputeResource</type>
                                      <path>resourcePool</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>resourcePoolTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>resourcePoolVmTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>resourcePoolTraversalSpec</name>
                                      <type>ResourcePool</type>
                                      <path>resourcePool</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>resourcePoolTraversalSpec</name>
                                    </selectSet>
                                    <selectSet>
                                      <name>resourcePoolVmTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>hostVmTraversalSpec</name>
                                      <type>HostSystem</type>
                                      <path>vm</path>
                                      <skip>0</skip>
                                    <selectSet>
                                      <name>folderTraversalSpec</name>
                                    </selectSet>
                                    </selectSet>
                                    <selectSet xsi:type="TraversalSpec">
                                      <name>resourcePoolVmTraversalSpec</name>
                                      <type>ResourcePool</type>
                                      <path>vm</path>
                                      <skip>0</skip>
                                    </selectSet>
                                </objectSet>
                            </specSet>
                        </RetrieveProperties>',
                        search:"HostSystem");

  if(!recv) {
    _esx_error(data:'ESXi 4.x: Preparing failed\n');
    return FALSE;
  }  

  hs = eregmatch(pattern:'<obj type="HostSystem">(.*)</obj>', string:recv);

  if(!isnull(hs[1])) {
    HostSystem = hs[1];
  } else {
    HostSystem = "ha-host";
  }  

  recv = _esxi_soap_request(data:'
                                <RetrieveProperties xmlns="urn:vim25">
                                <_this type="PropertyCollector">ha-property-collector</_this>
                                <specSet>
                                  <propSet>
                                    <type>HostSystem</type>
                                    <all>0</all>
                                    <pathSet>configManager.patchManager</pathSet>
                                  </propSet>
                                  <objectSet>
                                    <obj type="HostSystem">' + HostSystem + '</obj>
                                  </objectSet>
                                </specSet>
                                </RetrieveProperties>',
                                search:"PatchManager");

  if(!recv) {
    _esx_error(data:'ESXi 4.x: Initiating patch Manager failed\n');
    return FALSE;
  }  
 
  hpm = eregmatch(pattern:'<val type="HostPatchManager" xsi:type="ManagedObjectReference">(.*)</val>', string: recv);

  if(!isnull(hpm[1])) {
    HostPatchManager = hpm[1];
  }  else {
    _esx_error(data:'ESXi 4.x: Patch manager not found on this system.\n\n');
    return FALSE;
  }  

  recv = _esxi_soap_request(data:'
                                <QueryHostPatch_Task xmlns="urn:vim25">
                                  <_this type="HostPatchManager">' + HostPatchManager + '</_this>
                                </QueryHostPatch_Task>',
                                search:"Query-");

  if(!recv) {
    _esx_error(data:'ESXi 4.x: Getting Task ID failed\n');
    return FALSE;
  }  

  t = eregmatch(pattern:'<returnval type="Task">(.*Query-[0-9]+)</returnval>',string:recv);
  if(isnull(t[1])) {
    _esx_error(data:'ESXi 4.x: Could not extract Task ID\n');
    return FALSE;
  }  

  task = t[1];

  recv = get_installed_esxi_4_x_bulletins(task);

  if(!recv) {
    _esx_error(data:'ESXi 4.x: Could not get installed bulletins/patches.\n');
    return FALSE;
  }

  if(resp = parse_esxi_4_x_response(recv:recv)) return TRUE;

  return FALSE;

}

function get_installed_esxi_4_x_bulletins(task) {

  local_var i, max_retries, recv;

  max_retries = 15;
  i = 0;

  while (TRUE) {

    i++;

    if( i > max_retries )  {
      response = 'Failed to get the installed bulletins/patches after ' + max_retries + ' retries\nLast recv was: ' + recv + '\n';
      return FALSE;
    }

    recv = _esxi_soap_request(data:'
                <RetrieveProperties xmlns="urn:vim25">
                <_this type="PropertyCollector">ha-property-collector</_this>
                <specSet>
                  <propSet>
                    <type>Task</type>
                    <all>1</all>
                  </propSet>
                  <objectSet>
                    <obj type="Task">' +  task   +'</obj>
                  </objectSet>
                </specSet>
              </RetrieveProperties>');

    if(!recv) {
      return FALSE;
    }  

    recv = str_replace(string:recv,find:"&gt;",replace:">");
    recv = str_replace(string:recv,find:"&lt;",replace:"<");
    recv = str_replace(string:recv,find:"&quot;",replace:'"');

    if('<state>error</state>' >< recv) {
      return FALSE;
    }  

    if('<state>success</state>' >< recv) {

      if('<error' >< recv && '</error>' >< recv) { # even if we got a success on the _query_, the PatchManager could fail...

        ec = eregmatch(pattern:'<errorCode>([0-9]+)</errorCode>', string: recv);
        if(!isnull(ec[1])) {
          response = 'Code: ' + ec[1] + '\n';
        }

        ed = eregmatch(pattern:'<errorDesc>(.*)</errorDesc>', string:recv);
        if(!isnull(ed[1])) {
           response = response + 'Message: ' + ed[1] + '\n\n';
        }

        return FALSE;

      }  

      return recv;
    }

    sleep (3);

  }

  return FALSE;

}  

function parse_esxi_4_x_response(recv) {

  local_var last, date, datematch, bl, recv, bulletins, line;

  if(strlen(recv) < 1) {
    _esx_error(data:'ESXi 4.x: Unexpected response from ESXi server\n');
    return FALSE;
  } 

  bl = split(recv);

  recv = str_replace(string:recv, find:string("\n"),replace:"");
  recv = str_replace(string:recv, find:string("\r"),replace:"");
  recv = str_replace(string:recv, find:string("\t"),replace:"");
  recv = ereg_replace(string:recv, pattern:">[ ]+<", replace:"><");

  bulletins = eregmatch(pattern:"(<bulletin>.*</bulletin>)", string:recv);

  if(isnull(bulletins[1])) {
     set_kb_item(name:'VMware/ESXi/' + esxi_version  + '/unpatched', value:TRUE);
     log_message(data:'Could not found a single bulletin installed on this host. Assuming this\nis a completely unpatched system. \nRECV:\n' + recv + '\n');
     return TRUE;
  }   

  last = NULL;

  foreach line (bl) {

    if("<releaseDate>" >< line) {

      datematch = eregmatch(pattern:"(20[0-9][0-9]-[012][0-9]-[0-3][0-9])",string:line);
      date = datematch[1];

      if (!isnull(date) && (isnull(last) || date > last)) last = date;
    }

    if(ib = eregmatch(pattern:"<id>(ESXi?[0-9]+-.*)</id>", string: line)) {
      if(!isnull(ib[1])) {
        installed_bulletins += ib[1] + '\n';
      }  
    }  

  }

  if(installed_bulletins) {
    set_kb_item(name:'VMware/esxi/' + esxi_version  + '/bulletins', value:chomp(str_replace(string:installed_bulletins, find:'\n', replace:" ")));
  }

  if(!isnull(last)) {
    set_kb_item(name:'VMware/esxi/' + esxi_version  + '/last_bulletin', value:last);
    return TRUE;
  } else {
    _esx_error(data:'ESXi 4.x: Could not extract date of last bulletin\n');
    return FALSE;
  }  

}

function parse_esxi_5_0_response(recv) {

  local_var last, date, datematch, bl, recv, line, bulletins;

  if(strlen(recv) < 1) {
    _esx_error(data:'ESXi 5.x: Unexpected response from ESXi server\n');
    return FALSE;
  }

  bl = split(recv,sep:"><",keep:FALSE);

  bulletins = eregmatch(pattern:"(<returnval>.*</returnval>)", string:recv);

  recv = str_replace(string:recv, find:string("\n"),replace:"");
  recv = str_replace(string:recv, find:string("\r"),replace:"");
  recv = str_replace(string:recv, find:string("\t"),replace:"");
  recv = ereg_replace(string:recv, pattern:">[ ]+<", replace:"><");

   foreach line (bl) {

      if("releasedate" >< tolower(line) || "creationdate" >< tolower(line)) {

        datematch = eregmatch(pattern:"(20[0-9][0-9]-[012][0-9]-[0-3][0-9])",string:line);
        date = datematch[1];

        if (!isnull(date) && (isnull(last) || date > last)) last =  date;

      }

      if(ib = eregmatch(pattern:"ID>([^<]+)</ID", string: line)) {
        if(!isnull(ib[1])) {
          installed_bulletins += ib[1] + '\n';
        }  
      }  
   }

  if(installed_bulletins) {
    ibs = chomp(str_replace(string:installed_bulletins,find:'\n', replace:" "));
    if(last) {
      installed_bulletins += '\n\nLast ReleaseDate: ' + last + '\n';
    }  
    set_kb_item(name:"VMware/esxi/" + esxi_version  + "/bulletins", value:ibs);
  }

  if(!isnull(last)) {
    set_kb_item(name:'VMware/esxi/' + esxi_version  + '/last_bulletin', value:last);
    return TRUE;
  } else {
    _esx_error(data:'ESXi 5.x: Could not extract date of last bulletin\n');
    return FALSE;
  }


}  

function _esxi_build_soap(data) {

  soap_header = '<?xml version="1.0" encoding="UTF-8"?>
   <soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                     xmlns:xsd="http://www.w3.org/2001/XMLSchema"
                     xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
   <soapenv:Body>';

  soap_footer = '</soapenv:Body>
               </soapenv:Envelope>';

  
  return soap_header + data + soap_footer;

}

function _esxi_soap_request(data,search) {

  local_var soc, soap, len, req, buf, recv;

  response = NULL;

  soc = open_sock_tcp(port,transport:get_port_transport(port));
  if(!soc) {
    _esx_error(data:'ESXi: Could not create socket\n');
    return FALSE;
  }  

  soap = _esxi_build_soap(data:data);

  len = strlen(soap);
  host = get_host_name();

  req = string("POST /sdk/webService HTTP/1.1\r\n",
               "Connection: Close\r\n",
               "User-Agent: VI Perl\r\n",
               "Host: ",host,"\r\n",
               "Content-Length: ",len,"\r\n",
               'SOAPAction: "urn:vim25/5.0"',"\r\n");

  if(strlen(sess)) {
    req += string('Cookie: vmware_soap_session="',sess,'"',"\r\n");
  }    

  req += string("Content-Type: text/xml\r\n",
                 "\r\n");

  req += soap;
  req += string("\r\n");
 
  send(socket:soc,data:req);

  while(buf = recv(socket:soc,length:1024)) {
    recv += buf;
  }

  if(!recv)return FALSE;

  response = recv;

  if(recv !~ "HTTP/1.[0|1] 200") {

    if("<faultstring>" >< response) {

      fault = eregmatch(pattern:"<faultstring>(.*)</faultstring>", string: response);
      if(!isnull(fault[1])) {
        response = fault[1];
        detail = eregmatch(pattern:"<detail>(.*)</detail>", string: response);
        if(!isnull(detail[1])) {
          response = response + '\n\nDetail:\n' + detail[1] + '\n';
        }  
        return FALSE;
      }  

    }  

  return FALSE; 

  }

  close(soc);

  if(search) {
    if(search >!< recv)return FALSE;
  }  

  return recv;

}  

function _esx_error(data) {

  esxi_error = data + '\nResponse:\n ' + response + '\n';

}  

function _esxi_patch_missing(esxi_version, patch) {

  local_var esxi_version, patch, bulletins, last_bulletin, pd, pdate, pa, pa_version, ip, iv;

  if(!esxi_version || !patch) return FALSE;

  esxi_kb_version = get_kb_item("VMware/ESX/version");

  if(!esxi_kb_version) return FALSE;

  if(esxi_version != esxi_kb_version) {
    return FALSE;
  }  

  if(get_kb_item('VMware/ESXi/' + esxi_kb_version  + '/unpatched')) return TRUE;

  bulletins = get_kb_item('VMware/esxi/' + esxi_kb_version  + '/bulletins');

  if("VIB:" >< patch) {

    p = split(patch,sep:":",keep:FALSE);

    if(isnull(p[1]) || isnull(p[2])) return FALSE;

    pa = p[1];
    pa_version = p[2];

    bulletins = split(bulletins, sep:" ", keep:FALSE);

    foreach bulletin (bulletins) {
 
      if('_' + pa + '_' >< bulletin) {

        installed = split(bulletin, sep:"_", keep:FALSE);

        ip = installed[max_index(installed)-2];
        iv = installed[max_index(installed)-1];

        if(ip >!< pa) return FALSE;

        if(("vmw" >< pa_version && "vmw" >!< iv) || ("vmw" >< iv && "vmw" >!< pa_version)) return FALSE;

        if("vmw" >< pa_version && "vmw" >< iv) {
          pa_version = str_replace(string:pa_version, find:"vmw", replace:"");
          iv = str_replace(string:iv, find:"vmw", replace:"");
        }

        if(version_is_less(version:iv, test_version:pa_version)) {
          return TRUE;
        }  

        return FALSE;

      }  
    }  

  } else { 

    if(bulletins) {

      if(patch =~ "-Update[0-9]+:") {
        check_patch = split(patch, sep:":", keep:FALSE);
        if(isnull(check_patch[0])) return FALSE;
        check_patch = check_patch[0];
      } else {
        check_patch = patch;
      }  

      if(egrep(pattern:check_patch, string:bulletins)) {
        return FALSE;
      }
    } 

    last_bulletin = get_kb_item('VMware/esxi/' + esxi_kb_version  + '/last_bulletin');

    if(last_bulletin) {

      if(patch !~ "-Update[0-9]+") {
        pd = eregmatch(string: patch, pattern: "^ESXi?[0-9]+-(20[0-9][0-9])([0-9][0-9])[0-9]+-[A-Z]+$");

        if(isnull(pd[1]) || isnull(pd[2]))return FALSE;
        pdate = pd[1] + '-' + pd[2] + '-01';
      }

      if(patch =~ "-Update[0-9]+:") {
        patch_date = split(patch,sep:":", keep:FALSE);
        if(isnull(patch_date[1])) return FALSE;
        pdate = patch_date[1];
      }  

      if (pdate <= last_bulletin) return FALSE;

    }  
  }  

  return TRUE;

} 

function esxi_remote_report(ver, build, fixed_build, typ) {

  if(isnull(ver) || isnull(build) || isnull(fixed_build))return;

  space = ' ';
  if(!typ)
  {
    typ = 'ESXi';
    space = '    ';
  }  

  report = typ + ' Version:' + space  + ver + '\n' +
           'Detected Build:  ' + build + '\n' +
           'Fixed Build:     ' + fixed_build + '\n';

  return report;

}

