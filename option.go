package gwpool

import "time"

type Option func(*fixedWorkerPool)

func WithTaskQueueSize(size int) Option {
	return func(p *fixedWorkerPool) {
		p.queueCapacity = nearestPowerOfTwo(size)
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(p *fixedWorkerPool) {
		p.timeout = timeout
	}
}

func WithRetryCount(retry int) Option {
	return func(p *fixedWorkerPool) {
		p.retryCount = retry
	}
}

// nearestPowerOfTwo rounds up to the nearest power of two.
func nearestPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}
