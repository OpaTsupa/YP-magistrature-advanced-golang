package storage

import (
	"sync"
	"testing"
)

func TestSaveResolveList(t *testing.T) {
	m := NewMemory()
	if !m.Save("x1", "alice", "https://example.com/one", "http://localhost:8080/x1") {
		t.Fatal("first save should succeed")
	}
	if m.Save("x1", "bob", "https://example.com/two", "http://localhost:8080/x1") {
		t.Fatal("duplicate short ID must be rejected")
	}
	got, ok := m.Resolve("x1")
	if !ok || got != "https://example.com/one" {
		t.Fatalf("resolve: %q, %v", got, ok)
	}
	if _, ok := m.Resolve("missing"); ok {
		t.Fatal("missing ID unexpectedly resolved")
	}
	alice := m.List("alice")
	if len(alice) != 1 || alice[0].OriginalURL != got {
		t.Fatalf("unexpected Alice list: %#v", alice)
	}
	if len(m.List("bob")) != 0 {
		t.Fatal("Bob must have no links")
	}
	alice[0].OriginalURL = "tampered"
	if m.List("alice")[0].OriginalURL != got {
		t.Fatal("List must return a copy")
	}
}

func TestConcurrentSaves(t *testing.T) {
	m := NewMemory()
	const count = 100
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune(i + 1000))
			if !m.Save(id, "alice", "https://example.com", "http://localhost:8080/"+id) {
				t.Errorf("unexpected duplicate for %q", id)
			}
		}(i)
	}
	wg.Wait()
	if got := len(m.List("alice")); got != count {
		t.Fatalf("got %d links; want %d", got, count)
	}
}
