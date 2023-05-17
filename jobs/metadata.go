package jobs

import (
	"app/controllers"
	"app/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
)

type process struct {
	ProcessID      string `json:"processId"`
	ProcessVersion string `json:"processVersion"`
}

type image struct {
	ImageURI    string `json:"imageURI"`
	ImageDigest string `json:"imageDigest"`
}

// Define a metaData object
type metaData struct {
	Context string  `json:"@context"`
	JobID   string  `json:"apiJobId"`
	User    string  `json:"apiUser"`
	Process process `json:"process"`
	Image   image   `json:"image"`
	// ComputeEnvironmentURI    string    // ARN
	// ComputeEnvironmentDigest string    // required for reproducibility, will need to be custom implemented
	Commands        []string  `json:"containerCommands"`
	GeneratedAtTime time.Time `json:"generatedAtTime"`
	StartedAtTime   time.Time `json:"startedAtTime"`
	EndedAtTime     time.Time `json:"endedAtTime"`
}

// Write metadata at the job's metadata location
func (j *AWSBatchJob) WriteMeta(c *controllers.AWSBatchController) {

	if j.MetaDataLocation == "" {
		return
	}

	imgURI, err := c.GetImageURI(j.JobDef)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	// imgDgst would be incorrect if tag has been updated in between
	// if there are multiple architechture available for same image tag
	var imgDgst string
	if strings.Contains(imgURI, "amazonaws.com/") {
		imgDgst, err = getECRImageDigest(imgURI)
		if err != nil {
			j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
			return
		}
	} else {
		imgDgst, err = getDkrHubImageDigest(imgURI, "dummy")
		if err != nil {
			j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
			return
		}
	}

	p := process{j.ProcessID(), j.ProcessVersion}
	i := image{imgURI, imgDgst}

	g, s, e, err := c.GetJobTimes(j.AWSBatchID)

	md := metaData{
		Context:         "https://github.com/Dewberry/process-api/blob/main/context.jsonld",
		JobID:           j.UUID,
		Process:         p,
		Image:           i,
		Commands:        j.Cmd,
		GeneratedAtTime: g,
		StartedAtTime:   s,
		EndedAtTime:     e,
	}

	jsonBytes, err := json.Marshal(md)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	utils.WriteToS3(jsonBytes, j.MetaDataLocation, &j.APILogs, "application/json", 0)

	return
}

// Get image digest from ecr
func getECRImageDigest(imgURI string) (string, error) {
	var imgDgst string

	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return "", err
	}
	ecrClient := ecr.New(sess)

	accountID, repositoryName, imgTag, err := parseECRImgURI(imgURI)
	if err != nil {
		return "", err
	}

	// Retrieve the image details from ECR
	describeImagesInput := &ecr.DescribeImagesInput{
		RegistryId:     aws.String(accountID),
		RepositoryName: aws.String(repositoryName),
		ImageIds: []*ecr.ImageIdentifier{
			{
				ImageTag: aws.String(imgTag),
			},
		},
	}

	describeImagesOutput, err := ecrClient.DescribeImages(describeImagesInput)
	if err != nil {
		return "", err
	}

	// Get the digest from the image details
	if len(describeImagesOutput.ImageDetails) > 0 {
		imgDgst = aws.StringValue(describeImagesOutput.ImageDetails[0].ImageDigest)
	} else {
		return "", fmt.Errorf("image not found in ECR")
	}

	return imgDgst, nil
}

// Helper function to parse the ECR Image URI
func parseECRImgURI(imgURI string) (string, string, string, error) {
	// Split the repository URI into account ID, repository name, and image tag
	parts := strings.Split(imgURI, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid repository URI: %s", imgURI)
	}

	accountID := strings.Split(parts[0], ".")[0]
	imageWithTag := parts[1]

	imageParts := strings.SplitN(imageWithTag, ":", 2)
	if len(imageParts) != 2 {
		return "", "", "", fmt.Errorf("invalid image tag in repository URI: %s", imgURI)
	}

	repositoryName := imageParts[0]
	imageTag := imageParts[1]

	return accountID, repositoryName, imageTag, nil
}

// Get Image Digest from Docker Hub
// arch based digest not yet implemented, arch is not used
func getDkrHubImageDigest(imgURI string, arch string) (string, error) {
	parts := strings.Split(imgURI, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image Name: %s", imgURI)
	}

	imageName := parts[0]
	imageTag := parts[1]

	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/%s/images", imageName, imageTag)

	client := http.Client{
		Timeout: 10 * time.Second, // Set a timeout for the request
	}

	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Error sending request: %s\n", err)

	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("Error reading response: %s\n", err)
	}

	var result []interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("Error parsing JSON: %s\n", err)
	}

	// Currently it gets just the first image, while there can be more than 1. This is incorrect
	digest, ok := result[0].(map[string]interface{})["digest"].(string)
	if !ok {
		return "", fmt.Errorf("Error retrieving image digest")
	}

	return digest, nil
}
