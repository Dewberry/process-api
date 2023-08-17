package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Given bytes and an S3 location write a file on S3 with expiration policy
// 0 value for expDays means no expiry
// If failure occurs append error message to the logs stream
// This function does not panic to safeguard server
func WriteToS3(svc *s3.S3, b []byte, key string, logs *[]string, contType string, expDays int) error {

	defer func(logs *[]string) {
		if r := recover(); r != nil {
			// Handle the panic gracefully
			msg := fmt.Sprintf("Failure writing `%s` to `%s`: %v", key, os.Getenv("S3_BUCKET"), r)
			*logs = append(*logs, msg)
		}
	}(logs)

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
		return err
	}
	msg := fmt.Sprintf("Metadata file `%s` successfully written to `%s`", key, os.Getenv("S3_BUCKET"))
	*logs = append(*logs, msg)
	return nil
}

// Check if an S3 Key exists
func KeyExists(key string, svc *s3.S3) (bool, error) {

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

// Assumes file exist
func GetS3JsonData(key string, svc *s3.S3) (interface{}, error) {
	// Create a new S3GetObjectInput object to specify the file you want to read
	params := &s3.GetObjectInput{
		Bucket: aws.String(os.Getenv("S3_BUCKET")),
		Key:    aws.String(key),
	}

	// Use the S3 service object to download the file into a byte slice
	resp, err := svc.GetObject(params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the file contents into a byte slice
	jsonBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Declare an empty interface{} value to hold the unmarshalled data
	var data interface{}

	// Unmarshal the JSON data into the interface{} value
	err = json.Unmarshal(jsonBytes, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
