package utils

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Given text and an S3 location write a file on S3 with expiration policy
// If failure occurs append error message to the logs stream
// This function does not panic to safeguard server
func WriteToS3(text string, key string, logs *[]string, contType string) {

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

	textBytes := []byte(text)

	expDays, _ := strconv.Atoi(os.Getenv("EXPIRY_DAYS"))

	expirationDate := time.Now().AddDate(0, 0, expDays)

	// Upload the data to S3
	_, err := svc.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(os.Getenv("S3_BUCKET")),
		Key:         aws.String(key),
		Body:        bytes.NewReader(textBytes),
		Expires:     aws.Time(expirationDate),
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
