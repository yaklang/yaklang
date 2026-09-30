/* SPDX-License-Identifier: CC0-1.0. All wire bytes are produced by open62541. */
#include <open62541/client_config_default.h>
#include <open62541/client_highlevel.h>
#include <open62541/server.h>
#include <open62541/server_config_default.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
static volatile UA_Boolean running = true;
static void stop(int sig) { (void)sig; running=false; }
static void check(UA_StatusCode code) { if(code!=UA_STATUSCODE_GOOD) {fprintf(stderr,"ORACLE failure %s\n",UA_StatusCode_name(code));exit(2);} }
static void readMany(UA_Client *c, unsigned round) {
 UA_ReadRequest q; UA_ReadRequest_init(&q);
 q.nodesToReadSize=2048;
 q.nodesToRead=UA_Array_new(q.nodesToReadSize,&UA_TYPES[UA_TYPES_READVALUEID]);
 for(size_t i=0;i<q.nodesToReadSize;i++) { q.nodesToRead[i].nodeId=UA_NODEID_NUMERIC(0,UA_NS0ID_SERVER_SERVERSTATUS_STATE);q.nodesToRead[i].attributeId=UA_ATTRIBUTEID_VALUE; }
 UA_ReadResponse r=UA_Client_Service_read(c,q);check(r.responseHeader.serviceResult);
 if(r.resultsSize!=q.nodesToReadSize) exit(3);
 for(size_t i=0;i<r.resultsSize;i++) {check(r.results[i].status);if(!UA_Variant_hasScalarType(&r.results[i].value,&UA_TYPES[UA_TYPES_INT32])||*(UA_Int32*)r.results[i].value.data!=UA_SERVERSTATE_RUNNING)exit(4);}
 printf("ORACLE round=%u results=%zu all=Running\n",round,r.resultsSize);fflush(stdout);
 UA_ReadResponse_clear(&r);UA_ReadRequest_clear(&q);
}
int main(int argc,char **argv) {
 if(argc!=2)return 1;
 signal(SIGINT,stop);signal(SIGTERM,stop);
 if(!strcmp(argv[1],"server")) {
  UA_Server *s=UA_Server_new();UA_ServerConfig *sc=UA_Server_getConfig(s);check(UA_ServerConfig_setMinimal(sc,14846,NULL));
  sc->tcpBufSize=8192;
  UA_Array_delete(sc->serverUrls,sc->serverUrlsSize,&UA_TYPES[UA_TYPES_STRING]);sc->serverUrls=UA_Array_new(1,&UA_TYPES[UA_TYPES_STRING]);sc->serverUrlsSize=1;sc->serverUrls[0]=UA_STRING_ALLOC("opc.tcp://127.0.0.1:14846");check(UA_Server_run_startup(s));
  while(running)UA_Server_run_iterate(s,true);
  check(UA_Server_run_shutdown(s));UA_Server_delete(s);
 } else {
  UA_Client *c=UA_Client_new();UA_ClientConfig *cc=UA_Client_getConfig(c);check(UA_ClientConfig_setDefault(cc));
  cc->secureChannelLifeTime=2000;cc->localConnectionConfig.recvBufferSize=8192;cc->localConnectionConfig.sendBufferSize=8192;
  check(UA_Client_connect(c,"opc.tcp://127.0.0.1:14846"));readMany(c,1);
  time_t until=time(NULL)+5;while(time(NULL)<until)check(UA_Client_run_iterate(c,50));
  readMany(c,2);check(UA_Client_disconnect(c));UA_Client_delete(c);
 }
 return 0;
}
