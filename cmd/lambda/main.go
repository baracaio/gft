package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/baracaio/gft/internal/handler"
	"github.com/baracaio/gft/internal/repository"
)

func main() {
	domainName := os.Getenv("DOMAIN")
	region     := os.Getenv("AWS_REGION")
	table      := os.Getenv("TABLE_NAME")
	failTable  := os.Getenv("FAILURES_TABLE")
	endpoint   := os.Getenv("DYNAMODB_ENDPOINT")
	queue      := os.Getenv("QUEUE_NAME")

	repo, err := repository.New(context.Background(), repository.Config{
		Region:        region,
		TableName:     table,
		FailuresTable: failTable,
		EndpointURL:   endpoint,
	})
	if err != nil {
		log.Fatalf("failed to init repository: %v", err)
	}

	h := handler.New(domainName, repo, queue)
	lambda.Start(h.Handle)
}
