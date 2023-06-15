# OpenVAS Vulnerability Test
# $Id$
# Description: Support methods to handle NFS
#
# Authors:
# Renaud Deraison
#
# Copyright:
# Copyright (C) 2003 Renaud Deraison
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

# mount(soc, share)	- Mounts an NFS share and returns a file handle
#			  (soc is a socket opened to the mount daemon)
#
# readdir(soc, fid)	- Returns the content of the directory pointed by
#			  <fid>. (soc is a socket opened to the NFS daemon)
#
# cwd(soc, fid, dir)	- Changes the working directory to <dir>. Returns
#			  a new fid
#
# umount(soc, share)	- Tells the remote NFS server we don't need its services
#		          any more (soc is a socket opened to the mount daemon)

function padsz(len)
{
 if(len % 4)
  return 4 - len % 4;
 else
  return 0;
}

function rpclong(val)
{
 local_var ret;
 
 ret = raw_string(val / (256*256*256),
 		  val / (256*256),
		  val /  256,
		  val % 256);
 return ret;		  
}


function str2long(val, idx)
{
 local_var ret;
 ret = ord(val[idx]) * 256 * 256 * 256 + 
       ord(val[idx+1]) * 256 * 256 +
       ord(val[idx+2]) * 256 +
       ord(val[idx+3]);
       
  return int(ret);
}



function rpcpad(pad)
{
 return crap(length:pad, data:raw_string(0));
}




function mount(soc, share)
{
  local_var pad, req, len, r, ret, i;
  
  pad = padsz(len:strlen(this_host_name()));
  len = 52 + strlen(this_host_name()) + pad;
  
  req = 	   rpclong(val:rand()) +
  		   rpclong(val:0) +
		   rpclong(val:2) +
		   rpclong(val:100005) +
		   rpclong(val:1) +
		   rpclong(val:1) +
		   rpclong(val:1) +
		   rpclong(val:len) +
		   rpclong(val:rand()) +
		   rpclong(val:strlen(this_host_name())) +
		   this_host_name() +
		   rpcpad(pad:pad) +
		   rpclong(val:0)  +	
		   rpclong(val:0)  +	
		   rpclong(val:7)  +	
		   rpclong(val:0)  +	
		   rpclong(val:2)  + 	
		   rpclong(val:3)  +	
		   rpclong(val:4)  +
		   rpclong(val:5)  +
		   rpclong(val:20) +
		   rpclong(val:31) +
		   rpclong(val:0)  +	
		   rpclong(val:0)  +
		   rpclong(val:0)  +
					
		   rpclong(val:strlen(share)) +
		   share +
		   rpcpad(pad:padsz(len:strlen(share)));
		   
  send(socket:soc, data:req);
  r = recv(socket:soc, length:4096);
  if(strlen(r) < 24)
   return NULL;
  else
   {
    if(str2long(val:r, idx:24) != 0)
      return NULL;
      
    ret = "";
    for(i=28;i<60;i++)ret += r[i];
    return ret;
   }
}

function readdir(soc, fid)
{
 local_var req, r, i, dir, ret;
 
 req = 		rpclong(val:rand()) +
 		rpclong(val:0) 	    +
		rpclong(val:2)      +
		rpclong(val:100003) +
		rpclong(val:2)      +
		rpclong(val:16)     +
		rpclong(val:1)      +
		rpclong(val:48)     +
		rpclong(val:0)          +
		rpclong(val:0)          +
		rpclong(val:0)          +
		rpclong(val:0)	    +
		rpclong(val:7)	    +
		rpclong(val:0)	    +
		rpclong(val:2)	    +
		rpclong(val:3)	    +
		rpclong(val:4)	    +
		rpclong(val:5)          +
		rpclong(val:20)         +
		rpclong(val:31)	    +
		
		rpclong(val:0)	    +
		rpclong(val:0)	    +
		fid		    +
		rpclong(val:0)	    +
		rpclong(val:8192);
		
 send(socket:soc, data:req);
 r = recv(socket:soc, length:8192);	
 if(strlen(r) <= 24) return NULL;
 if(str2long(val:r, idx:24) != 0) return NULL; # Could not read dir
 
 i = 28;
 ret = make_list();
 while(str2long(val:r, idx:i) == 1)
 {
  if ( i > strlen(r)) break;
  i += 4;
  i += 4; # File ID - don't care
  len = str2long(val:r, idx:i);
  i+=4;
  dir = substr(r, i, i + len - 1);
  i += len;
  i += padsz(len:len);
  i += 4;
  ret = make_list(ret, dir);
 }
 return ret;	
}


