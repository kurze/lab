package chat

import (
	"sync"
	"testing"
)

func TestNicknamePoolAllocation(t *testing.T) {
	pool := NewNicknamePool()

	nick1 := pool.Allocate()
	nick2 := pool.Allocate()

	if nick1 == "" || nick2 == "" {
		t.Error("Allocated nicknames should not be empty")
	}

	if nick1 == nick2 {
		t.Error("Allocated nicknames should be unique")
	}

	if pool.Used() != 2 {
		t.Errorf("Expected 2 used nicknames, got %d", pool.Used())
	}
}

func TestNicknamePoolRelease(t *testing.T) {
	pool := NewNicknamePool()

	nick := pool.Allocate()
	initialUsed := pool.Used()
	initialAvailable := pool.Available()

	pool.Release(nick)

	if pool.Used() != initialUsed-1 {
		t.Errorf("Expected used count to decrease by 1")
	}

	if pool.Available() != initialAvailable+1 {
		t.Errorf("Expected available count to increase by 1")
	}

	nick2 := pool.Allocate()
	if nick2 != nick {
		t.Errorf("Released nickname should be reused, got '%s' instead of '%s'", nick2, nick)
	}
}

func TestNicknamePoolDoubleRelease(t *testing.T) {
	pool := NewNicknamePool()

	nick := pool.Allocate()
	pool.Release(nick)

	availableBefore := pool.Available()
	usedBefore := pool.Used()

	pool.Release(nick)

	if pool.Available() != availableBefore || pool.Used() != usedBefore {
		t.Error("Double release should be no-op")
	}
}

func TestNicknamePoolConcurrentAllocation(t *testing.T) {
	pool := NewNicknamePool()

	const numGoroutines = 100
	var wg sync.WaitGroup
	nicknames := make([]string, numGoroutines)
	mu := sync.Mutex{}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nick := pool.Allocate()
			mu.Lock()
			nicknames[id] = nick
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	seen := make(map[string]bool)
	for _, nick := range nicknames {
		if nick == "" {
			t.Error("Got empty nickname")
			continue
		}
		if seen[nick] {
			t.Errorf("Duplicate nickname allocated: %s", nick)
		}
		seen[nick] = true
	}

	if len(seen) != numGoroutines {
		t.Errorf("Expected %d unique nicknames, got %d", numGoroutines, len(seen))
	}
}

func TestNicknamePoolConcurrentReleaseAndAllocate(t *testing.T) {
	pool := NewNicknamePool()

	const numOperations = 1000
	var wg sync.WaitGroup

	for i := 0; i < numOperations; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			nick := pool.Allocate()
			if nick == "" {
				t.Error("Got empty nickname")
			}
		}()

		go func() {
			defer wg.Done()
			nick := pool.Allocate()
			pool.Release(nick)
		}()
	}

	wg.Wait()

	if pool.Used() < numOperations {
		t.Logf("Used: %d (some allocations were released)", pool.Used())
	}
}

func TestNicknamePoolFormat(t *testing.T) {
	pool := NewNicknamePool()

	for i := 0; i < 10; i++ {
		nick := pool.Allocate()

		if len(nick) < 3 {
			t.Errorf("Nickname too short: %s", nick)
		}

		if len(nick) > 50 {
			t.Errorf("Nickname too long: %s", nick)
		}

		parts := splitNickname(nick)
		if len(parts) != 2 {
			t.Errorf("Nickname should have format 'adjective_name', got: %s", nick)
		}
	}
}

func TestNicknamePoolShuffleOptimization(t *testing.T) {
	pool := NewNicknamePool()

	totalCombinations := len(adjectives) * len(names)
	halfCombinations := totalCombinations / 2

	for i := 0; i < halfCombinations+10; i++ {
		pool.Allocate()
	}

	collisionProbability := float64(pool.Used()) / float64(totalCombinations)
	if collisionProbability > 0.5 {
		t.Logf("Collision probability: %.2f%% - shuffle should be triggered", collisionProbability*100)
	}

	moreNicks := make([]string, 100)
	for i := 0; i < 100; i++ {
		moreNicks[i] = pool.Allocate()
	}

	for i, nick := range moreNicks {
		if nick == "" {
			t.Errorf("Nickname %d is empty after shuffle optimization", i)
		}
	}
}

func TestNicknamePoolExhaustion(t *testing.T) {
	pool := NewNicknamePool()

	totalCombinations := len(adjectives) * len(names)

	allocated := make([]string, 0, totalCombinations)
	seen := make(map[string]bool)

	for i := 0; i < totalCombinations; i++ {
		nick := pool.Allocate()
		if seen[nick] {
			t.Logf("Duplicate at position %d: %s (expected due to exhaustion)", i, nick)
			break
		}
		seen[nick] = true
		allocated = append(allocated, nick)
	}

	t.Logf("Allocated %d unique nicknames out of %d possible", len(allocated), totalCombinations)
}

func TestNicknamePoolStats(t *testing.T) {
	pool := NewNicknamePool()

	initialAvailable, initialUsed := pool.Available(), pool.Used()
	if initialUsed != 0 {
		t.Errorf("Initial used should be 0, got %d", initialUsed)
	}

	nick1 := pool.Allocate()
	nick2 := pool.Allocate()

	available, used := pool.Available(), pool.Used()
	if used != 2 {
		t.Errorf("Expected 2 used, got %d", used)
	}
	if available >= initialAvailable {
		t.Errorf("Available should decrease after allocation")
	}

	pool.Release(nick1)
	available2, used2 := pool.Available(), pool.Used()
	if used2 != 1 {
		t.Errorf("Expected 1 used after release, got %d", used2)
	}
	if available2 <= available {
		t.Errorf("Available should increase after release")
	}

	pool.Release(nick2)
	used3 := pool.Used()
	if used3 != 0 {
		t.Errorf("Expected 0 used after releasing all, got %d", used3)
	}
}

func splitNickname(nick string) []string {
	result := []string{}
	current := ""
	for _, ch := range nick {
		if ch == '_' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
