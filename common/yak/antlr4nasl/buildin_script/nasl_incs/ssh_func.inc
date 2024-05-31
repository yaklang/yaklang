###############################################################################
# OpenVAS Include File
#
# Methods for handling SSH
#
# Authors:
# Nicolas Pouvesle
#
# Copyright:
# Copyright (C) Nicolas Pouvesle
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
###############################################################################

# To enable the new libssh based code, you need to set the environment
# variable OPENVAS_USE_LIBSSH to any value.  This is a temporary
# requirement until we can retire the NASL based code.

global_var session_id, enc_keys, seqn_w, seqn_r;
global_var local_channel, remote_channel;
global_var l_window_size, received_size;
global_var r_window_size, r_packet_size;
global_var dh_pub, dh_priv;
global_var _ssh_banner;
global_var _ssh_server_version;
global_var _ssh_supported_authentication;
global_var _ssh_cmd_error;
global_var _ssh_error;
global_var _last_libssh_sess;
global_var bugged_sshd, bugged_first, bugged_channels, bugged_rws, bugged_rps;
global_var server_host_key_blob;
global_var ssh_host_key_algos;

ssh_host_key_algos = make_list( "ssh-rsa",
                                "ssh-dss",
                                "ecdsa-sha2-nistp256",
                                "ecdsa-sha2-nistp384",
                                "ecdsa-sha2-nistp521", # libssh >= 0.6.4 is needed for ecdsa-sha2-nistp384 and ecdsa-sha2-nistp521
                                "ssh-ed25519" ); # libssh >= 0.7.0 for ed25519

                                #"rsa-sha2-256", # Those two seems to be currently unsupported in libssh
                                #"rsa-sha2-512" );

function kb_ssh_login() {
  return string( get_kb_item( "Secret/SSH/login" ) );
}

function kb_ssh_password() {
  return string( get_kb_item( "Secret/SSH/password" ) );
}

function kb_ssh_privatekey() {
  return string( get_kb_item( "Secret/SSH/privatekey" ) );
}

function kb_ssh_publickey() {
  return string( get_kb_item( "Secret/SSH/publickey" ) );
}

function kb_ssh_passphrase() {
  return string( get_kb_item( "Secret/SSH/passphrase" ) );
}

function kb_ssh_transport() {

  local_var r;
  r = get_preference( "auth_port_ssh" );
  if ( r )
    return int( r );

  r = get_kb_item( "Services/ssh" );

  if ( r )
    return int( r );
  else
    return 22;
}

