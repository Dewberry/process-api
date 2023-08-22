package controllers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/labstack/gommon/log"
)

const DOCKER_NETWORK = "process_api_net"

type DockerController struct {
	cli *client.Client
}

func createDockerNetwork(cli *client.Client, ctx context.Context, networkName string) error {
	_, err := cli.NetworkInspect(ctx, networkName, types.NetworkInspectOptions{})
	if err == nil {
		// Network already exists
		return nil
	}

	// Create the network
	_, err = cli.NetworkCreate(ctx, networkName, types.NetworkCreate{})
	if err != nil {
		return err
	}

	return nil
}

type DockerResources container.Resources

func NewDockerController() (*DockerController, error) {
	c := new(DockerController)
	var err error
	c.cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if err != nil {
		return nil, err
	}
	return c, nil
}

// returns container id, error
func (c *DockerController) ContainerRun(ctx context.Context, image string, command []string, volumes []VolumeMount, envVars map[string]string, resources DockerResources) (string, error) {
	hostConfig := container.HostConfig{
		Resources: container.Resources(resources),
	}

	log.Debug("Initialize Container run")
	//	hostConfig.Mounts = make([]mount.Mount,0);

	mounts := make([]mount.Mount, len(volumes))

	for i, volume := range volumes {
		mount := mount.Mount{
			Type:   mount.TypeVolume,
			Source: volume.Volume.Name,
			Target: volume.HostPath,
		}
		mounts[i] = mount
	}

	hostConfig.Mounts = mounts
	envs := make([]string, len(envVars))
	var i int
	for k, v := range envVars {
		envs[i] = k + "=" + v
		i++
	}

	err := createDockerNetwork(c.cli, ctx, DOCKER_NETWORK)
	if err != nil {
		log.Error(err)
		return "", err
	}

	// Define the network mode
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DOCKER_NETWORK: {},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Tty:   true,
		Image: image,
		Cmd:   command,
		Env:   envs,
	}, &hostConfig, netConfig, nil, "")
	// log.Info("Container Create response", resp)
	if err != nil {
		log.Error(err)
		return "", err
	}

	// log.Info("Start Container")
	err = c.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Error(err)
		return "", err
	}

	return resp.ID, nil
}

func (c *DockerController) Version() string {
	return c.cli.ClientVersion()
}

// returns container logs as string, error
func (c *DockerController) ContainerLog(ctx context.Context, id string) ([]string, error) {

	reader, err := c.cli.ContainerLogs(ctx, id, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true})

	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(reader)
	var logs []string

	for scanner.Scan() {
		logs = append(logs, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}

// returns container status code, error
func (c *DockerController) ContainerWait(ctx context.Context, id string) (int64, error) {
	resultC, errC := c.cli.ContainerWait(ctx, id, "")
	select {
	case err := <-errC:
		return 0, err
	case result := <-resultC:
		return result.StatusCode, nil
	}
}

func (c *DockerController) ContainerRemove(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{})
}

func (c *DockerController) ContainerKillAndRemove(ctx context.Context, containerID string, signal string) error {
	err := c.cli.ContainerKill(ctx, containerID, signal)
	if err != nil {
		return err
	}
	return c.ContainerRemove(ctx, containerID)
}

// https://gist.github.com/miguelmota/4980b18d750fb3b1eb571c3e207b1b92
func (c *DockerController) EnsureImage(ctx context.Context, image string, verbose bool) error {
	images, err := c.cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return err
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if strings.EqualFold(tag, image) {
				// Image already exists, return nil
				return nil
			}
		}
	}

	reader, err := c.cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	var writer io.Writer
	if verbose {
		writer = os.Stdout
	} else {
		writer = io.Discard
	}

	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}

	return nil
}

type VolumeMount struct {
	HostPath string
	Volume   *volumetypes.Volume
}

func (c *DockerController) FindVolume(name string) (*volumetypes.Volume, error) {
	volumes, err := c.cli.VolumeList(context.Background(), filters.NewArgs())
	if err != nil {
		return nil, err
	}

	for _, v := range volumes.Volumes {
		if v.Name == name {
			return v, nil
		}
	}
	return nil, nil
}

func (c *DockerController) EnsureVolume(name string) (*volumetypes.Volume, error) {
	volume, err := c.FindVolume(name)
	if err != nil {
		return nil, err
	}

	if volume != nil {
		return volume, nil
	}

	vol, err := c.cli.VolumeCreate(context.Background(), volumetypes.CreateOptions{
		Driver: "local",
		//		DriverOpts: map[string]string{},
		//		Labels:     map[string]string{},
		Name: name,
	})

	return &vol, err
}

func (c *DockerController) RemoveVolume(name string) error {
	vol, err := c.FindVolume(name)
	if err != nil {
		return err
	}

	if vol == nil {
		return nil
	}

	err = c.cli.VolumeRemove(context.Background(), name, true)
	if err != nil {
		return err
	}

	return nil
}

// Get Image Digest from Image URI
func (c *DockerController) GetImageDigest(imageURI string) (string, error) {
	ctx := context.Background()
	imageInspect, _, err := c.cli.ImageInspectWithRaw(ctx, imageURI)
	if err != nil {
		return "", err
	}
	// Get the digest from the image inspect response
	imageDigest := imageInspect.ID
	return imageDigest, nil
}

// Get job execution times
func (c *DockerController) GetJobTimes(containerID string) (cp time.Time, cr time.Time, st time.Time, err error) {

	// Get the container details
	containerInfo, err := c.cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("error getting container details: %v", err)
	}

	cp, err = time.Parse(time.RFC3339Nano, containerInfo.Created)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("error parsing launch time: %v", err)
	}
	if containerInfo.State.StartedAt != "" {
		cr, err = time.Parse(time.RFC3339Nano, containerInfo.State.StartedAt)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("error parsing start time: %v", err)
		}
	}

	if containerInfo.State.FinishedAt != "" {
		st, err = time.Parse(time.RFC3339Nano, containerInfo.State.FinishedAt)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("error parsing stop time: %v", err)
		}
	}

	return
}
