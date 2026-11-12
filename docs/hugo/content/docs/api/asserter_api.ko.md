+++
title = "토큰 어설션 API"
description = "토큰을 검증하고 인가 요청에 주체 식별자를 제공하기 위한 토큰 어설션 웹훅 구현"
tags = ["v0.1.0", "api", "guide", "core", "asserter", "pluggable"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

토큰 어설션 웹훅 참조 사양입니다. errCode는 정수이고 attributes는 객체입니다. 실제 엔드포인트는 ADS 어서터 구성에 지정됩니다.

[보안 및 엔드포인트 구성](/docs/security/). 이 문서는 인터페이스 사양을 설명하며 speedle.io는 호스팅형 백엔드를 제공하지 않습니다. Try it out은 꺼져 있습니다. 사양에 서버 호스트가 없어서, 켜 두면 요청이 이 문서 사이트로 갑니다.

{{< swagger-spec url="/swagger/asserter.yaml" tryitout="false" >}}
