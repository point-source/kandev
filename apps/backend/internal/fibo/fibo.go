// Package fibo computes Fibonacci numbers via an iterative loop.
package fibo

// Fib returns the n-th Fibonacci number (Fib(0)=0, Fib(1)=1).
// Negative n is treated as 0.
func Fib(n int) uint64 {
	if n <= 0 {
		return 0
	}
	var a, b uint64 = 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}
