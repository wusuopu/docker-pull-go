package utils

import "fmt"

func PullImage(image string, dir string) error {
	fmt.Println("Pull Image:", image, dir)
	return nil
}