#-----------------------------------------------------------------#
# Get the SSH Host Key. See ssh_kex2().                           #
#-----------------------------------------------------------------#
function get_server_host_key( sess_id ) {

  if( isnull( sess_id ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sess_id#-#get_server_host_key" );

  if( defined_func( "ssh_get_host_key" ) ) {
    if( sess_id && int( sess_id ) > 0 )
      _last_libssh_sess = sess_id;

    return ssh_get_host_key( _last_libssh_sess );
  } else {
    return server_host_key_blob;
  }
}

#-----------------------------------------------------------------#
# Set SSH debugging error msg                                     #
#-----------------------------------------------------------------#
function set_ssh_error( msg ) {

  local_var msg;

  _ssh_error = msg;
}

#-----------------------------------------------------------------#
# Get SSH debugging error msg                                     #
#-----------------------------------------------------------------#
function get_ssh_error() {
  return _ssh_error;
}

#-----------------------------------------------------------------#
# Get SSH supported authentication methods
#-----------------------------------------------------------------#
function get_ssh_supported_authentication( sess_id ) {

  if( isnull( sess_id ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sess_id#-#get_ssh_supported_authentication" );

  if( defined_func( "ssh_get_auth_methods" ) ) {

    if( sess_id && int( sess_id ) > 0 )
      _last_libssh_sess = sess_id;

    return ssh_get_auth_methods( _last_libssh_sess );
  }
  return _ssh_supported_authentication;
}

#-----------------------------------------------------------------#
# Get SSH server's version                                        #
#-----------------------------------------------------------------#
function get_ssh_server_version() {
  return _ssh_server_version;
}

#-----------------------------------------------------------------#
# Get SSH banner                                                  #
#                                                                 #
# Please use ssh_get_session_banner directly for new code.        #
# The saved session id is just a bad hack.                        #
#-----------------------------------------------------------------#
function get_ssh_banner( sess_id ) {

  if( isnull( sess_id ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sess_id#-#get_ssh_banner" );

  if( sess_id && int( sess_id ) > 0 )
    _last_libssh_sess = sess_id;

  return ssh_get_issue_banner( _last_libssh_sess );
}

#-----------------------------------------------------------------#
# Convert network long (buffer) to long                           #
#-----------------------------------------------------------------#
function ntol(buffer,begin)
{
 local_var len;

 len = 16777216*ord(buffer[begin]) +
       ord(buffer[begin+1])*65536 +
       ord(buffer[begin+2])*256 +
       ord(buffer[begin+3]);

 return len;
}

#-----------------------------------------------------------------#
# Convert int to network long (raw_string)                        #
#-----------------------------------------------------------------#
function raw_int32(i)
{
 local_var buf;

 buf = raw_string (
        (i>>24) & 255,
        (i>>16) & 255,
        (i>>8) & 255,
        (i) & 255
                  );
 return buf;
}

#-----------------------------------------------------------------#
# Convert char to network char (raw_string)                       #
#-----------------------------------------------------------------#
function raw_int8(i)
{
 local_var buf;

 buf = raw_string (
        (i) & 255
                  );
 return buf;
}


#-----------------------------------------------------------------#
# Init packet sequence number and channel number                  #
#-----------------------------------------------------------------#
function init()
{
 # sequence packet = 0
 seqn_w = seqn_r = 0;
 local_channel = 0;
 _ssh_banner = "";
 _ssh_server_version = "";
 _ssh_supported_authentication = "";
 _ssh_cmd_error = "";
 _ssh_error = "";
 _last_libssh_sess = 0;
 bugged_sshd = 0;
 bugged_first = 1;
}


#-----------------------------------------------------------------#
# Decode base64 string - ported from public domain code           #
#-----------------------------------------------------------------#
function base64decode(str)
{
 local_var len, i, j, k, ret, base64, b64;
 len = strlen(str);
 ret = "";

 base64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

 for (i = 0; i < 256; i++)
   b64[i] = 0;
 for (i = 0; i < strlen(base64); i++)
   b64[ord(base64[i])] = i;

 for(j=0;j<len;j+=4)
 {
   for (i = 0; i < 4; i++)
   {
    c = ord(str[j+i]);
    a[i] = c;
    b[i] = b64[c];
   }

   o[0] = (b[0] << 2) | (b[1] >> 4);
   o[1] = (b[1] << 4) | (b[2] >> 2);
   o[2] = (b[2] << 6) | b[3];
   if (a[2] == ord('='))
     i = 1;
   else if (a[3] == ord('='))
     i = 2;
   else
     i = 3;
   for(k=0;k<i;k++)
      ret += raw_int8(i:o[k]);

   if (i < 3)
     break;
 }

 return ret;
}


#-----------------------------------------------------------------#
# Reads a SSH packet (comes from smb_nt.inc)                      #
#-----------------------------------------------------------------#
function ssh_recv(socket, length)
{
  local_var header, len, trailer, cmpt, payload, ret;

  header = recv(socket:socket, length:4, min:4);
  if (strlen(header) < 4)return(NULL);
  len = ntol (buffer:header, begin:0);
  if ((len == 0) || (len > 32768)) return(header);
  trailer = recv(socket:socket, length:len, min:len);
  if(strlen(trailer) < len )return(NULL);

  seqn_r++;

  # SSH servers can send IGNORE (code 2) or BANNER (code 53) msg
  ret = ord(trailer[1]);
  if ((ret == 2) || (ret == 53))
  {
    if (ret == 53)
      _ssh_banner += getstring (buffer:trailer, pos:2);

    return ssh_recv(socket:socket, length:length);
  }

  return strcat(header, trailer);
}


#-----------------------------------------------------------------#
# Detect if remote ssh server is known to be bugged (SunSSH1.0)   #
#-----------------------------------------------------------------#
function is_sshd_bugged(banner)
{
 if (ereg(string:banner, pattern:"^SSH-2\.0-Sun_SSH_1\.0"))
   return 1;

 return 0;
}


#-----------------------------------------------------------------#
# Waits for the server identification string, and sends our own   #
# identification string.                                          #
#-----------------------------------------------------------------#
function ssh_exchange_identification(socket)
{
 local_var buf, sshversion, num, prot;

 buf = recv_line(socket:socket, length:1024);

 if (!buf)
 {
   set_ssh_error(msg: "Scanner did not receive server's version");
   return 0;
 }

 # server ident : SSH-%d.%d-servername #
 if (!ereg(string:buf, pattern:"^SSH-*[0-9]\.*[0-9]-*[^\n]"))
 {
   set_ssh_error(msg: "Remote service is not a valid SSH service");
   return 0;
 }

 sshversion = split(buf, sep:"-", keep:0);
 num = split(sshversion[1], sep:".", keep:0);

 # version supported = 2 & 1.99
 if ((num[0] != 2) && !((num[0] == 1) && (num[1] == 99)))
 {
   set_ssh_error(msg: "Scanner only supports SSHv2");
   return 0;
 }

 # We use 2.0 protocol
 prot = "SSH-2.0-OpenVAS"+raw_string(0x0a);
 send(socket:socket, data:prot);

 if ( '\r\n' >< buf ) buf = buf - '\r\n';
   else buf = buf - '\n';

 if (is_sshd_bugged(banner:buf))
 {
   bugged_sshd = 1;
 }

 # all is correct
 return buf;
}


#-----------------------------------------------------------------#
# check pattern in buffer                                         #
# return next len in buffer or -1                                 #
#-----------------------------------------------------------------#
function check_pattern(buffer,pattern,length)
{
 local_var alglen, len, alg;

 alglen = ntol (buffer:buffer, begin:length);
 len = length+4+alglen;
 alg = substr(buffer,length+4,len-1);
 if (!ereg(string:alg, pattern:pattern))
  return -1;

 return len;
}

#-----------------------------------------------------------------#
# Create key exchange packet                                      #
#-----------------------------------------------------------------#
function kex_packet(payload,code)
{
 local_var len, padding_len, full_len, kex;

 len =
   # padding length
     1 +
     # msg code
     1 +
     # payload length
     strlen(payload);

 #padding (mod 8) = 8 - ( (len+packet_len(4) ) % 8 )
 padding_len = 8 - ((len + 4) % 8);

 # if padding len is less than 4 add block size
 if (padding_len < 4)
   padding_len += 8;

 full_len = len + padding_len;

 kex =
    # packet length
    raw_int32 (i:full_len) +
    # padding length
    raw_int8 (i:padding_len) +
    #msg code (32 = Diffie-Hellman GEX Init)
    code +
    # Payload (Pub key)
    payload +
    # Padding
    crap(data:raw_string(0),length:padding_len);

 return kex;
}


#-----------------------------------------------------------------#
# mac compute                                                     #
#-----------------------------------------------------------------#
function mac_compute(data, type)
{
 local_var to_hash;

 # we only support sha1! enc_keys[5] == mac_out key
 if (!type)
 {
  to_hash = raw_int32(i:seqn_w) + data;
  hash = HMAC_SHA1(data:to_hash, key:enc_keys[4]);
 }
 else
 {
  to_hash = raw_int32(i:seqn_r) + data;
  hash = HMAC_SHA1(data:to_hash, key:enc_keys[5]);
 }

 return hash;
}


#-----------------------------------------------------------------#
# crypt data                                                      #
#-----------------------------------------------------------------#
function crypt(data)
{
 local_var crypted;

 crypted = bf_cbc_encrypt(data:data, key:enc_keys[2], iv:enc_keys[0]);

 enc_keys[0] = crypted[1];

 return crypted[0];
}

#-----------------------------------------------------------------#
# decrypt data                                                    #
#-----------------------------------------------------------------#
function decrypt(data)
{
 local_var decrypted;

 decrypted = bf_cbc_decrypt(data:data, key:enc_keys[3], iv:enc_keys[1]);

 enc_keys[1] = decrypted[1];
 return decrypted[0];
}

#-----------------------------------------------------------------#
# Send ssh packet                                                 #
#-----------------------------------------------------------------#
function send_ssh_packet(socket,payload,code)
{
 local_var i, len, padding_len, full_len, buf, res, macbuf, crypted;

 len =
   # padding length
     1 +
     # msg code
     1 +
     # payload length
     strlen(payload);

 #padding (mod 8) = 8 - ( (len+packet_len(4) ) % 8 )
 padding_len = 8 - ((len + 4) % 8);

 # if padding len is less than 4 add block size
 if (padding_len < 4)
   padding_len += 8;

 full_len = len + padding_len;

 padding = "";
 for (i=0;i<padding_len;i++)
    padding = padding + raw_int8(i:(rand() % 256));


 buf =
    # packet length
    raw_int32 (i:full_len) +
    # padding length
    raw_int8 (i:padding_len) +
    #msg code (32 = Diffie-Hellman GEX Init)
    code +
    # Payload (Pub key)
    payload +
    # Padding
    padding;

 macbuf = mac_compute(data:buf, type:0);

 crypted = crypt(data:buf);

 buf = crypted + macbuf;

 send(socket:socket, data:buf);

 seqn_w++;
}


#-----------------------------------------------------------------#
# Receive ssh packet                                              #
#-----------------------------------------------------------------#
function recv_ssh_packet(socket, timeout)
{
 local_var len, need, padding_len, full_len, buf, res, macbuf, decrypted;
 local_var hmac, hmacbuf, mac, payload, ret;


 payload = raw_int8(i:0);

 # blockbytes = 8 for blowfish-cbc
 buf = recv(socket:socket, length:8, min:8, timeout:timeout);
 if (strlen(buf) != 8)
   return payload;

 decrypted = decrypt(data:buf);

 len = ntol(buffer:decrypted, begin:0);
 # Maximum packet size is 32768 bytes
 if (len > 32768)
   return payload;
 # 8 = blocksize ... i know it is not generic and it will be hard to change all
 need = 4 + len - 8;
 buf = recv(socket:socket, length:need, min:need, timeout:timeout);
 if (strlen(buf) != need)
   return payload;

 decrypted = decrypted + decrypt(data:buf);

 # hmac-sha1 length = 20 ... same comment as before
 mac = recv(socket:socket, length:20, min:20, timeout:timeout);
 if (strlen(mac) != 20)
   return payload;

 macbuf = mac_compute(data:decrypted, type:1);

 hmac = hexstr(mac);
 hmacbuf = hexstr(macbuf);
 if (hmac >!< hmacbuf)
   return payload;

 payload = substr(decrypted, 5, strlen(decrypted)-ord(decrypted[4])-1);
 seqn_r++;

 # SSH servers can send IGNORE (code 2) or BANNER (code 53) msg
 ret = ord(payload[0]);
 if ((ret == 2) || (ret == 53) || ret == 4)
 {
   if (ret == 53)
     _ssh_banner += getstring (buffer:payload, pos:1);

   return recv_ssh_packet(socket:socket, timeout:timeout);
 }

 return payload;
}


#-----------------------------------------------------------------#
# Get payload from packet                                         #
#-----------------------------------------------------------------#
function packet_payload(packet,code)
{
 local_var packetlen, paddinglen, msgcode;

 packetlen = ntol(buffer:packet, begin:0);
 paddinglen = ord(packet[4]);
 msgcode = ord(packet[5]);

 # Diffie-Hellman Key Exchange Reply
 if (msgcode != code)
  return 0;

 payload = substr(packet,6,packetlen-1);
 return payload;
}

#-----------------------------------------------------------------#
# Get string (length,string)                                      #
#-----------------------------------------------------------------#
function getstring(buffer,pos)
{
 local_var buf_len, buf;

 buf_len = ntol (buffer:buffer,begin:pos);
 buf = substr(buffer,pos+4,pos+4+buf_len-1);

 return buf;
}

#-----------------------------------------------------------------#
# Put string (string)                                             #
#-----------------------------------------------------------------#
function putstring(buffer)
{
 local_var buf;

 buf = raw_int32(i:strlen(buffer)) + buffer;

 return buf;
}

#-----------------------------------------------------------------#
# Put bignum (string)                                             #
#-----------------------------------------------------------------#
function putbignum(buffer)
{
 local_var len, buf;

 if (ord(buffer[0]) & 0x80)
 {
   len = strlen(buffer)+1;
   buf = raw_int32(i:len) + raw_string(0x00) + buffer;
 }
 else
   buf = raw_int32(i:strlen(buffer)) + buffer;

 return buf;
}

#-----------------------------------------------------------------#
# RSA verify signature                                            #
#-----------------------------------------------------------------#
function ssh_rsa_verify(e, n, signature, data)
{
 local_var hash, id_sha1, sigtype, nlen,
           next, tmp_sig, siglen, len, sig,
           hdecoid, hshaoid, hhash, decrypted ;

#comes directly from OpenBSD
 id_sha1 = raw_string(
        0x30, 0x21,
        0x30, 0x09,
        0x06, 0x05,
        0x2b, 0x0e, 0x03, 0x02, 0x1a,
        0x05, 0x00,
        0x04, 0x14
        );

 if (!n)
   return 0;

 sigtype = getstring(buffer:signature, pos:0);
 if (sigtype >!< "ssh-rsa")
   return 0;

 nlen = strlen(n);
 if (ord(n[0]) == 0)
   nlen--;

 # check minimum n size
 if ( (nlen*8) < 768 )
   return 0;

 next = 4 + strlen(sigtype);
 tmp_sig = getstring(buffer:signature,pos:next);
 siglen = strlen(tmp_sig);

 # bad signature (should be less than n)
 if (siglen > nlen)
   return 0;

 # Add padding if needed
 if (siglen < nlen)
 {
   len = nlen - siglen;
   sig = crap(data:raw_string(0x00), length:len) + tmp_sig;
 }
 else
   sig = tmp_sig;

 hash = SHA1(data);

 if (strlen(hash) != 20)
   return 0;

 # must call RSA_public_decrypt from openssl, so convert arg - see ssh-rsa.c
 decrypted = rsa_public_decrypt(sig:sig,e:e,n:n);
 if (!decrypted)
   return 0;

 if (strlen(decrypted) != (strlen(id_sha1)+20))
   return 0;

 hdecoid = hexstr(substr(decrypted,0,strlen(id_sha1)-1));
 hshaoid = hexstr(id_sha1);

 if (hdecoid >!< hshaoid)
   return 0;

 hdecoid = hexstr(substr(decrypted,strlen(id_sha1),strlen(decrypted)-1));
 hhash = hexstr(hash);

 if (hdecoid >!< hhash)
   return 0;

 return 1;
}


#-----------------------------------------------------------------#
# DSA verify signature                                            #
#-----------------------------------------------------------------#
function ssh_dss_verify(p, q, g, pub, signature, data)
{
 local_var sigtype, next, tmp_sig, siglen, r, s, hash;

 sigtype = getstring(buffer:signature, pos:0);
 if (sigtype >!< "ssh-dss")
   return 0;

 next = 4 + strlen(sigtype);
 tmp_sig = getstring(buffer:signature,pos:next);
 siglen = strlen(tmp_sig);

 r = substr(tmp_sig, 20, 39);
 s = substr(tmp_sig, 40, 59);

 hash = SHA1(data);

 return dsa_do_verify(p:p,g:g,q:q,pub:pub,r:r,s:s,data:hash);
}


#-----------------------------------------------------------------#
# Derive keys  (this function works because we only use sha1 and  #
# blowfish so keylen is 20 due to sha1)                           #
#-----------------------------------------------------------------#
function derive_keys(hash,shared,session_id)
{
 local_var c, i, to_hash, keys;
 # c = 'A';
 c = 65;
 for (i = 0;i < 6; i++)
 {
   to_hash = putbignum(buffer:shared) + hash + raw_int8(i:c) + session_id;
   keys[i] = SHA1(to_hash);
   c++;
 }

 #         MODE OUT  MODE IN
 # enc.iv    0         1
 # enc.key   2         3
 # mac.key   4         5

 return keys;
}


#-----------------------------------------------------------------#
# Check is public key is correct                                  #
#-----------------------------------------------------------------#
function dh_valid_key(key, p)
{
 local_var val,i;

 if (ord(key[0]) > 0x80)
   return 0;

 val = 0;
 for(i=0;i<strlen(key);i++)
 {
  val = val + ord(key[i]);
  if (val > 1)
    break;
 }

 # ok if key < p
 if ((val>1) && (bn_cmp(key1:key,key2:p) == -1))
   return 1;

 return 0;
}


#-----------------------------------------------------------------#
# Generate dh public & private keys                              #
#-----------------------------------------------------------------#
function dh_gen_key(p, g)
{
 local_var tries,keys;

 dh_pub = dh_priv = "";

 tries = 0;

 if (!p)
   return keys;

 # { "blowfish-cbc", SSH_CIPHER_SSH2, 8, 16, EVP_bf_cbc }
 # hash = sha1 = 20 (len) = 20 * 8 (bits)
 need = 20 * 8;

 # need won't be > than INT_MAX / 2
 # maybe we must test if 2*need >= numbits ...
 #if (need > INT_MAX / 2 || 2 * need >= BN_num_bits(dh->p))
 #  return keys;

 for (tries = 0; tries < 10; tries++)
 {
   dh_priv = bn_random(need:(need*2));
   if (!dh_priv)
     return -1;
   dh_pub = dh_generate_key(p:p, g:g, priv:dh_priv);
   if (!dh_pub)
     return -1;
   if (dh_valid_key(key:dh_pub, p:p))
     break;
 }

 if (tries++ >= 10)
   return -1;

 return 0;
}


#-----------------------------------------------------------------#
# Waits for the server identification string, and sends our own   #
# identification string.                                          #
#-----------------------------------------------------------------#
function ssh_kex2(socket,server_version)
{
 local_var packetlen, paddinglen, msgcode, len, len2;
 local_var a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p;
 local_var skexkex, kexr, skexr, gex, gexr, nk, nkey;
 local_var sinit, scookie, ccookie, cinit;
 local_var payload;
 local_var keys, shared;
 local_var alg_type, type;
 local_var start, next, correct, groupex;
 local_var rsa_e, rsa_n, dsa_p, dsa_q, dsa_g, dsa_pub_key;
 local_var server_dh_public_key, signed_h, to_hash, hash;


 # supported algorithms
 key_exchange_algo        = "diffie-hellman-group-exchange-sha1,diffie-hellman-group1-sha1";
 server_host_key_algo     = "ssh-rsa,ssh-dss";
 enc_alg_client_to_server = "blowfish-cbc";
 enc_alg_server_to_client = "blowfish-cbc";
 mac_alg_client_to_server = "hmac-sha1";
 mac_alg_server_to_client = "hmac-sha1";
 cmp_alg_client_to_server = "none";
 cmp_alg_server_to_client = "none";

 # version
 client_version = "SSH-2.0-OpenVAS";


### Key exchange send client Init ###

 skex = ssh_recv(socket:socket, length:2000);

 if(isnull(skex)) {
   set_ssh_error(msg: "ssh_recv() failed in function ssh_kex2()");
   return -1;
 }

 packetlen = ntol (buffer:skex, begin:0);
 paddinglen = ord(skex[4]);
 msgcode = ord(skex[5]);
 if (msgcode != 20)
 {
   set_ssh_error(msg:string("Received code was not SSH_MSG_KEXINIT (20). It was : ", msgcode));
   return -1;
 }

 sinit = substr(skex,6,packetlen+4-paddinglen-1);

 scookie = substr(skex,6,21);

 len = check_pattern(buffer:skex, pattern:"diffie-hellman-group-exchange-sha1", length:22);
 if (len == -1)
 {
   len = check_pattern(buffer:skex, pattern:"diffie-hellman-group1-sha1", length:22);
   if (len == -1)
   {
     set_ssh_error(msg:"Remote SSH server does not support DH exchanges (bugged ?)");
     return -1;
   }
   groupex = 0;
 }
 else
 {
  groupex = 1;
 }

 len2 = check_pattern(buffer:skex, pattern:"ssh-rsa", length:len);
 if (len2 == -1)
 {
   len2 = check_pattern(buffer:skex, pattern:"ssh-dss", length:len);
   if (len2 == -1)
   {
     set_ssh_error(msg:"Remote SSH server does not support DSA and RSA (bugged ?)");
     return -1;
   }
 }

 len = check_pattern(buffer:skex, pattern:enc_alg_client_to_server, length:len2);
 if (len == -1)
 {
  set_ssh_error(msg:"Remote SSH server does not support blowfish-cbc encryption");
  return -1;
 }

 len2 = check_pattern(buffer:skex, pattern:enc_alg_server_to_client, length:len);
 if (len2 == -1)
 {
  set_ssh_error(msg:"Remote SSH server does not support blowfish-cbc encryption");
  return -1;
 }

 len = check_pattern(buffer:skex, pattern:mac_alg_client_to_server, length:len2);
 if (len == -1)
 {
  set_ssh_error(msg:"Remote SSH server does not support hmac-sha1 encryption");
  return -1;
 }

 len2 = check_pattern(buffer:skex, pattern:mac_alg_server_to_client, length:len);
 if (len2 == -1)
 {
  set_ssh_error(msg:"Remote SSH server does not support hmac-sha1 encryption");
  return -1;
 }

 len = check_pattern(buffer:skex, pattern:cmp_alg_client_to_server, length:len2);
 if (len == -1)
 {
  set_ssh_error(msg:"Remote SSH server only supports compressed packets");
  return -1;
 }

 len2 = check_pattern(buffer:skex, pattern:cmp_alg_server_to_client, length:len);
 if (len2 == -1)
 {
  set_ssh_error(msg:"Remote SSH server only supports compressed packets");
  return -1;
 }


### Key exchange recv server Init ###

 ccookie = "";
 for (i=0;i<16;i++)
    ccookie = ccookie + raw_int8(i:(rand() % 256));

 cinit =
    # cookie (random)
    ccookie +
    # key algorithms (length + string)
    raw_int32(i:strlen(key_exchange_algo)) + key_exchange_algo +
    # server host key algorithms (length + string)
    raw_int32(i:strlen(server_host_key_algo)) + server_host_key_algo +
    # encryption algorithms client to server (length + string)
    raw_int32(i:strlen(enc_alg_client_to_server)) + enc_alg_client_to_server +
    # encryption algorithms server to client (length + string)
    raw_int32(i:strlen(enc_alg_server_to_client)) + enc_alg_server_to_client +
    # mac algorithms client to server (length + string)
    raw_int32(i:strlen(mac_alg_client_to_server)) + mac_alg_client_to_server +
    # mac algorithms server to client (length + string)
    raw_int32(i:strlen(mac_alg_server_to_client)) + mac_alg_server_to_client +
    # compression algorithms client to server (length + string)
    raw_int32(i:strlen(cmp_alg_client_to_server)) + cmp_alg_client_to_server +
    # compression algorithms server to client (length + string)
    raw_int32(i:strlen(cmp_alg_server_to_client)) + cmp_alg_server_to_client +
    # languages client to server (length)
    raw_int32(i:0) +
    # languages server to client (length)
    raw_int32(i:0) +
    # payload
    crap(data:raw_string(0x00), length:5);

 # msg code (20 = key exchange init)
 kex = kex_packet(payload:cinit,code:raw_string(0X14));

 send(socket:socket, data:kex);
 seqn_w++;

 if (groupex)
 {
  ### Key exchange Request : Diffie-Hellman GEX Request ###

  payload = raw_string(0x00,0x00,0x04,0x00,0x00,0x00,0x08,0x00,0x00,0x00,0x20,0x00);
  # msg code (34 = Diffie-Hellman GEX Request)
  kexr = kex_packet(payload:payload,code:raw_string(34));

  send(socket:socket, data:kexr);
  seqn_w++;

  ### Key exchange Reply : Diffie-Hellman Key Exchange Reply ###

  skexr = ssh_recv(socket:socket, length:1000);

  # code = 31 (Diffie-Hellman Key Exchange Reply)
  payload = packet_payload(packet:skexr, code:31);
  if (!payload)
  {
   set_ssh_error(msg:"Received code was not SSH_MSG_KEXDH_REPLY (31)");
   return -1;
  }

  # get p bignum for dh group
  p = getstring (buffer:payload,pos:0);

  # get g bignum for dh group
  start = 4+strlen(p);
  g = getstring(buffer:payload,pos:start);
 }
 else
 {
  p = raw_string (0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
                  0xC9, 0x0F, 0xDA, 0xA2, 0x21, 0x68, 0xC2, 0x34,
                  0xC4, 0xC6, 0x62, 0x8B, 0x80, 0xDC, 0x1C, 0xD1,
                  0x29, 0x02, 0x4E, 0x08, 0x8A, 0x67, 0xCC, 0x74,
                  0x02, 0x0B, 0xBE, 0xA6, 0x3B, 0x13, 0x9B, 0x22,
                  0x51, 0x4A, 0x08, 0x79, 0x8E, 0x34, 0x04, 0xDD,
                  0xEF, 0x95, 0x19, 0xB3, 0xCD, 0x3A, 0x43, 0x1B,
                  0x30, 0x2B, 0x0A, 0x6D, 0xF2, 0x5F, 0x14, 0x37,
                  0x4F, 0xE1, 0x35, 0x6D, 0x6D, 0x51, 0xC2, 0x45,
                  0xE4, 0x85, 0xB5, 0x76, 0x62, 0x5E, 0x7E, 0xC6,
                  0xF4, 0x4C, 0x42, 0xE9, 0xA6, 0x37, 0xED, 0x6B,
                  0x0B, 0xFF, 0x5C, 0xB6, 0xF4, 0x06, 0xB7, 0xED,
                  0xEE, 0x38, 0x6B, 0xFB, 0x5A, 0x89, 0x9F, 0xA5,
                  0xAE, 0x9F, 0x24, 0x11, 0x7C, 0x4B, 0x1F, 0xE6,
                  0x49, 0x28, 0x66, 0x51, 0xEC, 0xE6, 0x53, 0x81,
                  0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF);

  g = raw_int8(i:2);
 }
 # generate public and private keys
 ret = dh_gen_key(p:p,g:g);
 if (ret != 0)
  {
   set_ssh_error(msg:"Error during DH keys generation");
   return -1;
  }

### Diffie Hellman GEX Init ###

 payload = raw_int32(i:strlen(dh_pub)) + dh_pub;

 if (groupex)
   codereq = raw_int8(i:32);
 else
   codereq = raw_int8(i:30);

 # msg code (32 = Diffie-Hellman GEX Init)
 gex = kex_packet(payload:payload,code:codereq);

 send(socket:socket, data:gex);
 seqn_w++;

### Diffie Hellman GEX Reply ###

 gexr = ssh_recv(socket:socket, length:2000);

 if (groupex)
   codereq = 33;
 else
   codereq = 31;
 payload = packet_payload(packet:gexr, code:codereq);
 if (!payload)
 {
  set_ssh_error(msg:"Received code was not SSH_MSG_KEXDH_REPLY (31 or 33)");
  return -1;
 }

 # server host key blob
 server_host_key_blob = getstring (buffer:payload,pos:0);

 # extract server host key
 alg_type = getstring (buffer:server_host_key_blob, pos:0);
 next = 4 + strlen(alg_type);

 if (alg_type >< "ssh-rsa")
  {
   # rsa type == 0
   type = 0;

   # e
   rsa_e = getstring (buffer:server_host_key_blob,pos:next);
   next = next + 4 + strlen(rsa_e);

   # n
   rsa_n = getstring (buffer:server_host_key_blob,pos:next);
   next = next + 4 + strlen(rsa_n);
  }
 else
  {
   if (alg_type >< "ssh-dss")
    {
     # dsa type == 1
     type = 1;

     # p
     dsa_p = getstring (buffer:server_host_key_blob,pos:next);
     next = next + 4 + strlen(dsa_p);

     # q
     dsa_q = getstring (buffer:server_host_key_blob,pos:next);
     next = next + 4 + strlen(dsa_q);

     # g
     dsa_g = getstring (buffer:server_host_key_blob,pos:next);
     next = next + 4 + strlen(dsa_g);

     # pub key
     dsa_pub_key = getstring (buffer:server_host_key_blob,pos:next);
     next = next + 4 + strlen(dsa_pub_key);
    }
   else
    # bad key algo - should not occur
   {
     set_ssh_error(msg:"Server's host keys format is not supported");
     return -1;
   }
  }


 # server dh public key
 start = 4 + strlen(server_host_key_blob);
 server_dh_public_key = getstring(buffer:payload,pos:start);

 # signed H
 start = start + 4 + strlen(server_dh_public_key);
 signed_h = getstring(buffer:payload,pos:start);

 if (!dh_valid_key(key:server_dh_public_key, p:p))
 {
   set_ssh_error(msg:"Server DH public key is not valid!");
   return -1;
 }

 # shared secret #
 shared = dh_compute_key(p:p,g:g,dh_server_pub:server_dh_public_key,
  pub_key:dh_pub,priv_key:dh_priv);
 if (!shared)
 {
   set_ssh_error(msg:"Error during shared secret computing");
   return -1;
 }

 # hash data
 to_hash =
        # client version
        putstring(buffer:client_version) +
        # server version
        putstring(buffer:server_version) +
        # client cookie (cookielen,SSH_MSG_KEXINIT,cookie)
        raw_int32(i:(strlen(cinit)+1)) + raw_int8(i:20) + cinit +
        # server cookie (cookielen,SSH_MSG_KEXINIT,cookie)
        raw_int32(i:(strlen(sinit)+1)) + raw_int8(i:20) + sinit +
        # server host key blob
        putstring(buffer:server_host_key_blob);

 if (groupex)
 {
  to_hash +=
        # min,wantbits,max
        raw_string(0x00,0x00,0x04,0x00,0x00,0x00,0x08,0x00,0x00,0x00,0x20,0x00) +
        # p bignum
        putbignum(buffer:p) +
        # g bignum
        putbignum(buffer:g);
 }

 to_hash +=
        # public key bignum
        putbignum(buffer:dh_pub) +
        # server dh public key bignum
        putbignum(buffer:server_dh_public_key) +
        # shared bignum
        putbignum(buffer:shared);

 hash = SHA1(to_hash);

 if (type == 0)
   correct = ssh_rsa_verify(e:rsa_e, n:rsa_n, signature:signed_h, data:hash);
 else
   # Not implemented
   correct = ssh_dss_verify(p:dsa_p, q:dsa_q, g:dsa_g, pub:dsa_pub_key, signature:signed_h, data:hash);

 if (!correct)
 {
   set_ssh_error(msg:"Server's signature is not valid!");
   return -1;
 }

 session_id = hash;
 enc_keys = derive_keys(hash:hash,shared:shared,session_id:session_id);

### New keys ###

 nkey = ssh_recv(socket:socket, length:1000);
 # msg code (21 = New keys)
 payload = packet_payload(packet:nkey, code:21);
 if (!payload)
 {
  set_ssh_error(msg:"Received code was not SSH_MSG_NEWKEYS (21)");
  return -1;
 }

 payload = NULL;

 # msg code (21 = New keys)
 nk = kex_packet(payload:payload,code:raw_string(0x15));

 send(socket:socket, data:nk);
 seqn_w++;

 # all is correct
 return 0;
}


#-----------------------------------------------------------------#
# Authenticate to SSH server                                      #
#-----------------------------------------------------------------#
function ssh_userauth2(socket, server_user, password, pub, priv, passphrase)
{
 local_var payload, buf, support, pass, pkey, kb, authenticated, blobpub, signature, blobpriv, privkey, typestr, next, e, n, hash2, hash, public, line, num, pubtab, i, crap, kb_ok;

 pass = pkey = authenticated = kb = 0;

 payload = putstring(buffer:"ssh-userauth");

 # code 5 (SSH_MSG_SERVICE_REQUEST)
 send_ssh_packet(socket:socket, payload:payload, code:raw_string(0x05));

 # code 6 (SSH_MSG_SERVICE_ACCEPT)
 payload = recv_ssh_packet(socket:socket);
 if (ord(payload[0]) != 6)
 {
   set_ssh_error(msg:string("Server does not support ssh-userauth service. Received code was : " , ord(payload[0])));
   return -1;
 }

 # service accepted
 # code 50 (SSH_MSG_USERAUTH_REQUEST)
 # none request: we need to know what authentication server supports
 payload = putstring(buffer:server_user) + putstring(buffer:"ssh-connection") +
           putstring(buffer:"none");
 send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:50));

 # (51 == SSH_MSG_USERAUTH_FAILURE)
 payload = recv_ssh_packet(socket:socket);
 if (ord(payload[0]) != 51)
 {
   set_ssh_error(msg:string("Server did not reject none authentication method. Received code was : " , ord(payload[0])));
   return -1;
 }

 support = getstring(buffer:payload,pos:1);

 _ssh_supported_authentication = support;

 if (pub && priv)
 {
   if (ereg(string:support, pattern:"publickey"))
     pkey = 1;
   else
   {
     set_ssh_error(msg:string("Error : Remote server does not support publickey authentication method! It supports : " , support));
     return -1;
   }
 }
 else if (password)
 {
   if (ereg(string:support, pattern:"password"))
     pass = 1;
   else if (ereg(string:support, pattern:"keyboard-interactive"))
     kb = 1;
   else
   {
     set_ssh_error(msg:string("Error : Remote server does not support one of the following password authentication methods : password, keyboard-interactive. It supports : " , support));
     return -1;
   }
 }
 else
 {
   set_ssh_error(msg:"OpenVAS needs public and private keys or a password!");
   return -1;
 }


 if (pass)
 {
  # code 50 (SSH_MSG_USERAUTH_REQUEST)
  ###### need extra pad !!!! ######
  ###### Is it useful ?? #####
  payload = putstring(buffer:server_user) + putstring(buffer:"ssh-connection") +
            putstring(buffer:"password") + raw_int8(i:0) + putstring(buffer:password);

  send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:50));

  # code 52 (SSH_MSG_USERAUTH_SUCCESS)
  payload = recv_ssh_packet(socket:socket);
  if (ord(payload[0]) == 52)
     authenticated = 1;

  if (!authenticated)
  {
    set_ssh_error(msg:"Password authentication failed. Please check Username and Password.");
    return -1;
  }
 }
 else if (kb)
 {
  # code 50 (SSH_MSG_USERAUTH_REQUEST)
  payload = putstring(buffer:server_user) + putstring(buffer:"ssh-connection") +
            putstring(buffer:"keyboard-interactive") + putstring(buffer:"en-US") + putstring(buffer:"");

  send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:50));

  # code 60 (SSH_MSG_USERAUTH_INFO_REQUEST)
  payload = recv_ssh_packet(socket:socket);
  if (ord(payload[0]) != 60)
  {
    set_ssh_error(msg:string("Server did not reply with SSH_MSG_USERAUTH_INFO_REQUEST during keyboard-interactive exchange. It replied with :", ord(payload[0])));
    return -1;
  }

  # Method name
  crap = getstring (buffer:payload,pos:1);
  next = 1 + 4 + strlen(crap);

  # Method name complement
  crap = getstring (buffer:payload,pos:next);
  next = next + 4 + strlen(crap);

  # Language
  crap = getstring (buffer:payload,pos:next);
  next = next + 4 + strlen(crap);

  # Number of request
  num = ntol(buffer:payload, begin:next);
  next += 4;

  kb_ok = 0;
  if (num > 0)
  {
    crap = getstring (buffer:payload,pos:next);
    if ("Password:" >< crap)
      kb_ok = 1;
  }

  if (!kb_ok)
  {
    set_ssh_error(msg:"Remote server keyboard-interactive method does not support Password.");
    return -1;
  }

  # code 61 (SSH_MSG_USERAUTH_INFO_RESPONSE)
  payload = raw_int32(i:1) + putstring(buffer:password);

  send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:61));

  payload = recv_ssh_packet(socket:socket);

  # From draft-ietf-secsh-auth-kbdinteract-06.txt document :
  # Server should now send SSH_MSG_USERAUTH_INFO_REQUEST.
  # REQUEST can ask additional information (like a new password).
  # But if all is correct num-prompts is set to 0 and client must
  # reply with an empty SSH_MSG_USERAUTH_INFO_RESPONSE.
  # So we just send an empty response and look if authentication
  # works. If remote server asked for additional information,
  # authentication will just failed.
  if (ord(payload[0]) == 60)
  {
   # code 61 (SSH_MSG_USERAUTH_INFO_RESPONSE)
   payload = raw_int32(i:0);
   send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:61));
   payload = recv_ssh_packet(socket:socket);
  }
  if (ord(payload[0]) != 52)
  {
    set_ssh_error(msg:string("Server did not reply with SSH_MSG_USERAUTH_SUCCESS during keyboard-interactive exchange. It replied with :",ord(payload[0])));
    return -1;
  }

  authenticated = 1;
 }

 else if (!authenticated && pkey)
 {
  # SSH Public Key File Format (draft-ietf-secsh-publickeyfile-05.txt)
  # ---- BEGIN SSH2 PUBLIC KEY ----
  # Comment: "1024-bit RSA, converted from OpenSSH by galb@test1"
  # AAAAB3NzaC1yc2EAAAABIwAAAIEA1on8gxCGJJWSRT4uOrR13mUaUk0hRf4RzxSZ1zRbYY
  # Fw8pfGesIFoEuVth4HKyF8k1y4mRUnYHP1XNMNMJl1JcEArC2asV8sHf6zSPVffozZ5TT4
  # SfsUu/iKy9lUcCfXzwre4WWZSXXcPff+EHtWshahu3WzBdnGxm5Xoi89zcE=
  # ---- END SSH2 PUBLIC KEY ----
  if ("---" >< pub)
  {
    public = "";
    pubtab = split(pub, sep:'\n', keep:0);
    num = max_index(pubtab);
    for (i=0; i<num; i++)
    {
      line = pubtab[i];
      if (("---" >!< line) && (":" >!< line))
      {
        if ('\r' >< line)
          line -= '\r';
        public += line;
      }
    }
  }
  else
  {
    # OpenSSH Public key file format
    public = ereg_replace(pattern:"[^ ]* ([^ ]*) [^ ]*$",
                          string:pub,
                          replace:"\1");
  }

  blobpub = base64decode(str:public);

  # code 50 (SSH_MSG_USERAUTH_REQUEST)
  ###### need extra pad !!!! ######
  ###### Is it useful ?? #####

  payload = putstring(buffer:server_user) + putstring(buffer:"ssh-connection") +
            putstring(buffer:"publickey") + raw_int8(i:1);

  to_hash = putstring(buffer:session_id) + raw_int8(i:50);

  typestr = getstring(buffer:blobpub, pos:0);
  if ("ssh-rsa" >< typestr)
  {
    next = 4 + strlen(typestr);
    e = getstring(buffer:blobpub, pos:next);
    next = next + 4 + strlen(e);
    n = getstring(buffer:blobpub, pos:next);

    privkey = pem_to_rsa(priv:priv, passphrase:passphrase);
    if (!privkey)
     {
      set_ssh_error(msg:"Scanner failed to load SSH private key (RSA)");
      return -1;
     }

    payload += putstring(buffer:"ssh-rsa") + putstring(buffer:blobpub);

    to_hash += payload;

    hash = SHA1(to_hash);
    # FIXME: rsa_sign was changed to use the private key in priv
    # directly.  The above code to extract the parameters n, e and
    # privkey can probably be removed.
    signature = rsa_sign(priv:priv, passphrase:passphrase, data:hash);
    if (!signature)
    {
      set_ssh_error(msg:"Error during client's RSA signature computing");
      return -1;
    }

    signature = putstring(buffer:"ssh-rsa") + putstring(buffer:signature);
  }
  else if ("ssh-dss" >< typestr)
  {
    # p
    next = 4 + strlen(typestr);
    p = getstring (buffer:blobpub,pos:next);
    next = next + 4 + strlen(p);

    # q
    q = getstring (buffer:blobpub,pos:next);
    next = next + 4 + strlen(q);

    # g
    g = getstring (buffer:blobpub,pos:next);
    next = next + 4 + strlen(g);

    # pub key
    pub_key = getstring (buffer:blobpub,pos:next);

    privkey = pem_to_dsa(priv:priv, passphrase:passphrase);
    if (!privkey)
    {
      set_ssh_error(msg:"Scanner failed to load SSH private key (DSA)");
      return -1;
    }

    payload += putstring(buffer:"ssh-dss") + putstring(buffer:blobpub);

    to_hash += payload;

    hash = SHA1(to_hash);
    signature = dsa_do_sign(p:p, q:q, g:g, pub:pub_key, priv:privkey, data:hash);
    if (!signature)
     {
      set_ssh_error(msg:"Error during client's DSA signature computing");
      return -1;
     }

    signature = putstring(buffer:"ssh-dss") + putstring(buffer:signature);

  }
  else
  {
    set_ssh_error(msg:"Client's private key type is not supported");
    return -1;
  }

  payload += putstring(buffer:signature);

  send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:50));

  # code 60 (SSH_MSG_USERAUTH_PK_OK)
  payload = recv_ssh_packet(socket:socket);
  if ((ord(payload[0]) == 52) || (ord(payload[0]) == 60))
     authenticated = 1;

  if (!authenticated)
  {
    payload = "Public key authentication failed.";
    if (password)
    {
      payload += '
It seems you provided both public/private keys and password.
In this case Scanner only use your public and private keys.
Scanner did not try both. As password authentication is
vulnerable to Man-In-The-Middle attack, that implies your keys
are useless (a "malicious server" will just reject your public
key authentication and accept any provided password).
';
    }
    set_ssh_error(msg:payload);

    return -1;
  }
 }

 # all is ok, user is authenticated
 return 0;
}


