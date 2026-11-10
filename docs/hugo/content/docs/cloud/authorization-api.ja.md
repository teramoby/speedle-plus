+++
title = "Speedle Cloud 認可 API プレビュー"
description = "IsAllowed リクエストおよびレスポンス契約、Node.js 22 クライアントの例、およびサービス統合の制約。"
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-08T21:11:24+08:00
+++

Speedle Cloud は、高速で低レイテンシのアクセス決定のための認可 API を提供します。

### リクエスト契約

アプリケーションは、認可決定サービス (ADS) に対して `POST /authz-check/v1/is-allowed` を呼び出します。

### Node.js 22 の例

以下の Node.js の例は、ベアラートークンを使用して認可リクエストを送信する方法を示しています:

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

ポリシー例 `grant user alice from corp get /books` に対して、この関数を呼び出すと期待どおりの認可決定が返されます。
