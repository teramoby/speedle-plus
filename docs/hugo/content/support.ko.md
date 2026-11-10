+++
draft= false
title = "지원"
description = "Speedle에 대한 자주 묻는 질문과 답변"
toc = false
tocheading = "h2"
tocsidebar = false
icon = "8. Community.svg"
lastmod = 2026-11-07T20:54:01+08:00
+++

## FAQ

### Speedle은 누구를 위한 것인가요?

Speedle은 접근 제어가 필요한 애플리케이션을 위한 오픈 소스 인가 엔진입니다. 시스템에 임베드하거나 서비스로 실행할 수 있어 인가 정책을 애플리케이션 코드에 하드코딩하는 대신 중앙에서 관리할 수 있습니다.

### 왜 또 다른 정책 엔진인가요?

[시작하기](/ko/quick-start/) 가이드는 약 15분 만에 전체 워크플로를 안내합니다. 다른 대안들과 Speedle의 정책 모델 및 결정 흐름을 비교하는 가장 빠른 방법입니다.

### 어떻게 기여할 수 있나요?

[기여 가이드](/ko/contribute/#code)를 참조하세요.

### Speedle의 향후 계획은 무엇인가요?

Speedle은 오픈 소스 프로젝트로 유지 관리됩니다. 커뮤니티의 필요에 따라 버그 수정, 문서화, 새로운 기능 개발이 진행됩니다.

### OPA, Kubernetes RBAC, Istio RBAC와의 차이점은 무엇인가요?

Speedle, OPA, Kubernetes RBAC, Istio RBAC는 모두 인가 또는 접근 제어를 제공합니다. Kubernetes RBAC와 Istio RBAC는 각각 Kubernetes 및 Istio 내부의 인가를 다루며, Speedle과 OPA는 범용 인가 솔루션입니다. Casbin과 Ladon도 다른 범용 옵션입니다. 비교 시 다음 사항을 고려하세요:

1. 정책 언어가 사용하기 쉬운가?
2. 한 번의 인가 결정에 시간이 얼마나 걸리는가?
3. 확장성이 우수한가?
4. 지원되는 정책 모델이 충분히 유연한가?

### Speedle에 내장 인증 메커니즘이 없는 이유는 무엇인가요?

Speedle은 인가(Authorization)에 집중합니다. 인증(Authentication)은 제공하지 않습니다.

### 자체 백엔드 스토리지를 연결하려면 어떻게 해야 하나요?

[스토리지 확장](/ko/docs/store/)을 참조하세요.

### 자체 토큰 어설터를 연결하려면 어떻게 해야 하나요?

[토큰 어설터](/ko/docs/assertor/)를 참조하세요.

<div class="row" style="padding-top: 50px">
    <div class="col col-2"></div>
    <div class="col col-8 center" style="font-size: 25px">
    <div>
        <p>원하는 답변을 찾을 수 없나요?</p>
        <p>Speedle Discord 커뮤니티에 참여하세요</p>
        <span>
          <a class="button primary started" style="font-size:18px" href="https://discord.gg/uaY2xVsbbT">
            <span>Discord 참여 →</span>
          </a>
        </span>
    </div>
    </div>
    <div class="col col-2 right">
      <a title="Discord" href="https://discord.gg/uaY2xVsbbT">
        <img class="svg" src="/img/speedle/Discord-color.svg" />
      </a>
    </div>
</div>
