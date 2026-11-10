+++
title = "Java SDK"
description = "Añada el SDK de Speedle para Java y compruebe autorizaciones frente a un Speedle local o Speedle Cloud."
weight = 10
aliases = ["/es/docs/api/sdk-java/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Añada io.speedle:speedle-sdk 0.2.0 con Maven o Gradle, y luego llame a isAllowed y check frente a un Speedle local o Speedle Cloud."
+++

El SDK de Java requiere Java 17 o posterior y no tiene dependencias de tiempo de ejecución. Paquete: [`io.speedle:speedle-sdk`](https://repo1.maven.org/maven2/io/speedle/speedle-sdk/0.2.0/) 0.2.0 en Maven Central. Referencia: [speedle-sdk/java](https://github.com/teramoby/speedle-sdk/tree/main/java).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

Maven, un `pom.xml` completo:

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

Gradle, un `build.gradle.kts` completo (con un `settings.gradle.kts` que define `rootProject.name`):

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

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Guarde este programa como `src/main/java/Main.java`:

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

Ejecútelo con Maven o Gradle:

```bash
mvn -q compile exec:java -Dexec.mainClass=Main
gradle -q run
```

Salida:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## Llamar a Speedle Cloud

> **Speedle Cloud estará disponible próximamente.**

El programa lee el nombre del servicio desde `SPEEDLE_SERVICE` y el token del servicio desde `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` anula el endpoint predeterminado `https://cloud.speedle.io`. Sustituya `src/main/java/Main.java` por:

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

Configure las variables de entorno y ejecútelo. `read -rs` evita que el token quede registrado en el historial de la terminal o en pantalla:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
mvn -q compile exec:java -Dexec.mainClass=Main
```

Salida, cuando el servicio cuenta con la política de ejemplo:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

El token es un secreto del lado del servidor. Nunca lo distribuya a navegadores, aplicaciones móviles o clientes de escritorio.

## Gestión de errores

Una solicitud denegada no constituye un error: `isAllowed` devuelve `false`, y `check` devuelve un `Decision` con `allowed()` en false y el motivo, tal como se muestra en la llamada a `delete` anterior. Con la configuración predeterminada, cuando una llamada no puede devolver una decisión, el SDK genera una excepción `SpeedleException`. Los ejemplos la capturan, imprimen `Speedle error: <mensaje>` en la salida de error estándar y finalizan con el código de salida 1.
