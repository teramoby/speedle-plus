+++
title = "API 레퍼런스"
description = "공개 Speedle Cloud API의 읽기 전용 Swagger UI입니다. Try it out은 꺼져 있습니다."
date = 2026-11-12T10:00:00+08:00
lastmod = 2026-11-12T10:00:00+08:00
weight = 35
draft = false
toc = false
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
bref = "공개 Cloud API를 Swagger UI로 보여 줍니다. Try it out은 꺼져 있습니다."
+++

이 페이지는 공개 Speedle Cloud API를 렌더링합니다. 사양 본문은 이 사이트의 모든 언어에서 영어입니다. Try it out이 꺼져 있으므로 애플리케이션 토큰을 브라우저에 입력하지 않습니다.

여기에 렌더링되는 파일은 [함께 제공되는 공개 부분집합](/openapi/cloud-public.yaml)입니다. Speedle Cloud가 공개 OpenAPI 문서를 게시하기 전까지의 자리표시자이며 인가 확인 경로만 담습니다. [Cloud 문서 색인](/docs/cloud/)에서 링크하는, 저장소에 체크인된 전체 사양이 아닙니다.

[인가 API 프리뷰](/docs/cloud/authorization-api/)에서 IsAllowed 호출을 설명합니다. [제한 및 오류](/docs/cloud/limits-and-errors/)에 쿼터와 오류 응답이 있습니다.

{{< swagger-spec url="/openapi/cloud-public.yaml" tryitout="false" >}}
