+++
draft= false
title = "サポート"
description = "Speedle に関するよくある質問への回答"
toc = false
tocheading = "h2"
tocsidebar = false
icon = "8. Community.svg"
lastmod = 2026-11-07T20:54:01+08:00
+++

## FAQ

### Speedle は誰向けですか？

Speedle は、アクセス制御を必要とするアプリケーション向けのオープンソース認可エンジンです。システムに組み込むことも、サービスとして実行することもでき、認可ポリシーをアプリケーションコードにハードコードする代わりに一元管理できます。

### なぜ別のポリシーエンジンが必要なのですか？

[クイックスタート](/ja/quick-start/)では、約15分で一連のワークフローを体験できます。Speedle のポリシーモデルと決定フローを他の選択肢と比較する最も迅速な方法です。

### どのように貢献できますか？

[貢献ガイド](/ja/contribute/#code)を参照してください。

### Speedle の今後の計画はどうなっていますか？

Speedle はオープンソースプロジェクトとして保守されています。コミュニティのニーズに応じて、バグ修正、ドキュメントの改善、新機能の開発を進めています。

### OPA、Kubernetes RBAC、Istio RBAC との違いは何ですか？

Speedle、OPA、Kubernetes RBAC、Istio RBAC はすべて認可またはアクセス制御を提供します。Kubernetes RBAC と Istio RBAC はそれぞれ Kubernetes および Istio 内部の認可を扱い、Speedle と OPA は汎用の認可ソリューションです。Casbin と Ladon も汎用の選択肢です。これらを比較する際は、以下の点を検討してください:

1. ポリシー言語が使いやすいか？各製品がポリシー言語を提供しているため、それぞれでポリシーを作成して比較してください。
2. 1回の認可決定にどのくらい時間がかかるか？
3. スケーラビリティは十分か？
4. サポートされているポリシーモデルは十分柔軟か？

### なぜ Speedle には組み込みの認証メカニズムがないのですか？

Speedle は認可（Authorization）に専念しています。認証（Authentication）は提供しません。

### 独自のバックエンドストレージを接続するにはどうすればよいですか？

[ストレージ拡張](/ja/docs/store/)を参照してください。

### 独自のトークンアサーターを接続するにはどうすればよいですか？

[トークンアサーター](/ja/docs/assertor/)を参照してください。

<div class="row" style="padding-top: 50px">
    <div class="col col-2"></div>
    <div class="col col-8 center" style="font-size: 25px">
    <div>
        <p>お探しの回答が見つかりませんか？</p>
        <p>Speedle Discord コミュニティに参加してください</p>
        <span>
          <a class="button primary started" style="font-size:18px" href="https://discord.gg/uaY2xVsbbT">
            <span>Discord に参加 →</span>
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
