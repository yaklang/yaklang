/* Build: gcc -O1 -fno-pie -no-pie -o hello hello.c */
#include <stdio.h>

int main(void) {
    puts("hello, reify");
    return 0;
}
