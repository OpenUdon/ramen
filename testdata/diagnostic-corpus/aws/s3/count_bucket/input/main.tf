resource "aws_s3_bucket" "example" {
  count  = 2
  bucket = "ramen-semantic-loss-${count.index}"
}
