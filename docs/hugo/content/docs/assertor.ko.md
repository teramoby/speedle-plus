+++
title = "토큰 어서터"
description = "ID 토큰을 검증하고 Speedle 인가 요청에 신뢰할 수 있는 주체와 신원 도메인을 제공"
date = 2019-01-21T09:28:30+08:00
weight = 60
draft = false
bref = "평가 요청에는 사용자를 나타내는 ID 토큰이 포함될 수 있습니다. 이 경우 평가 엔진은 어서터 서비스를 호출하여 사용자의 명시적 신원을 얻을 수 있습니다"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["Identity", "Token asserter"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## 토큰 어서터의 역할

구성된 어서터는 토큰을 검증하고 신원 도메인을 포함한 명시적 주체를 반환합니다. Speedle은 해당 신원을 사용하여 정책을 평가합니다. 프로덕션 구현에서는 토큰 서명 또는 인트로스펙션 결과, 발급자, 대상, 유효 기간 및 공급자별 요구 사항을 검증해야 합니다. 웹훅 인터페이스의 존재 자체가 임의의 OAuth/OIDC 토큰과의 자동 호환성을 의미하지는 않습니다.

[토큰 어서션 API](/ko/docs/api/asserter_api/)의 `errCode`는 정수형이며, 선택적 속성은 **객체**이며 유형화된 속성 배열이 아닙니다. `asserterWebhookConfig.endpoint`는 전체 URL입니다. 아래 예제에서는 `/v1/assert`를 사용합니다.

## 결정론적 로컬 목(Mock) 실행

이 목은 `githubtoken`/`github`, `googletoken`/`google`, `gitlabtoken`/`gitlab`의 3개 입력 조합만 인식하며, 각각 해당 도메인의 `user1`을 나타냅니다. 토큰과 공급자 쌍이 일치하지 않으면 실패합니다. **이것은 테스트용 픽스처이며 실제 토큰 유효성 검사가 아닙니다.** 토큰 값을 기록하지 않습니다. 루프백 주소에만 바인딩하고 프로덕션 환경에서는 실제 공급자 통합으로 교체하십시오.

저장소 루트에서 별도의 터미널을 열고 실행합니다:

<!-- docs-check: mock-asserter -->
```bash
go run ./website/tests/examples/asserter
```


<!-- docs-source: website/tests/examples/asserter/main.go -->
```go
// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
```


## 예제 실행 전 준비

[빠른 시작](/ko/quick-start/) 지침에 따라 저장소 루트에서 `./bin/spctl`, `./bin/speedle-pms`, `./bin/speedle-ads`를 빌드합니다. Go 1.26.10 또는 Go 1.27.3(또는 동일한 릴리스 브랜치의 최신 패치 버전), Bash, curl, Python 3이 필요합니다. 각 터미널은 저장소 루트에서 실행합니다. 이 페이지를 위해 새로운 정책 파일을 사용하고 두 서비스를 모두 중지한 후에만 파일을 재설정하십시오.

빈 정책 파일을 초기화합니다:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

터미널 1에서 PMS를 시작하고 계속 실행합니다:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

터미널 2에서 ADS를 시작하고 계속 실행합니다:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true --asserter-endpoint http://127.0.0.1:8080/v1/assert
```

나머지 명령은 세 번째 터미널에서 실행합니다. PMS와 ADS는 동일한 파일을 공유하며, 변경 사항은 watch를 통해 비동기적으로 전파됩니다. 새 정책을 기반으로 결정을 내리기 전에 업데이트가 반영되었는지 확인하십시오. `--insecure=true`는 로컬 루프백 테스트 전용입니다. 프로덕션 연결은 [보안 구성](/ko/docs/security/)을 참조하십시오. 명시적인 PMS 주소를 지정하므로 `~/.spctlconfig`를 수정할 필요가 없습니다. 완료되면 두 서버 터미널에서 Ctrl-C를 누릅니다.


## 신원 도메인별 테스트 정책 생성

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/idd.json
```


이 픽스처는 GitHub의 `user1`에 읽기 권한, Google의 `user1`에 쓰기 권한, 모든 도메인에 대여 권한을 부여합니다. [신원 도메인 예제](/ko/docs/idd/)와 동일한 규칙입니다.

## 토큰 기반 인가 결정 검증

순서대로 Allow, Deny, Allow를 예상합니다. 두 번째 요청은 유효한 **GitLab** 신원이므로 GitHub 전용 읽기 권한을 사용할 수 없습니다.

<!-- docs-check: token-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"githubtoken","tokenType":"github"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


잘못된 토큰은 인가 정책 Deny가 아니라 어서션 오류입니다. 현재 REST 구현은 여전히 HTTP 200과 함께 `allowed=false` 및 비어 있지 않은 `errorMessage`를 반환할 수 있습니다. HTTP 상태뿐만 아니라 응답 본문도 확인하십시오. `reason`만으로 평가 성공 여부를 판단하지 마십시오:

<!-- docs-check: token-invalid -->
```bash
curl --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"invalid-token","tokenType":"google"},"serviceName":"booksvc","resource":"book","action":"write"}' \
  -w '\nHTTP %{http_code}\n' http://127.0.0.1:6734/authz-check/v1/is-allowed
```


완료되면 목 및 두 서버 터미널에서 Ctrl-C를 눌러 중지합니다.

## 공급자 통합 구성

로컬 플래그 대신 완전한 JSON 구성을 통해 다음을 설정할 수 있습니다:

```json
{
  "asserterWebhookConfig": {
    "endpoint": "https://identity.example/v1/assert",
    "clientCert": "/absolute/path/client.crt",
    "clientKey": "/absolute/path/client.key",
    "caCert": "/absolute/path/issuer-ca.crt",
    "httpTimeout": 5
  }
}
```

이것은 서버 구성의 어서터 부분입니다. 배포 시 스토리지 및 서버 설정도 함께 제공하십시오. `caCert`는 웹훅 서버를 확인하고, `clientCert` 및 `clientKey`는 해당 서버에 상호 TLS(mTLS)가 필요한 경우 사용됩니다. 공급자의 문서화된 [OpenID Connect](https://openid.net/connect/) 또는 [OAuth](https://oauth.net/2/) 통합에 따라 토큰을 수집하고 해당 공급자가 실제로 수락하는 토큰 유형을 확인하십시오. PMS/ADS 호출자는 [보안 구성](/ko/docs/security/)에 설명된 대로 별도로 보호하십시오.
