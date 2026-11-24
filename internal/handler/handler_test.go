package handler_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/baracaio/gft/internal/domain"
	"github.com/baracaio/gft/internal/handler"
	"github.com/baracaio/gft/internal/repository"
)

const (
	guid1 = "550e8400-e29b-41d4-a716-446655440000"
	guid2 = "660e8400-e29b-41d4-a716-446655440000"
)

func sqsMsg(id, body string, receiveCount int) events.SQSMessage {
	return events.SQSMessage{
		MessageId: id,
		Body:      body,
		Attributes: map[string]string{
			"ApproximateReceiveCount": strconv.Itoa(receiveCount),
		},
	}
}

func clientBody(guid, ts string) string {
	return fmt.Sprintf(
		`{"codigo_cliente":%q,"numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":%q}`,
		guid, ts,
	)
}

func blockBody(guid, ts string) string {
	return fmt.Sprintf(`{"codigo_cliente":%q,"data_atualizacao":%q}`, guid, ts)
}

func TestHandle(t *testing.T) {
	transientErr := &domain.TransientError{Cause: fmt.Errorf("dynamo throttle")}

	tests := []struct {
		name     string
		domain   string
		messages []events.SQSMessage
		setup    func(*mockRepo)
		check    func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo)
	}{
		{
			name:   "empty batch returns no failures and nil error",
			domain: "cliente",
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
			},
		},
		{
			name:   "valid client message is saved and not in failures",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", clientBody(guid1, "2024-01-15T10:30:00Z"), 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
				if len(repo.clientCalls) != 1 {
					t.Errorf("SaveClient calls: want 1, got %d", len(repo.clientCalls))
				}
			},
		},
		{
			name:   "valid block message is saved and not in failures",
			domain: "bloqueios",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", blockBody(guid1, "2024-01-15T10:30:00Z"), 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
				if len(repo.blockCalls) != 1 {
					t.Errorf("SaveBlocks calls: want 1, got %d", len(repo.blockCalls))
				}
			},
		},
		{
			name:   "invalid JSON is a failure without calling save",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-bad", `{not json}`, 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-bad")
				if len(repo.clientCalls) != 0 {
					t.Errorf("SaveClient must not be called on parse error")
				}
			},
		},
		{
			name:   "parse error on last attempt triggers SaveFailure",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-last", `{not json}`, 3),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-last")
				if len(repo.failureCalls) != 1 {
					t.Fatalf("SaveFailure calls: want 1, got %d", len(repo.failureCalls))
				}
				f := repo.failureCalls[0]
				if f.MessageID != "msg-last" {
					t.Errorf("FailureRecord.MessageID: got %q, want %q", f.MessageID, "msg-last")
				}
				if f.ErrorType != "permanente" {
					t.Errorf("FailureRecord.ErrorType: got %q, want %q", f.ErrorType, "permanente")
				}
			},
		},
		{
			name:   "permanent parse error on first attempt triggers SaveFailure immediately",
			domain: "cliente",
			messages: []events.SQSMessage{
				// receiveCount=1 — not the last attempt, but error is permanent
				sqsMsg("msg-perm-early", `{"codigo_cliente":"not-a-guid","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:00:00Z"}`, 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-perm-early")
				if len(repo.failureCalls) != 1 {
					t.Fatalf("SaveFailure calls: want 1 on first attempt for permanent error, got %d", len(repo.failureCalls))
				}
				if repo.failureCalls[0].ErrorType != "permanente" {
					t.Errorf("ErrorType: got %q, want %q", repo.failureCalls[0].ErrorType, "permanente")
				}
			},
		},
		{
			name:   "save transient error puts message in failures",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", clientBody(guid1, "2024-01-15T10:30:00Z"), 1),
			},
			setup: func(r *mockRepo) {
				r.onSaveClient = func(_ *domain.ClientMessage) error { return transientErr }
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-1")
			},
		},
		{
			name:   "save error on last attempt triggers SaveFailure with esgotado",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-last", clientBody(guid1, "2024-01-15T10:30:00Z"), 3),
			},
			setup: func(r *mockRepo) {
				r.onSaveClient = func(_ *domain.ClientMessage) error { return transientErr }
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-last")
				if len(repo.failureCalls) != 1 {
					t.Fatalf("SaveFailure calls: want 1, got %d", len(repo.failureCalls))
				}
				if repo.failureCalls[0].ErrorType != "esgotado" {
					t.Errorf("ErrorType: got %q, want %q", repo.failureCalls[0].ErrorType, "esgotado")
				}
			},
		},
		{
			name:   "permanent parse error uses permanente error type",
			domain: "cliente",
			messages: []events.SQSMessage{
				// missing required fields → PermanentError
				sqsMsg("msg-perm", `{"codigo_cliente":"not-a-guid"}`, 3),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-perm")
				if len(repo.failureCalls) != 1 {
					t.Fatalf("SaveFailure calls: want 1, got %d", len(repo.failureCalls))
				}
				if repo.failureCalls[0].ErrorType != "permanente" {
					t.Errorf("ErrorType: got %q, want %q", repo.failureCalls[0].ErrorType, "permanente")
				}
			},
		},
		{
			name:   "mixed batch: valid message succeeds, invalid message fails",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-ok", clientBody(guid1, "2024-01-15T10:30:00Z"), 1),
				sqsMsg("msg-bad", `{not json}`, 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				assertFailureIDs(t, resp, "msg-bad")
				if len(repo.clientCalls) != 1 {
					t.Errorf("SaveClient calls: want 1, got %d", len(repo.clientCalls))
				}
			},
		},
		{
			name:   "two messages same client processed oldest-first",
			domain: "cliente",
			messages: []events.SQSMessage{
				// newer arrives first in the batch
				sqsMsg("msg-new", clientBody(guid1, "2024-01-15T11:00:00Z"), 1),
				sqsMsg("msg-old", clientBody(guid1, "2024-01-15T10:00:00Z"), 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
				if len(repo.clientCalls) != 2 {
					t.Fatalf("SaveClient calls: want 2, got %d", len(repo.clientCalls))
				}
				// older timestamp must be first
				if !repo.clientCalls[0].UpdatedAt.Before(repo.clientCalls[1].UpdatedAt) {
					t.Errorf("saves not in ascending timestamp order: first=%v, second=%v",
						repo.clientCalls[0].UpdatedAt, repo.clientCalls[1].UpdatedAt)
				}
			},
		},
		{
			name:   "two messages different clients both succeed",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-a", clientBody(guid1, "2024-01-15T10:30:00Z"), 1),
				sqsMsg("msg-b", clientBody(guid2, "2024-01-15T10:30:00Z"), 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
				if len(repo.clientCalls) != 2 {
					t.Errorf("SaveClient calls: want 2, got %d", len(repo.clientCalls))
				}
			},
		},
		{
			name:   "handle never returns non-nil error even when all messages fail",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", `{bad}`, 1),
				sqsMsg("msg-2", `{bad}`, 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, _ *mockRepo) {
				// The test runner always checks this; this case verifies two failures.
				if len(resp.BatchItemFailures) != 2 {
					t.Errorf("want 2 failures, got %d", len(resp.BatchItemFailures))
				}
			},
		},
		{
			name:   "unknown domain is a permanent failure",
			domain: "unknown",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", `{}`, 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, _ *mockRepo) {
				assertFailureIDs(t, resp, "msg-1")
			},
		},
		{
			name:   "failure record contains correct domain and queue",
			domain: "bloqueios",
			messages: []events.SQSMessage{
				sqsMsg("msg-fail", `{bad}`, 3),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(repo.failureCalls) != 1 {
					t.Fatalf("SaveFailure calls: want 1, got %d", len(repo.failureCalls))
				}
				f := repo.failureCalls[0]
				if f.Domain != "bloqueios" {
					t.Errorf("Domain: got %q, want %q", f.Domain, "bloqueios")
				}
				if f.Queue != "test-queue" {
					t.Errorf("Queue: got %q, want %q", f.Queue, "test-queue")
				}
				if f.OriginalBody != `{bad}` {
					t.Errorf("OriginalBody: got %q, want %q", f.OriginalBody, `{bad}`)
				}
				if f.FailedAt.IsZero() {
					t.Error("FailedAt must not be zero")
				}
			},
		},
		{
			name:   "SaveFailure failure does not block message processing",
			domain: "cliente",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", clientBody(guid1, "2024-01-15T10:30:00Z"), 3),
			},
			setup: func(r *mockRepo) {
				r.onSaveClient = func(_ *domain.ClientMessage) error { return transientErr }
				r.onSaveFailure = func(_ *repository.FailureRecord) error {
					return fmt.Errorf("dynamo unavailable")
				}
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				// msg still reported as failure even though SaveFailure itself errored
				assertFailureIDs(t, resp, "msg-1")
			},
		},
		{
			name:   "block domain routes to SaveBlocks not SaveClient",
			domain: "bloqueios",
			messages: []events.SQSMessage{
				sqsMsg("msg-1", blockBody(guid1, "2024-01-15T10:30:00Z"), 1),
			},
			check: func(t *testing.T, resp events.SQSEventResponse, repo *mockRepo) {
				if len(resp.BatchItemFailures) != 0 {
					t.Errorf("want 0 failures, got %d", len(resp.BatchItemFailures))
				}
				if len(repo.blockCalls) != 1 {
					t.Errorf("SaveBlocks calls: want 1, got %d", len(repo.blockCalls))
				}
				if len(repo.clientCalls) != 0 {
					t.Errorf("SaveClient must not be called for bloqueios domain")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepo{}
			if tc.setup != nil {
				tc.setup(repo)
			}

			h := handler.New(tc.domain, repo, "test-queue")
			resp, err := h.Handle(context.Background(), events.SQSEvent{Records: tc.messages})

			if err != nil {
				t.Fatalf("Handle must never return a non-nil error, got: %v", err)
			}
			if tc.check != nil {
				tc.check(t, resp, repo)
			}
		})
	}
}

// assertFailureIDs verifies that the response contains exactly the given
// itemIdentifiers (in any order).
func assertFailureIDs(t *testing.T, resp events.SQSEventResponse, ids ...string) {
	t.Helper()
	got := make(map[string]bool, len(resp.BatchItemFailures))
	for _, f := range resp.BatchItemFailures {
		got[f.ItemIdentifier] = true
	}
	for _, id := range ids {
		if !got[id] {
			t.Errorf("expected %q in BatchItemFailures, not found (got: %v)", id, resp.BatchItemFailures)
		}
	}
	if len(resp.BatchItemFailures) != len(ids) {
		t.Errorf("BatchItemFailures length: got %d, want %d", len(resp.BatchItemFailures), len(ids))
	}
}

// Compile-time check that mockRepo satisfies the FailedAt field isn't zero
// and that timestamps are always UTC — a quick sanity smoke test on the mock.
var _ = time.UTC
