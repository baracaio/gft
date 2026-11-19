package handler_test

import (
	"context"
	"sync"

	"github.com/baracaio/gft/internal/domain"
	"github.com/baracaio/gft/internal/repository"
)

// mockRepo implements repository.Repository for testing. All method fields
// are optional — nil means "succeed silently". All calls are recorded and
// protected by a mutex so they are safe across the handler's goroutine pool.
type mockRepo struct {
	mu sync.Mutex

	onSaveClient  func(*domain.ClientMessage) error
	onSaveBlocks  func(*domain.BlockMessage) error
	onSaveFailure func(*repository.FailureRecord) error

	clientCalls  []*domain.ClientMessage
	blockCalls   []*domain.BlockMessage
	failureCalls []*repository.FailureRecord
}

func (m *mockRepo) SaveClient(_ context.Context, msg *domain.ClientMessage) error {
	m.mu.Lock()
	m.clientCalls = append(m.clientCalls, msg)
	fn := m.onSaveClient
	m.mu.Unlock()
	if fn != nil {
		return fn(msg)
	}
	return nil
}

func (m *mockRepo) SaveBlocks(_ context.Context, msg *domain.BlockMessage) error {
	m.mu.Lock()
	m.blockCalls = append(m.blockCalls, msg)
	fn := m.onSaveBlocks
	m.mu.Unlock()
	if fn != nil {
		return fn(msg)
	}
	return nil
}

func (m *mockRepo) SaveFailure(_ context.Context, f *repository.FailureRecord) error {
	m.mu.Lock()
	m.failureCalls = append(m.failureCalls, f)
	fn := m.onSaveFailure
	m.mu.Unlock()
	if fn != nil {
		return fn(f)
	}
	return nil
}
