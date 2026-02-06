#include <stdio.h>
#include <stdlib.h>

typedef struct {
    int x;
    int y;
} Point;

typedef struct {
    Point *points;
    int count;
} PointArray;

int main() {
    Point *p;
    PointArray *pa;
    Point points[10];
    
    // sizeof with pointer dereference
    size_t s1 = sizeof(*p);
    
    // sizeof with array element dereference
    size_t s2 = sizeof(*points);
    size_t s3 = sizeof(points[0]);
    
    // sizeof with struct member pointer dereference
    size_t s4 = sizeof(*pa->points);
    size_t s5 = sizeof(pa->points[0]);
    
    // sizeof with nested structure
    PointArray arrays[5];
    size_t s6 = sizeof(*arrays[0].points);
    
    // sizeof in malloc calls
    p = malloc(sizeof(*p));
    pa = malloc(sizeof(*pa));
    pa->points = malloc(sizeof(*pa->points) * 10);
    
    // sizeof with cast
    void *ptr = malloc(sizeof(*(Point*)ptr));
    
    return 0;
}

