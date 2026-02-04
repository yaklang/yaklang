#include <stdio.h>
#include <stdlib.h>

int main() {
    void *dst;
    char *str = "test";
    int value = 42;
    int64_t time_val = 1000;
    float float_val = 3.14f;
    double double_val = 2.718;
    
    // Pointer cast and assignment - the issue from cmdutils.c
    *(char **)dst = str;
    *(int *)dst = value;
    *(int64_t *)dst = time_val;
    *(float *)dst = float_val;
    *(double *)dst = double_val;
    
    // More complex pointer casts
    int **pptr;
    *(int **)dst = pptr;
    
    // Nested pointer casts
    struct node {
        int data;
        struct node *next;
    };
    struct node *node_ptr;
    *(struct node **)dst = node_ptr;
    
    return 0;
}

