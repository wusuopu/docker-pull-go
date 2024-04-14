package utils

import (
	"os"
)

// 确保目录存在
func ensureDir(dir string) error {
  _, err := os.Stat(dir)
	if err != nil {
		return os.MkdirAll(dir, os.ModePerm)
	}
	return nil
}

func checkExist(path string) bool {
  _, err := os.Stat(path)
  return err == nil
}