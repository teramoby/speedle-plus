+++
title = "배포"
description = "Go 애플리케이션에 Speedle을 임베드하거나, 공개 컨테이너 이미지 또는 Homebrew로 실행하거나, 독립 실행형 PMS 및 ADS 서비스로 실행합니다."
weight = 304
draft = false
toc = true
linksection = true
tocheading = "h2"
categories = ["docs"]
bref = "임베디드 모드, Docker 이미지, Homebrew, 소스 빌드 및 독립 실행형 PMS와 ADS 서비스."
lastmod = 2026-11-14T10:30:00+08:00
+++

Speedle은 Go 애플리케이션 내에 임베드하거나, 공개된 컨테이너 이미지로 실행하거나, 독립 실행형 PMS 및 ADS 서비스로 실행할 수 있습니다.

- [임베디드 모드](/docs/deployment/embedded/)는 Go 프로세스 내에서 평가 엔진을 실행합니다.
- [Docker 이미지](/ko/docs/deployment/docker/)는 `docker.io/speedle/speedle` 또는 `ghcr.io/teramoby/speedle`에서 `speedle-pms`, `speedle-ads`, `spctl`을 실행합니다.
- [Homebrew](/ko/docs/deployment/homebrew/)는 명령줄 도구와 두 서비스를 설치하고 로컬에서 데모를 실행합니다.

## 배포 방식 선택

Go 프로세스 내에서 평가 엔진을 실행하려면 [임베디드 모드](/docs/deployment/embedded/)를 사용합니다. 애플리케이션이 REST 또는 gRPC를 통해 Speedle을 호출하는 경우 [Docker 이미지](/ko/docs/deployment/docker/)나 독립 실행형 PMS 및 ADS를 사용합니다. [Homebrew](/ko/docs/deployment/homebrew/)는 이 프로그램들을 설치하고 로컬에서 데모를 실행합니다. [빠른 시작](../../quick-start/)에서는 로컬 정책 관리와 인가 확인의 전체 과정을 안내합니다.

## 최신 Go 버전으로 빌드

Linux 또는 macOS 환경에서 Go 1.26.10 또는 Go 1.27.3(또는 동일 릴리스 브랜치의 최신 패치 버전)(Go 1.27.3 권장), Git 및 명령줄 셸을 준비합니다. Go modules를 사용하므로 소스 코드는 쓰기 가능한 임의의 디렉터리에 둘 수 있으며, `GOROOT`, `GOPATH`, `dep` 또는 GOPATH 소스 레이아웃을 수동으로 구성할 필요가 없습니다.

<!-- deployment-check: source-build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

실행 파일은 `bin/spctl`, `bin/speedle-pms`, `bin/speedle-ads`입니다. 로컬 예제를 위해 체크아웃 루트에서 단일 공유 파일을 사용합니다:

<!-- deployment-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

별도의 터미널 창에서 동일한 디렉터리로부터 ADS를 시작합니다:

<!-- deployment-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

PMS 엔드포인트를 지정하여 `spctl`로 서비스와 정책을 생성합니다:

<!-- deployment-check: spctl -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

ADS 엔드포인트를 지정하여 결정을 확인합니다:

<!-- deployment-check: check -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --user alice --action get --resource /docs/public --service-name=docs
```

출력 결과는 다음과 같습니다:

```text
{"allowed":true,"reason":0}
```

## 빠른 시작용 Kubernetes 매니페스트

이 매니페스트는 [빠른 시작](../../quick-start/)과 동일한 단일 파드 개발 구성을 실행하며, 빈 볼륨을 통해 파일 저장소를 공유합니다. 이 구성을 `speedle-dev.yaml`로 저장합니다:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: speedle-dev
spec:
  replicas: 1
  selector:
    matchLabels:
      app: speedle-dev
  template:
    metadata:
      labels:
        app: speedle-dev
    spec:
      containers:
        - name: pms
          image: docker.io/speedle/speedle:<release-tag>
          args: ["speedle-pms", "--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6733", "--insecure=true"]
          ports:
            - containerPort: 6733
          readinessProbe:
            tcpSocket:
              port: 6733
          volumeMounts:
            - name: policies
              mountPath: /data
        - name: ads
          image: docker.io/speedle/speedle:<release-tag>
          args: ["speedle-ads", "--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6734", "--insecure=true"]
          ports:
            - containerPort: 6734
          readinessProbe:
            tcpSocket:
              port: 6734
          volumeMounts:
            - name: policies
              mountPath: /data
      volumes:
        - name: policies
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: speedle-dev
spec:
  type: ClusterIP
  selector:
    app: speedle-dev
  ports:
    - name: pms
      port: 6733
      targetPort: 6733
    - name: ads
      port: 6734
      targetPort: 6734
```

`SPEEDLE_IMAGE`를 배포할 정확한 이미지(예: 릴리스 태그 또는 다이제스트)로 설정합니다. 적용하기 전에 로컬에서 대체하고, 빠른 시작 클라이언트를 사용하는 동안 포트 포워딩을 유지합니다:

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$SPEEDLE_IMAGE" ads="$SPEEDLE_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## 프로덕션 배포

영구 정책 저장소를 사용하고, 컨테이너 이미지 다이제스트를 배포하는 릴리스 버전에 고정하며(릴리스 재시도로 버전 태그가 다시 게시될 수 있음), 관리 및 결정 엔드포인트를 보호하고, 클라이언트 및 정책 백엔드에 TLS/mTLS를 구성하십시오. [플러그형 저장소](../store/)에서는 최신 저장소 구성을 설명하며, [보안](../security/)에서는 최신 전송 계층 플래그를 다룹니다. 사용이 중단된 etcd-operator 예제가 아닌 최신 etcd 문서를 참고하여 etcd 배포 규모를 산정하고 운영하십시오.

저장소에는 과거의 Helm/Kubernetes 예제가 포함되어 있습니다. 이전의 Helm 2 `helm install -n ...`, `stable/etcd-operator`, `apps/v1beta2` 및 `r.authz.fun` 이미지 지침은 위의 배포 흐름과 다릅니다. 컨테이너 풀, 클러스터 어드미션, 레지스트리 접근, 스토리지 및 롤아웃은 사용자 환경에 따라 달라지므로 프로덕션에 적용하기 전에 대상 클러스터에서 확인하십시오.
