terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# When aws_endpoint is set (Floci/LocalStack), all service endpoints are
# overridden to that address and credential validation is skipped.
# Leave aws_endpoint empty to target real AWS using the default credential chain.
provider "aws" {
  region     = var.aws_region
  access_key = var.aws_endpoint != "" ? "localstack" : null
  secret_key = var.aws_endpoint != "" ? "localstack" : null

  skip_credentials_validation = var.aws_endpoint != ""
  skip_requesting_account_id  = var.aws_endpoint != ""
  skip_metadata_api_check     = var.aws_endpoint != ""

  endpoints {
    sqs      = var.aws_endpoint
    dynamodb = var.aws_endpoint
    lambda   = var.aws_endpoint
    iam      = var.aws_endpoint
    sts      = var.aws_endpoint
    logs     = var.aws_endpoint
  }
}