#-----------------------------------------------------------------#
# Open channel                                                    #
#-----------------------------------------------------------------#
function ssh_open_channel(socket)
{
 local_var payload, i;

 if (bugged_sshd && !bugged_first)
 {
   local_channel++;
   if (local_channel > 10)
     return -1;
   l_window_size = 32768;
   received_size = 0;
   remote_channel = bugged_channels[local_channel];
   r_window_size = bugged_rws[local_channel];
   r_packet_size = bugged_rps[local_channel];
   return 0;
 }

 local_channel++;

 # session = "session"
 # initial window size = 32768
 # maximum packet size = 32768
 l_window_size = 32768;
 received_size = 0;
 payload = putstring(buffer:"session") + raw_int32(i:local_channel) +
           raw_int32(i:32768) + raw_int32(i:32768);

 # SSH_MSG_CHANNEL_OPEN == 90
 send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:90));

 # SSH_MSG_CHANNEL_OPEN_CONFIRMATION == 91 (92 == failure)
 payload = recv_ssh_packet(socket:socket);
 if (ord(payload[0]) != 91)
 {
   set_ssh_error(msg:string("Received code was not  SSH_MSG_CHANNEL_OPEN_CONFIRMATION (91). It was : " , ord(payload[0])));
   return -1;
 }

 remote_channel = ntol(buffer:payload, begin:5);
 r_window_size = ntol(buffer:payload, begin:9);
 r_packet_size = ntol(buffer:payload, begin:13);

 # For bugged channel like Sun_SSH_1.0 we create 10 channels at startup
 # Sun_SSH supports only 10 channels at the same time.
 if (bugged_sshd)
 {
   bugged_first = 0;
   bugged_channels = bugged_rws = bugged_rps = NULL;
   bugged_channels[0] = 0;
   bugged_rws[0] = 0;
   bugged_rps[0] = 0;
   bugged_channels[local_channel] = remote_channel;
   for (i=local_channel+1; i < 11; i++)
   {
      payload = putstring(buffer:"session") + raw_int32(i:i) +
                raw_int32(i:32768) + raw_int32(i:32768);

      send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:90));

      payload = recv_ssh_packet(socket:socket);
      if (ord(payload[0]) != 91)
      {
        set_ssh_error(msg:string("Received code was not  SSH_MSG_CHANNEL_OPEN_CONFIRMATION (91). It was : " , ord(payload[0])));
        return -1;
      }

      bugged_channels[i] = ntol(buffer:payload, begin:5);
      bugged_rws[i] = ntol(buffer:payload, begin:9);
      bugged_rps[i] = ntol(buffer:payload, begin:13);
   }
 }

 # all is ok, user is authenticated
 return 0;
}

