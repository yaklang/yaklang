// The system sandbox-exec strips DYLD variables. Set them only after entering
// the sandbox, in this unprivileged, locally built observer launcher.
#include <stdlib.h>
#include <unistd.h>
int main(int argc,char **argv) {
    if(argc<3) return 2;
    if(setenv("DYLD_INSERT_LIBRARIES",argv[1],1)) return 3;
    execv(argv[2],argv+2);
    return 4;
}
