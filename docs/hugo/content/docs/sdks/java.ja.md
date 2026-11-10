+++
title = "Java SDK"
description = "Speedle Java SDK を追加し、ローカルの Speedle または Speedle Cloud に対して認可チェックを実行"
weight = 10
aliases = ["/ja/docs/api/sdk-java/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Maven または Gradle で io.speedle:speedle-sdk 0.2.0 を追加し、ローカルの Speedle または Speedle Cloud に対して isAllowed と check を呼び出します。"
lastmod = 2026-11-08T19:23:52+08:00
+++

Java SDK は Java 17 以降が必要であり、ランタイム依存関係はありません。パッケージ: Maven Central の [`io.speedle:speedle-sdk`](https://repo1.maven.org/maven2/io/speedle/speedle-sdk/0.2.0/) 0.2.0。リファレンス: [speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

Maven の場合、完全な `pom.xml`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.example</groupId>
  <artifactId>speedle-example</artifactId>
  <version>1.0</version>

  <properties>
    <maven.compiler.release>17</maven.compiler.release>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>

  <dependencies>
    <dependency>
      <groupId>io.speedle</groupId>
      <artifactId>speedle-sdk</artifactId>
      <version>0.2.0</version>
    </dependency>
  </dependencies>
</project>
```

Gradle の場合、完全な `build.gradle.kts`（`rootProject.name` を設定する `settings.gradle.kts` を含む）:

```kotlin
plugins {
    application
}

repositories {
    mavenCentral()
}

dependencies {
    implementation("io.speedle:speedle-sdk:0.2.0")
}

application {
    mainClass = "Main"
}
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。このプログラムを `src/main/java/Main.java` として保存します:

```java
import io.speedle.sdk.Auth;
import io.speedle.sdk.Backend;
import io.speedle.sdk.Decision;
import io.speedle.sdk.SpeedleClient;
import io.speedle.sdk.SpeedleException;
import io.speedle.sdk.Subject;
import java.net.URI;
import java.util.List;

public class Main {
  public static void main(String[] args) {
    try (SpeedleClient speedle = SpeedleClient.builder()
        .backend(Backend.SPEEDLE)
        .endpoint(URI.create("http://localhost:6734"))
        .auth(Auth.none())
        .serviceName("docs")
        .build()) {
      boolean allowed = speedle.isAllowed(Subject.user("alice"), "get", "/docs/public", List.of());
      System.out.println("alice get /docs/public: allowed=" + allowed);

      Decision d = speedle.check(Subject.user("alice"), "delete", "/docs/public", List.of());
      System.out.println("alice delete /docs/public: allowed=" + d.allowed() + " reason=" + d.reason());
    } catch (SpeedleException e) {
      System.err.println("Speedle error: " + e.getMessage());
      System.exit(1);
    }
  }
}
```

Maven または Gradle で実行します:

```bash
mvn -q compile exec:java -Dexec.mainClass=Main
gradle -q run
```

出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## Speedle Cloud の呼び出し

> **Speedle Cloud はまもなく公開されます。**

プログラムは `SPEEDLE_SERVICE` からサービス名を、`SPEEDLE_TOKEN` からサービスのトークンを読み取ります。`SPEEDLE_ENDPOINT` はデフォルトのエンドポイント `https://cloud.speedle.io` を上書きします。`src/main/java/Main.java` を以下に置き換えます:

```java
import io.speedle.sdk.Auth;
import io.speedle.sdk.Backend;
import io.speedle.sdk.Decision;
import io.speedle.sdk.SpeedleClient;
import io.speedle.sdk.SpeedleException;
import io.speedle.sdk.Subject;
import java.net.URI;
import java.util.List;

public class Main {
  public static void main(String[] args) {
    String endpoint = System.getenv().getOrDefault("SPEEDLE_ENDPOINT", "https://cloud.speedle.io");
    try (SpeedleClient speedle = SpeedleClient.builder()
        .backend(Backend.SPEEDLE_CLOUD)
        .endpoint(URI.create(endpoint))
        .auth(Auth.bearer(System.getenv("SPEEDLE_TOKEN")))
        .serviceName(System.getenv("SPEEDLE_SERVICE"))
        .build()) {
      boolean allowed = speedle.isAllowed(Subject.user("alice"), "get", "/docs/public", List.of());
      System.out.println("alice get /docs/public: allowed=" + allowed);

      Decision d = speedle.check(Subject.user("alice"), "delete", "/docs/public", List.of());
      System.out.println("alice delete /docs/public: allowed=" + d.allowed() + " reason=" + d.reason());
    } catch (SpeedleException e) {
      System.err.println("Speedle error: " + e.getMessage());
      System.exit(1);
    }
  }
}
```

環境変数を設定して実行します。`read -rs` を使用すると、トークンがシェルの履歴や画面に表示されません:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
mvn -q compile exec:java -Dexec.mainClass=Main
```

サービスにサンプルポリシーが存在する場合の出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

トークンはサーバー側のシークレットです。ブラウザ、モバイルアプリ、またはデスクトップクライアントには絶対に配布しないでください。

## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`isAllowed` は `false` を返し、`check` は `allowed()` が false で理由を含む `Decision` を返します。デフォルト設定では、呼び出しが決定を返せない場合、SDK は `SpeedleException` をスローします。サンプルではそれをキャッチし、標準エラー出力に `Speedle error: <message>` を出力して終了ステータス 1 で終了します。