#-----------------------------------------------------------------#
# Close channel                                                   #
#-----------------------------------------------------------------#
function ssh_close_channel(socket,end)
{
 local_var payload;

 # session = "session"
 # sender channel = 4444  / should we try different number on failure ?
 # initial window size = 32768
 # maximum packet size = 32768
 payload = raw_int32(i:remote_channel);

 # SSH_MSG_CHANNEL_CLOSE == 97
 send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:97));

 if (!end)
 {
   # SSH_MSG_CHANNEL_CLOSE == 97
   payload = recv_ssh_packet(socket:socket);
   while((ord(payload[0]) != 97) && (ord(payload[0]) != 0))
     payload = recv_ssh_packet(socket:socket);

   if (ord(payload[0]) != 97)
   {
     set_ssh_error(msg:string( "Received code was not  SSH_MSG_CHANNEL_CLOSE (97). It was : " , ord(payload[0])));
     return -1;
   }
 }

 # all is ok, user is authenticated
 return 0;

}


#-----------------------------------------------------------------#
# Login to SSH server                                             #
#-----------------------------------------------------------------#
function ssh_login( socket, login, password, pub, priv, passphrase, keytype ) {

  local_var socket, login, password, pub, priv, passphrase, keytype;
  local_var oid, sess;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ssh_login" );
    return -1;
  }

  if( isnull( login ) ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.100259" ) # ssh_proto_version.nasl is not passing a login to ssh_login. TODO: Check this
      set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#login#-#ssh_login" );
  }

  sess = ssh_connect( socket:socket, keytype:keytype );
  if( ! sess )
    return -1;

  _last_libssh_sess = sess;

  if( ssh_userauth( sess, login:login, password:password, privatekey:priv, passphrase:passphrase ) )
    return -1;

  # none method fix
  ssh_supported_authentication = get_ssh_supported_authentication( sess_id:sess );
  if( ssh_supported_authentication == "" ||
      ssh_supported_authentication == "none" ||
      ord( ssh_supported_authentication ) == "0" ) {
    return -2;
  }
  # end none method fix

  return 0;
}

