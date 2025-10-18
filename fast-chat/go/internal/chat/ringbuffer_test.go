package chat

import (
	"sync"
	"testing"
)

func TestRingBufferConcurrentPush(t *testing.T) {
	rb := NewRingBuffer(1000)
	var wg sync.WaitGroup

	const goroutines = 100
	const messagesPerGoroutine = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := NewMessage(int64(id*1000+j), "user", "text")
				rb.Push(msg)
			}
		}(i)
	}

	wg.Wait()

	if rb.Count() != 1000 {
		t.Errorf("Expected 1000 messages, got %d", rb.Count())
	}

	msgs := rb.GetLast(1000)
	for i, msg := range msgs {
		if msg == nil {
			t.Errorf("Message %d is nil (race condition!)", i)
		}
	}
}

func TestRingBufferPushAndGet(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 5; i++ {
		msg := NewMessage(int64(i), "user", "text")
		rb.Push(msg)
	}

	if rb.Count() != 5 {
		t.Errorf("Expected 5 messages, got %d", rb.Count())
	}

	msgs := rb.GetLast(3)
	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].ID != 2 || msgs[1].ID != 3 || msgs[2].ID != 4 {
		t.Errorf("Got wrong messages: %v", msgs)
	}
}

func TestRingBufferOverflow(t *testing.T) {
	rb := NewRingBuffer(5)

	for i := 0; i < 10; i++ {
		msg := NewMessage(int64(i), "user", "text")
		rb.Push(msg)
	}

	if rb.Count() != 5 {
		t.Errorf("Expected 5 messages (buffer full), got %d", rb.Count())
	}

	msgs := rb.GetLast(5)
	if msgs[0].ID != 5 || msgs[4].ID != 9 {
		t.Errorf("Expected messages 5-9, got %d-%d", msgs[0].ID, msgs[4].ID)
	}
}

func TestRingBufferHistory(t *testing.T) {
	rb := NewRingBuffer(100)

	for i := 0; i < 50; i++ {
		msg := NewMessage(int64(i), "user", "text")
		rb.Push(msg)
	}

	msgs := rb.GetHistory(10, 5)
	if len(msgs) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(msgs))
	}

	if msgs[0].ID != 35 || msgs[4].ID != 39 {
		t.Errorf("Expected messages 35-39, got %d-%d", msgs[0].ID, msgs[4].ID)
	}
}
