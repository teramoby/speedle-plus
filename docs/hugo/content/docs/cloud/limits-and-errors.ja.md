+++
title = "Speedle Cloud の制限とエラー"
description = "アカウントクォータ、レート制限、エラーレスポンス構造、および再試行戦略。"
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
+++

### レート制限とクォータ

無料パブリックベータ期間中、Speedle Cloud では公正な利用を確保するためにアカウントごとのクォータが適用されます。

### エラーレスポンス

エラーレスポンスは標準の HTTP ステータスコードと JSON エラーオブジェクトを返します:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Too many requests. Please retry later."
  }
}
```

### 再試行のガイダンス

一時的なネットワークエラーや `429 Too Many Requests` の場合は、指数バックオフとジッターを伴う再試行を実装してください。
