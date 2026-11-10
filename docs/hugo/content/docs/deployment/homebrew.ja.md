+++
title = "Homebrew でのインストール"
description = "Homebrew で speedle-spctl、speedle-pms、speedle-ads をインストールし、ローカルでデモを実行します。"
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Homebrew はコマンドラインツールと両方のサービスをインストールし、127.0.0.1 上で 1 つのポリシーファイルを共有して実行します。"
lastmod = 2026-11-10T13:49:59+08:00
+++

macOS または Linux で [Homebrew](https://brew.sh/) を使い、Speedle のコマンドラインツールをインストールします。Homebrew は、公式組織以外の tap にある formula を信頼するまで、その formula をインストールまたは起動しません。

```bash
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
```

Formula の名前は `spctl` です。macOS でのコマンドは `speedle-spctl` なので、このインストールは Apple Gatekeeper の `/usr/sbin/spctl` を置き換えません。Linux では同じプログラムを `spctl` としても使えます。

ポリシー管理サービスと認可決定サービスは別の formula です。

```bash
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

コマンドラインツールと両方のサービスをまとめてインストールします。

```bash
brew install teramoby/tap/speedle
```

`spctl`、`speedle-pms`、`speedle-ads` はそれぞれ、そのリリースの macOS または Linux バイナリをインストールし、そのリリースの `checksums.txt` と照合します。

<a id="demo-with-homebrew"></a>

## Homebrew でのデモ

両方のサービスを起動します。これらは `127.0.0.1` で待ち受け、1 つのファイルポリシーストアを共有します。

```bash
brew services start speedle-pms
brew services start speedle-ads
```

`speedle-pms` は `127.0.0.1:6733` で待ち受けます。`speedle-ads` は `127.0.0.1:6734` で待ち受けます。どちらも `--store-type file` を使い、ポリシーファイルは `$(brew --prefix)/var/speedle/policies.json` です。ログは `$(brew --prefix)/var/log/` にあります。`--insecure=true` はこのループバックのデモ専用です。TLS は[セキュリティ](/ja/docs/security/)を参照してください。

サービスとポリシーを作成します。Linux では `speedle-spctl` の代わりに `spctl` を使えます。

```bash
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

ADS に、`user1` が `res1` を get できるか問い合わせます。

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

応答には `"allowed":true` が含まれます。両方のサービスは共有ファイルからポリシーの変更を読み取ります。停止するには次を実行します。

```bash
brew services stop speedle-ads
brew services stop speedle-pms
```
