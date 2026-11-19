package repository

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/baracaio/gft/internal/domain"
)

const maxRetries = 3

// Repository is the write surface consumed by the SQS handler.
type Repository interface {
	SaveClient(ctx context.Context, msg *domain.ClientMessage) error
	SaveBlocks(ctx context.Context, msg *domain.BlockMessage) error
	SaveFailure(ctx context.Context, f *FailureRecord) error
}

// Config holds everything needed to construct a DynamoRepository.
type Config struct {
	Region        string
	TableName     string // "cadastro_geral"
	FailuresTable string // "falhas_processamento"
	// EndpointURL overrides the DynamoDB endpoint. Set to the Floci/LocalStack
	// address (e.g. "http://localhost:4566") for local runs; leave empty for AWS.
	EndpointURL string
}

// DynamoRepository is the concrete Repository backed by Amazon DynamoDB.
type DynamoRepository struct {
	client        *dynamodb.Client
	table         string
	failuresTable string
}

// New creates a DynamoRepository. AWS credentials and region are sourced from
// the environment (AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) or
// the default credential chain. EndpointURL, when set, overrides the DynamoDB
// service endpoint — this is the only change needed to target Floci/LocalStack.
func New(ctx context.Context, cfg Config) (*DynamoRepository, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	opts := []func(*dynamodb.Options){}
	if cfg.EndpointURL != "" {
		opts = append(opts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(cfg.EndpointURL)
		})
	}

	return &DynamoRepository{
		client:        dynamodb.NewFromConfig(awsCfg, opts...),
		table:         cfg.TableName,
		failuresTable: cfg.FailuresTable,
	}, nil
}

// record is used only to unmarshal the fields needed for timestamp
// comparison and optimistic locking from GetItem results.
type record struct {
	Version      int64
	ClientUpdate *string
	BlockUpdate  *string
}

// getItem performs a consistent read of codigo_cliente from cadastro_geral,
// projecting only the fields required for the write decision.
// Returns (nil, nil) when the item does not exist.
func (r *DynamoRepository) getItem(ctx context.Context, codigoCliente string) (*record, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"codigo_cliente": &types.AttributeValueMemberS{Value: codigoCliente},
		},
		ProjectionExpression: aws.String("version, data_atualizacao_cliente, data_atualizacao_bloqueios"),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}

	item := &record{}

	if v, ok := out.Item["version"]; ok {
		if n, ok := v.(*types.AttributeValueMemberN); ok {
			item.Version, _ = strconv.ParseInt(n.Value, 10, 64)
		}
	}
	if v, ok := out.Item["data_atualizacao_cliente"]; ok {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			item.ClientUpdate = &s.Value
		}
	}
	if v, ok := out.Item["data_atualizacao_bloqueios"]; ok {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			item.BlockUpdate = &s.Value
		}
	}

	return item, nil
}

// isConditionalCheckFailed reports whether an error is a DynamoDB
// ConditionalCheckFailedException (version conflict or PK already exists).
func isConditionalCheckFailed(err error) bool {
	var ccf *types.ConditionalCheckFailedException
	return errors.As(err, &ccf)
}

// retryBackoff sleeps for base*2^attempt + jitter before the next retry.
func retryBackoff(attempt int) {
	base := 10 * time.Millisecond
	sleep := base * (1 << attempt)
	jitter := time.Duration(rand.Intn(20)) * time.Millisecond
	time.Sleep(sleep + jitter)
}

// nStr returns a DynamoDB N attribute value from an int64.
func nStr(v int64) *types.AttributeValueMemberN {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}