#-----------------------------------------------------------------#
# Return size we can send on the channel and in the packet        #
#-----------------------------------------------------------------#
function get_data_size()
{
 local_var len;

 if (r_window_size <= r_packet_size)
    len = r_window_size;
 else
    len = r_packet_size;

 # packet option ~= 50 bytes
 len -= 50;

 # Remote server has not readjusted his window
 if (len <= 0)
    return -1;

 return len;
}


#-----------------------------------------------------------------#
# Update Window channel size on SSH server                        #
#-----------------------------------------------------------------#
function update_window_size(socket,size)
{
 local_var len;

 l_window_size -= size;
 received_size += size;

 # Maximum allocated memory is 20 MB
 # A correct server will send 32768 bytes packet. So we stop before
 # to get data.
 # A non standard server will just be stopped by openvasd.
 if (received_size > 19000000)
   return -1;

 if (l_window_size < 1000)
 {
   len = 32768 - l_window_size;
   payload = raw_int32(i:remote_channel) + raw_int32(i:len);

   # SSH_MSG_CHANNEL_WINDOW_ADJUST == 93
   send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:93));

   l_window_size += len;
 }

 return 0;
}

function del_esc_seq( data )
{
  local_var data;

  data = ereg_replace( pattern:'\x1b\x5b[0-9;]*[mK]', replace:'', string:data );
  data = ereg_replace( pattern:'\x1b\x3e', replace:'', string:data );
  data = ereg_replace( pattern:'[\r|\x07|\x1b|\x08|\\[|\x0c]', replace:'', string:data );

  return data;
}

