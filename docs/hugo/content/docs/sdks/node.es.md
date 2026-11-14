+++
title = "Node.js/TypeScript SDK"
description = "Añada el SDK de Speedle para Node.js/TypeScript y compruebe autorizaciones frente a un Speedle local."
weight = 60
aliases = ["/es/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Instale @speedle/sdk con npm, y luego llame a isAllowed y check frente a un Speedle local."
lastmod = 2026-11-10T09:48:45+08:00
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
