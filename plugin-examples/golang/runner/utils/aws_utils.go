package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func SessionManager() (*s3.S3, error) {
	region := os.Getenv("AWS_REGION")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	s3Mock, err := strconv.ParseBool(os.Getenv("S3_MOCK"))
	if err != nil {
		log.Fatal("TODO", err)
	}

	if s3Mock {
		fmt.Println("Using minio to mock s3")
		endpoint := os.Getenv("S3_ENDPOINT")
		if endpoint == "" {
			return nil, errors.New("`S3_ENDPOINT` env var required if using Minio (S3_MOCK). Set `S3_MOCK` to false or add an `S3_ENDPOINT` to the env")
		}

		sess, err := session.NewSession(&aws.Config{
			Endpoint:         aws.String(endpoint),
			Region:           aws.String(region),
			Credentials:      credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
			S3ForcePathStyle: aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("error connecting to minio session: %s", err.Error())
		}

		return s3.New(sess), nil

	} else {

		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		})
		if err != nil {
			return nil, fmt.Errorf("error creating s3 session: %s", err.Error())
		}

		return s3.New(sess), nil

	}
}

func DownloadFilesFromS3(s3Client *s3.S3, bucketName, prefix string, config *Configuration) error {
	// Download the JSON file
	output, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(prefix),
	})
	if err != nil {
		return fmt.Errorf("unable to retrieve inputs file `%s`. please verify this key exists and is accessible", prefix)
	}
	defer output.Body.Close()

	// Read the JSON configuration directly from the response body
	jsonData, err := io.ReadAll(output.Body)
	if err != nil {
		return errors.New("error parsing JSON: " + err.Error())
	}

	err = json.Unmarshal(jsonData, config)
	if err != nil {
		return errors.New("error parsing JSON: " + err.Error())
	}

	// Create the model directory if it doesn't exist
	topLevelDir := "model"
	err = os.MkdirAll(topLevelDir, os.ModePerm)
	if err != nil {
		return errors.New("Error creating top-level directory " + topLevelDir + ": " + err.Error())

	}

	// Create the directories for inputs and outputs within the top-level directory
	for _, fileName := range append(config.Inputs, config.Outputs...) {
		dir := filepath.Join(topLevelDir, filepath.Dir(fileName))
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			fmt.Printf("Error creating directory %s: %s", dir, err.Error())
			continue
		}
	}

	// Download input files
	var unreachableFiles []string
	for _, fileName := range config.Inputs {
		objectKey := fileName

		output, err := s3Client.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			unreachableFiles = append(unreachableFiles, fileName)
			fmt.Printf("error unable to download %s : %s\n", fileName, err)
			continue
		}
		defer output.Body.Close()

		file, err := os.Create(filepath.Join(topLevelDir, fileName))
		if err != nil {

			fmt.Printf("error creating file %s: %s\n", fileName, err.Error())
			continue
		}
		defer file.Close()

		_, err = file.ReadFrom(output.Body)
		if err != nil {
			fmt.Printf("error writing to file %s : %s\n", fileName, err.Error())
		} else {
			fmt.Printf("successfully downloaded: %s\n", fileName)
		}
	}

	if len(unreachableFiles) > 0 {
		return fmt.Errorf("unable to download files: %v. Verify filepaths are correct and reachable", unreachableFiles)
	}
	return nil
}

func UploadOutputsToS3(s3Client *s3.S3, bucketName string, config *Configuration, verbose bool) error {

	topLevelDir := "model"

	if verbose {
		fmt.Println("")
		fmt.Println("--------Uploading outputs----------")
		fmt.Println("")
	}

	var unreachableFiles []string
	for _, fileName := range config.Outputs {
		localFilePath := filepath.Join(topLevelDir, fileName)

		file, err := os.Open(localFilePath)
		if err != nil {
			unreachableFiles = append(unreachableFiles, localFilePath)
			fmt.Println("Error opening local file", localFilePath, ":", err)
			continue
		}
		defer file.Close()

		// TODO: Modify the object key as needed
		objectKey := filepath.Join(config.JobRootPrefix, fileName)

		_, err = s3Client.PutObject(&s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectKey),
			Body:   file,
		})
		if err != nil {
			fmt.Println("Error uploading file to S3:", err)
			continue
		} else if verbose {
			fmt.Println("Uploaded file", localFilePath, "to S3 as", objectKey)
		}

	}

	if len(unreachableFiles) > 0 {
		return fmt.Errorf("unable to donwload files: %v. Verify filepaths are correct and reachable", unreachableFiles)
	}
	return nil
}