function ssh_read_from_shell( sess, pattern, timeout, retry ) {

  local_var sess, timeout, t, x, ret, buf, pattern, retry;

  if( ! sess ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sess#-#ssh_read_from_shell" );
    return FALSE;
  }

  if( ! timeout ) timeout = 15;
  if( ! retry )   retry = 3;

  x = 1;

  while ( TRUE ) {

    buf = del_esc_seq( data:ssh_shell_read( sess ) );

    len = strlen( buf );
    t++;

    if( len > 0 ) {
      ret += buf;
      x = 1;
    } else {
      if( x++ >= retry ) return ret;
    }

    if( pattern && ret )
      if( eregmatch( pattern:pattern, string:ret ) ) return ret;

    if( t >= timeout ) return ret;

    sleep( 1 );

    if( "Press Enter to continue" >< ret || "--More" >< ret || "<--- More --->" >< ret ) ssh_shell_write( sess, cmd: '\n' );
  }
  return ret;
}

function ssh_cmd_pty( sess, cmd, pattern, timeout, retry, clear_buffer ) {

  local_var sess, cmd, pattern, timeout, retry, clear_buffer;
  local_var extra_cmd, c, ret;

  if( ! cmd ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cmd#-#ssh_cmd_pty" );
    return FALSE;
  }

  if( ! sess ) sess = _last_libssh_sess;
  if( ! sess || int( sess ) < 1 ) return FALSE;

  sess = ssh_shell_open( sess );
  if ( ! sess ) return FALSE;

  if( ! timeout ) timeout = 15;
  if( ! retry ) retry = 3;

  if( extra_cmd = get_kb_item( "ssh/send_extra_cmd" ) ) {
    ssh_shell_write( sess, cmd:extra_cmd );
    sleep( 5 );
  }

  if( ! clear_buffer )
    if( get_kb_item( "ssh/force/clear_buffer" ) ) clear_buffer = TRUE;

  # Clear the buffer before sending the command. This is needed for
  # Cisco NGIPS for example. Without clearing the buffer you didn't
  # get anything back for the executed command. I'd only see this
  # against Cisco NGIPS currently.
  # Do not use "clear_buffer" in gather-package-list.nasl for the
  # "uname" command. We need the complete buffer there...
  if( clear_buffer ) {
    while( TRUE ) {
      c = ssh_read_from_shell( sess:sess, timeout:3, retry:3 );
      if( strlen( c ) <= 0 ) break;
    }
  }

  ssh_shell_write( sess, cmd:cmd + '\n' );
  sleep( 1 );

  if( ! ret = ssh_read_from_shell( sess:sess, pattern:pattern, timeout:timeout, retry:retry ) ) {
    if( defined_func( "ssh_shell_close" ) ) ssh_shell_close( sess );
    return FALSE;
  }

  if( defined_func( "ssh_shell_close" ) ) ssh_shell_close( sess );

  ret = str_replace( string:ret, find:cmd + '\n', replace:"" );

  return ret;
}

#-----------------------------------------------------------------#
# Execute command on SSH server                                   #
#-----------------------------------------------------------------#
function ssh_cmd( socket, cmd, timeout, nosh, return_errors, pty, pattern, retry, clear_buffer ) {

  local_var socket, cmd, timeout, nosh, return_errors, pty, pattern, retry, clear_buffer;
  local_var payload, ret, tempbuf, end, cret, messid_data, ret_payload, ret_ssh_buf, sess;

  if( ! cmd ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cmd#-#ssh_cmd" );
    return;
  }

  if( ! socket ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ssh_cmd" );

  if( ! nosh )
    if( get_kb_item( "ssh/force/nosh" ) ) nosh = TRUE; # NX-OS/IOS/JunOS/F5

  if( get_kb_item( "ssh/no_linux_shell" ) ) {
    nosh = TRUE;
    if( cmd =~ "^(/bin|/sbin|cat |perl |cmd /|netstat |find |which |whereis |locate |/opt/slad|dmidecode )" ) return FALSE; # don't run find, which ... if target doesn't support it
  }

  if( ! nosh ) {
    cmd = str_replace( string:cmd, find:"'", replace:'"' );
    cmd = string( "/bin/sh -c ", "'LANG=C; LC_ALL=C; " , cmd , "'" );
  }

  if( ! pty )
    if( get_kb_item( "ssh/force/pty" ) ) pty = TRUE;

  sess = ssh_session_id_from_sock( socket );
  if ( sess ) {
    # Note: Setting stdout and stderr to 0 enables a
    # compatibility mode to be bug compliant to the NASL ssh code.
    if( pty && defined_func( "ssh_shell_open" ) )
      ret_ssh_buf = ssh_cmd_pty( sess:sess, cmd:cmd, pattern:pattern, timeout:timeout, retry:retry, clear_buffer:clear_buffer );
    else
      ret_ssh_buf = ssh_request_exec( sess, cmd:cmd, stdout:0, stderr:0 );

    ret_ssh_buf = ssh_clean_cmd_from_err( data:ret_ssh_buf );

    if( get_kb_item( "global_settings/ssh/debug" ) ) {
      debug_str = 'SSH DEBUG: ' + get_host_ip() + ':\n' +
                  'PTY:  ' + pty + '\n' +
                  'NOSH: ' + nosh + '\n' +
                  'CMD:  ' + cmd + '\n' +
                  'RES:  ' + ret_ssh_buf + '\n' +
                  '---------------------------------------------------------\n\n';
      display( debug_str );
    }

    if( "Cmd exec error" >< ret_ssh_buf ||
        "error: unknown command" >< ret_ssh_buf ||
        "Unknown command: " >< ret_ssh_buf ||
        "Invalid input detected" >< ret_ssh_buf ||
        ": No such command" >< ret_ssh_buf ||
        "-----unknown keyword " >< ret_ssh_buf ||
        "Unknown action 0" >< ret_ssh_buf ||
        "Invalid command" >< ret_ssh_buf || # HP iLO 100
        "is not a recognized command" >< ret_ssh_buf || # NetApp Data ONTAP 9.x
        "not found.  Type '?' for a list of commands" >< ret_ssh_buf || # NetApp Data ONTAP 7.x
        "Syntax Error: unexpected argument" >< ret_ssh_buf ) { # NX-OS/IOS/JunOS/F5

      set_kb_item( name:"ssh/force/nosh", value:TRUE );
      if( ! return_errors )
        return '';
    }

    # e.g.:
    # /bin/sh: 1: /usr/share/bash-completion/completions/xfreerdp: Permission denied
    # grep: /usr/include/freetype2/freetype/freetype.h: No such file or directory
    if( ": not found" >< ret_ssh_buf || ": Permission denied" >< ret_ssh_buf
    	|| "No such file or directory" >< ret_ssh_buf || "command not found" >< tolower( ret_ssh_buf ) ) {
      if( ! return_errors )
        return '';
    }

    return chomp( ret_ssh_buf );
  }

#
# TODO: Check if code below is still used or makes sense if no session ID was obtained before.
#

 # Flash error buffer
 _ssh_cmd_error = "";

 # Message id flag
 messid_data = 0;

 # Open channel
 ret = ssh_open_channel(socket:socket);
 if(ret != 0)
 {
   _ssh_cmd_error = "Scanner failed to open a new SSH channel. " + get_ssh_error();
   return NULL;
 }

 payload = raw_int32(i:remote_channel) + putstring(buffer:"exec") + raw_int8(i:0) +
           putstring(buffer:cmd);

 # SSH_MSG_CHANNEL_REQUEST == 98
 send_ssh_packet(socket:socket, payload:payload, code:raw_int8(i:98));

 ret = NULL;

 # SSH_MSG_CHANNEL_SUCCESS == 99
 # while for previous useless messages (SSH_MSG_CHANNEL_WINDOW_ADJUST, ...)
 payload = recv_ssh_packet(socket:socket, timeout:timeout);
 while((ord(payload[0]) == 93) || (ord(payload[0]) == 95) || (ord(payload[0])  == 98))
 {
  if(ord(payload[0]) == 95)
  {
    payload = getstring(buffer:payload, pos:9);
    _ssh_cmd_error += payload;
    ret_payload += payload;
    val = update_window_size(socket:socket,size:strlen(payload));
    if (val != 0)
      break;
  }
  payload = recv_ssh_packet(socket:socket, timeout:timeout);
  if("exit-signal" >< payload) ret_payload += string(payload);
 }

 while((ord(payload[0]) != 97) && (ord(payload[0]) != 0))
 {
  if(ord(payload[0]) == 98)
  {
    payload = getstring(buffer:payload, pos:5);
    if ("exit-status" >!< payload)
      break;
  }
  else if((ord(payload[0]) == 94) || (ord(payload[0]) == 95))
  {
    tempbuf = getstring(buffer:payload, pos:5);
    if(ord(payload[0]) == 94)
    {
      ret += tempbuf;
      messid_data = 1;
    }
    val = update_window_size(socket:socket,size:strlen(tempbuf));
    if(val != 0) {
      break;
    }
  }
  else if(ord(payload[0]) == 96){
     if(messid_data == 0){
        ret += ret_payload;
     }
  }

  payload = recv_ssh_packet(socket:socket, timeout:timeout);
 }

 end = 0;
 if (ord(payload[0]) == 97)
   end = 1;

 # Close channel
 cret = ssh_close_channel(socket:socket, end:end);
 if (cret != 0)
 {
   _ssh_cmd_error = "Scanner failed to close SSH channel. " + get_ssh_error();
   return NULL;
 }

 return ret;

}


#-----------------------------------------------------------------#
# Return errors happened during ssh_cmd()                        #
#-----------------------------------------------------------------#
function ssh_cmd_error()
{
 return _ssh_cmd_error;
}



#------------------------------------------------------------------#
# Connection re-use                                                #
#------------------------------------------------------------------#

# Returns a socket to an already established connection
#
# It seems that this function is not used at all.  The SVN shows that
# it has been here since 2005 without any change. (wk 2012-08-16).
function ssh_reuse_connection()
{
}


#
# Release the shared SSH connection so that another script can write to
# it
#
# This function is called by many scripts but obviously only for
# cleanup purposes.  Given that ssh_cmd takes a socket argument it is
# questionable how this function was supposed to work.  A global ssh
# socket is not in use. (wk 2012-08-16)
function ssh_close_connection()
{
}


# Open an ssh connection and return the socket on success.
# Authentication data needs to be passed to this function.
#
# Similar to ssh_reuse_connection and ssh_close_connection, this
# function is pretty old and has never been implemented in the way its
# name may surmise.  The function returns a socket on success but 0 on
# error - this is a bit surprising.
function ssh_login_or_reuse_connection() {

  local_var soc, sess;
  local_var login, password, pub, priv, passphrase;

  login      = kb_ssh_login();
  password   = kb_ssh_password();
  pub        = kb_ssh_publickey();
  priv       = kb_ssh_privatekey();
  passphrase = kb_ssh_passphrase();

  if( ! login && ( ! password && ! priv ) )
    return 0;

  sess = ssh_connect();
  if( ! sess )
    return 0;

  if( ssh_userauth( sess, login:login, password:password, privatekey:priv, passphrase:passphrase ) ) {
    ssh_disconnect( sess );
    last_sess = 0;
    return 0;
  }

  soc = ssh_get_sock( sess );
  _last_libssh_sess = sess;
  return soc;
}

# Connect to the host HOSTNAME via TCP and setup an ssh connection.
# If SOCKET is given that socket will be used instead of a creating a
# new tcp connection.  On success an ssh session to the host has been
# established; the caller may then run an authentication.  If this
# connection is no longer needed, ssh_disconnect may be used to
# disconnect and close the socket.  On success the function returns a
# non-zero integer identifying the ssh session; zero is returned on
# error.
#
# internal function ssh_connect(socket)


# Disconnect the ssh session SESSION.  This also closes the underlying
# socket; regardless whether it was opened by ssh_connect or passed to
# it.
#
# internal function ssh_disconnect(session)


# This is a standalone version of ssh_cmd which does the same as
#    ssh host CMD
# would do on a command line.  Its implementation uses libssh.  It
# should be used only for a single command to a host because each use
# requires running the expensive ssh handshake.
function ssh_cmd_exec( cmd ) {

  local_var cmd, login, password, priv, passphrase, sess, result;

  if( ! cmd ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cmd#-#ssh_cmd_exec" );
    return;
  }

  login      = kb_ssh_login();
  password   = kb_ssh_password();
  priv       = kb_ssh_privatekey();
  passphrase = kb_ssh_passphrase();

  sess = ssh_connect();
  if( ! sess )
    return NULL;

  if( ssh_userauth( sess, login:login, password:password, privatekey:priv, passphrase:passphrase ) ) {
    ssh_disconnect( sess );
    return NULL;
  }
  result = ssh_request_exec( sess, cmd:cmd, stdout:1, stderr:1 );
  ssh_disconnect( sess );
  return chomp( result );
}

# Workaround to get the server banner from a freshly opened socket.
# This should be replaced as soon as we can switch to libssh 0.6 which
# allows us to use ssh_get_server_banner.

# Previous comment from ssh_detect.nasl:
# The ssh_get_server_banner function will only be available after
# we switch to libssh 0.6. Thus for the time being, we use a
# workaround.

# Previous comment from gb_greenbone_os_detect_ssh.nasl:
# We do not need to login to get the banner. Until we can
# switch to libssh 0.6 we use our hacked up version.
# After the switch we may want to have a login function
# which terminates the connection right before the KEX
# protocol part. This will allows us to get the server
# banner without a need to try a login.

function ssh_hack_get_server_version( socket ) {

  local_var socket, buf;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ssh_hack_get_server_version" );
    return;
  }

  buf = recv_line( socket:socket, length:1024 );

  if( ! buf ) {
    set_ssh_error( msg:"Scanner did not receive server's version" );
    return 0;
  }

  # server ident : SSH-%d.%d-servername #
  if( ! ereg( string:buf, pattern:"^SSH-*[0-9]\.*[0-9]-*[^\n]" ) ) {
    set_ssh_error( msg:"Remote service is not a valid SSH service" );
    return 0;
  }

  if( '\r\n' >< buf )
    buf = buf - '\r\n';
  else
    buf = buf - '\n';

  return buf;
}

