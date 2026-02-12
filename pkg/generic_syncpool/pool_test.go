// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_syncpool

import (
	"testing"
)

type testData struct {
	Value int
	Name  string
}

func TestPool_BasicUsage(t *testing.T) {
	pool := NewPool(func() *testData {
		return &testData{Value: 0, Name: ""}
	})

	// Get from pool
	data := pool.Get()
	if data == nil {
		t.Fatal("Expected non-nil data from pool")
	}

	// Use the data
	data.Value = 42
	data.Name = "test"

	// Put back to pool
	pool.Put(data)

	// Get again - should reuse
	data2 := pool.Get()
	if data2 == nil {
		t.Fatal("Expected non-nil data from pool")
	}

	// Note: We can't guarantee it's the same instance due to sync.Pool behavior,
	// but we can verify the type is correct and it works
	data2.Value = 100
	pool.Put(data2)
}

func TestPool_TypeSafety(t *testing.T) {
	// This test demonstrates compile-time type safety
	pool := NewPool(func() *testData {
		return &testData{Value: 1, Name: "default"}
	})

	// Get returns exactly *testData, no type assertion needed
	data := pool.Get()

	// We can directly access fields without type assertion
	data.Value = 42
	data.Name = "typed"

	if data.Value != 42 {
		t.Errorf("Expected Value=42, got %d", data.Value)
	}
	if data.Name != "typed" {
		t.Errorf("Expected Name='typed', got %s", data.Name)
	}

	pool.Put(data)
}

func TestPool_WithSlice(t *testing.T) {
	// Example: pooling slices
	type sliceWrapper struct {
		data []int
	}

	pool := NewPool(func() *sliceWrapper {
		return &sliceWrapper{
			data: make([]int, 0, 16),
		}
	})

	wrapper := pool.Get()

	// Use the slice
	wrapper.data = append(wrapper.data, 1, 2, 3)
	if len(wrapper.data) != 3 {
		t.Errorf("Expected length 3, got %d", len(wrapper.data))
	}

	// Clear before returning to pool
	wrapper.data = wrapper.data[:0]
	pool.Put(wrapper)

	// Get again and verify it's reset
	wrapper2 := pool.Get()
	if len(wrapper2.data) > 0 {
		t.Errorf("Expected empty slice, got length %d", len(wrapper2.data))
	}
	pool.Put(wrapper2)
}
