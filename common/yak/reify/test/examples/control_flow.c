/* Build: gcc -O0 -fno-pie -no-pie -fno-stack-protector -o control_flow control_flow.c */
#include <stdio.h>

static int classify(int value) {
    if (value > 10) {
        return 1;
    } else if (value == 10) {
        return 0;
    } else {
        return -1;
    }
}

static int accumulate(void) {
    int total = 0;
    for (int i = 0; i < 5; i++) {
        total += classify(i * 4);
    }
    return total;
}

int main(void) {
    int total = accumulate();
    if (total > 10) {
        printf("total=%d\n", total);
    } else {
        printf("small=%d\n", total);
    }
    return 0;
}
