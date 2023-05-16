package jobs

import (
	"app/controllers"
	"app/utils"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/labstack/gommon/log"
)

// Define a metaData object
type metaData struct {
	Context                  string `json:"@context"`
	JobID                    string `json:"apiJobId"`
	User                     string
	ProcessID                string `json:"apiProcessId"`
	ProcessVersion           string
	ImageURI                 string `json:"imageURI"`
	ImageDigest              string `json:"imageDigest"`
	ComputeEnvironmentURI    string // ARN
	ComputeEnvironmentDigest string // required for reproducibility, will need to be custom implemented
	Commands                 []string
	TimeCompleted            time.Time
}

func (j *AWSBatchJob) WriteMeta(c *controllers.AWSBatchController) {

	if j.MetaDataLocation == "" {
		return
	}

	imgURI, err := c.GetImageURI(j.JobDef)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	imgDgst, err := getImageDigest(imgURI)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	md := metaData{
		Context:       "http://schema.org/",
		JobID:         j.UUID,
		ProcessID:     j.ProcessID(),
		ImageURI:      imgURI,
		ImageDigest:   imgDgst,
		TimeCompleted: j.UpdateTime,
		Commands:      j.Cmd,
	}

	jsonBytes, err := json.Marshal(md)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	utils.WriteToS3(jsonBytes, j.MetaDataLocation, &j.APILogs, "application/json", 0)

	return
}

// Get image digest
func getImageDigest(imgURI string) (string, error) {
	var imgDgst string
	if strings.Contains(imgURI, "amazonaws.com/") {

		sess, err := session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		})
		if err != nil {
			return "", err
		}
		ecrClient := ecr.New(sess)

		accountID, repositoryName, imgTag, err := parseImgURI(imgURI)
		if err != nil {
			return "", err
		}

		// Retrieve the image details from ECR
		log.Warn(accountID)
		log.Warn(repositoryName)
		log.Warn(imgTag)
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
	} else { // dockerHub image
		// this only works with pulled images
		cD, err := controllers.NewDockerController()
		if err != nil {
			return "", err
		}

		imgDgst, err = cD.GetImageDigest(imgURI)
		if err != nil {
			return "", err
		}
	}
	return imgDgst, nil
}

// Helper function to parse the ECR repository URI
func parseImgURI(imgURI string) (string, string, string, error) {
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
