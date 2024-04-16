package cmd

import (
	"os"

	"main.go/utils"
)

type PushCmd struct {
	Username string `optional:""`
	Password string `optional:""`
	Os string `optional:""`
	Architecture string `optional:""`
	Variant string `optional:""`

	InsecureRegistry bool `optional:""`		// 指定使用 http 协议，否则使用 https
	File struct {
		File string `arg:""`
		Image struct {
			Image string `arg:""`
		} `arg`
	} `arg`
}
func (c *PushCmd) Run(debug bool) error {
	username := c.Username
	passowrd := c.Password
	if len(username) == 0 {
		username = os.Getenv("GO_DOCKER_USERNAME")
	}
	if len(passowrd) == 0 {
		passowrd = os.Getenv("GO_DOCKER_PASSWORD")
	}

	osName := c.Os
	architecture := c.Architecture
	variant := c.Variant

	if len(osName) == 0 {								// 当前系统类型，默认下载 linux 的镜像；可选 linux, windows
		osName = "linux"
	}
	if len(architecture) == 0 {					// 当前系统架构，默认下载 amd64 的镜像；可选 386, amd64, arm, arm64
		architecture = "amd64"
	}

	image := utils.NewImage(c.File.Image.Image, username, passowrd, c.InsecureRegistry, "", osName, architecture, variant)

	return utils.PushImage(c.File.File, &image)
}