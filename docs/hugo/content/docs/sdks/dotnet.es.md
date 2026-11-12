+++
title = "C#/.NET SDK"
description = "Añada el SDK de Speedle para .NET y compruebe autorizaciones frente a un Speedle local o Speedle Cloud."
weight = 40
aliases = ["/es/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Añada Speedle.Sdk 0.2.0 con dotnet add package, y luego llame a IsAllowedAsync y CheckAsync frente a un Speedle local o Speedle Cloud."
lastmod = 2026-11-07T19:43:15+08:00
+++

El SDK de .NET es compatible con .NET 8 o posterior y no tiene dependencias. Paquete: [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.2.0 en NuGet, espacio de nombres `Speedle.Sdk`. Referencia: [speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

En un nuevo proyecto de consola:

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.2.0
```

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Sustituya `Program.cs` por:

```csharp
using Speedle.Sdk;

try
{
    await using var speedle = new SpeedleClient(new SpeedleClientOptions
    {
        Backend = Backend.Speedle,
        Endpoint = new Uri("http://localhost:6734"),
        Auth = Auth.None(),
        ServiceName = "docs",
    });

    bool allowed = await speedle.IsAllowedAsync("alice", "get", "/docs/public");
    Console.WriteLine($"alice get /docs/public: allowed={allowed}");

    Decision d = await speedle.CheckAsync("alice", "delete", "/docs/public");
    Console.WriteLine($"alice delete /docs/public: allowed={d.Allowed} reason={d.Reason}");
}
catch (SpeedleException e)
{
    Console.Error.WriteLine($"Speedle error: {e.Message}");
    return 1;
}
return 0;
```

Ejecución:

```bash
dotnet run
```

Salida:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

## Llamar a Speedle Cloud

> **Speedle Cloud estará disponible próximamente.**

El programa lee el nombre del servicio desde `SPEEDLE_SERVICE` y una clave API de cuenta con ámbito `ads` desde `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` anula el endpoint predeterminado `https://cloud.speedle.io`. Sustituya `Program.cs` por:

```csharp
using Speedle.Sdk;

try
{
    await using var speedle = new SpeedleClient(new SpeedleClientOptions
    {
        Backend = Backend.SpeedleCloud,
        Endpoint = new Uri(Environment.GetEnvironmentVariable("SPEEDLE_ENDPOINT") ?? "https://cloud.speedle.io"),
        Auth = Auth.Bearer(Environment.GetEnvironmentVariable("SPEEDLE_TOKEN") ?? ""),
        ServiceName = Environment.GetEnvironmentVariable("SPEEDLE_SERVICE") ?? "",
    });

    bool allowed = await speedle.IsAllowedAsync("alice", "get", "/docs/public");
    Console.WriteLine($"alice get /docs/public: allowed={allowed}");

    Decision d = await speedle.CheckAsync("alice", "delete", "/docs/public");
    Console.WriteLine($"alice delete /docs/public: allowed={d.Allowed} reason={d.Reason}");
}
catch (SpeedleException e)
{
    Console.Error.WriteLine($"Speedle error: {e.Message}");
    return 1;
}
return 0;
```

Configure las variables de entorno y ejecútelo. `read -rs` evita que la clave API quede registrada en el historial de la terminal o en pantalla:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
dotnet run
```

Salida, cuando el servicio cuenta con la política de ejemplo:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

La clave API es un secreto del lado del servidor. Nunca lo distribuya a navegadores, aplicaciones móviles o clientes de escritorio.

## Gestión de errores

Una solicitud denegada no constituye un error: `IsAllowedAsync` devuelve `false`, y `CheckAsync` devuelve un `Decision` con `Allowed` en false y el motivo, tal como se muestra en la llamada a `delete` anterior. Con la configuración predeterminada, cuando una llamada no puede devolver una decisión, el SDK genera una excepción `SpeedleException`. Los ejemplos la capturan, imprimen `Speedle error: <mensaje>` en la salida de error estándar y devuelven el código de salida 1.