function cwd(soc, dir, fid)
{
 local_var req, ret, i;
 
 
 req = rpclong(val:rand()) +
       rpclong(val:0)	  +
       rpclong(val:2)	  +
       rpclong(val:100003) +
       rpclong(val:2)	  +
       rpclong(val:4)	  +
       rpclong(val:1)	  +
       rpclong(val:48)	  +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:7)	   +
       rpclong(val:0)	   +
       rpclong(val:2)	   +
       rpclong(val:3)	   +
       rpclong(val:4)	   +
       rpclong(val:5)	   +
       rpclong(val:20)	   +
       rpclong(val:31)	   +
       rpclong(val:0)      +
       rpclong(val:0)      +
       fid +
       rpclong(val:strlen(dir)) +
       dir +
       rpcpad(pad:padsz(len:strlen(dir)));
       
   send(socket:soc, data:req);
   r = recv(socket:soc, length:8192);
   if(strlen(r) < 24)
    return NULL;
   
  if(strlen(r) < 24)
    return NULL;
   else
   {
    if(str2long(val:r, idx:24) != 0)
      return NULL;
      
    ret = "";
    for(i=28;i<56;i++)ret += r[i];
    ret += rpclong(val:0);
    return ret;
   }
}






function open(soc, file, fid)
{
 local_var req, ret, i;
 
 
 req = rpclong(val:rand()) +
       rpclong(val:0)	  +
       rpclong(val:2)	  +
       rpclong(val:100003) +
       rpclong(val:2)	  +
       rpclong(val:4)	  +
       rpclong(val:1)	  +
       rpclong(val:48)	  +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:7)	   +
       rpclong(val:0)	   +
       rpclong(val:2)	   +
       rpclong(val:3)	   +
       rpclong(val:3)	   +
       rpclong(val:5)	   +
       rpclong(val:20)	   +
       rpclong(val:31)	   +
       rpclong(val:0)      +
       rpclong(val:0)      +
       fid +
       rpclong(val:strlen(file)) +
       file +
       rpcpad(pad:padsz(len:strlen(file)));
       
   send(socket:soc, data:req);
   r = recv(socket:soc, length:8192);
   if(strlen(r) < 24)
    return NULL;
   
  if(strlen(r) < 24)
    return NULL;
   else
   {
    if(str2long(val:r, idx:24) != 0)
      return NULL;
      
    ret = "";
    for(i=28;i<56;i++)ret += r[i];
    ret += rpclong(val:0);
    return ret;
   }
}

function read(soc, fid, length, off)
{
 local_var req, ret, i, len;
 
 
 req = rpclong(val:rand()) +
       rpclong(val:0)	  +
       rpclong(val:2)	  +
       rpclong(val:100003) +
       rpclong(val:2)	  +
       rpclong(val:6)	  +
       rpclong(val:1)	  +
       rpclong(val:48)	  +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:0)	   +
       rpclong(val:7)	   +
       rpclong(val:0)	   +
       rpclong(val:2)	   +
       rpclong(val:3)	   +
       rpclong(val:4)	   +
       rpclong(val:5)	   +
       rpclong(val:20)	   +
       rpclong(val:31)	   +
       rpclong(val:0)      +
       rpclong(val:0)      +
       fid +
       rpclong(val:off) +
       rpclong(val:length) + 
       rpclong(val:0);
       
   send(socket:soc, data:req);
   r = recv(socket:soc, length:length + 33);
   if(strlen(r) <= 32)
    return NULL;
   
   return substr(r, 32, strlen(r) - 1);
}


function umount(soc, share)
{
 local_var pad, req, len, r, ret, i;
  
  pad = padsz(len:strlen(this_host_name()));
  len = 52 + strlen(this_host_name()) + pad;
 
 req =  rpclong(val:rand()) +
  		   rpclong(val:0) +
		   rpclong(val:2) +
		   rpclong(val:100005) +
		   rpclong(val:1) +
		   rpclong(val:3) +
		   rpclong(val:1) +
		   rpclong(val:len) +
		   rpclong(val:rand()) +
		   rpclong(val:strlen(this_host_name())) +
		   this_host_name() +
		   rpcpad(pad:pad) +
		   rpclong(val:0)  +	
		   rpclong(val:0)  +	
		   rpclong(val:7)  +	
		   rpclong(val:0)  +	
		   rpclong(val:2)  + 	
		   rpclong(val:3)  +	
		   rpclong(val:4)  +
		   rpclong(val:5)  +
		   rpclong(val:20) +
		   rpclong(val:31) +
		   rpclong(val:0)  +	
		   rpclong(val:0)  +
		   rpclong(val:0)  +
					
		   rpclong(val:strlen(share)) +
		   share +
		   rpcpad(pad:padsz(len:strlen(share)));
		   
  send(socket:soc, data:req);
  r = recv(socket:soc, length:8192);		   
}

