// Development-only observation of Darwin libc boundaries used by Go 1.22.
// Not linked into SCA; no CGO production or test dependency is introduced.
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>
#include <stdio.h>
#include <stdarg.h>
#include <string.h>

#define INTERPOSE(new,old) __attribute__((used)) static struct {const void *replacement; const void *original;} interpose_##old __attribute__((section("__DATA,__interpose"))) = {(const void *)(unsigned long)&new, (const void *)(unsigned long)&old};
static int active;
static void event(const char *kind, const char *path) {
    if (!active) return;
    char line[9000];
    int n = snprintf(line, sizeof line, "SCA_AUDIT\t%s\t", kind);
    if (path) for (size_t i=0; path[i] && i<4096; i++) {
        static const char hex[]="0123456789abcdef";
        unsigned char c=path[i]; line[n++]=hex[c>>4]; line[n++]=hex[c&15];
    }
    line[n++]='\n';
    // One write per event, to the observer's inherited stderr pipe. No log
    // file is opened by the instrumented program; values/file contents aren't logged.
    (void)write(STDERR_FILENO,line,n);
}
__attribute__((constructor)) static void loaded(void) {active=1;event("loaded",NULL);}
static int watched_open(const char *p,int flags,...) {
    mode_t m=0;if(flags&O_CREAT){va_list a;va_start(a,flags);m=va_arg(a,int);va_end(a);}
    event((flags&(O_WRONLY|O_RDWR|O_CREAT|O_TRUNC|O_APPEND))?"write-open":"read-open",p);
    return open(p,flags,m);
}
INTERPOSE(watched_open,open)
static int watched_openat(int fd,const char *p,int flags,...) {
    mode_t m=0;if(flags&O_CREAT){va_list a;va_start(a,flags);m=va_arg(a,int);va_end(a);}
    event((flags&(O_WRONLY|O_RDWR|O_CREAT|O_TRUNC|O_APPEND))?"write-openat":"read-openat",p);
    return openat(fd,p,flags,m);
}
INTERPOSE(watched_openat,openat)
static int watched_socket(int d,int t,int p){event("socket",NULL);return socket(d,t,p);}
INTERPOSE(watched_socket,socket)
static int watched_connect(int s,const struct sockaddr *a,socklen_t n){event("connect",NULL);return connect(s,a,n);}
INTERPOSE(watched_connect,connect)
static int watched_bind(int s,const struct sockaddr *a,socklen_t n){event("bind",NULL);return bind(s,a,n);}
INTERPOSE(watched_bind,bind)
static int watched_listen(int s,int n){event("listen",NULL);return listen(s,n);}
INTERPOSE(watched_listen,listen)
static ssize_t watched_sendto(int s,const void*b,size_t n,int f,const struct sockaddr*a,socklen_t l){event("sendto",NULL);return sendto(s,b,n,f,a,l);}
INTERPOSE(watched_sendto,sendto)
static pid_t watched_fork(void){event("fork",NULL);return fork();}
INTERPOSE(watched_fork,fork)
static int watched_execve(const char*p,char *const a[],char *const e[]){event("execve",p);return execve(p,a,e);}
INTERPOSE(watched_execve,execve)
static int watched_stat(const char*p,struct stat*s){event("stat",p);return stat(p,s);}
INTERPOSE(watched_stat,stat)
static int watched_lstat(const char*p,struct stat*s){event("lstat",p);return lstat(p,s);}
INTERPOSE(watched_lstat,lstat)
static int watched_fstatat(int fd,const char*p,struct stat*s,int f){event("fstatat",p);return fstatat(fd,p,s,f);}
INTERPOSE(watched_fstatat,fstatat)
static int watched_access(const char*p,int m){event("access",p);return access(p,m);}
INTERPOSE(watched_access,access)
static ssize_t watched_readlink(const char*p,char*b,size_t n){event("readlink",p);return readlink(p,b,n);}
INTERPOSE(watched_readlink,readlink)
static int watched_mkdir(const char*p,mode_t m){event("write-mkdir",p);return mkdir(p,m);}
INTERPOSE(watched_mkdir,mkdir)
static int watched_unlink(const char*p){event("write-unlink",p);return unlink(p);}
INTERPOSE(watched_unlink,unlink)
static int watched_unlinkat(int d,const char*p,int f){event("write-unlinkat",p);return unlinkat(d,p,f);}
INTERPOSE(watched_unlinkat,unlinkat)
static int watched_rename(const char*a,const char*b){event("write-rename-from",a);event("write-rename-to",b);return rename(a,b);}
INTERPOSE(watched_rename,rename)
static int watched_symlink(const char*a,const char*b){event("write-symlink",b);return symlink(a,b);}
INTERPOSE(watched_symlink,symlink)
static int watched_truncate(const char*p,off_t n){event("write-truncate",p);return truncate(p,n);}
INTERPOSE(watched_truncate,truncate)
static int watched_ftruncate(int d,off_t n){event("write-ftruncate",NULL);return ftruncate(d,n);}
INTERPOSE(watched_ftruncate,ftruncate)
