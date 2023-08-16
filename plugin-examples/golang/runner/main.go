package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runner/utils"
)

func init() {
	requiredEnvVars := []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_REGION",
		"S3_MOCK",
		"S3_BUCKET",
	}

	for _, envVar := range requiredEnvVars {
		if value := os.Getenv(envVar); value == "" {
			fmt.Printf("Error: Missing environment variable %s\n", envVar)
			os.Exit(1)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: ./main '{\"jobID\":\"s4-df-5r\", \"prefix\" : \"root-prefix/payload.json\"}'")
	}

	s3Client, err := utils.SessionManager()
	if err != nil {
		log.Fatal("Error connecting to s3: ", err.Error())
	}

	bucketName := os.Getenv("S3_BUCKET")

	jsonArg := os.Args[1]
	var jobPayload utils.JobPayload

	err = json.Unmarshal([]byte(jsonArg), &jobPayload)
	if err != nil {
		log.Fatal("Error parsing JSON argument:", err.Error())
	}

	jobPayload.PrintPreviw()

	fmt.Println("")
	fmt.Println("--------Downloading inputs----------")
	var config utils.Configuration

	err = utils.DownloadFilesFromS3(s3Client, bucketName, jobPayload.Prefix, &config)
	if err != nil {
		log.Fatalf("download error: %s", err.Error())
	}

	// // Run the model
	fmt.Println("")
	fmt.Println("---------Run Model Simulation----------")
	cmd := exec.Command("echo", ".....insert executable or call the model here......")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd = exec.Command("touch", "/app/model/demo/simulation/model-run-log.txt")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Fatalf("compute error: %s", err.Error())
	}

	err = utils.UploadOutputsToS3(s3Client, bucketName, &config, true)
	if err != nil {
		log.Fatalf("upload error: %s", err.Error())
	}

	fmt.Println("compute successful!")

	results, err := config.PluginResults()
	if err != nil {
		log.Fatalf("unable to print results: %s", err.Error())
	}
	fmt.Println(string(results))
}
