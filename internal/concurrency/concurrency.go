// Package concurrency 提供命令层复用的并发原语。
//
// 目前提供 RunIndexed：一个固定大小的 worker 池，把 0..n-1 的索引分发给
// 最多 maxWorkers 个 worker 并行处理。它抽取自 cmd/install.go 与
// cmd/update.go 中两段几乎逐字相同的"channel 分发索引 + WaitGroup"样板。
//
// Input: runtime, sync
// Output: func RunIndexed
// Pos: 基础层-并发原语
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package concurrency

import (
	"runtime"
	"sync"
)

// RunIndexed 用固定大小的 worker 池并行处理 0..n-1 的索引。
//
// worker 数 = clamp(runtime.NumCPU(), 1, min(maxWorkers, n))：
// I/O/网络密集型任务允许真实并行，但封顶 maxWorkers（通常 8）避免压垮
// 主机或触发远端限流；任务数不足时按任务数缩减，至少 1 个 worker。
//
// fn 在不同 worker 中并发执行，调用方自行负责对外部共享状态（结果数组、
// 标准输出）的同步（典型做法是 sync.Mutex 序列化输出）。
//
// 阻塞直到所有索引处理完毕。
func RunIndexed(n, maxWorkers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	workers := clamp(runtime.NumCPU(), 1, min(maxWorkers, n))

	var wg sync.WaitGroup
	jobCh := make(chan int, workers) // 带缓冲：省去独立的分发 goroutine
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range jobCh {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobCh <- i
	}
	close(jobCh)
	wg.Wait()
}

// clamp 限制 v 到 [lo, hi]。
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
