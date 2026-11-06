+++
title = "Java SDK"
description = "Add the Speedle Java SDK and check authorization against a local Speedle+ or Speedle Cloud."
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Add io.speedle:speedle-sdk 0.1.0 with Maven or Gradle, then call isAllowed and check against a local Speedle+ or Speedle Cloud."
+++

The Java SDK needs Java 17 or later and has no runtime dependencies. Package: [`io.speedle:speedle-sdk`](https://central.sonatype.com/artifact/io.speedle/speedle-sdk) 0.1.0 on Maven Central. Reference: [speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java).

The examples check the policy described in [SDKs](/docs/api/#sdks).

## Add the dependency

Maven, a complete `pom.xml`:

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

Gradle, a complete `build.gradle.kts` (with a `settings.gradle.kts` that sets `rootProject.name`):

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

## Call a local Speedle+

Start Speedle+ and create the example policy as described in [SDKs](/docs/api/#sdks). Save this program as `src/main/java/Main.java`:

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

Run it with Maven or Gradle:

```bash
mvn -q compile exec:java -Dexec.mainClass=Main
gradle -q run
```

Output:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## Call Speedle Cloud

> **Speedle Cloud is coming soon.**

The program reads the service name from `SPEEDLE_SERVICE` and the service's token from `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` overrides the default endpoint `https://cloud.speedle.io`. Replace `src/main/java/Main.java` with:

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

Set the environment and run it. `read -rs` keeps the token out of your shell history and screen:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
mvn -q compile exec:java -Dexec.mainClass=Main
```

Output, when the service has the example policy:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

The token is a server-side secret. Never ship it to browsers, mobile apps or desktop clients.
