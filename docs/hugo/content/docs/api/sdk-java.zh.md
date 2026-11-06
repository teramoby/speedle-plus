+++
title = "Java SDK"
description = "添加 Speedle Java SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 Maven 或 Gradle 添加 io.speedle:speedle-sdk 0.1.0，再向本地 Speedle 或 Speedle Cloud 调用 isAllowed 与 check。"
+++

Java SDK 需要 Java 17 或更高版本，没有运行时依赖。软件包：Maven Central 上的 [`io.speedle:speedle-sdk`](https://central.sonatype.com/artifact/io.speedle/speedle-sdk) 0.1.0。参考文档：[speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java)。

示例检查的策略见 [SDK](/docs/api/#sdk)。

## 添加依赖

Maven，完整的 `pom.xml`：

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
      <version>0.1.0</version>
    </dependency>
  </dependencies>
</project>
```

Gradle，完整的 `build.gradle.kts`（另需一个设置 `rootProject.name` 的 `settings.gradle.kts`）：

```kotlin
plugins {
    application
}

repositories {
    mavenCentral()
}

dependencies {
    implementation("io.speedle:speedle-sdk:0.1.0")
}

application {
    mainClass = "Main"
}
```

## 调用本地 Speedle

按 [SDK](/docs/api/#sdk) 一节启动 Speedle 并创建示例策略。将以下程序保存为 `src/main/java/Main.java`：

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
        .backend(Backend.SPEEDLE_PLUS)
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

用 Maven 或 Gradle 运行：

```bash
mvn -q compile exec:java -Dexec.mainClass=Main
gradle -q run
```

输出：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取该服务的令牌。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `src/main/java/Main.java` 替换为：

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

设置环境变量后运行。`read -rs` 不会把令牌写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
mvn -q compile exec:java -Dexec.mainClass=Main
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。
