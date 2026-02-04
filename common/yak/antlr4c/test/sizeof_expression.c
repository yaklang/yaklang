#include <stdio.h>
#include <stdlib.h>

struct test_struct {
    int value;
    char *name;
};

int main() {
    int *ptr;
    int arr[10];
    struct test_struct *obj;
    struct test_struct objects[5];
    
    // sizeof with dereferenced pointer
    size_t size1 = sizeof(*ptr);
    
    // sizeof with dereferenced array element (the specific issue we fixed)
    size_t size2 = sizeof(*arr);
    size_t size3 = sizeof(*objects);
    
    // sizeof with nested array access and dereference
    struct test_struct **ptr_array;
    size_t size4 = sizeof(*ptr_array[0]);
    
    // sizeof with struct member access
    size_t size5 = sizeof(obj->value);
    size_t size6 = sizeof(objects[0].name);
    
    // sizeof with complex expression
    size_t size7 = sizeof(*((struct test_struct*)ptr));
    
    // sizeof with function call result (if applicable)
    void *mem = malloc(sizeof(*obj));
    
    // sizeof with nested pointer dereference
    int **pptr;
    size_t size8 = sizeof(**pptr);
    
    // sizeof with array indexing and dereference
    int *arr_ptr[10];
    size_t size9 = sizeof(*arr_ptr[0]);
    
    return 0;
}

