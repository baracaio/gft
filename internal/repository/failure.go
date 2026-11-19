package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/baracaio/gft/internal/domain"
)

// FailureRecord represents a message that exhausted all SQS retries or encountered
// a permanent error on the last attempt. Written to falhas_processamento.
type FailureRecord struct {
	MessageID    string
	Domain       string // "cliente" | "bloqueios"
	Queue        string
	ClientCode   string // best-effort; may be empty if not extractable from body
	OriginalBody string
	ErrorType    string // "permanente" | "transitório" | "esgotado"
	ErrorMessage string
	Attempts     int
	FailedAt     time.Time
}

// SaveFailure writes a FailureRecord to falhas_processamento. This is best-effort:
// if the write fails, the caller should proceed normally (the message is already
// headed to the DLQ; losing the failure record is preferable to masking the error).
func (r *DynamoRepository) SaveFailure(ctx context.Context, f *FailureRecord) error {
	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.failuresTable),
		Item: map[string]types.AttributeValue{
			"id_mensagem":   &types.AttributeValueMemberS{Value: f.MessageID},
			"dominio":       &types.AttributeValueMemberS{Value: f.Domain},
			"fila":          &types.AttributeValueMemberS{Value: f.Queue},
			"codigo_cliente": &types.AttributeValueMemberS{Value: f.ClientCode},
			"corpo_original": &types.AttributeValueMemberS{Value: f.OriginalBody},
			"tipo_erro":     &types.AttributeValueMemberS{Value: f.ErrorType},
			"mensagem_erro": &types.AttributeValueMemberS{Value: f.ErrorMessage},
			"tentativas":    &types.AttributeValueMemberN{Value: strconv.Itoa(f.Attempts)},
			"data_falha":    &types.AttributeValueMemberS{Value: f.FailedAt.UTC().Format(domain.StoredTimestampFormat)},
		},
	})
	return err
}
