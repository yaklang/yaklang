###############################################################################
# OpenVAS Vulnerability Test
# $Id$
# Description: Provides basic tftp functionality
#
# Authors:
# Martin O'Neal of Corsaire (http://www.corsaire.com)
#
# Copyright:
# Copyright (C) 2005 Corsaire Limited
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
# $Revision$
#
###############################################################################

# DISCLAIMER
# The information contained within this script is supplied "as-is" with 
# no warranties or guarantees of fitness of use or otherwise. Corsaire 
# accepts no responsibility for any damage caused by the use or misuse of 
# this information.

############## declarations ################

# declare function
function tftp_get(port,path)
{

        if( ! port ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#tftp_get" );
        if( ! path ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#path#-#tftp_get" );

	# initialise variables
	local_var request_data;
	local_var request_ip;
	local_var request_udp;
	local_var response_udp;
	local_var response_data;
	local_var filter;
	local_var source_port;
	local_var destination_port;
	source_port=23793+int((rand()%6157));
	destination_port=port;
		
	# create request
	request_data=raw_string(0x00,0x01)+path+raw_string(0x00)+'octet'+raw_string(0x00);
	request_ip=forge_ip_packet(ip_hl:5,ip_v:4,ip_tos:0,ip_len:20,ip_id:rand(),ip_off:0,ip_ttl:64,ip_p:IPPROTO_UDP,ip_src:this_host());
	request_udp=forge_udp_packet(ip:request_ip,uh_sport:source_port,uh_dport:destination_port,uh_ulen:8+strlen(request_data),data:request_data);

	# create filter
	filter='udp and dst port '+source_port+' and src host '+get_host_ip();

	# send packet
	response_udp=send_packet(request_udp,pcap_active:TRUE,pcap_filter:filter);	

	# check if response received
	if(response_udp)
	{
		# retrieve udp data
		response_data=get_udp_element(udp:response_udp,element:"data");
		
		# check if udp data is valid positive tftp data response
		if(ord(response_data[0])==0x00 && ord(response_data[1])==0x03 && ord(response_data[2])==0x00 && ord(response_data[3])==0x01)
		{
			# remove tftp protocol header from data
			response_data=substr(response_data,4);
			
			return(response_data);
		}
	}
	
	return NULL;
}



# declare function
function tftp_put(port,path)
{

        if( ! port ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#tftp_put" );
        if( ! path ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#path#-#tftp_put" );

	# initialise variables
	local_var request_data;
	local_var request_ip;
	local_var request_udp;
	local_var response_udp;
	local_var response_data;
	local_var filter;
	local_var source_port;
	local_var destination_port;
	source_port=32288+int((rand()%7354));
	destination_port=port;
		
	# create request
	request_data=raw_string(0x00,0x02)+path+raw_string(0x00)+'octet'+raw_string(0x00);
	request_ip=forge_ip_packet(ip_hl:5,ip_v:4,ip_tos:0,ip_len:20,ip_id:rand(),ip_off:0,ip_ttl:64,ip_p:IPPROTO_UDP,ip_src:this_host());
	request_udp=forge_udp_packet(ip:request_ip,uh_sport:source_port,uh_dport:destination_port,uh_ulen:8+strlen(request_data),data:request_data);

	# create filter
	filter='udp and dst port '+source_port+' and src host '+get_host_ip();

	# send packet
	response_udp=send_packet(request_udp,pcap_active:TRUE,pcap_filter:filter);	

	# check if response received
	if(response_udp)
	{
		# retrieve udp data
		response_data=get_udp_element(udp:response_udp,element:"data");
		
		# check if udp data is valid positive tftp data response
		if(ord(response_data[0])==0x00 && ord(response_data[1])==0x04 && ord(response_data[2])==0x00 && ord(response_data[3])==0x00)
		{
			return(TRUE);
		}
	}
	
	return NULL;
}

function tftp_alive(port)
{

 if( ! port ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#tftp_alive" );

 local_var  req, rep, sport, ip, u, filter, data, i;

 req = '\x00\x01OpenVAS'+rand()+'\0netascii\0';

 sport = rand() % 64512 + 1024;
 ip = forge_ip_packet(ip_hl : 5, ip_v: 4,  ip_tos:0,
        ip_len:20, ip_off:0, ip_ttl:64, ip_p:IPPROTO_UDP,
        ip_src: this_host());

 u = forge_udp_packet(ip:ip, uh_sport: sport, uh_dport:port, uh_ulen: 8 + strlen(req), data:req);

 filter = 'udp and dst port ' + sport + ' and src host ' + get_host_ip() + ' and udp[8:1]=0x00';

 data = NULL;
 for (i = 0; i < 2; i ++)       # Try twice
 {
  rep = send_packet(u, pcap_active:TRUE, pcap_filter:filter);
  if(rep)
  {
   data = get_udp_element(udp: rep, element:"data");
   if (data[0] == '\0' && (data[1] == '\x03' || data[1] == '\x05'))
   {
    return TRUE;
   }
  }
 }
}
