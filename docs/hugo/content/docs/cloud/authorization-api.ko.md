+++
title = "Speedle Cloud 인가 API 프리뷰"
description = "IsAllowed 요청 및 응답 계약, Node.js 22 클라이언트 예제 및 서비스 통합 제약 사항"
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-08T21:11:24+08:00
+++

Speedle Cloud는 빠르고 지연 시간이 짧은 접근 결정을 위한 인가 API를 제공합니다.

### 요청 계약

애플리케이션은 인가 결정 서비스(ADS)에 대해 `POST /authz-check/v1/is-allowed`를 호출합니다.

### Node.js 22 예제

다음 Node.js 예제는 전달자(Bearer) 토큰을 사용하여 인가 요청을 보내는 방법을 보여줍니다:

```javascript
import { request } from 'node:https';

export async function checkAccess(token, authenticatedUserId, authenticatedUserIdd, resource, action) {
  if (!authenticatedUserId || !authenticatedUserIdd) {
    throw new Error('principal name and identity domain are required');
  }

  const payload = JSON.stringify({
    subject: {
      principals: [
        {
          type: 'user',
          name: authenticatedUserId,
          idd: authenticatedUserIdd
        }
      ]
    },
    serviceName: 'default',
    resource: resource,
    action: action
  });

  return new Promise((resolve, reject) => {
    const req = request('https://cloud.speedle.io/authz-check/v1/is-allowed', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      }
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(data));
        } else {
          reject(new Error(`Authorization failed: ${res.statusCode} ${data}`));
        }
      });
    });
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}
```

예제 정책 `grant user alice from corp get /books`에 대해 이 함수를 호출하면 예상대로 인가 결정이 반환됩니다.
