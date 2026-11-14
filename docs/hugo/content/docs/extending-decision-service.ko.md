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

`store.StoreBuilder`를 구현하고 애플리케이션에 필요한 `pms.PolicyStoreManager` 인터페이스를 충족하는 저장소를 반환합니다. 패키지의 `init` 함수에서 `store.Register`로 고유한 이름을 등록한 뒤 실행 프로그램에서 패키지를 가져옵니다. 표준 프로그램은 `cmd/speedle-ads/stores.go`와 `cmd/speedle-pms/stores.go`에서 저장소를 가져옵니다. `GetStoreParams`는 명령줄 플래그를 저장소 속성에 연결합니다.

임베디드 서비스에서는 저장소를 `eval.NewWithStore`에 전달하고 반환된 평가기를 `adsrest.NewRouter` 또는 `adsgrpc.NewGRPCService`에 전달합니다. 변경 이벤트를 구독해야 하는 저장소는 `store.WatchRequirement`를 구현할 수 있습니다. `(*eval.PolicyEvalImpl).DistributionStore()`는 외부 어댑터가 기반 저장소에 접근할 수 있도록 유지됩니다. 엔진은 사용자 정의 게이트웨이 인터페이스를 해석하지 않습니다. 게이트웨이나 라우팅은 자체 실행 프로그램에서 구성하세요.

[저장소 인터페이스](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)와 [임베디드 배포](/ko/docs/deployment/embedded/)를 참조하세요.
