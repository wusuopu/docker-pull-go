package utils

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	resty "github.com/go-resty/resty/v2"
	"github.com/valyala/fastjson"
)

func parseJson(data []byte) *fastjson.Value {
	var p fastjson.Parser

	value, err := p.ParseBytes(data)
	ThrowIfError(err)
	return value
}
func parseJsonString(data string) *fastjson.Value {
	var p fastjson.Parser

	value, err := p.Parse(data)
	ThrowIfError(err)
	return value
}

type Image struct {
	Namespace string;
	ImageName string;
	Tag string;
	Slug string;
	Repository string;
	Registry string;

	mirror string;
	protocol string;
	username string;
	password string;
	pullToken string;
	pushToken string;

	platform struct {
		architecture string;
		osName string;
		variant string;
 };
}

func (i *Image) ParseImage(image string) {
	// 默认名字
	namespace := "library"
	tag := "latest"
	imageName := ""

	imgParts := strings.Split(image, "/")
	name := imgParts[len(imgParts) - 1]
	if strings.Contains(name, "@") {            // 参数是 <image>@sha256:<hash> 的格式
		nameParts := strings.Split(name, "@")
		imageName = nameParts[0]
		tag = nameParts[1]                        // sha256:<hash>
	} else if strings.Contains(name, ":") {     // 参数是 <image>:<tag> 的格式
		nameParts := strings.Split(name, ":")
		imageName = nameParts[0]
		tag = nameParts[1]                        // <tag>
	} else {                                    // 参数是 <image> 的格式
		imageName = name
		tag = "latest"
	}

	i.Tag = tag
	i.ImageName = imageName

	if len(imgParts) > 1 && (strings.Contains(imgParts[0], ".") || strings.Contains(imgParts[0], ":")) {			// 第三方仓库: <registry>/<namespace>/<image>
		i.Registry = imgParts[0]
		namespace = strings.Join(imgParts[1:len(imgParts)-1], "/")
		i.Slug = fmt.Sprintf("%s/%s/%s", i.Registry, namespace, imageName)
	} else {
		i.Registry = "registry-1.docker.io"
		if len(imgParts) == 1 {																					// 官方镜像: <image>
			namespace = "library"
			i.Slug = imageName
		} else {																												// 用户镜像：<namespace>/<image>
			namespace = strings.Join(imgParts[0:len(imgParts)-1], "/")
			i.Slug = fmt.Sprintf("%s/%s", namespace, imageName)
		}
	}
	i.Repository = fmt.Sprintf("%s/%s", namespace, imageName)
	i.Namespace = namespace
}

func (i *Image) requestToken(action string) string {
	// https://distribution.github.io/distribution/spec/auth/token/
	manifestUrl := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", i.protocol, i.Registry, i.Repository, i.Tag)

	client := resty.New()
	// client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	req := client.NewRequest()
	client.SetTimeout(10 * time.Second)

	resp, err := req.Get(manifestUrl)
	ThrowIfError(err)

	if resp.StatusCode() != 401 && resp.StatusCode() != 200{
		ThrowIfError(fmt.Errorf("Request Token with status %d", resp.StatusCode()))
	}

	// 获取 token 认证的 url
	wwwAuth := resp.Header().Get("www-authenticate")
	if !strings.HasPrefix(wwwAuth, "Bearer realm=\"") {
		// Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:node:pull
		err = fmt.Errorf("Invalid www-authenticate header: %s", wwwAuth)
		ThrowIfError(err)
	}
	url := strings.Split(wwwAuth, `"`)[1]
	req = client.NewRequest()
	req.SetQueryParam("service", regexp.MustCompile(`service="([^"]+)"`).FindStringSubmatch(wwwAuth)[1])
	req.SetQueryParam("scope", fmt.Sprintf("repository:%s:%s", i.Repository, action))
	if len(i.username) > 0 && len(i.password) > 0 {
		req.SetBasicAuth(i.username, i.password)
	}
	resp, err = req.Get(url)

	data := parseJson(resp.Body())
	return string(data.Get("token").GetStringBytes())
}

func (i *Image) GetToken(action string) string {
	if action == "pull" {
		if len(i.pullToken) == 0 {
			i.pullToken = i.requestToken("pull")
		}

		return i.pullToken
	}

	if len(i.pushToken) == 0 {
		i.pushToken = i.requestToken("pull,push")
	}

	return i.pushToken
}

func (i *Image) FetchManifest(digest string) *fastjson.Value {
	token := i.GetToken("pull")
	if len(digest) == 0 {
		digest = i.Tag
	}
	manifestUrl := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", i.protocol, i.Registry, i.Repository, digest)

	client := resty.New()
	client.SetTimeout(10 * time.Second)

	req := client.NewRequest()
	req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	headers := map[string][]string{
		"Accept": []string{
			"application/vnd.docker.distribution.manifest.v2+json",
			"application/vnd.docker.distribution.manifest.list.v2+json",
			"application/vnd.docker.distribution.manifest.v1+json",
		},
	}
	req.SetHeaderMultiValues(headers)
	resp, err := req.Get(manifestUrl)
	ThrowIfError(err)

	if resp.StatusCode() != 200 {
		fmt.Println("manifestUrl:", manifestUrl)
		fmt.Println(string(resp.Body()))
		ThrowIfError(fmt.Errorf("FetchManifest with status %d", resp.StatusCode()))
	}

	data := parseJson(resp.Body())
	return data
}


func NewImage(
	name string, username string, password string, insecureRegistry bool, mirror string,
	osName string, architecture string, variant string,
) Image {
	var i Image
	i.ParseImage(name)
	i.mirror = mirror
	i.username = username
	i.password = password
	i.protocol = "https"
	if insecureRegistry {
		i.protocol = "http"
	}

	i.platform.architecture = architecture
	i.platform.osName = osName
	i.platform.variant = variant
	return i
}