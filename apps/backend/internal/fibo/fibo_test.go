package fibo

import "testing"

func TestFib(t *testing.T) {
	cases := []struct {
		n    int
		want uint64
	}{
		{-1, 0},
		{0, 0},
		{1, 1},
		{2, 1},
		{3, 2},
		{10, 55},
		{20, 6765},
		{50, 12586269025},
	}
	for _, c := range cases {
		if got := Fib(c.n); got != c.want {
			t.Errorf("Fib(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}
