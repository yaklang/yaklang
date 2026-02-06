#include <stdio.h>
#include <stdlib.h>

struct filter_graph {
    int index;
    struct output **outputs;
    struct input **inputs;
    int nb_outputs;
    int nb_inputs;
};

struct output {
    int ost;
    struct filter_graph *graph;
};

int main() {
    struct filter_graph *fg;
    
    // This is the exact pattern from the error: sizeof(*fg->outputs[0])
    if (!(fg->outputs[0] = malloc(sizeof(*fg->outputs[0])))) {
        return 1;
    }
    
    // Similar pattern with inputs
    if (!(fg->inputs[0] = malloc(sizeof(*fg->inputs[0])))) {
        return 1;
    }
    
    // Nested array access
    if (!(fg->inputs[fg->nb_inputs - 1] = malloc(sizeof(*fg->inputs[0])))) {
        return 1;
    }
    
    // Multiple levels of indirection
    struct output ***output_array;
    if (!(output_array[0][0] = malloc(sizeof(*output_array[0][0])))) {
        return 1;
    }
    
    return 0;
}

