variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region for all resources."
}

variable "aws_endpoint" {
  type        = string
  default     = ""
  description = "Override all AWS service endpoints. Set to http://localhost:4566 for Floci/LocalStack. Leave empty for real AWS."
}

variable "lambda_timeout" {
  type        = number
  default     = 30
  description = "Lambda function timeout in seconds. Visibility timeout is set to 6× this value."
}

variable "lambda_arch" {
  type        = string
  default     = "amd64"
  description = "Lambda binary architecture: amd64 (x86_64) or arm64. Must match the Go build target."
  validation {
    condition     = contains(["amd64", "arm64"], var.lambda_arch)
    error_message = "lambda_arch must be amd64 or arm64."
  }
}

variable "lambda_zip_path" {
  type        = string
  default     = "../build/lambda.zip"
  description = "Path to the compiled Lambda zip (must contain a file named bootstrap)."
}

variable "table_name" {
  type        = string
  default     = "cadastro_geral"
  description = "DynamoDB table name for the main ingestion records."
}

variable "failures_table_name" {
  type        = string
  default     = "falhas_processamento"
  description = "DynamoDB table name for failed message records."
}
