resource "aws_iam_user" "test" {
  name = var.rName
}


variable "rName" {
  description = "Name for resource"
  type        = string
  nullable    = false
}
