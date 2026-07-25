package concurrency

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunIndexedCoversAll(t *testing.T) {
	for _, n := range []int{0, 1, 5, 23} {
		var seen int64
		var mu sync.Mutex
		got := make(map[int]bool, n)
		RunIndexed(n, 8, func(i int) {
			mu.Lock()
			got[i] = true
			mu.Unlock()
			atomic.AddInt64(&seen, 1)
		})
		if int(seen) != n {
			t.Errorf("n=%d: fn called %d times, want %d", n, seen, n)
		}
		for i := 0; i < n; i++ {
			if !got[i] {
				t.Errorf("n=%d: index %d never invoked", n, i)
			}
		}
	}
}

func TestRunIndexedNoDuplicates(t *testing.T) {
	// 并发下每个索引应恰好被调用一次。
	const n = 100
	var counts [n]int64
	var mu sync.Mutex
	count := make(map[int]int)
	RunIndexed(n, 8, func(i int) {
		mu.Lock()
		count[i]++
		mu.Unlock()
		atomic.AddInt64(&counts[i], 1)
	})
	for i, c := range counts {
		if c != 1 {
			t.Errorf("index %d invoked %d times, want 1", i, c)
		}
	}
}

func TestRunIndexedWorkerCap(t *testing.T) {
	// maxWorkers=2 且 n 很大：函数仍应全部执行，不泄漏、不漏项。
	const n = 500
	seen := make(map[int]bool)
	var mu sync.Mutex
	RunIndexed(n, 2, func(i int) {
		mu.Lock()
		seen[i] = true
		mu.Unlock()
	})
	if len(seen) != n {
		t.Errorf("maxWorkers=2: saw %d indices, want %d", len(seen), n)
	}
}
