+++
title = "Speedle Cloud 제한 및 오류"
description = "계정 쿼터, 전송 속도 제한, 오류 응답 구조 및 재시도 전략"
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-08T17:11:18+08:00
+++

### 전송 속도 제한 및 쿼터

무료 공개 베타 기간 동안 Speedle Cloud에서는 공정한 사용을 보장하기 위해 계정별 쿼터가 적용됩니다.

### 오류 응답

오류 응답은 표준 HTTP 상태 코드와 JSON 오류 객체를 반환합니다:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Too many requests. Please retry later."
  }
}
```

### 재시도 지침

일시적인 네트워크 오류나 `429 Too Many Requests`의 경우 지수 백오프와 지터를 동반한 재시도를 구현하십시오.
