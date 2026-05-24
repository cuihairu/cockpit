package protocol

import (
	"strings"
	"sync"
	"testing"
)

func TestGenerateID(t *testing.T) {
	id := GenerateID()

	if id == "" {
		t.Error("GenerateID() should return non-empty string")
	}

	// ID should have two parts separated by hyphen
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		t.Errorf("ID should have 2 parts separated by hyphen, got %d parts", len(parts))
	}
}

func TestGenerateIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	numIDs := 100

	for i := 0; i < numIDs; i++ {
		id := GenerateID()
		if ids[id] {
			t.Errorf("GenerateID() generated duplicate ID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != numIDs {
		t.Errorf("Expected %d unique IDs, got %d", numIDs, len(ids))
	}
}

func TestGenerateIDConcurrent(t *testing.T) {
	const goroutines = 100
	idsPerGoroutine := 10

	ids := make(chan string, goroutines*idsPerGoroutine)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				ids <- GenerateID()
			}
		}()
	}

	wg.Wait()
	close(ids)

	uniqueIDs := make(map[string]bool)
	for id := range ids {
		if uniqueIDs[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		uniqueIDs[id] = true
	}

	expectedCount := goroutines * idsPerGoroutine
	if len(uniqueIDs) != expectedCount {
		t.Errorf("Expected %d unique IDs, got %d", expectedCount, len(uniqueIDs))
	}
}

func TestGenerateIDWithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "with prefix",
			prefix: "agent",
		},
		{
			name:   "with dashed prefix",
			prefix: "my-agent",
		},
		{
			name:   "empty prefix",
			prefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := GenerateIDWithPrefix(tt.prefix)

			if tt.prefix != "" {
				if !strings.HasPrefix(id, tt.prefix+"-") {
					t.Errorf("ID should start with prefix '%s-', got %s", tt.prefix, id)
				}
			}

			if id == "" {
				t.Error("GenerateIDWithPrefix() should return non-empty string")
			}
		})
	}
}

func TestGenerateIDWithPrefixUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	prefix := "test"

	for i := 0; i < 100; i++ {
		id := GenerateIDWithPrefix(prefix)
		if ids[id] {
			t.Errorf("GenerateIDWithPrefix() generated duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestFormatID(t *testing.T) {
	tests := []struct {
		name   string
		count  uint64
		random string
	}{
		{
			name:   "zero count",
			count:  0,
			random: "abcd1234",
		},
		{
			name:   "small count",
			count:  1,
			random: "abcd1234",
		},
		{
			name:   "large count",
			count:  0xFFFFFFFF,
			random: "abcd1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := formatID(tt.count, tt.random)

			parts := strings.Split(id, "-")
			if len(parts) != 2 {
				t.Errorf("formatID() should return 2 parts, got %d", len(parts))
			}

			if parts[1] != tt.random {
				t.Errorf("random part = %s, want %s", parts[1], tt.random)
			}

			// Count part should be hex encoded
			if len(parts[0]) != 8 { // 4 bytes = 8 hex chars
				t.Errorf("count part length = %d, want 8", len(parts[0]))
			}
		})
	}
}

func TestIDCounterIncrement(t *testing.T) {
	// Get multiple IDs and verify counter is incrementing
	id1 := GenerateID()
	id2 := GenerateID()
	id3 := GenerateID()

	if id1 == id2 {
		t.Error("IDs should be unique")
	}

	if id2 == id3 {
		t.Error("IDs should be unique")
	}

	if id1 == id3 {
		t.Error("IDs should be unique")
	}
}

func TestIDStructure(t *testing.T) {
	id := GenerateID()

	// Check format: hex(8 bytes) - hex(8 bytes)
	// The count part is encoded from 4 bytes
	parts := strings.Split(id, "-")

	if len(parts) != 2 {
		t.Fatalf("ID should have format 'count-random', got: %s", id)
	}

	countPart := parts[0]
	randomPart := parts[1]

	// Count part should be valid hex (8 chars from 4 bytes)
	if len(countPart) != 8 {
		t.Errorf("Count part should be 8 hex chars, got length %d: %s", len(countPart), countPart)
	}

	// Random part should be valid hex (8 chars from 4 bytes)
	if len(randomPart) != 8 {
		t.Errorf("Random part should be 8 hex chars, got length %d: %s", len(randomPart), randomPart)
	}
}

func BenchmarkGenerateID(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		GenerateID()
	}
}

func BenchmarkGenerateIDWithPrefix(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		GenerateIDWithPrefix("agent")
	}
}

func BenchmarkGenerateIDParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GenerateID()
		}
	})
}
