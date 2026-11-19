# Shared trust policy: both Lambda functions use the same assume-role document.
data "aws_iam_policy_document" "lambda_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

# ---------------------------------------------------------------------------
# Lambda Clientes
# ---------------------------------------------------------------------------

resource "aws_iam_role" "lambda_clientes" {
  name               = "lambda-cadastro-clientes"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "lambda_clientes" {
  statement {
    sid       = "SQSClientes"
    effect    = "Allow"
    actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
    resources = [aws_sqs_queue.clientes.arn]
  }

  statement {
    sid       = "DynamoDBCadastro"
    effect    = "Allow"
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem"]
    resources = [aws_dynamodb_table.cadastro_geral.arn]
  }

  statement {
    sid       = "DynamoDBFalhas"
    effect    = "Allow"
    actions   = ["dynamodb:PutItem"]
    resources = [aws_dynamodb_table.falhas_processamento.arn]
  }

  statement {
    sid       = "Logs"
    effect    = "Allow"
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["arn:aws:logs:*:*:*"]
  }
}

resource "aws_iam_role_policy" "lambda_clientes" {
  name   = "lambda-clientes-policy"
  role   = aws_iam_role.lambda_clientes.id
  policy = data.aws_iam_policy_document.lambda_clientes.json
}

# ---------------------------------------------------------------------------
# Lambda Bloqueios
# ---------------------------------------------------------------------------

resource "aws_iam_role" "lambda_bloqueios" {
  name               = "lambda-cadastro-bloqueios"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "lambda_bloqueios" {
  statement {
    sid       = "SQSBloqueios"
    effect    = "Allow"
    actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
    resources = [aws_sqs_queue.bloqueios.arn]
  }

  statement {
    sid       = "DynamoDBCadastro"
    effect    = "Allow"
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem"]
    resources = [aws_dynamodb_table.cadastro_geral.arn]
  }

  statement {
    sid       = "DynamoDBFalhas"
    effect    = "Allow"
    actions   = ["dynamodb:PutItem"]
    resources = [aws_dynamodb_table.falhas_processamento.arn]
  }

  statement {
    sid       = "Logs"
    effect    = "Allow"
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["arn:aws:logs:*:*:*"]
  }
}

resource "aws_iam_role_policy" "lambda_bloqueios" {
  name   = "lambda-bloqueios-policy"
  role   = aws_iam_role.lambda_bloqueios.id
  policy = data.aws_iam_policy_document.lambda_bloqueios.json
}
