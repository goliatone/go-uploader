package uploader

import (
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jszwec/s3fs/v2"
)

func NewFileFS(client *s3.Client, bucket string) fs.FS {
	return s3fs.New(client, bucket)
}
