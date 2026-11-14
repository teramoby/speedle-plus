+++
title = "결정 서비스 확장"
description = "사용자 정의 정책 저장소나 서비스 어댑터를 사용합니다."
weight = 310
lastmod = 2026-11-14T18:30:00+08:00
draft = false
toc = false
categories = ["docs"]
bref = "사용자 정의 정책 저장소나 서비스 어댑터를 사용합니다."
+++

`store.StoreBuilder`를 구현하고 `store.Register`로 고유한 이름을 등록한 뒤 실행 프로그램에서 패키지를 가져옵니다. `GetStoreParams`는 추가 플래그를 저장소 속성에 연결합니다. 자체 플래그는 시작 전에 파싱합니다. 라이브러리는 플래그를 파싱하거나 신호 처리기를 설치하지 않습니다.

`github.com/teramoby/speedle-plus/pkg/ads`의 `ads.Run(ctx, params, conf, options...)`로 표준 ADS 시작을 재사용합니다. `params`는 `*flags.Parameters`, `conf`는 `*cfg.Config`이며 `ctx` 취소 시 두 서버가 중지됩니다. `ads.WithGate(g)`는 두 프로토콜의 게이트 탐색을 재정의하고 `ads.WithGate(nil)`은 비활성화합니다. `ads.WithListeners(rest, rpc)`는 미리 바인딩한 `net.Listener`를 받으며 nil은 설정된 엔드포인트를 사용합니다. Run은 시작 실패 시에도 리스너를 소유하고 닫습니다. REST TLS 설정과 gRPC 서버 자격 증명은 유지됩니다. `ads.WithHTTPServer(func(*http.Server))`로 핸들러를 감싸거나 경로를 추가할 수 있습니다. `ads.WithGRPCOptions(...)`로 서버 옵션을 추가합니다. 추가 인터셉터에는 `grpc.ChainUnaryInterceptor(...)`를 사용하여 표준 panic 복구를 유지합니다.

프로토콜에 독립적인 `pkg/svcs/gate.Gate`는 `Route(serviceName string) gate.Route`로 `gate.Local`, `gate.Fallback`, `gate.Unavailable`을 선택하고 `Forward(context.Context, gate.Operation, *ads.RequestContext) (*gate.Response, error)`로 타입이 지정된 결과를 반환합니다. Local은 정상 평가를 계속합니다. Fallback은 허용·거부, 역할, 권한을 반환할 수 있으며 실패해도 로컬 평가로 전환하지 않습니다. Diagnose와 Discover는 Fallback에서 거부됩니다. `*gate.UnavailableError`는 공개 사유 코드를 전달합니다. 다른 오류와 nil 응답은 `fallback_unavailable`로 매핑됩니다. REST는 503, `authorization_unavailable`, 사유, `Cache-Control: no-store`, `Retry-After: 30`을 반환하며 gRPC는 Unavailable을 반환합니다. 응답의 `ErrorMessage`는 평가 오류입니다. REST IsAllowed는 정상 200 오류 본문, 다른 REST 작업은 500, gRPC는 Internal을 반환합니다. 성공한 대체 응답에는 `X-Speedle-Decision-Source: upstream`이 포함됩니다(gRPC 메타데이터 키는 소문자). 선택적인 `gate.Auditor.Audit(ctx, event)`는 비로컬 경로마다 한 번 호출되며 주체, 토큰, 요청 본문을 포함하지 않습니다. 표준 감사 로그도 기록합니다. 게이트는 동시 요청을 지원하고 자체 대체 인증과 자원을 관리해야 합니다.

저장소가 `gate.GateProvider.RequestGate() gate.Gate`를 구현하면 REST와 gRPC가 평가기의 저장소 접근자로 이를 탐색합니다. 임베디드 환경은 `eval.NewWithStore` 후 `adsrest.NewRouter(evaluator, adsrest.WithGate(g))` 또는 `adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(g))`를 사용할 수 있습니다. 옵션 생략 시 저장소 게이트를 탐색하고 명시적인 nil은 비활성화합니다. 게이트가 없으면 로컬 동작은 그대로입니다.

`store.WatchRequirement.RequireWatch()`는 설정에서 감시를 꺼도 구독을 강제합니다. 선택적인 `ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error)`는 스냅샷과 설치 성공 후에만 호출되는 확인 콜백을 제공합니다. `(*eval.PolicyEvalImpl).DistributionStore()`는 어댑터에 기반 저장소를 제공합니다. 평가기를 닫으면 감시를 중지하고 닫을 수 있는 저장소를 닫습니다. `ads.Run`이 정리하며 명시적으로 전달한 게이트는 호출자가 소유합니다.

취소 시 gRPC를 먼저 중지하고 HTTP 요청에 5초의 유예 기간을 준 뒤 연결을 닫습니다. `ads.WithShutdownTimeout(d)`로 양의 유예 기간을 설정합니다. 자체 핸들러와 게이트는 요청 컨텍스트 취소를 따라야 합니다. 두 프로토콜 모두 로컬 토큰 검증 전에 경로를 선택하며 대체 처리는 원래 토큰과 주체의 인증을 담당합니다. 로컬 경로는 정상 검증을 유지합니다. 평가 오류가 있으면 응답이 허용을 주장해도 `Allowed`를 false, `Reason`을 `ERROR_IN_EVALUATION`으로 바꾸고 `evaluation_error`를 감사에 기록합니다.

[테스트 전용 저장소와 게이트](https://github.com/teramoby/speedle-plus/blob/master/pkg/ads/extensions_test.go), [저장소 인터페이스](https://github.com/teramoby/speedle-plus/tree/master/pkg/store), [임베디드 배포](/ko/docs/deployment/embedded/)를 참조하세요.