function ssh_reconnect( sock ) {

  local_var sock;

  if( ! sock ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#ssh_reconnect" );
    return;
  }

  ssh_disconnect( ssh_session_id_from_sock( sock ) );
  sleep( 1 );
  sock = ssh_login_or_reuse_connection();

  if( ! sock ) return;

  return sock;

}

function get_ssh_port( default ) {

  local_var default, port, p;

  if( ! default ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_ssh_port" );

  port = get_kb_item( "Services/ssh" );
  if( port ) default = port;

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( ssh_is_fragile_port( port:default ) ) exit( 0 );

  if( ! get_port_state( default ) ) exit( 0 );

  return default;
}

# Duplicated from misc_func.inc
function ssh_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ssh_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}

# @brief Checks if any random user/pass pair is accepted by the SSH service. The function
#        will save the status (broken/not broken) and if the status was already checked
#        within the KB to avoid that the same check is conducted multiple times.
#
# @param port The TCP port of the remote SSH service to check.
#
# @return TRUE if any random username/password pair is accepted by the SSH service, FALSE
#         otherwise and NULL if it was not possible to get a connection to the service or if
#         no port parameter was passed.
#
function ssh_broken_random_login( port ) {

  local_var port;
  local_var textbanner, banner, count, logins, i, user, pass, soc, login;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ssh_broken_random_login" );
    return NULL;
  }

  if( get_kb_item( "SSH/" + port + "/broken/random_login" ) )
    return TRUE;

  if( get_kb_item( "SSH/" + port + "/broken/random_login/checked" ) )
    return FALSE;

  textbanner = get_kb_item( "SSH/textbanner/" + port );
  banner = get_ssh_server_banner( port:port );
  count  = 0;
  logins = 0;

  for( i = 1; i < 3; i++ ) {

    # nb: See gb_ssh_os_detection.nasl
    if( i == 1 && ( banner =~ "(cisco|FIPS User Access Verification)" || "Cisco Systems, Inc. All rights Reserved" >< textbanner ) ) {
      user = "Anonymous";
      pass = "";
    } else {
      user = ssh_rand_str( length:7 + i, charset:"abcdefghiklmnopqrstuvwxyz" );
      pass = ssh_rand_str( length:7 + i, charset:"ABCDEFGHIJKLMNOPQRSTUVWXTZabcdefghiklmnopqrstuvwxyz" );
    }

    if( ! soc = open_sock_tcp( port ) ) {
      sleep( 1 );
      continue;
    }

    count++;
    login = ssh_login( socket:soc, login:user, password:pass, pub:NULL, priv:NULL, passphrase:NULL );
    close( soc );
    if( login == 0 )
      logins++;

    sleep( 1 );
  }

  if( count > 0 )
    set_kb_item( name:"SSH/" + port + "/broken/random_login/checked", value:TRUE );

  if( logins > 0 ) {
    set_kb_item( name:"SSH/" + port + "/broken/random_login", value:TRUE );
    return TRUE;
  }

  if( count == 0 )
    return NULL;
  else
    return FALSE;
}

function find_bin( prog_name, sock ) {

  local_var r, whe, whi, cl, prog_name, sock;

  whe = "";
  whi = "";

  if( get_kb_item( "ssh/no_linux_shell" ) )
    return;

  if( ! prog_name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#prog_name#-#find_bin" );
    return;
  }

  if( ! sock )
    sock = ssh_login_or_reuse_connection();

  if( sock ) {

    cl = ssh_check_locate( sock:sock );

    if( cl )
      r = split( ssh_cmd( socket:sock, cmd:"locate -ei *bin/" + prog_name, timeout:60 ) );
    else
      r = NULL;
  } else {
    r = NULL;
  }

  if( "bin/" + prog_name >!< r ) {
    if( sock ) {
      whe = ssh_cmd( socket:sock, cmd:"whereis -b " + prog_name, timeout:60 );
      if( "bin/" + prog_name >< whe ) {
        r = split( substr( whe - ( prog_name + ":" ), 1 ), sep:" " );
      } else {
        r = NULL;
      }
    }
  }

  if( isnull( r ) ) {
    if( sock ) {
      whi = ssh_cmd( socket:sock, cmd:"which -a " + prog_name, timeout:60 );
      if( ( "which: no " + prog_name ) >!< whi ) {
        r = split( whi );
      } else {
        r = NULL;
      }
    }
  }

  return( r );
}

# @brief Searches for a file on the target SSH server
#
# @param file_name   The filename to search for (e.g. ssh, sshd, ...)
# @param file_path   The path to start the search (e.g. / )
# @param useregex    If set to TRUE specifies if a basic regexp search in the locate (-r/--regexp) is used.
# @param useextregex If set to TRUE specifies if a extended regexp search in the locate (--regex) is used
# @param regexpar    The regex to use if useregex or extregex was set to TRUE.
# @param sock        The socket for the connection to the remote SSH service.
#
# @example Example call to search for all (ssh and sshd) files:
#          file_name:"sshd"
#          file_path:"/"
#          useextregex:TRUE
#          regexpar:"?$"
#
#          The "?" makes the ending "d" optional and so all "ssh" and "sshd" files are returned.
#
# @note If useregex and useextregex are both set to TRUE useextregex is used.
#
# @return A list containing the found files or NULL if none where found or the function was started against a non-linux service.

