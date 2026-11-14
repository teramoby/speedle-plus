+++
title = "Java SDK"
description = "添加 Speedle Java SDK，向本地 Speedle 发起授权检查。"
weight = 10
aliases = ["/zh/docs/api/sdk-java/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 Maven 或 Gradle 添加 io.speedle:speedle-sdk 0.2.0，再向本地 Speedle 调用 isAllowed 与 check。"
lastmod = 2026-11-08T19:23:52+08:00
+++

Java SDK 需要 Java 17 或更高版本，没有运行时依赖。软件包：Maven Central 上的 [`io.speedle:speedle-sdk`](https://repo1.maven.org/maven2/io/speedle/speedle-sdk/0.2.0/) 0.2.0。参考文档：[speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

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
      <version>0.2.0</version>
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
    implementation("io.speedle:speedle-sdk:0.2.0")
}

application {
    mainClass = "Main"
}
```

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将以下程序保存为 `src/main/java/Main.java`：

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


## 错误处理

授权请求返回 Deny 并不代表调用出错：`isAllowed` 返回 `false`，`check` 返回 `allowed()` 为 false 的 `Decision` 及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，SDK 抛出 `SpeedleException`。示例捕获该异常，向标准错误输出 `Speedle error: <消息>`，并以状态码 1 退出。
