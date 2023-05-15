package utils

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Given bytes and an S3 location write a file on S3 with expiration policy
// 0 value for expDays means no expiry
// If failure occurs append error message to the logs stream
// This function does not panic to safeguard server
func WriteToS3(b []byte, key string, logs *[]string, contType string, expDays int) {

	defer func(logs *[]string) {
		if r := recover(); r != nil {
			// Handle the panic gracefully
			*logs = append(*logs, fmt.Sprintf("Failure writing to S3. Log writing routine panicked: %v", r))
		}
	}(logs)

	// Set up a session with AWS credentials and region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	var expirationDate *time.Time
	if expDays != 0 {
		expDate := time.Now().AddDate(0, 0, expDays)
		expirationDate = &expDate
	}

	// Upload the data to S3
	_, err := svc.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(os.Getenv("S3_BUCKET")),
		Key:         aws.String(key),
		Body:        bytes.NewReader(b),
		Expires:     expirationDate,
		ContentType: &contType,
	})

	if err != nil {
		*logs = append(*logs, "Failure writing to S3. Error: "+err.Error())
	}
}

// Check if an S3 Key exists
func KeyExists(key string, svc *s3.S3) (bool, error) {

	// it should be HeadObject here, but headbject is giving 403 forbidden error for some reason
	_, err := svc.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(os.Getenv("S3_BUCKET")),
		Key:    aws.String(key),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "NotFound", "Forbidden": // s3.ErrCodeNoSuchKey does not work, aws is missing this error code so we hardwire a string
				return false, nil
			default:
				return false, err
			}
		}
		return false, err
	}

	return true, nil
}

// Check if a string is in string slice
func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
