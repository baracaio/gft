# DLQs are declared first. The main queues reference their ARNs in redrive_policy.

resource "aws_sqs_queue" "clientes_dlq" {
  name                      = "cadastro-clientes-dlq"
  message_retention_seconds = 86400 # 1 day
}

resource "aws_sqs_queue" "bloqueios_dlq" {
  name                      = "cadastro-bloqueios-dlq"
  message_retention_seconds = 86400
}

resource "aws_sqs_queue" "clientes" {
  name                       = "cadastro-clientes"
  message_retention_seconds  = 86400
  visibility_timeout_seconds = local.visibility_timeout

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.clientes_dlq.arn
    maxReceiveCount     = 3 # 1 attempt + 2 retries, then DLQ
  })
}

resource "aws_sqs_queue" "bloqueios" {
  name                       = "cadastro-bloqueios"
  message_retention_seconds  = 86400
  visibility_timeout_seconds = local.visibility_timeout

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.bloqueios_dlq.arn
    maxReceiveCount     = 3
  })
}
