# OpenVAS Vulnerability Test
# $Id: uddi.inc 10964 2011-05-18 20:50:52Z jan $
# Description: Method for handling uddi
#
# Authors:
# John Lampe
#
# Copyright:
# Copyright (C) 2002 John Lampe
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2,
# as published by the Free Software Foundation
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#

function create_uddi_xml (ktype,path,key,name) 
{
    envelope_header = string("<?xml version='1.0' encoding='UTF-8'?> <s:Envelope");
    envelope_header = string(envelope_header, " xmlns:s='http://schemas.xmlsoap.org/soap/envelope/'><s:Body>");

    get_servicedetail = string("<get_serviceDetail generic='1.0' xmlns='urn:uddi-org:api'>"); 
    get_servicedetail = string(get_servicedetail, "<serviceKey>", key, "</serviceKey></get_serviceDetail>");

    find_business = string("<find_business generic='1.0' xmlns='urn:uddi-org:api'>");
    find_business = string(find_business, "<name>",name,"</name></find_business>");

    find_service = string("<find_service generic='1.0' xmlns='urn:uddi-org:api' businessKey='",key, "'>");
    find_service = string(find_service, "<name>", name, "</name></find_service>");

    close_envelope = string("</s:Body></s:Envelope>");

    if (ktype == "UDDI_QUERY_FBUSINESS")       method = find_business;
    if (ktype == "UDDI_QUERY_FSERVICE")        method = find_service;
    if (ktype == "UDDI_QUERY_GSERVICE_DETAIL") method = get_servicedetail;

    xml = string(envelope_header, method, close_envelope);
    len = strlen(xml);

    finished_message =  string("POST ", path,  " HTTP/1.0\r\n");
    finished_message =  string(finished_message, "Accept: text/xml\r\n");
    finished_message =  string(finished_message, "Accept: multipart/*\r\n");
    finished_message =  string(finished_message, "Host: ", get_host_ip(), "\r\n");
    finished_message =  string(finished_message, "User-Agent: OPENVAS::SOAP\r\n");
    finished_message =  string(finished_message, "Content-Length: ", len, "\r\n");
    finished_message =  string(finished_message, "Content-Type: text/xml; charset=utf-8\r\n"); 
    finished_message =  string(finished_message, "SOAPAction: ''\r\n\r\n", xml);
    return(finished_message);
}


