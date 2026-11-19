# Only key attributes need to be declared in the schema; all other attributes
# (numero_agencia, bloqueios, version, etc.) are written dynamically by the Lambda.

resource "aws_dynamodb_table" "cadastro_geral" {
  name         = var.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "codigo_cliente"

  attribute {
    name = "codigo_cliente"
    type = "S"
  }
}

resource "aws_dynamodb_table" "falhas_processamento" {
  name         = var.failures_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id_mensagem"

  attribute {
    name = "id_mensagem"
    type = "S"
  }
}
