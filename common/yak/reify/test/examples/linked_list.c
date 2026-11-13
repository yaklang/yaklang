/* Build: gcc -O0 -fno-pie -no-pie -fstack-protector-strong -o linked_list linked_list.c */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct Point {
    int x;
    int y;
} Point;

typedef struct Node {
    struct Node *next;
    Point point;
    char name[16];
} Node;

static int sum_points(const Point *points, int count) {
    int total = 0;
    for (int i = 0; i < count; i++) {
        total += points[i].x + points[i].y;
    }
    return total;
}

static Node *make_node(const Point *point, const char *name) {
    Node *node = (Node *)malloc(sizeof(Node));
    if (node == NULL) {
        return NULL;
    }
    node->next = NULL;
    node->point = *point;
    strncpy(node->name, name, sizeof(node->name) - 1);
    node->name[sizeof(node->name) - 1] = '\0';
    return node;
}

static void free_list(Node *head) {
    while (head != NULL) {
        Node *next = head->next;
        free(head);
        head = next;
    }
}

int main(void) {
    Point points[3] = {{1, 2}, {3, 4}, {5, 6}};
    int total = sum_points(points, 3);
    Node *head = NULL;

    for (int i = 0; i < 3; i++) {
        Node *node = make_node(&points[i], "point");
        if (node == NULL) {
            free_list(head);
            return 1;
        }
        node->next = head;
        head = node;
    }

    if (total > 10 && head != NULL) {
        printf("total=%d name=%s\n", total, head->name);
    } else {
        printf("total=%d\n", total);
    }

    free_list(head);
    return 0;
}
