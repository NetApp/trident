// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCountingRWMutex_Waiting(t *testing.T) {
	var m CountingRWMutex
	m.Lock()

	acquired := make(chan struct{})
	go func() {
		m.RLock()
		close(acquired)
		m.RUnlock()
	}()

	assert.Eventually(t, func() bool { return m.Waiting() == 1 }, time.Second, time.Millisecond)
	m.Unlock()
	<-acquired
	assert.Zero(t, m.Waiting())
}
