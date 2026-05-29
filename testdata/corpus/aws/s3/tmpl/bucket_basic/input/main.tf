resource "aws_s3_bucket" "test" {
  bucket = var.rName
}


variable "rName" {
  description = "Name for resource"
  type        = string
  nullable    = false
}
