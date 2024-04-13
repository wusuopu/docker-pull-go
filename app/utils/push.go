package utils

import "fmt"


func PushImage(filename string, image string) error {
	fmt.Println("push", filename, image)
	return nil
}