package controllers

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/batch"
)

type AWSBatchController struct {
	client *batch.Batch
}

func NewAWSBatchController(accessKey, secretAccessKey, region string) (*AWSBatchController, error) {
	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretAccessKey,
		}),
		Region: aws.String(region)},
	)
	if err != nil {
		return nil, err
	}

	return &AWSBatchController{batch.New(sess)}, nil
}

// returns the job id and an error
func (c *AWSBatchController) JobCreate(ctx context.Context,
	jobDef, jobName, jobQueue string, commandOverride []string,
	envVars map[string]string) (string, error) {

	envs := make([]*batch.KeyValuePair, 0)
	for k, v := range envVars {
		envs = append(envs, &batch.KeyValuePair{Name: aws.String(k), Value: aws.String(v)})
	}

	overrides := &batch.ContainerOverrides{
		Command:     aws.StringSlice(commandOverride),
		Environment: envs,
	}

	input := &batch.SubmitJobInput{
		JobDefinition:      aws.String(jobDef),
		JobName:            aws.String(jobName),
		JobQueue:           aws.String(jobQueue),
		ContainerOverrides: overrides,
	}

	output, err := c.client.SubmitJobWithContext(ctx, input)
	if err != nil {
		return "", err
	}

	return aws.StringValue(output.JobId), nil
}

func (c *AWSBatchController) JobMonitor(batchID string) (string, error) {

	input := &batch.DescribeJobsInput{Jobs: aws.StringSlice([]string{batchID})}
	output, err := c.client.DescribeJobs(input)
	if err != nil {
		return output.String(), nil
	}

	if len(output.Jobs) == 0 {
		return "", fmt.Errorf("no such job: %s", batchID)
	}

	status := aws.StringValue(output.Jobs[0].Status)

	if status == "FAILED" {
		reason := aws.StringValue(output.Jobs[0].StatusReason)
		// Non-standard reason used here to facilitate ogc implementation
		if reason == "DISMISSED" {
			return reason, nil
		} else {
			return status, fmt.Errorf("aws provided StatusReason for failure: %s", reason)
		}
	}

	switch status {
	case "SUBMITTED":
		return "ACCCEPTED", nil
	case "PENDING":
		return "ACCCEPTED", nil
	case "RUNNABLE":
		return "ACCCEPTED", nil
	case "STARTING":
		return "RUNNING", nil
	case "RUNNING":
		return "RUNNING", nil
	case "SUCCEEDED":
		return "SUCCEEDED", nil

	default:
		return status, fmt.Errorf("unrecognized status  %s", status)
	}
}

// combines JobTerminate and JobCancel by managing calls for you based on job status
func (c *AWSBatchController) JobKill(jobID string) (string, error) {
	input := &batch.DescribeJobsInput{Jobs: aws.StringSlice([]string{jobID})}

	output, err := c.client.DescribeJobs(input)
	if err != nil {
		return "", err
	}

	if len(output.Jobs) == 0 {
		return "", fmt.Errorf("no such job: %s", jobID)
	}

	if len(output.Jobs) > 1 {
		return "", fmt.Errorf("more than one job found for %s", jobID)
	}

	status := aws.StringValue(output.Jobs[0].Status)
	switch status {
	case "SUBMITTED", "PENDING", "RUNNABLE":
		output, err := c.JobCancel(jobID, "DISMISSED")
		if err != nil {
			return "", err
		}
		return output, nil

	case "STARTING", "jobs.RUNNING":
		output, err := c.JobTerminate(jobID, "DISMISSED")
		if err != nil {
			return "", err
		}
		return output, nil

	case "FAILED", "SUCCEEDED":
		return "", nil
	}

	// Add some mechanism to clean up s3 if needed

	return "", fmt.Errorf("unknown status for job %s: %s", jobID, status)
}

// for jobs with the following statuses: "STARTING", "jobs.RUNNING"
func (c *AWSBatchController) JobTerminate(jobID, reason string) (string, error) {
	input := &batch.TerminateJobInput{
		JobId:  aws.String(jobID),
		Reason: aws.String(reason),
	}

	output, err := c.client.TerminateJob(input)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

// for jobs with the following statuses: "SUBMITTED", "PENDING", "RUNNABLE"
func (c *AWSBatchController) JobCancel(jobID, reason string) (string, error) {
	input := &batch.CancelJobInput{
		JobId:  aws.String(jobID),
		Reason: aws.String(reason),
	}

	output, err := c.client.CancelJob(input)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}
