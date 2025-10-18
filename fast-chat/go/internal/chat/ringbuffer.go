package chat

import (
	"sync"
)

type RingBuffer struct {
	buffer []*Message
	size   int
	head   int
	tail   int
	count  int
	mu     sync.Mutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]*Message, capacity),
		size:   capacity,
	}
}

func (rb *RingBuffer) Push(msg *Message) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buffer[rb.head] = msg
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	} else {
		rb.tail = (rb.tail + 1) % rb.size
	}
}

func (rb *RingBuffer) GetLast(n int) []*Message {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	if n > rb.count {
		n = rb.count
	}

	result := make([]*Message, n)
	startPos := (rb.head - n + rb.size) % rb.size

	for i := 0; i < n; i++ {
		pos := (startPos + i) % rb.size
		result[i] = rb.buffer[pos]
	}

	return result
}

func (rb *RingBuffer) GetHistory(skip, take int) []*Message {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	end := rb.count - skip
	if end <= 0 {
		return nil
	}

	start := end - take
	if start < 0 {
		start = 0
	}

	actualTake := end - start
	result := make([]*Message, actualTake)

	startPos := (rb.head - rb.count + start + rb.size) % rb.size

	for i := 0; i < actualTake; i++ {
		pos := (startPos + i) % rb.size
		result[i] = rb.buffer[pos]
	}

	return result
}

func (rb *RingBuffer) Count() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *RingBuffer) Capacity() int {
	return rb.size
}
