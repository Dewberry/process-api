package utils

import (
	"bytes"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Given text and an S3 location write a file on S3
// If failure occurs append error message to the logs stream
func WriteToS3(text string, key string, logs *[]string) {
	// Set up a session with AWS credentials and region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	textBytes := []byte(text)

	// Upload the data to S3
	_, err := svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(os.Getenv("S3_BUCKET")),
		Key:    aws.String(key),
		Body:   bytes.NewReader(textBytes),
	})

	if err != nil {
		*logs = append(*logs, "Failure writing to S3. Error: "+err.Error())
	}
}
