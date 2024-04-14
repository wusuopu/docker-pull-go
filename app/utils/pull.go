package utils

import (
	"archive/tar"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	resty "github.com/go-resty/resty/v2"
	"github.com/samber/lo"
	"github.com/valyala/fastjson"
)

func PullImage(image *Image, dir string) error {
	fmt.Printf("Pull Image %s/%s:%s to %s\n", image.Registry, image.Repository, image.Tag, dir)

	manifest := image.FetchManifest()
	schemaVersion := manifest.Get("schemaVersion").GetInt()
	if schemaVersion == 1 {
		return pullV1(image, manifest, dir)
	} else if schemaVersion == 2 {
		return pullV2(image, manifest, dir)
	} else {
		return fmt.Errorf("Unsupported schema version %d", schemaVersion)
	}
}

// ==================== schema v1 ====================
func pullV1(image *Image, manifest *fastjson.Value, dir string) error {
	targetFolder := fmt.Sprintf("%s/%s:%s-%s", image.Registry, image.Repository, image.Tag, image.platform.architecture)
	targetFolder = strings.ReplaceAll(targetFolder, "/", "---")
	targetPath := path.Join(dir, targetFolder)
	err := ensureDir(targetPath)

	fmt.Println(targetPath)
	ThrowIfError(err)

  client := resty.New()
	client.SetTimeout(10 * time.Second)

	fsLayers, err := manifest.Get("fsLayers").Array()
	ThrowIfError(err)

	historyRaw, err := manifest.Get("history").Array()
	ThrowIfError(err)
	history := lo.Map(historyRaw, func(item *fastjson.Value, index int) *fastjson.Value {
		data := string(item.Get("v1Compatibility").GetStringBytes())
		// fmt.Println(index, data)
		return parseJsonString(data)
	})

	lo.ForEach(fsLayers, func(item *fastjson.Value, index int) {
		blobSum := string(item.GetStringBytes("blobSum"))

		layerJson := history[index]
		layerId := string(layerJson.GetStringBytes("id"))

		fmt.Printf("Downloading layer Id: %s hash: %s\n", layerId, blobSum)

		layerDir := path.Join(targetPath, layerId)
		err := ensureDir(layerDir)
		ThrowIfError(err)

		os.WriteFile(path.Join(layerDir, "VERSION"), []byte("1.0"), fs.ModePerm)
		os.WriteFile(path.Join(layerDir, "json"), (layerJson.MarshalTo(nil)), fs.ModePerm)

		err = fetchBlob(image, blobSum, path.Join(layerDir, "layer.tar"))
		ThrowIfError(err)
	})

	// 创建 repositories 文件
	fp, err := os.OpenFile(path.Join(targetPath, "repositories"), os.O_CREATE|os.O_RDWR, 0644)
	ThrowIfError(err)
	_, err = fmt.Fprintf(fp, `{"%s": {"%s":"%s"}}`, image.Slug, image.Tag, history[0].GetStringBytes("id"))
	ThrowIfError(err)
	fp.Close()

	fs := os.DirFS(targetPath)
	fp, err = os.OpenFile(targetPath + ".tar", os.O_CREATE|os.O_RDWR, 0644)
	ThrowIfError(err)

	tarFile := tar.NewWriter(fp)
	err = tarFile.AddFS(fs)
	ThrowIfError(err)

	fmt.Printf("to load image file: docker load -i %s.tar", targetFolder)
	return tarFile.Close()
}
// ==================== schema v2 ====================
func pullV2(image *Image, manifest *fastjson.Value, dir string) error {
	return nil
}


// 检测 blob 的下载地址
func detectBlobUrl(image *Image, blobSum string) string {
	baseUrl := fmt.Sprintf("%s://%s", image.protocol, image.Registry)
	if image.Registry == "registry-1.docker.io" && len(image.mirror) > 0 && strings.HasPrefix(image.mirror, "http") {
		// 从 mirror 下载 image blob
		token := image.GetToken("pull")
		baseUrl = image.mirror
		url := fmt.Sprintf("%s/v2/%s/blobs/%s", baseUrl, image.Repository, blobSum)
		resp, err := resty.New().SetTimeout(5 * time.Second).R().
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			Head(url)

		if err == nil && resp.StatusCode() < 300 {
			return url
		}
	}
	// 从官方地址下载
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", baseUrl, image.Repository, blobSum)
	return url
}
func fetchBlob(image *Image, blobSum string, output string) error {
	token := image.GetToken("pull")
	url := detectBlobUrl(image, blobSum)
	fmt.Printf("Downlaoding blob file: %q\n", url)

	return continueDownload(url, output, token, 0)
}

// 断点续传下载
func continueDownload(url string, output string, token string, totalSize int64) error {
	if totalSize == 0 {
		resp, err := resty.New().SetTimeout(5 * time.Second).R().
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			Head(url)
		ThrowIfError(err)

		// 检查该文件的大小
		totalSize, err = strconv.ParseInt(resp.Header().Get("Content-Length"), 10, 64)
		ThrowIfError(err)
	}

	var currentSize int64 = 0
	stat, err := os.Stat(output)
	if err == nil {
		currentSize = stat.Size()
	}
	if currentSize  == totalSize {
		// Blob already exists
		fmt.Printf("skipping existing %s\n", output)
		return nil
	}

	resp, err := resty.New().R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetHeader("Range", fmt.Sprintf("bytes=%d-", currentSize)).
		SetOutput(output).
		Get(url)

	if resp.StatusCode() != 200 || err != nil {
		// os.Remove(output)
		ThrowIfError(err)
		ThrowIfError(fmt.Errorf("Failed to download blob with code %d", resp.StatusCode()))
	}
	return nil
}