package cmd

import (
	"main.go/utils"
)

type PushCmd struct {
	File struct {
		File string `arg:""`
		Image struct {
			Image string `arg:""`
		} `arg`
	} `arg`
}
func (c *PushCmd) Run(debug bool) error {
	return utils.PushImage(c.File.File, c.File.Image.Image)
}