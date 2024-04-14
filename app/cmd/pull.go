package cmd

import (
	"os"

	"main.go/utils"
)

type PullCmd struct {
	Username string `optional:""`
	Password string `optional:""`
	Os string `optional:""`
	Architecture string `optional:""`
	Variant string `optional:""`
	Mirror string `optional:""`

	InsecureRegistry bool `optional:""`		// 指定使用 http 协议，否则使用 https
	Image struct {
		Image string `arg:""`
		Dir struct {
			Dir string `arg:"" optional:""`
		} `arg`
	} `arg:""`
}
func (c *PullCmd) Run(debug bool) error {
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

	dir := c.Image.Dir.Dir
	if len(dir) == 0 {									// 默认下载到当前目录
		dir, _ = os.Getwd()
	}

	image := utils.NewImage(c.Image.Image, username, passowrd, c.InsecureRegistry, c.Mirror, osName, architecture, variant)

	return utils.PullImage(&image, dir)
}