resource "aws_s3_object" "test" {
  bucket  = "example-bucket"
  key     = "example.txt"
  content = "example"
}
