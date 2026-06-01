#include <stdio.h>
#include <stdlib.h>

unsigned long long fibonacci(unsigned int n) {
    unsigned long long a = 0, b = 1, t;
    for (unsigned int i = 0; i < n; i++) {
        t = a + b;
        a = b;
        b = t;
    }
    return a;
}

int main(int argc, char *argv[]) {
    unsigned int n = (argc > 1) ? (unsigned int)atoi(argv[1]) : 10;
    for (unsigned int i = 0; i <= n; i++) {
        printf("fib(%u) = %llu\n", i, fibonacci(i));
    }
    return 0;
}
