#include <stdio.h>
#include <string.h>

static int score_for(const char *name) {
    if (strcmp(name, "point") == 0) {
        return 4;
    }
    if (strcmp(name, "int64") == 0) {
        return 6;
    }
    if (strcmp(name, "source") == 0) {
        return 8;
    }
    return 0;
}

int main(void) {
    int total = score_for("point") + score_for("int64") + score_for("source");
    printf("total=%d\n", total);
    return total > 10 ? 0 : 1;
}
