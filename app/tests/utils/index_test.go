package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"main.go/utils"
)

func Test_ParseImage(t *testing.T) {
	var ret utils.Image
	// 官方镜像

	ret.ParseImage("node")
	assert.Equal(t, "library", ret.Namespace)
	assert.Equal(t, "node", ret.ImageName)
	assert.Equal(t, "latest", ret.Tag)
	assert.Equal(t, "node", ret.Slug)
	assert.Equal(t, "library/node", ret.Repository)
	assert.Equal(t, "registry-1.docker.io", ret.Registry)

	ret.ParseImage("node:10-apline")
	assert.Equal(t, "library", ret.Namespace)
	assert.Equal(t, "node", ret.ImageName)
	assert.Equal(t, "10-apline", ret.Tag)
	assert.Equal(t, "node", ret.Slug)
	assert.Equal(t, "library/node", ret.Repository)
	assert.Equal(t, "registry-1.docker.io", ret.Registry)

	// 用户镜像
	ret.ParseImage("user/image")
	assert.Equal(t, "user", ret.Namespace)
	assert.Equal(t, "image", ret.ImageName)
	assert.Equal(t, "latest", ret.Tag)
	assert.Equal(t, "user/image", ret.Slug)
	assert.Equal(t, "user/image", ret.Repository)
	assert.Equal(t, "registry-1.docker.io", ret.Registry)

	ret.ParseImage("user/image:tag")
	assert.Equal(t, "user", ret.Namespace)
	assert.Equal(t, "image", ret.ImageName)
	assert.Equal(t, "tag", ret.Tag)
	assert.Equal(t, "user/image", ret.Slug)
	assert.Equal(t, "user/image", ret.Repository)
	assert.Equal(t, "registry-1.docker.io", ret.Registry)

	// 第三方仓库
	ret.ParseImage("localhost:5000/user/image")
	assert.Equal(t, "user", ret.Namespace)
	assert.Equal(t, "image", ret.ImageName)
	assert.Equal(t, "latest", ret.Tag)
	assert.Equal(t, "localhost:5000/user/image", ret.Slug)
	assert.Equal(t, "user/image", ret.Repository)
	assert.Equal(t, "localhost:5000", ret.Registry)

	ret.ParseImage("127.0.0.1/user/image:tag")
	assert.Equal(t, "user", ret.Namespace)
	assert.Equal(t, "image", ret.ImageName)
	assert.Equal(t, "tag", ret.Tag)
	assert.Equal(t, "127.0.0.1/user/image", ret.Slug)
	assert.Equal(t, "user/image", ret.Repository)
	assert.Equal(t, "127.0.0.1", ret.Registry)

	// tag是hash
	ret.ParseImage("node@sha256:075012d2072be942e17da73a35278be89707266010fb6977bfc43dae5d492ab4")
	assert.Equal(t, "library", ret.Namespace)
	assert.Equal(t, "node", ret.ImageName)
	assert.Equal(t, "sha256:075012d2072be942e17da73a35278be89707266010fb6977bfc43dae5d492ab4", ret.Tag)
	assert.Equal(t, "node", ret.Slug)
	assert.Equal(t, "library/node", ret.Repository)
	assert.Equal(t, "registry-1.docker.io", ret.Registry)
}