package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/baracaio/gft/internal/domain"
)

// SaveClient persists the client domain fields (numero_agencia, numero_conta,
// numero_dac, data_atualizacao_cliente) to cadastro_geral. It discards the
// write silently when the stored timestamp is equal to or newer than the message.
// It retries on version conflict up to maxRetries times before returning a
// TransientError.
func (r *DynamoRepository) SaveClient(ctx context.Context, msg *domain.ClientMessage) error {
	pk := msg.Code.String()

	for attempt := range maxRetries {
		item, err := r.getItem(ctx, pk)
		if err != nil {
			return &domain.TransientError{Cause: fmt.Errorf("SaveClient GetItem: %w", err)}
		}

		if item == nil {
			err = r.putNewClient(ctx, pk, msg)
		} else {
			stored := parseStoredTimestamp(item.ClientUpdate)
			if !msg.IsNewerThan(stored) {
				return nil // discard: equal or older timestamp
			}
			err = r.updateClient(ctx, pk, msg, item.Version)
		}

		if err == nil {
			return nil
		}
		if isConditionalCheckFailed(err) {
			if attempt < maxRetries-1 {
				retryBackoff(attempt)
				continue
			}
			return &domain.TransientError{Cause: fmt.Errorf("SaveClient version conflict after %d retries", maxRetries)}
		}
		return &domain.TransientError{Cause: fmt.Errorf("SaveClient write: %w", err)}
	}

	return &domain.TransientError{Cause: fmt.Errorf("SaveClient exhausted retries")}
}

func (r *DynamoRepository) putNewClient(ctx context.Context, pk string, msg *domain.ClientMessage) error {
	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		ConditionExpression: aws.String("attribute_not_exists(codigo_cliente)"),
		Item: map[string]types.AttributeValue{
			"codigo_cliente":           &types.AttributeValueMemberS{Value: pk},
			"numero_agencia":           nStr(int64(msg.Branch)),
			"numero_conta":             nStr(int64(msg.Account)),
			"numero_dac":               nStr(int64(msg.Dac)),
			"data_atualizacao_cliente": &types.AttributeValueMemberS{Value: msg.UpdatedAt.UTC().Format(domain.StoredTimestampFormat)},
			"version":                  nStr(1),
		},
	})
	return err
}

func (r *DynamoRepository) updateClient(ctx context.Context, pk string, msg *domain.ClientMessage, version int64) error {
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"codigo_cliente": &types.AttributeValueMemberS{Value: pk},
		},
		ConditionExpression: aws.String("version = :v"),
		UpdateExpression:    aws.String("SET numero_agencia = :ag, numero_conta = :ct, numero_dac = :dac, data_atualizacao_cliente = :ts, version = version + :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v":   nStr(version),
			":ag":  nStr(int64(msg.Branch)),
			":ct":  nStr(int64(msg.Account)),
			":dac": nStr(int64(msg.Dac)),
			":ts":  &types.AttributeValueMemberS{Value: msg.UpdatedAt.UTC().Format(domain.StoredTimestampFormat)},
			":one": nStr(1),
		},
	})
	return err
}

// parseStoredTimestamp parses a nullable stored timestamp attribute.
// Returns the zero time when the attribute is absent (item is new or domain
// has never written to it), which makes IsNewerThan return true for any real timestamp.
func parseStoredTimestamp(s *string) time.Time {
	if s == nil {
		return time.Time{}
	}
	t, err := time.Parse(domain.StoredTimestampFormat, *s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
