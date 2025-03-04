package utils

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	urlLib "net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	resty "github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/samber/lo"
	"github.com/valyala/fastjson"
)

// https://docker-docs.uclv.cu/registry/spec/api/#pulling-an-image
func PullImage(image *Image, dir string) error {
	fmt.Printf("Pull Image %s/%s:%s to %s\n", image.Registry, image.Repository, image.Tag, dir)

	manifest := image.FetchManifest("")
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

	fmt.Println(targetPath, "v1")
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

		fmt.Printf("(%d/%d) Downloading layer Id: %s hash: %s\n", index + 1, len(fsLayers), layerId, blobSum)

		layerDir := path.Join(targetPath, layerId)
		err := ensureDir(layerDir)
		ThrowIfError(err)

		os.WriteFile(path.Join(layerDir, "VERSION"), []byte("1.0"), fs.ModePerm)
		os.WriteFile(path.Join(layerDir, "json"), (layerJson.MarshalTo(nil)), fs.ModePerm)

		err = fetchBlob(image, blobSum, path.Join(layerDir, "layer.tar"), 0)
		ThrowIfError(err)
	})

	// 创建 repositories 文件
	fp, err := os.OpenFile(path.Join(targetPath, "repositories"), os.O_CREATE|os.O_RDWR, 0644)
	ThrowIfError(err)
	_, err = fmt.Fprintf(fp, `{"%s": {"%s":"%s"}}`, image.Slug, image.Tag, history[0].GetStringBytes("id"))
	ThrowIfError(err)
	fp.Close()

	// 打包所有的layer文件
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
	if manifest.Exists("manifests") {
		// 该Tag对应有多个platform的镜像
		digest := ""
		manifestList := manifest.GetArray("manifests")
		info, exist := lo.Find(manifestList, func(item *fastjson.Value) bool {
			if string(item.GetStringBytes("platform", "os")) == image.platform.osName && string(item.GetStringBytes("platform", "architecture")) == image.platform.architecture {
				variant := string(item.GetStringBytes("platform", "variant"))
				if len(image.platform.variant) == 0 || len(variant) == 0 {
					return true
				}
				return image.platform.variant == variant
			}
			return false
		})
		if !exist {
			return fmt.Errorf("Not found platform %s/%s", image.platform.osName, image.platform.architecture)
		}

		digest = string(info.GetStringBytes("digest"))
		manifest = image.FetchManifest(digest)
	}

	digest := string(manifest.GetStringBytes("config", "digest"))

	// 创建目录
	targetFolder := fmt.Sprintf("%s/%s:%s-%s", image.Registry, image.Repository, image.Tag, image.platform.architecture)
	targetFolder = strings.ReplaceAll(targetFolder, "/", "---")
	targetPath := path.Join(dir, targetFolder)
	err := ensureDir(targetPath)

	fmt.Println(targetPath, "v2")
	ThrowIfError(err)

	blobJsonFile := path.Join(targetPath, strings.Split(digest, ":")[1] + ".json")
	err = fetchBlob(image, digest, blobJsonFile, manifest.GetInt64("config", "size"))
	ThrowIfError(err)

	var a fastjson.Arena
	blobJsonText, err := os.ReadFile(blobJsonFile)
	blobJson := parseJson(blobJsonText)
	defaultBlobJson := parseJsonString(`{
		"created": "1970-01-01T08:00:00+08:00",
		"container_config": {
			"Hostname": "",
			"Domainname": "",
			"User": "",
			"AttachStdin": false,
			"AttachStdout": false,
			"AttachStderr": false,
			"Tty": false,
			"OpenStdin": false,
			"StdinOnce": false,
			"Env": null,
			"Cmd": null,
			"Image": "",
			"Volumes": null,
			"WorkingDir": "",
			"Entrypoint": null,
			"OnBuild": null,
			"Labels": null
		}
	}`)

	manifestJson := parseJsonString(fmt.Sprintf(`[{
		"Config": "%s.json",
		"RepoTags": ["%s:%s"],
		"Layers": []
	}]`, strings.Split(digest, ":")[1], image.Slug, image.Tag))

	layers, err := manifest.Get("layers").Array()
	parentId := ""
	fakeLayerid := ""

	lo.ForEach(layers, func(item *fastjson.Value, index int) {
		blobDigest := string(item.GetStringBytes("digest"))
		fakeLayerid = fmt.Sprintf("%x", sha256.Sum256([]byte(parentId+"\n"+blobDigest+"\n")))

		fmt.Printf("[%d/%d (%d)] Downloading layer Id: %s hash: %s\n", index + 1, len(layers), item.GetInt64("size"), fakeLayerid, blobDigest)

		manifestJson.Get("0", "Layers").SetArrayItem(index, a.NewString(path.Join(fakeLayerid, "layer.tar")))

		layerDir := path.Join(targetPath, fakeLayerid)
		err := ensureDir(layerDir)
		ThrowIfError(err)

		// Creating VERSION file
		os.WriteFile(path.Join(layerDir, "VERSION"), []byte("1.0"), fs.ModePerm)
		// 在 layer tar 目录下创建一个 json 文件 =======================
		var jsonData *fastjson.Value
		if index == (len(layers) - 1) {
			// 最后一个 layer 文件 =================================
			blobJson.Del("history")
			blobJson.Del("rootfs")
			blobJson.Del("rootfS")		// Because Microsoft loves case insensitiveness

			jsonData = blobJson
		} else {
			jsonData = defaultBlobJson
		}

		jsonData.Set("id", a.NewString(fakeLayerid))
		if len(parentId) > 0 {
			jsonData.Set("parent", a.NewString(parentId))
		}
		os.WriteFile(path.Join(layerDir, "json"), (jsonData.MarshalTo(nil)), fs.ModePerm)
		parentId = fakeLayerid

		// https://github.com/opencontainers/image-spec/blob/main/layer.md
		// 检查 mediaType
		mediaType := string(item.GetStringBytes("mediaType"))

		layerTarFile := path.Join(layerDir, "layer.tar")
		stat, err := os.Stat(layerTarFile)
		if err == nil && stat.Size() >= item.GetInt64("size") {
			// Blob already exists
			os.Remove(layerTarFile + ".gz")
			return
		}

		savedFile := layerTarFile
		if strings.HasSuffix(mediaType, "gzip") {
			savedFile = layerTarFile + ".gz"
		} else if strings.HasSuffix(mediaType, "zstd") {
			savedFile = layerTarFile + ".zstd"
		}
		err = fetchBlob(image, blobDigest, savedFile, item.GetInt64("size"))
		ThrowIfError(err)

		if strings.HasSuffix(mediaType, ".tar") {
			// layer 文件没有压缩
			return
		}

		fp1, err := os.Open(savedFile)
		ThrowIfError(err)
		defer func () {
			fp1.Close()
		}()

		fp2, err := os.OpenFile(layerTarFile, os.O_CREATE|os.O_RDWR, 0644)
		ThrowIfError(err)
		defer func () {
			fp2.Close()
		}()

		if strings.HasSuffix(mediaType, "gzip") {
			gzipFile, err := gzip.NewReader(fp1)
			ThrowIfError(err)
			io.Copy(fp2, gzipFile)
			gzipFile.Close()
		} else if strings.HasSuffix(mediaType, "zstd") {
			zFile, err := zstd.NewReader(fp1)
			ThrowIfError(err)
			io.Copy(fp2, zFile)
			zFile.Close()
		}

		os.Remove(savedFile)
	})

	// 创建 repositories 文件
	fp, err := os.OpenFile(path.Join(targetPath, "repositories"), os.O_CREATE|os.O_RDWR, 0644)
	ThrowIfError(err)
	_, err = fmt.Fprintf(fp, `{"%s": {"%s":"%s"}}`, image.Slug, image.Tag, fakeLayerid)
	ThrowIfError(err)
	fp.Close()
	// 创建 manifest 文件
	os.WriteFile(path.Join(targetPath, "manifest.json"), (manifestJson.MarshalTo(nil)), fs.ModePerm)

	// 打包所有的layer文件
	fs := os.DirFS(targetPath)
	fp, err = os.OpenFile(targetPath + ".tar", os.O_CREATE|os.O_RDWR, 0644)
	ThrowIfError(err)

	tarFile := tar.NewWriter(fp)
	err = tarFile.AddFS(fs)
	ThrowIfError(err)

	fmt.Printf("to load image file: docker load -i %s.tar", targetFolder)
	return tarFile.Close()
}


