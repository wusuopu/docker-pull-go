package cmd

import (
	"main.go/utils"
)

type PullCmd struct {
	Image struct {
		Image string `arg:""`
		Dir struct {
			Dir string `arg:"" optional:""`
		} `arg`
	} `arg:""`
}
func (c *PullCmd) Run(debug bool) error {
	return utils.PullImage(c.Image.Image, c.Image.Dir.Dir)
}