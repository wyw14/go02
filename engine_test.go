package lungo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEngineCloseUnblocksBlockedBegin(t *testing.T) {
	engine, err := CreateEngine(Options{Store: NewMemoryStore()})
	assert.NoError(t, err)

	// hold the token
	_, err = engine.Begin(nil, true)
	assert.NoError(t, err)

	// start blocked Begins
	const n = 4
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := engine.Begin(nil, true)
			errs <- err
		}()
	}

	// give them time to enter Acquire
	time.Sleep(50 * time.Millisecond)

	runWithin(t, 2*time.Second, "Engine.Close deadlocked", engine.Close)

	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			assert.Equal(t, ErrEngineClosed, err)
		case <-time.After(2 * time.Second):
			t.Fatal("blocked Begin never returned after Close")
		}
	}
}

func TestEngineBeginRejectedAfterClose(t *testing.T) {
	engine, err := CreateEngine(Options{Store: NewMemoryStore()})
	assert.NoError(t, err)
	engine.Close()

	_, err = engine.Begin(nil, true)
	assert.Equal(t, ErrEngineClosed, err)

	_, err = engine.Begin(nil, false)
	assert.Equal(t, ErrEngineClosed, err)
}

func TestStreamCloseAfterEngineClose(t *testing.T) {
	engine, err := CreateEngine(Options{Store: NewMemoryStore()})
	assert.NoError(t, err)

	stream, err := engine.Watch(Handle{"db", "coll"}, nil, nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, stream)

	engine.Close()

	// must not panic
	assert.NoError(t, stream.Close(nil))
	// idempotent
	assert.NoError(t, stream.Close(nil))
}
