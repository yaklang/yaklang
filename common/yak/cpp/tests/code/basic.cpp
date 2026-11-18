#include <cstdio>

int add(int a, int b) {
    return a + b;
}

int main() {
    int x = 1;
    x = add(x, 2);
    if (x > 2) {
        x = x - 1;
    } else {
        x = x + 1;
    }
    for (int i = 0; i < 3; i++) {
        x += i;
    }
    while (x < 10) {
        x++;
    }
    switch (x) {
        case 1:
            x = 2;
            break;
        default:
            x = 3;
    }
    printf("%d\n", x);
    return 0;
}
