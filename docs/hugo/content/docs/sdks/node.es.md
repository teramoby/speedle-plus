+++
title = "Node.js/TypeScript SDK"
description = "Añada el SDK de Speedle para Node.js/TypeScript y compruebe autorizaciones frente a un Speedle local o Speedle Cloud."
weight = 60
aliases = ["/es/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Instale @speedle/sdk con npm, y luego llame a isAllowed y check frente a un Speedle local o Speedle Cloud."
+++

El SDK de Node.js requiere Node.js 22 o posterior, incluye distribuciones ESM y CommonJS con tipos de TypeScript, y no tiene dependencias de tiempo de ejecución. Paquete: [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk), publicado en npm. Referencia: [speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

En un paquete de módulos ES nuevo:

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
```

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Guarde este programa como `main.ts`:

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle",
    endpoint: "http://localhost:6734",
    auth: Auth.none(),
    serviceName: "docs",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

Ejecútelo. Node.js 22.18 o posterior ejecuta archivos TypeScript directamente:

```bash
node main.ts
```

Salida:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## Llamar a Speedle Cloud

> **Speedle Cloud estará disponible próximamente.**

El programa lee el nombre del servicio desde `SPEEDLE_SERVICE` y el token del servicio desde `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` anula el endpoint predeterminado `https://cloud.speedle.io`. Sustituya `main.ts` por:

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle-cloud",
    endpoint: process.env.SPEEDLE_ENDPOINT ?? "https://cloud.speedle.io",
    auth: Auth.bearer(process.env.SPEEDLE_TOKEN ?? ""),
    serviceName: process.env.SPEEDLE_SERVICE ?? "",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

Configure las variables de entorno y ejecútelo. `read -rs` evita que el token quede registrado en el historial de la terminal o en pantalla:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
node main.ts
```

Salida, cuando el servicio cuenta con la política de ejemplo:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

El token es un secreto del lado del servidor. Nunca lo distribuya a navegadores, aplicaciones móviles o clientes de escritorio.

## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp) está publicado en npm. Speedle MCP (vista previa de lanzamiento) es un paquete con modos `check` y `manage`. Instálelo con npm:

```bash
npm install @speedle/mcp@0.2.1
```

O ejecute el binario `speedle-mcp` directamente:

```bash
npx @speedle/mcp@0.2.1 check
```

Ejecutar `speedle-mcp` sin especificar un modo muestra las instrucciones de uso y finaliza con un estado distinto de cero. Referencia: [speedle-mcp](https://github.com/teramoby/speedle-mcp).

## Gestión de errores

Una solicitud denegada no constituye un error: `isAllowed` se resuelve en `false`, y `check` se resuelve en una decisión con `allowed` en false y el motivo, tal como se muestra en la llamada a `delete` anterior. Con la configuración predeterminada, cuando una llamada no puede devolver una decisión, la promesa se rechaza con `SpeedleError`. Los ejemplos la capturan, imprimen `Speedle error: <mensaje>` en la salida de error estándar y configuran `process.exitCode = 1`; cualquier otro error se vuelve a propagar.
