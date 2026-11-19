package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/baracaio/gft/internal/domain"
	"github.com/baracaio/gft/internal/repository"
)

const defaultWorkers = 10

// parsedMsg holds the result of parsing one SQS message.
// Exactly one of client/block is non-nil on success; err is non-nil on failure.
type parsedMsg struct {
	sqsID        string
	body         string
	receiveCount int
	clientCode   string
	updatedAt    time.Time
	client       *domain.ClientMessage
	block        *domain.BlockMessage
	err          error
}

func (m *parsedMsg) isLastAttempt() bool { return m.receiveCount >= 3 }

// Handler processes SQS batches for one domain (selected at construction time).
type Handler struct {
	domainName string
	repo       repository.Repository
	queue      string
	workers    int
}

// New creates a Handler for the given domain. domainName must be "cliente" or
// "bloqueios" — this matches the DOMAIN env var set on the Lambda function.
func New(domainName string, repo repository.Repository, queue string) *Handler {
	return &Handler{
		domainName: domainName,
		repo:       repo,
		queue:      queue,
		workers:    defaultWorkers,
	}
}

// Handle is the Lambda handler function. It never returns a non-nil error; all
// failures are communicated via BatchItemFailures in the response so that only
// the failing messages are redriven, not the entire batch.
func (h *Handler) Handle(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	parsed := make([]parsedMsg, len(event.Records))
	for i, r := range event.Records {
		parsed[i] = h.parse(r)
	}

	var failures []events.SQSBatchItemFailure
	var valid []parsedMsg

	for i := range parsed {
		m := &parsed[i]
		if m.err != nil {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: m.sqsID})
			if m.isLastAttempt() {
				_ = h.repo.SaveFailure(ctx, h.buildFailureRecord(m, m.err))
			}
			continue
		}
		valid = append(valid, *m)
	}

	failures = append(failures, h.processGroups(ctx, valid)...)

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func (h *Handler) parse(r events.SQSMessage) parsedMsg {
	rc, _ := strconv.Atoi(r.Attributes["ApproximateReceiveCount"])
	base := parsedMsg{sqsID: r.MessageId, body: r.Body, receiveCount: rc}

	switch h.domainName {
	case "cliente":
		m, err := domain.ParseClientMessage(r.Body)
		if err != nil {
			return parsedMsg{sqsID: base.sqsID, body: base.body, receiveCount: base.receiveCount, err: err}
		}
		return parsedMsg{
			sqsID: base.sqsID, body: base.body, receiveCount: base.receiveCount,
			clientCode: m.Code.String(), updatedAt: m.UpdatedAt, client: m,
		}
	case "bloqueios":
		m, err := domain.ParseBlockMessage(r.Body)
		if err != nil {
			return parsedMsg{sqsID: base.sqsID, body: base.body, receiveCount: base.receiveCount, err: err}
		}
		return parsedMsg{
			sqsID: base.sqsID, body: base.body, receiveCount: base.receiveCount,
			clientCode: m.Code.String(), updatedAt: m.UpdatedAt, block: m,
		}
	default:
		return parsedMsg{
			sqsID: base.sqsID, body: base.body, receiveCount: base.receiveCount,
			err: &domain.PermanentError{Message: "unknown domain: " + h.domainName},
		}
	}
}

func (h *Handler) save(ctx context.Context, m parsedMsg) error {
	if m.client != nil {
		return h.repo.SaveClient(ctx, m.client)
	}
	if m.block != nil {
		return h.repo.SaveBlocks(ctx, m.block)
	}
	return nil
}

func (h *Handler) buildFailureRecord(m *parsedMsg, err error) *repository.FailureRecord {
	errType := "transitório"
	var pErr *domain.PermanentError
	if errors.As(err, &pErr) {
		errType = "permanente"
	} else if m.isLastAttempt() {
		errType = "esgotado"
	}
	return &repository.FailureRecord{
		MessageID:    m.sqsID,
		Domain:       h.domainName,
		Queue:        h.queue,
		ClientCode:   m.clientCode,
		OriginalBody: m.body,
		ErrorType:    errType,
		ErrorMessage: err.Error(),
		Attempts:     m.receiveCount,
		FailedAt:     time.Now().UTC(),
	}
}
