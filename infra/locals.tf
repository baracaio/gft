locals {
  # Spec decision #9: visibility timeout must be >= 6× the Lambda timeout so
  # that a message does not become visible again while the function is still running.
  visibility_timeout = var.lambda_timeout * 6

  # The Go toolchain names the architecture "amd64", but the AWS/Terraform API
  # uses "x86_64". Map between them here so variable values stay Go-conventional.
  lambda_arch_tf = var.lambda_arch == "amd64" ? "x86_64" : "arm64"
}