// 检测 blob 的下载地址
func detectBlobUrl(image *Image, blobSum string) string {
	baseUrl := fmt.Sprintf("%s://%s", image.protocol, image.Registry)
	// 从官方地址下载
	originalUrl := fmt.Sprintf("%s/v2/%s/blobs/%s", baseUrl, image.Repository, blobSum)

	if image.Registry == "registry-1.docker.io" && len(image.mirror) > 0 && strings.HasPrefix(image.mirror, "http") {
		// 从 mirror 下载 image blob
		token := image.GetToken("pull")
		url := fmt.Sprintf("%s/v2/%s/blobs/%s", image.mirror, image.Repository, blobSum)
		resp, _ := resty.New().SetTimeout(5 * time.Second).
		 	// 不自动重定向
			SetRedirectPolicy(resty.NoRedirectPolicy()).R().
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			Head(url)

		if resp.StatusCode() >= 200 && resp.StatusCode() < 400 {
			proxy := os.Getenv("DOCKER_BLOB_REVERSE_PROXY")
			if proxy != "" && strings.HasPrefix(proxy, "http") && resp.StatusCode() >= 300 {
				// 重定向到新的地址
				newUrl := resp.Header().Get("location")

				parsedURL, err := urlLib.Parse(newUrl)
				if err != nil {
					return originalUrl
				}
				parsedURL.Scheme = ""
				parsedURL.Host = ""
				url = proxy + parsedURL.String()
			}
			return url
		}
	}

	return originalUrl
}
func fetchBlob(image *Image, blobSum string, output string, totalSize int64) error {
	if totalSize > 0 {
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
	}

	token := image.GetToken("pull")
	url := detectBlobUrl(image, blobSum)
	fmt.Printf("Downlaoding blob file: %q\n", url)

	return continueDownload(url, output, token, totalSize)
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
		SetDoNotParseResponse(true).
		Get(url)

	defer func ()  {
		resp.RawResponse.Body.Close()
	}()

	if resp.StatusCode() >= 300 || err != nil {
		ThrowIfError(err)
		ThrowIfError(fmt.Errorf("Failed to download blob with code %d", resp.StatusCode()))
	}
	var file *os.File
	if resp.StatusCode() == 206 {
		// 断点续传
		file, err = os.OpenFile(output, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	} else {
		file, err = os.OpenFile(output, os.O_RDWR|os.O_CREATE, 0644)
	}
	ThrowIfError(err)

	defer func ()  {
		file.Close()
	}()
	_, err = io.Copy(file, resp.RawResponse.Body)

	return err
}
