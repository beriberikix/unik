package compilers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	log "github.com/Sirupsen/logrus"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/engine-api/types/strslice"
	unikos "github.com/emc-advanced-dev/unik/pkg/os"
	"golang.org/x/net/context"
)

func BuildBootableImage(kernel, cmdline string) (string, error) {
	directory, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	kernelBaseName := "program.bin"

	if err := unikos.CopyFile(kernel, path.Join(directory, kernelBaseName)); err != nil {
		return "", err
	}

	cmds := []string{"-d", "/opt/vol", "-p", kernelBaseName, "-a", cmdline}
	binds := []string{directory + ":/opt/vol/"}

	if err := RunContainer("boot-creator", cmds, binds, true); err != nil {
		return "", err
	}

	resultFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	resultFile.Close()

	if err := os.Rename(path.Join(directory, "vol.img"), resultFile.Name()); err != nil {
		return "", err
	}

	return resultFile.Name(), nil
}

func RunContainer(imageName string, cmds, binds []string, privileged bool) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}

	config := &container.Config{
		Image: imageName,
		Cmd:   strslice.StrSlice(cmds),
	}
	hostConfig := &container.HostConfig{
		Binds:      binds,
		Privileged: privileged,
	}
	networkingConfig := &network.NetworkingConfig{}
	containerName := ""

	container, err := cli.ContainerCreate(context.Background(), config, hostConfig, networkingConfig, containerName)
	if err != nil {
		log.WithField("err", err).Error("Error creating container")
		return err
	}
	defer cli.ContainerRemove(context.Background(), types.ContainerRemoveOptions{ContainerID: container.ID})

	log.WithField("id", container.ID).Error("Created container")

	if err := cli.ContainerStart(context.Background(), container.ID); err != nil {
		log.WithField("err", err).Error("ContainerStart")
		return err
	}

	status, err := cli.ContainerWait(context.Background(), container.ID)
	if err != nil {
		return err
	}

	if status != 0 {
		log.WithField("status", status).Error("Container exit status non zero")

		options := types.ContainerLogsOptions{
			ContainerID: container.ID,
			ShowStdout:  true,
			ShowStderr:  true,
			Follow:      true,
			Tail:        "all",
		}
		reader, err := cli.ContainerLogs(context.Background(), options)
		if err != nil {
			log.WithField("err", err).Error("ContainerLogs")
			return err
		}
		defer reader.Close()

		if res, err := ioutil.ReadAll(reader); err == nil {
			log.Error(string(res))
		} else {
			log.WithField("err", err).Warn("failed to get logs")
		}

		return errors.New("Returned non zero status")
	}

	return nil
}

// rump special json
func ToRumpJson(c RumpConfig) (string, error) {

	blk := c.Blk
	c.Blk = nil

	jsonConfig, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	blks := ""
	for _, b := range blk {

		blkjson, err := json.Marshal(b)
		if err != nil {
			return "", err
		}
		blks += fmt.Sprintf("\"blk\": %s,", string(blkjson))
	}
	var jsonString string
	if len(blks) > 0 {

		jsonString = string(jsonConfig[:len(jsonConfig)-1]) + "," + blks[:len(blks)-1] + "}"

	} else {
		jsonString = string(jsonConfig)
	}

	return jsonString, nil

}
