// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_syncpool

import "sync"

// Pool is a generic wrapper around sync.Pool that provides type-safe Get and Put methods.
// This eliminates the need for type assertions in the calling code and allows the compiler
// to optimize better since exact types are known at compile time.
//
// Example usage:
//
//	type MyData struct { /* ... */ }
//	pool := utils.NewPool(func() *MyData { return &MyData{} })
//	data := pool.Get()  // Returns *MyData, no type assertion needed
//	// ... use data ...
//	pool.Put(data)
type Pool[T any] struct {
	sync.Pool
}

// Get retrieves a value from the pool and returns it with the correct type.
// The type assertion is hidden inside this method, keeping calling code clean.
func (p *Pool[T]) Get() T {
	return p.Pool.Get().(T)
}

// Put adds a value back to the pool for reuse.
func (p *Pool[T]) Put(x T) {
	p.Pool.Put(x)
}

// NewPool creates a new generic Pool with the given constructor function.
// The constructor is called when the pool is empty and needs to create a new value.
func NewPool[T any](newF func() T) *Pool[T] {
	return &Pool[T]{
		Pool: sync.Pool{
			New: func() any {
				return newF()
			},
		},
	}
}
