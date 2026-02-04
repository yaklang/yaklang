#include <stdio.h>
#include <stdint.h>

struct node {
    int data;
    struct node *next;
};

int main() {
    int x;
    int *ptr;
    int arr[10];
    struct node *head;
    
    // sizeof with type names (should work)
    size_t t1 = sizeof(int);
    size_t t2 = sizeof(struct node);
    size_t t3 = sizeof(int*);
    size_t t4 = sizeof(int[10]);
    
    // sizeof with expressions (should work after our fix)
    size_t e1 = sizeof(x);
    size_t e2 = sizeof(*ptr);
    size_t e3 = sizeof(arr);
    size_t e4 = sizeof(*arr);
    size_t e5 = sizeof(head->data);
    size_t e6 = sizeof(*head);
    
    // sizeof with parentheses and expressions
    size_t e7 = sizeof((x));
    size_t e8 = sizeof((*ptr));
    size_t e9 = sizeof((arr[0]));
    
    // sizeof without parentheses (expression form)
    size_t e10 = sizeof x;
    size_t e11 = sizeof *ptr;
    size_t e12 = sizeof arr;
    
    return 0;
}

