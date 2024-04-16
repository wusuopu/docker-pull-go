# docker-pull-go
pull/push docker images without docker cli.

## Build
requirement: golang 1.22

```
cd app/
go build main.go
```

## Usage
### Pull Image
```
main pull <image> <dir> [--username=STRING] [--password=STRING] [--insecure-registry]

eg:
# pull from docker hub
main pull golang:1.20.6-alpine ~/Downloads/
# pull from docker hub with other os/arch; default is linux/amd64
main pull nginx:stable ~/Downloads/ --os linux --architecture arm --variant v5
# pull from private registry
main pull my-registry.com/namespace/repo:tag ~/Downloads/ --username <username> --password <password> --insecure-registry
```


### Push Image
```
main push <file> <image> [--username=STRING] [--password=STRING] [--insecure-registry]

eg:
# push to docker hub from tar file
main push image.tar <user>/<repo>:<tag> --username <username> --password <password>
# push to private registry from files in a folder
main push image_files/ my-registry.com/namespace/repo:tag --username <username> --password <password> --insecure-registry
```

### TODO
  * Chunked Upload large blob file when push image
  * Support multiple os/arch when push image

