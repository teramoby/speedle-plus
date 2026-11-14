+++
title = "Go SDK"
description = "Añada el SDK de Speedle para Go y compruebe autorizaciones frente a un Speedle local."
weight = 30
aliases = ["/es/docs/api/sdk-go/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Añada github.com/teramoby/speedle-sdk/go v0.2.0 con go get, y llame a IsAllowed y Check frente a un Speedle local."
lastmod = 2026-11-07T19:43:15+08:00
+++

El SDK de Go es compatible con las dos versiones más recientes de Go y utiliza únicamente la biblioteca estándar. Módulo: [`github.com/teramoby/speedle-sdk/go`](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) v0.2.0, paquete `speedle`. Referencia: [speedle-sdk/go](https://github.com/teramoby/speedle-sdk/tree/main/go).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

En un módulo nuevo:

```bash
go mod init example.com/speedle-example
go get github.com/teramoby/speedle-sdk/go@v0.2.0
```

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Guarde este programa como `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	speedle "github.com/teramoby/speedle-sdk/go"
)

func main() {
	client, err := speedle.NewClient(speedle.Config{
		Backend:     speedle.BackendSpeedle,
		Endpoint:    "http://localhost:6734",
		Auth:        speedle.NoAuth(),
		ServiceName: "docs",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	allowed, err := client.IsAllowed(ctx, speedle.User("alice"), "get", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice get /docs/public: allowed=%t\n", allowed)

	d, err := client.Check(ctx, speedle.User("alice"), "delete", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice delete /docs/public: allowed=%t reason=%s\n", d.Allowed, d.Reason)
}
```

Ejecución:

```bash
go run .
```

Salida:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```


## Manejo de errores

Una solicitud denegada no es un error: como en la llamada a `delete` anterior, `IsAllowed` devuelve `false` y `Check` devuelve una decisión con `Allowed` en false y el motivo. En la configuración predeterminada, la llamada devuelve un `error` no nulo cuando no puede devolver una decisión (al igual que `NewClient` ante una configuración no válida). El ejemplo imprime `Speedle error: <mensaje>` en la salida de error estándar y sale con estado 1.
