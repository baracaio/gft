# Both functions use the same zip (var.lambda_zip_path). The binary inside must
# be named "bootstrap" — the required entry point for the provided.al2023 runtime.
# source_code_hash triggers redeployment only when the zip actually changes;
# run `make build` before `terraform apply`.

resource "aws_lambda_function" "clientes" {
  function_name    = "cadastro-clientes"
  filename         = var.lambda_zip_path
  source_code_hash = filebase64sha256(var.lambda_zip_path)
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_arch_tf]
  role             = aws_iam_role.lambda_clientes.arn
  timeout          = var.lambda_timeout

  environment {
    variables = {
      DOMAIN            = "cliente"
      TABLE_NAME        = aws_dynamodb_table.cadastro_geral.name
      FAILURES_TABLE    = aws_dynamodb_table.falhas_processamento.name
      AWS_REGION        = var.aws_region
      QUEUE_NAME        = aws_sqs_queue.clientes.name
      DYNAMODB_ENDPOINT = var.aws_endpoint
    }
  }
}

resource "aws_lambda_function" "bloqueios" {
  function_name    = "cadastro-bloqueios"
  filename         = var.lambda_zip_path
  source_code_hash = filebase64sha256(var.lambda_zip_path)
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_arch_tf]
  role             = aws_iam_role.lambda_bloqueios.arn
  timeout          = var.lambda_timeout

  environment {
    variables = {
      DOMAIN            = "bloqueios"
      TABLE_NAME        = aws_dynamodb_table.cadastro_geral.name
      FAILURES_TABLE    = aws_dynamodb_table.falhas_processamento.name
      AWS_REGION        = var.aws_region
      QUEUE_NAME        = aws_sqs_queue.bloqueios.name
      DYNAMODB_ENDPOINT = var.aws_endpoint
    }
  }
}
