output "clientes_queue_url" {
  description = "URL of the cadastro-clientes SQS queue (use with aws sqs send-message)."
  value       = aws_sqs_queue.clientes.url
}

output "bloqueios_queue_url" {
  description = "URL of the cadastro-bloqueios SQS queue."
  value       = aws_sqs_queue.bloqueios.url
}

output "clientes_dlq_url" {
  description = "URL of the cadastro-clientes dead-letter queue."
  value       = aws_sqs_queue.clientes_dlq.url
}

output "bloqueios_dlq_url" {
  description = "URL of the cadastro-bloqueios dead-letter queue."
  value       = aws_sqs_queue.bloqueios_dlq.url
}

output "cadastro_geral_table" {
  description = "Name of the main DynamoDB table."
  value       = aws_dynamodb_table.cadastro_geral.name
}

output "falhas_processamento_table" {
  description = "Name of the failure records DynamoDB table."
  value       = aws_dynamodb_table.falhas_processamento.name
}

output "lambda_clientes_arn" {
  description = "ARN of the cadastro-clientes Lambda function."
  value       = aws_lambda_function.clientes.arn
}

output "lambda_bloqueios_arn" {
  description = "ARN of the cadastro-bloqueios Lambda function."
  value       = aws_lambda_function.bloqueios.arn
}
