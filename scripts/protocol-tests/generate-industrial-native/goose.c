/* SPDX-License-Identifier: CC0-1.0
 * libiec61850 creates/consumes the entire L2 frame via its native HAL. */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "goose_publisher.h"
#include "goose_receiver.h"
#include "goose_subscriber.h"
#include "mms_value.h"
#include "hal_thread.h"
static volatile int received = 0;
static void listener(GooseSubscriber s, void *unused) {
    (void)unused;
    char text[1024]; MmsValue_printToBuffer(GooseSubscriber_getDataSetValues(s),text,sizeof(text));
    printf("ORACLE valid=%d stNum=%u sqNum=%u values=%s\n",GooseSubscriber_isValid(s),GooseSubscriber_getStNum(s),GooseSubscriber_getSqNum(s),text);
    fflush(stdout);
    if(GooseSubscriber_isValid(s)) received++;
}
int main(int argc, char **argv) {
    if(argc!=4) return 2;
    int variant=atoi(argv[3]);
    if(!strcmp(argv[1],"subscribe")) {
        GooseReceiver r=GooseReceiver_create(); GooseReceiver_setInterfaceId(r,argv[2]);
        GooseSubscriber s=GooseSubscriber_create("pr5013/LLN0$GO$native",NULL);
        GooseSubscriber_setAppId(s,1000); GooseSubscriber_setListener(s,listener,NULL);
        GooseReceiver_addSubscriber(r,s); GooseReceiver_start(r);
        if(!GooseReceiver_isRunning(r)) return 3;
        for(int i=0;i<50 && received<3;i++) Thread_sleep(100);
        GooseReceiver_stop(r); GooseReceiver_destroy(r); return received==3?0:4;
    }
    CommParameters p={0}; p.appId=1000;
    uint8_t dst[6]={1,12,205,1,0,1}; memcpy(p.dstAddress,dst,6);
    p.vlanId=variant==2?10:0; p.vlanPriority=4;
    GoosePublisher publisher=GoosePublisher_createEx(&p,argv[2],variant!=0);
    if(!publisher) return 5;
    GoosePublisher_setGoCbRef(publisher,"pr5013/LLN0$GO$native");
    GoosePublisher_setDataSetRef(publisher,"pr5013/LLN0$nativeData");
    if(variant==2) GoosePublisher_setGoID(publisher,"native-simulation");
    GoosePublisher_setConfRev(publisher,7);
    GoosePublisher_setTimeAllowedToLive(publisher,5000);
    GoosePublisher_setSimulation(publisher,variant==2);
    GoosePublisher_setNeedsCommission(publisher,variant==2);
    LinkedList values=LinkedList_create();
    LinkedList_add(values,MmsValue_newIntegerFromInt32(1234));
    LinkedList_add(values,MmsValue_newBoolean(true));
    LinkedList_add(values,MmsValue_newVisibleString("native-libiec61850"));
    for(int i=0;i<3;i++) { if(GoosePublisher_publish(publisher,values)) return 6; Thread_sleep(100); }
    GoosePublisher_destroy(publisher); LinkedList_destroyDeep(values,(LinkedListValueDeleteFunction)MmsValue_delete);
    return 0;
}