function find_file( file_name, file_path, useregex, useextregex, regexpar, sock ) {

  local_var r, f, fname, lparam, cl, file_name, file_path, useregex, useextregex, regexpar, sock;
  local_var use_find, descend_directories, cmd, start, end, current_timeout;

  if( get_kb_item( "ssh/no_linux_shell" ) )
    return;

  if( ! file_name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file_name#-#find_file" );
    return;
  }

  if( ! file_path ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file_path#-#find_file" );
    file_path = "/";
  }

  r = NULL;

  if( useextregex )
    lparam = "-ei --regex";
  else if( useregex )
    lparam = "-eir";
  else
    lparam = "-ei";

  use_find = get_kb_item( "ssh/lsc/enable_find" );
  descend_directories = get_kb_item( "ssh/lsc/descend_ofs" );

  if( isnull( use_find ) )
    use_find = "yes";

  if( isnull( descend_directories ) )
    descend_directories = "yes";

  fname = file_name;

  if( useregex )
    fname = ereg_replace( pattern:"\.\*", string:fname, replace:"" );

  fname = ereg_replace( pattern:"\*.+", string:fname, replace:"" );

  if( ! sock )
    sock = ssh_login_or_reuse_connection();

  if( sock ) {

    cl = ssh_check_locate( sock:sock );

    if ( cl )
      r = split( ssh_cmd( socket:sock, cmd:"locate " + lparam + " " + raw_string(0x22) + file_path + file_name + regexpar + raw_string(0x22), timeout:60));
    else
      r = NULL;

    if( fname >!< r && ( "yes" >< use_find ) ) {
      if( ! isnull( file_path ) ) {

        cmd = "find " + raw_string( 0x22 ) + file_path + raw_string( 0x22 );

        if( "no" >< descend_directories )
          cmd += " -xdev";

        # nb: /run is excluded because it is a new storage of transient state files on Debian
        # See https://wiki.debian.org/ReleaseGoals/RunDirectory for more info
        cmd += " -maxdepth 7 -mindepth 1";
        cmd += ' \\( -path "*/proc" -o -path "/run" -o -path "/dev" -o -path "/sys" -o -path "/media" -o -path "/tmp" -o -path "/var" \\) -prune -o ';
        cmd += " -name " + raw_string( 0x22 );
        cmd += file_name + raw_string( 0x22 ) +" -a  -type f  -print 2>/dev/null";

        start = unixtime();
        f = split( ssh_cmd( socket:sock, cmd:cmd, timeout:60 ) );
        end = unixtime();

        # A local find should take only a few seconds. If we're reaching more
        # then this we can try to add the -xdev parameter to find to bring down
        # the time if an network based file filesystem like NFS or AFS is mounted
        # at the target host. To be safe we're counting the timeouts and only add
        # the -xdev once we have reached the timeout three times.
        # TBD: If this doesn't help maybe lower the -maxdepth 7 above?
        current_timeout = get_kb_item( "ssh/lsc/find_timeout" );
        if( end - start > 14 ) { #nb: GVM-9 and below has a timeout of 15 seconds, current trunk has 30 seconds
          current_timeout++;
          replace_kb_item( name:"ssh/lsc/find_timeout", value:current_timeout );
        }
        if( "yes" >< descend_directories && current_timeout >= 3 ) {
          replace_kb_item( name:"ssh/lsc/descend_ofs", value:"no" );
        }
      }
    }
  }

  if( ! r && ! f )
    return NULL;

  if( fname >!< r ) {
    if( fname >< f ) {
      r = f;
    } else {
      r = NULL;
    }
  }
  return( r );
}

function get_bin_version( full_prog_name, version_argv, ver_pattern, sock ) {

  local_var loc_version, r, full_prog_name, version_argv, ver_pattern, sock;

  if( get_kb_item( "ssh/no_linux_shell" ) ) return;

  if( ! full_prog_name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#full_prog_name#-#get_bin_version" );
    return;
  }

  if( ! version_argv ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version_argv#-#get_bin_version" );
  if( ! ver_pattern )  set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ver_pattern#-#get_bin_version" );

  full_prog_name = chomp( full_prog_name );
  version_argv = chomp( version_argv );

  if( ! sock)
    sock = ssh_login_or_reuse_connection();

  if( sock )
    r = ssh_cmd( socket:sock, cmd:full_prog_name + " " + version_argv, timeout:60 );

  if( ! r )
    return;

  loc_version = eregmatch( pattern:ver_pattern, string:r );
  if( loc_version != NULL )
    loc_version[max_index( loc_version )] = r;

  return( loc_version );
}

# @brief Checks if the "locate" command is available on the remote SSH server.
#        Additionally save the reason (namely the result of the SSH command)
#        why the locate isn't available within the KB key ssh/locate/broken.
#
# @param sock the SSH socket of the connection to the remote SSH server.
#
# @return TRUE if the locate command is available, FALSE otherwise and NULL
#              if no sock parameter was passed.
#
function ssh_check_locate( sock ) {

  local_var sock, r;

  if( ! sock ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sock#-#ssh_check_locate" );
    return;
  }

  if( get_kb_item( "ssh/locate/available" ) ) return TRUE;
  if( get_kb_item( "ssh/no_linux_shell" ) )   return FALSE;
  if( get_kb_item( "ssh/locate/broken" ) )    return FALSE;

  r = ssh_cmd( socket:sock, cmd:"locate -S", timeout:60, return_errors:TRUE );

  # nb: slocate (and maybe other variants?) don't have the database statistic command
  # Just try a search instead, if the database isn't build it will return something like
  # warning: locate: could not open database: /path/to/db: No such file or directory or
  # locate: fatal error: Could not find user database '/path/to/db':  No such file or directory
  if( "locate: invalid option -- S" >< r || "locate: unknown option -- S" >< r ) {
    r = ssh_cmd( socket:sock, cmd:"locate /bin/bash", timeout:60 ); # nb: Assuming that /bin/bash exists on the target system
    if( "locate:" >!< r && "/bin/bash" >< r ) {
      set_kb_item( name:"ssh/locate/available", value:TRUE );
      return TRUE;
    } else if( "locate:" >!< r ) {
      r = ssh_cmd( socket:sock, cmd:"locate /bin/sh", timeout:60 ); # nb: Another try for /bin/sh if /bin/bash doesn't exists
      if( "locate:" >!< r && "/bin/sh" >< r ) {
        set_kb_item( name:"ssh/locate/available", value:TRUE );
        return TRUE;
      }
    }
  }

  if( ( "Database /" >!< r ||
      ( "bytes" >!< r &&
        "Compression" >!< r &&
        "Filenames"   >!< r &&
        "file names"  >!< r ) ) ||
        "command not found" >< r ||
        "locate: not found" >< r ||
        "locate:" >< r ) { # nb: Also includes an outdated database like "locate: warning: database /var/cache/locate/locatedb is more than 8 days old (actual age is 9,3 days)"

    set_kb_item( name:"ssh/locate/broken", value:r );
    return FALSE;
  }
  set_kb_item( name:"ssh/locate/available", value:TRUE );
  return TRUE;
}

function get_ssh_server_banner( port ) {

  local_var port, soc, banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_ssh_server_banner" );
    return;
  }

  if( ! get_port_state( port ) ) return;

  banner = get_kb_item( "SSH/server_banner/" + port );
  if( banner ) return banner;

  soc = open_sock_tcp( port );
  if( ! soc ) return;

  banner = ssh_hack_get_server_version( socket:soc );
  close( soc );

  if( banner )
    replace_kb_item( name:"SSH/server_banner/" + port, value:banner );

  return banner;
}

# @brief Cleans a passed string from various known messages returned
#        by @ref ssh_cmd which might interfere with various detection
#        pattern relying on a string without those messages.
#
# @param data the string to clean
#
# @return the cleaned string
#
function ssh_clean_cmd_from_err( data ) {

  local_var data;

  # uname: Could not chdir to home directory /home/test: No such file or directory
  # Could not chdir to home directory /home/test: No such file or directory
  # Could not chdir to home directory /home/test: Permission denied
  #
  # Those messages itself might show up if a user for the SSH connection is used without e.g. a
  # home directory on the target host.
  #
  # nb: Keep the pattern in single quotes...
  data = ereg_replace( string:data, pattern:'^([a-zA-Z]+: )?Could not chdir to home directory [^:]+: (No such file or directory|Permission denied)[\r\n]*', replace:"" );

  # Returned on Mac OS X if the subversion binary is called with the --version parameter.
  # It is unknown why some systems behaving like this and which other applications/binaries are affected.
  #
  # xcode-select: error: no developer tools were found at '/Applications/Xcode.app', and no install could be requested (perhaps no UI is present), please install manually from 'developer.apple.com'.
  #
  # nb: Keep the pattern in single quotes...
  data = ereg_replace( string:data, pattern:'^xcode-select: error: no developer tools were found at \'/Applications/Xcode\\.app\', and no install could be requested \\(perhaps no UI is present\\), please install manually from \'developer\\.apple\\.com\'\\.[\r\n]*', replace:"" );

  return data;
}

# copied from misc_func.inc as we don't want to taint ssh_func.inc with
# a third-party include
# TODO: Remove and use rand_str() from misc_func.inc once openvas-nasl-lint
# can handle .inc includes using a function from another include.
function ssh_rand_str( length, charset ) {

  local_var l, i, s, n, length, charset;

  if( ! charset )
    charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_";
  if( isnull( length ) )
    length = 8;
  l = strlen( charset );
  s = "";
  for( i = 0; i < length; i++ ) {
    n = rand() % l;
    s += charset[n];
  }
  return s;
}

# @brief looks on linux host via SSH if a given file can be found.
#
# @param sock an open SSH socket (optional). If non given, a new one will be opened.
# @param file the file to check the existence(filepath)
#
# @return TRUE if the file can be found, FALSE otherwise and NULL if not all
#         required parameters are passed or no connection could be opened to the target.
#
function ssh_check_file_existence( sock, file ) {

  local_var sock, file;
  local_var cmd, file_exists;

  if( ! sock ) {
    sock = ssh_login_or_reuse_connection();
  }

  if( ! file ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#ssh_check_file_existence" );
    return;
  }

  cmd = '[ -f ' + file + ' ] && echo "TRUE" || echo "FALSE"';
  file_exists = ssh_cmd( cmd:cmd, socket:sock );

  if( file_exists == 'TRUE' )
    return TRUE;
  else if( file_exists == 'FALSE' )
    return FALSE;
  else
    return;
}

# @brief looks on linux host via SSH if a given file can be read.
#
# @param sock an open SSH socket (optional). If non given, a new one will be opened.
# @param file the file to check if it can be read(filepath)
#
# @return TRUE if the file can be read, FALSE otherwise and NULL if not all
#         required parameters are passed or no connection could be opened to the target.
#
function ssh_check_file_readable( sock, file ) {

  local_var sock, file;
  local_var cmd, file_readable;

  if( ! sock ) {
    sock = ssh_login_or_reuse_connection();
  }

  if( ! file ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#ssh_check_file_existence" );
    return;
  }

  cmd = '[ -r ' + file + ' ] && echo "TRUE" || echo "FALSE"';
  file_readable = ssh_cmd( cmd:cmd, socket:sock );

  if( file_readable == 'TRUE' )
    return TRUE;
  else if( file_readable == 'FALSE' )
    return FALSE;
  else
    return;
}
