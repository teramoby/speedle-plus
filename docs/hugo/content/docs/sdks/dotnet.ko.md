+++
title = "C#/.NET SDK"
description = "Speedle .NET SDK를 추가하고 로컬 Speedle 또는 Speedle Cloud에 대해 인가 확인 수행"
weight = 40
aliases = ["/ko/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "dotnet add package로 Speedle.Sdk 0.2.0을 추가하고 로컬 Speedle 또는 Speedle Cloud에 대해 IsAllowedAsync 및 CheckAsync를 호출합니다."
+++

.NET SDK는 .NET 8 이상을 대상으로 하며 의존성이 없습니다. 패키지: NuGet의 [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.2.0, 네임스페이스 `Speedle.Sdk`. 참조 문서: [speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet).

예제에서는 [SDK 개요](/docs/sdks/#example-policy)에서 설명한 정책을 확인합니다.

## 의존성 추가

새 콘솔 프로젝트를 생성하고 패키지를 추가합니다:

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.2.0
```

## 로컬 Speedle 호출

[SDK 개요](/docs/sdks/#example-policy)의 설명에 따라 Speedle을 시작하고 예제 정책을 생성합니다. `Program.cs`를 다음과 같이 대체합니다:

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

실행합니다:

```bash
dotnet run
```

출력:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

## Speedle Cloud 호출

> **Speedle Cloud는 출시 예정입니다.**

프로그램은 `SPEEDLE_SERVICE`에서 서비스 이름을, `SPEEDLE_TOKEN`에서 서비스 토큰을 읽습니다. `SPEEDLE_ENDPOINT`는 기본 엔드포인트 `https://cloud.speedle.io`를 재정의합니다. `Program.cs`를 다음과 같이 대체합니다:

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

환경 변수를 설정하고 실행합니다. `read -rs`를 사용하면 토큰이 셸 기록이나 화면에 노출되지 않습니다:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
dotnet run
```

서비스에 예제 정책이 있는 경우 출력:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

토큰은 서버 측 비밀 값입니다. 브라우저, 모바일 앱 또는 데스크톱 클라이언트에 절대 배포하지 마십시오.

## 오류 처리

거부된 요청은 오류가 아닙니다. 위의 `delete` 호출과 같이 `IsAllowedAsync`는 `false`를 반환하고, `CheckAsync`는 `Allowed`가 false이며 이유를 포함하는 `Decision`을 반환합니다. 기본 구성에서 호출이 결정을 반환할 수 없는 경우 SDK는 `SpeedleException`을 발생시킵니다. 예제에서는 이를 포착하여 표준 오류로 `Speedle error: <message>`를 출력하고 종료 코드 1을 반환합니다.
