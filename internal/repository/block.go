package repository

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/baracaio/gft/internal/domain"
)

// SaveBlocks persists the blocks domain fields (bloqueios, data_atualizacao_bloqueios)
// to cadastro_geral. It follows the same timestamp-compare and optimistic-locking
// pattern as SaveClient, touching only the blocks domain columns.
func (r *DynamoRepository) SaveBlocks(ctx context.Context, msg *domain.BlockMessage) error {
	pk := msg.Code.String()

	for attempt := range maxRetries {
		item, err := r.getItem(ctx, pk)
		if err != nil {
			return &domain.TransientError{Cause: fmt.Errorf("SaveBlocks GetItem: %w", err)}
		}

		if item == nil {
			err = r.putNewBlocks(ctx, pk, msg)
		} else {
			stored := parseStoredTimestamp(item.BlockUpdate)
			if !msg.IsNewerThan(stored) {
				return nil // discard
			}
			err = r.updateBlocks(ctx, pk, msg, item.Version)
		}

		if err == nil {
			return nil
		}
		if isConditionalCheckFailed(err) {
			if attempt < maxRetries-1 {
				retryBackoff(attempt)
				continue
			}
			return &domain.TransientError{Cause: fmt.Errorf("SaveBlocks version conflict after %d retries", maxRetries)}
		}
		return &domain.TransientError{Cause: fmt.Errorf("SaveBlocks write: %w", err)}
	}

	return &domain.TransientError{Cause: fmt.Errorf("SaveBlocks exhausted retries")}
}

func (r *DynamoRepository) putNewBlocks(ctx context.Context, pk string, msg *domain.BlockMessage) error {
	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		ConditionExpression: aws.String("attribute_not_exists(codigo_cliente)"),
		Item: map[string]types.AttributeValue{
			"codigo_cliente":             &types.AttributeValueMemberS{Value: pk},
			"bloqueios":                  marshalBlocks(msg.Blocks),
			"data_atualizacao_bloqueios": &types.AttributeValueMemberS{Value: msg.UpdatedAt.UTC().Format(domain.StoredTimestampFormat)},
			"version":                    nStr(1),
		},
	})
	return err
}

func (r *DynamoRepository) updateBlocks(ctx context.Context, pk string, msg *domain.BlockMessage, version int64) error {
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"codigo_cliente": &types.AttributeValueMemberS{Value: pk},
		},
		ConditionExpression: aws.String("version = :v"),
		UpdateExpression:    aws.String("SET bloqueios = :bl, data_atualizacao_bloqueios = :ts, version = version + :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v":   nStr(version),
			":bl":  marshalBlocks(msg.Blocks),
			":ts":  &types.AttributeValueMemberS{Value: msg.UpdatedAt.UTC().Format(domain.StoredTimestampFormat)},
			":one": nStr(1),
		},
	})
	return err
}

// marshalBlocks converts []domain.Block to a DynamoDB L (list of maps).
// Each map has {codigo: N, valor: N}. block.Value is already a json.Number
// (exact decimal string), so it is used directly as the N attribute value.
func marshalBlocks(blocks []domain.Block) *types.AttributeValueMemberL {
	items := make([]types.AttributeValue, len(blocks))
	for i, b := range blocks {
		items[i] = &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"codigo": nStr(int64(b.Code)),
				"valor":  &types.AttributeValueMemberN{Value: string(b.Value)},
			},
		}
	}
	return &types.AttributeValueMemberL{Value: items}
}
