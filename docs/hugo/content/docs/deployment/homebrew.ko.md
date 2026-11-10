+++
title = "Homebrew 설치"
description = "Homebrew로 speedle-spctl, speedle-pms, speedle-ads를 설치하고 로컬에서 데모를 실행합니다."
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Homebrew는 명령줄 도구와 두 서비스를 설치한 뒤, 127.0.0.1에서 하나의 정책 파일을 공유하여 실행합니다."
lastmod = 2026-11-10T13:49:59+08:00
+++

macOS 또는 Linux에서 [Homebrew](https://brew.sh/)로 Speedle 명령줄 도구를 설치합니다. Homebrew는 공식 조직 밖의 tap에 있는 formula를 신뢰하기 전에는 그 formula를 설치하거나 시작하지 않습니다.

```bash
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
```

Formula 이름은 `spctl`입니다. macOS에서 명령은 `speedle-spctl`이므로, 이 설치는 Apple Gatekeeper의 `/usr/sbin/spctl`을 바꾸지 않습니다. Linux에서는 같은 프로그램을 `spctl`로도 호출할 수 있습니다.

정책 관리 서비스와 인가 결정 서비스는 별도의 formula입니다.

```bash
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

명령줄 도구와 두 서비스를 함께 설치합니다.

```bash
brew install teramoby/tap/speedle
```

`spctl`, `speedle-pms`, `speedle-ads`는 각각 해당 릴리스의 macOS 또는 Linux 바이너리를 설치하고, 그 릴리스의 `checksums.txt`와 대조합니다.

<a id="demo-with-homebrew"></a>

## Homebrew로 데모 실행

두 서비스를 시작합니다. 두 서비스는 `127.0.0.1`에서 수신하며 하나의 파일 정책 저장소를 공유합니다.

```bash
brew services start speedle-pms
brew services start speedle-ads
```

`speedle-pms`는 `127.0.0.1:6733`에서 수신합니다. `speedle-ads`는 `127.0.0.1:6734`에서 수신합니다. 둘 다 `--store-type file`을 사용하며, 정책 파일은 `$(brew --prefix)/var/speedle/policies.json`입니다. 로그는 `$(brew --prefix)/var/log/`에 있습니다. `--insecure=true`는 이 루프백 데모에만 사용합니다. TLS는 [보안](/ko/docs/security/)을 참조하세요.

서비스와 정책을 만듭니다. Linux에서는 `speedle-spctl` 대신 `spctl`을 사용할 수 있습니다.

```bash
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

ADS에 `user1`이 `res1`을 get 할 수 있는지 묻습니다.

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":true`가 포함됩니다. 두 서비스는 공유 파일에서 정책 변경을 읽습니다. 다음으로 중지합니다.

```bash
brew services stop speedle-ads
brew services stop speedle-pms
```
