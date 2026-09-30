/* SPDX-License-Identifier: CC0-1.0
 * A small native endpoint driver. Every wire byte is produced by open62541;
 * the driver only configures public APIs and verifies a server node read.
 */
#include <open62541/client_config_default.h>
#include <open62541/client_highlevel.h>
#include <open62541/server.h>
#include <open62541/server_config_default.h>
#include <open62541/plugin/securitypolicy_default.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include "common.h"

static volatile UA_Boolean running = true;
static void stop(int sig) { (void)sig; running = false; }
static void checked(UA_StatusCode result, const char *step) {
    if(result != UA_STATUSCODE_GOOD) {
        fprintf(stderr, "ORACLE failed %s: %s\n", step, UA_StatusCode_name(result));
        exit(2);
    }
}
int main(int argc, char **argv) {
    if(argc != 7) { fprintf(stderr,"role policy mode reverse cert-directory reverse-port\n"); return 2; }
    int isServer = !strcmp(argv[1], "server");
    int secure = strcmp(argv[2], "None") != 0;
    int ecc = !strcmp(argv[2], "ECC_nistP256");
    int reverse = atoi(argv[4]);
    UA_UInt16 reversePort = (UA_UInt16)atoi(argv[6]);
    char reverseUrl[96]; snprintf(reverseUrl,sizeof(reverseUrl),"opc.tcp://127.0.0.1:%u",reversePort);
    UA_MessageSecurityMode mode = (UA_MessageSecurityMode)atoi(argv[3]);
    const char *endpoint = "opc.tcp://127.0.0.1:14840";
    char policy[160], certPath[1024], keyPath[1024], peerPath[1024];
    snprintf(policy,sizeof(policy),"http://opcfoundation.org/UA/SecurityPolicy#%s",argv[2]);
    snprintf(certPath,sizeof(certPath),"%s/%s-%s.der",argv[5],ecc?"ecc":"rsa",argv[1]);
    snprintf(keyPath,sizeof(keyPath),"%s/%s-%s-key.der",argv[5],ecc?"ecc":"rsa",argv[1]);
    snprintf(peerPath,sizeof(peerPath),"%s/%s-%s.der",argv[5],ecc?"ecc":"rsa",isServer?"client":"server");
    UA_ByteString cert = secure ? loadFile(certPath) : UA_BYTESTRING_NULL;
    UA_ByteString key = secure ? loadFile(keyPath) : UA_BYTESTRING_NULL;
    UA_ByteString peer = secure ? loadFile(peerPath) : UA_BYTESTRING_NULL;
    signal(SIGINT,stop); signal(SIGTERM,stop);
    if(isServer) {
        UA_Server *server = UA_Server_new();
        UA_ServerConfig *sc = UA_Server_getConfig(server);
        if(secure) {
            checked(UA_ServerConfig_setDefaultWithSecurityPolicies(sc,14840,&cert,&key,&peer,1,NULL,0,NULL,0),"server config");
            if(ecc) {
                checked(UA_ServerConfig_addSecurityPolicyEccNistP256(sc,&cert,&key),"server ECC policy");
                checked(UA_ServerConfig_addAllEndpoints(sc),"server ECC endpoints");
            }
        } else checked(UA_ServerConfig_setMinimal(sc,14840,NULL),"server config");
        UA_Array_delete(sc->serverUrls,sc->serverUrlsSize,&UA_TYPES[UA_TYPES_STRING]);
        sc->serverUrls = UA_Array_new(1,&UA_TYPES[UA_TYPES_STRING]);
        sc->serverUrlsSize = 1; sc->serverUrls[0] = UA_STRING_ALLOC(endpoint);
        checked(UA_Server_run_startup(server),"server start");
        if(reverse) checked(UA_Server_addReverseConnect(server,UA_STRING(reverseUrl),NULL,NULL,NULL),"reverse connection");
        while(running) UA_Server_run_iterate(server,true);
        checked(UA_Server_run_shutdown(server),"server shutdown");
        UA_Server_delete(server);
    } else {
        UA_Client *client = UA_Client_new();
        UA_ClientConfig *cc = UA_Client_getConfig(client);
        if(secure) {
            checked(UA_ClientConfig_setDefaultEncryption(cc,cert,key,&peer,1,NULL,0),"client config");
            if(ecc) {
                cc->securityPolicies = UA_realloc(cc->securityPolicies,(cc->securityPoliciesSize+1)*sizeof(UA_SecurityPolicy));
                checked(UA_SecurityPolicy_EccNistP256(&cc->securityPolicies[cc->securityPoliciesSize],UA_APPLICATIONTYPE_CLIENT,cert,key,cc->logging),"client ECC policy");
                cc->securityPoliciesSize++;
            }
        } else checked(UA_ClientConfig_setDefault(cc),"client config");
        cc->securityMode = mode;
        cc->securityPolicyUri = UA_STRING_ALLOC(policy);
        if(reverse && secure) {
            cc->endpoint.securityMode = mode;
            cc->endpoint.securityPolicyUri = UA_STRING_ALLOC(policy);
            cc->endpoint.endpointUrl = UA_STRING_ALLOC(endpoint);
            cc->endpoint.transportProfileUri = UA_STRING_ALLOC("http://opcfoundation.org/UA-Profile/Transport/uatcp-uasc-uabinary");
            checked(UA_ByteString_copy(&peer,&cc->endpoint.serverCertificate),"endpoint certificate");
            cc->endpoint.userIdentityTokens = UA_Array_new(1,&UA_TYPES[UA_TYPES_USERTOKENPOLICY]);
            cc->endpoint.userIdentityTokensSize = 1;
            cc->endpoint.userIdentityTokens[0].tokenType = UA_USERTOKENTYPE_ANONYMOUS;
            cc->endpoint.userIdentityTokens[0].policyId = UA_STRING_ALLOC("open62541-anonymous-policy-sign+encrypt#ECC_nistP256");
        }
        if(reverse && secure) {
            // v1.5.5's reverse listener does not copy config.endpoint into its
            // selected endpoint. Public connect initialization does. Prime it
            // against the not-yet-started local server, retaining its selected
            // endpoint after the expected transport failure; no wire is forged.
            UA_StatusCode primed = UA_Client_connect(client,endpoint);
            if(primed != UA_STATUSCODE_BADCONNECTIONREJECTED) {
                fprintf(stderr,"ORACLE unexpected priming status %s\n",UA_StatusCode_name(primed)); return 5;
            }
        }
        if(reverse) {
            const UA_String listenHost = UA_STRING("127.0.0.1");
            checked(UA_Client_startListeningForReverseConnect(client,&listenHost,1,reversePort),"reverse listener");
            time_t deadline = time(NULL)+20;
            for(;;) {
                UA_SessionState state; UA_StatusCode result;
                UA_Client_getState(client,NULL,&state,&result);
                checked(result,"reverse state");
                if(state == UA_SESSIONSTATE_ACTIVATED) break;
                if(time(NULL)>deadline) { fprintf(stderr,"ORACLE reverse timeout\n"); return 3; }
                checked(UA_Client_run_iterate(client,50),"reverse iterate");
            }
        } else checked(UA_Client_connect(client,endpoint),"connect");
        UA_Variant value; UA_Variant_init(&value);
        checked(UA_Client_readValueAttribute(client,UA_NODEID_NUMERIC(0,UA_NS0ID_SERVER_SERVERSTATUS_STATE),&value),"read state");
        if(!UA_Variant_hasScalarType(&value,&UA_TYPES[UA_TYPES_INT32]) || *(UA_Int32*)value.data!=UA_SERVERSTATE_RUNNING) {
            fprintf(stderr,"ORACLE unexpected server state\n"); return 4;
        }
        printf("ORACLE read ServerStatus.State=Running policy=%s mode=%d reverse=%d\n",argv[2],mode,reverse);
        UA_Variant_clear(&value);
        checked(UA_Client_disconnect(client),"disconnect");
        UA_Client_delete(client);
    }
    UA_ByteString_clear(&cert); UA_ByteString_clear(&key); UA_ByteString_clear(&peer);
    return 0;
}
