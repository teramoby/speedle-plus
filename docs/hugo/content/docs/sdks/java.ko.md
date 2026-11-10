+++
title = "Java SDK"
description = "Speedle Java SDK를 추가하고 로컬 Speedle 또는 Speedle Cloud에 대해 인가 확인 수행"
weight = 10
aliases = ["/ko/docs/api/sdk-java/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Maven 또는 Gradle로 io.speedle:speedle-sdk 0.2.0을 추가하고 로컬 Speedle 또는 Speedle Cloud에 대해 isAllowed 및 check를 호출합니다."
+++

Java SDK는 Java 17 이상이 필요하며 런타임 의존성이 없습니다. 패키지: Maven Central의 [`io.speedle:speedle-sdk`](https://repo1.maven.org/maven2/io/speedle/speedle-sdk/0.2.0/) 0.2.0. 참조 문서: [speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java).

예제에서는 [SDK 개요](/docs/sdks/#example-policy)에서 설명한 정책을 확인합니다.

## 의존성 추가

Maven의 경우 전체 `pom.xml`:

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

Gradle의 경우 전체 `build.gradle.kts`(`rootProject.name`을 설정하는 `settings.gradle.kts` 포함):

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

## 로컬 Speedle 호출

[SDK 개요](/docs/sdks/#example-policy)의 설명에 따라 Speedle을 시작하고 예제 정책을 생성합니다. 이 프로그램을 `src/main/java/Main.java`로 저장합니다:

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

Maven 또는 Gradle로 실행합니다:

```bash
mvn -q compile exec:java -Dexec.mainClass=Main
gradle -q run
```

출력:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## Speedle Cloud 호출

> **Speedle Cloud는 출시 예정입니다.**

프로그램은 `SPEEDLE_SERVICE`에서 서비스 이름을, `SPEEDLE_TOKEN`에서 서비스 토큰을 읽습니다. `SPEEDLE_ENDPOINT`는 기본 엔드포인트 `https://cloud.speedle.io`를 재정의합니다. `src/main/java/Main.java`를 다음과 같이 대체합니다:

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

환경 변수를 설정하고 실행합니다. `read -rs`를 사용하면 토큰이 셸 기록이나 화면에 노출되지 않습니다:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
mvn -q compile exec:java -Dexec.mainClass=Main
```

서비스에 예제 정책이 있는 경우 출력:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

토큰은 서버 측 비밀 값입니다. 브라우저, 모바일 앱 또는 데스크톱 클라이언트에 절대 배포하지 마십시오.

## 오류 처리

거부된 요청은 오류가 아닙니다. 위의 `delete` 호출과 같이 `isAllowed`는 `false`를 반환하고, `check`는 `allowed()`가 false이며 이유를 포함하는 `Decision`을 반환합니다. 기본 구성에서 호출이 결정을 반환할 수 없는 경우 SDK는 `SpeedleException`을 발생시킵니다. 예제에서는 이를 포착하여 표준 오류로 `Speedle error: <message>`를 출력하고 종료 상태 1로 종료합니다.
