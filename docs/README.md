# Docker 拉取镜像的网络请求步骤分析
当 Docker 客户端执行 docker pull <镜像名> 时，会通过以下网络请求流程与 Docker Registry 交互：

1. 解析镜像名称
  * 示例：ubuntu:latest → 默认解析为 registry-1.docker.io/library/ubuntu:latest。
  * 拆分结构：<registry>/<repository>:<tag>，若未指定 registry，则默认为 Docker Hub。

2. 获取认证令牌（Token）
触发认证：首次请求 Manifest 时，Registry 返回 401 Unauthorized，并在响应头中携带认证信息：

```
WWW-Authenticate: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/ubuntu:pull"
```

请求 Token：客户端向认证服务发送请求，获取临时访问令牌：
```
GET https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/ubuntu:pull
```

响应示例：

```
{ "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." }
```

3. 拉取镜像 Manifest
请求 Manifest：使用 Token 访问镜像的 Manifest 文件（描述镜像结构和各层信息）：

```
GET /v2/library/ubuntu/manifests/latest
Headers:
  Authorization: Bearer <token>
  Accept: application/vnd.docker.distribution.manifest.v2+json
```

响应内容：返回 JSON 格式的 Manifest，包含镜像配置和各层（layers）的 digest。

4. 下载镜像层（Blobs）
逐层下载：根据 Manifest 中的 layers 列表，依次请求每个层的二进制数据：

```
GET /v2/library/ubuntu/blobs/<digest>
Headers:
  Authorization: Bearer <token>
```

存储层数据：每层以压缩包（如 .tar.gz）形式返回，需按 Docker 的存储格式保存。


5. 组装镜像
  * 生成元数据：根据 Manifest 和层数据生成镜像的配置文件（如 config.json）和目录结构。
  * 加载镜像：将下载内容整合为 Docker 可识别的格式（通常为 .tar 文件），通过 docker load 导入。


# 配置镜像拉取的反向代理

```
server {
  location ~ ^/docker-auth/ {
    rewrite    /docker-auth/(.+) /$1 break;
    proxy_pass https://auth.docker.io;
    proxy_set_header Host auth.docker.io;
  }
  location ~ ^/docker-registry/ {
    rewrite    /docker-registry/(.+) /$1 break;
    proxy_pass https://registry-1.docker.io;
    proxy_set_header Host registry-1.docker.io;
  }
  location ~ ^/docker-blob/ {
    rewrite    /docker-blob/(.+) /$1 break;
    proxy_pass https://production.cloudflare.docker.com;
    proxy_set_header Host production.cloudflare.docker.com;
  }
}
```

然后通过命令拉取镜像：
```
DOCKER_REGISTRY_REVERSE_PROXY=<host>/docker-registry DOCKER_AUTH_REVERSE_PROXY=<host>/docker-auth/token DOCKER_BLOB_REVERSE_PROXY=<host>/docker-blob  go run main.go pull alpine --mirror=<host>/docker-registry --insecure-registry
```
