// Package fibo computes Fibonacci numbers via top-down memoized recursion.
package fibo

// Fib returns the n-th Fibonacci number (Fib(0)=0, Fib(1)=1).
// Negative n is treated as 0.
func Fib(n int) uint64 {
	if n <= 0 {
		return 0
	}
	memo := make(map[int]uint64, n+1)
	return fibMemo(n, memo)
}

func fibMemo(n int, memo map[int]uint64) uint64 {
	if n < 2 {
		return uint64(n)
	}
	if v, ok := memo[n]; ok {
		return v
	}
	v := fibMemo(n-1, memo) + fibMemo(n-2, memo)
	memo[n] = v
	return v
}
