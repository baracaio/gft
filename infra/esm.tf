# Event Source Mappings wire each SQS queue to its Lambda function.
# ReportBatchItemFailures enables partial batch response: only the failed
# itemIdentifiers are redriven instead of the entire batch.

resource "aws_lambda_event_source_mapping" "clientes" {
  event_source_arn        = aws_sqs_queue.clientes.arn
  function_name           = aws_lambda_function.clientes.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}

resource "aws_lambda_event_source_mapping" "bloqueios" {
  event_source_arn        = aws_sqs_queue.bloqueios.arn
  function_name           = aws_lambda_function.bloqueios.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}
