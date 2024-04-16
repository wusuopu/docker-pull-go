package utils

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	resty "github.com/go-resty/resty/v2"
)


func PushImage(filename string, image *Image) error {
	fmt.Printf("Pull Image %s to %s/%s:%s\n", filename, image.Registry, image.Repository, image.Tag)
	dir, willDelete, err := detectImageFile(filename)
	if err != nil {
		return err
	}

	defer func () {
		if willDelete {
			os.RemoveAll(dir)
		}
	}()

	return pushDir(image, dir)
}

// 检查指定的镜像文件是否是一个目录，否则解压
func detectImageFile(filename string) (dir string, willDelete bool, err error) {
	var info os.FileInfo
	if info, err = os.Stat(filename); err != nil {
		return
	}
	if info.IsDir() {
		// 当前指定的路径是一个目录，则直接上传该目录下的文件
		dir = filename
		willDelete = false
		return
	}

	// 当前指定的文件路径是一个 tar 包，先解压到临时目录，上传完成后再删除临时文件
	var targetFolder string
	if targetFolder, err = os.MkdirTemp("", "oci-*"); err != nil {
		return
	}
	dir = targetFolder
	willDelete = true

	var fp *os.File
	if fp, err = os.Open(filename); err != nil {
		return
	}
	fmt.Printf("extract tar %s to %s\n", filename, targetFolder)

	tarFile := tar.NewReader(fp)
	for true {
		header, e := tarFile.Next()
		if e == io.EOF {
			break
		}
		fmt.Printf("extract file %s ...\n", header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(targetFolder+"/"+header.Name, 0755); err != nil {
				return
			}
		case tar.TypeReg:
			outFilename := path.Join(targetFolder, header.Name)
			ensureDir(path.Dir(outFilename))
			outFile, e := os.OpenFile(outFilename, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if e != nil {
				err = e
				return
			}
			io.Copy(outFile, tarFile)
			outFile.Close()
		}
	}

	return
}
func pushDir(image *Image, dir string) (err error) {
	var content []byte
	if content, err = os.ReadFile(path.Join(dir, "manifest.json")); err != nil {
		return
	}
	manifestJson := parseJson([]byte(content))
	config := manifestJson.GetStringBytes("0", "Config")
	if config == nil {
		config = manifestJson.GetStringBytes("0", "config")
	}

	configFilename := path.Join(dir, string(config))
	if content, err = os.ReadFile(configFilename); err != nil {
		return
	}

	var blobDigest string
	var blobSize int64
	// 1.上传 config json 文件
	if blobDigest, blobSize, err = uploadBlob(image, configFilename, "application/vnd.docker.container.image.v1+json"); err != nil {
		return
	}

	// 2.上传 layer 文件
	layers := manifestJson.GetArray("0", "Layers")
	if layers == nil {
		layers = manifestJson.GetArray("0", "layers")
	}
	
	newManifestJson := parseJsonString(fmt.Sprintf(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"digest": "%s",
			"size": %d,
			"mediaType": "application/vnd.docker.container.image.v1+json"
		},
		"layers": []
	}`, blobDigest, blobSize))
	for idx, item := range layers {
		name, _ := item.StringBytes()
		layerFilename := path.Join(dir, string(name))
		blobDigest, blobSize, err = uploadLayer(image, layerFilename)
		if err != nil {
			return
		}
		newManifestJson.Get("layers").SetArrayItem(idx, parseJsonString(fmt.Sprintf(`{
			"digest": "%s",
			"size": %d,
			"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip"
		}`, blobDigest, blobSize)))
	}
	// 3.上传 manifest.json 内容
	return uploadManifest(image, newManifestJson.MarshalTo(nil))
}


// https://docker-docs.uclv.cu/registry/spec/api/#pushing-an-image
func uploadBlob(image *Image, filename string, mediaType string) (digest string, size int64, err error) {
	fmt.Printf("Uploading blob %s ...\n", filename)

	if digest, err = computeDigest(filename); err != nil {
		return
	}
	var fileinfo os.FileInfo
	if fileinfo, err = os.Stat(filename); err != nil {
		return
	}
	size = fileinfo.Size()

	// image.pushToken = ""		// 每次申请新的 token，以免过期
	token := image.GetToken("push")
	baseUrl := fmt.Sprintf("%s://%s/v2/%s", image.protocol, image.Registry, image.Repository)
	client := resty.New()
	client.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))

	var resp *resty.Response
	if resp, err = client.R().Head(fmt.Sprintf("%s/blobs/%s", baseUrl, digest)); err != nil {
		return
	}
	if resp.StatusCode() == 200 {
		fmt.Printf("blob %s (%s) already exists\n", filename, digest)
		// 文件已存在，直接返回
		return
	}

	// POST /v2/<name>/blobs/uploads/ 创建一个 upload uuid
	if resp, err = client.R().Post(fmt.Sprintf("%s/blobs/uploads/", baseUrl)); err != nil {
		return
	}
	uploadUrl := resp.Header().Get("Location")
	if len(uploadUrl) == 0 {
		err = fmt.Errorf("upload url is empty with StatusCode: %d", resp.StatusCode())
		return
	}
	if !strings.HasPrefix(uploadUrl, "http") {
		// 若返回的 location 是相对路径，则补全 url 地址
		uploadUrl = fmt.Sprintf("%s://%s%s", image.protocol, image.Registry, uploadUrl)
	}

	// 整体上传 PUT /v2/<name>/blobs/uploads/<uuid>?digest=<digest>
  var fp *os.File
	if fp, err = os.Open(filename); err != nil {
		return
	}
	defer fp.Close()

	resp, err = client.R().
		SetQueryParam("digest", digest).
		SetHeader("Content-Length", fmt.Sprintf("%d", size)).
		SetHeader("Content-Type", "application/octet-stream").
		SetBody(fp).
		Put(uploadUrl)

	// TODO 对于大文件分片断点上传 PATCH /v2/<name>/blobs/uploads/<uuid>
	if err != nil {
		return
	}
	if resp.StatusCode() != 201 {
		err = fmt.Errorf("upload blob failed with StatusCode: %d", resp.StatusCode())
		return
	}

	fmt.Printf("blob %s to %s\n", filename, digest)
	return
}
func uploadLayer(image *Image, filename string) (digest string, size int64, err error) {
	// 检查当前的 layer 文件是否为 gzip 压缩
	var fp *os.File
	if fp, err = os.Open(filename); err != nil {
		return
	}
	defer fp.Close()

	buff := make([]byte, 512)
	if _, err = fp.Read(buff); err != nil {
		return
	}

	fileType := http.DetectContentType(buff)
	layerFilename := ""
	if !strings.HasSuffix(fileType, "gzip") {
		// 先对 layer 进行 gzip 压缩
		fp.Seek(0, io.SeekStart)
		layerFilename = filename + ".gz"

		var fp1 *os.File
		if fp1, err = os.OpenFile(layerFilename, os.O_CREATE|os.O_RDWR, 0644); err != nil {
			return
		}
		defer fp1.Close()

		gzipFile := gzip.NewWriter(fp1)
		if _, err = io.Copy(gzipFile, fp); err != nil {
			return
		}
		if err = gzipFile.Flush(); err != nil {
			return
		}
		if err = gzipFile.Close(); err != nil {
			return
		}
	} else {
		layerFilename = filename
	}

	return uploadBlob(image, layerFilename, "application/vnd.docker.image.rootfs.diff.tar.gzip")
}
func uploadManifest(image *Image, content []byte) (err error) {
	// PUT /v2/<name>/manifests/<reference>
	token := image.GetToken("push")
	url := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", image.protocol, image.Registry, image.Repository, image.Tag)

	client := resty.New()
	client.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))

	var resp *resty.Response
	resp, err = client.R().
		SetHeader("Content-Type", "application/vnd.docker.distribution.manifest.v2+json").
		SetBody(content).
		Put(url)

	if err != nil {
		return
	}
	if resp.StatusCode() != 201 {
		err = fmt.Errorf("upload blob failed with StatusCode: %d", resp.StatusCode())
		return
	}
	fmt.Printf("manifest to %s\n", resp.Header().Get("Location"))
	return
}

// 计算文件的 sha256
func computeDigest(filename string) (digest string, err error) {
  var fp *os.File
	if fp, err = os.Open(filename); err != nil {
		return
	}
	defer fp.Close()

	h := sha256.New()
	if _, err = io.Copy(h, fp); err != nil {
		return
	}

	digest = fmt.Sprintf("sha256:%x", h.Sum(nil))
	return
}