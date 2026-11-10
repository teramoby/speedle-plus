+++
title = "Modo integrado de Speedle"
description = "Integre Speedle en una aplicación Go usando módulos de Go y verifique decisiones de Permitir y Denegar."
weight = 10
aliases = ["/es/docs/deployment-embeded/"]
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
lastmod = 2026-11-10T14:45:43+08:00
+++

## ¿Qué es el modo integrado?

El modo integrado ejecuta el motor de decisiones de políticas de Speedle dentro de su aplicación Go. Llame al evaluador directamente sin iniciar servidores PMS ni ADS. Su aplicación autentica a los usuarios y proporciona la entidad principal (sujeto), el recurso y la acción para cada decisión.

Para probar políticas básicas antes de escribir código, abra el [Playground](/es/playground/). Para ejecutar Speedle como servicios independientes, siga el [Inicio rápido](/es/quick-start/).

## Requisitos previos

Utilice Linux o macOS con un shell y **Go 1.26.10 o Go 1.27.3** (o un parche más reciente en la misma rama de lanzamiento), coincidiendo con el [Inicio rápido](/es/quick-start/). Esta guía utiliza módulos de Go y la versión publicada **Speedle v1.6.0**. Go 1.27.3 es la cadena de herramientas recomendada; el ejemplo ha sido verificado con Go 1.26.10 y Go 1.27.3.

Instale Go siguiendo las [instrucciones oficiales de instalación](https://go.dev/doc/install), luego verifique su versión:

<!-- embedded: version -->
```bash
go version
```

Go configura automáticamente `GOROOT` y su `GOPATH` predeterminado; no se necesitan valores personalizados.

## Crear un módulo de aplicación

Comience en un directorio con permisos de escritura fuera del código fuente de Speedle. Utilice un directorio nuevo para esta guía:

<!-- embedded: setup -->
```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0
```

En un módulo de Go existente, ejecute solo el comando `go get` final desde el directorio de ese módulo.

Aquí, `go get` añade una **dependencia de biblioteca** con versión a `go.mod` y `go.sum`; no instala un programa. Esto difiere del Inicio rápido, que utiliza `git clone` y `go build` para compilar los ejecutables de los servicios independientes. Ambos utilizan módulos de Go. Mantenga `go.mod` y `go.sum` bajo control de versiones.

## Crear un archivo de políticas

En `speedle-embedded-demo`, guarde este contenido como `policies.spdl`. Concede permiso a `alice` para ejecutar la acción `get` sobre el recurso `res1` en el servicio `mysvc`:

<!-- embedded: policies -->
```ini
[service.mysvc]
[policy]
grant user alice get res1
```

El constructor `eval.NewFromFile` carga un archivo local que contiene políticas en SPDL o JSON. Esta guía utiliza SPDL.

## Inicializar el evaluador y solicitar decisiones

Guarde el siguiente programa completo como `main.go` en el mismo directorio:

<!-- embedded: program -->
```go
package main

import (
	"fmt"
	"log"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func main() {
	evaluator, err := eval.NewFromFile("./policies.spdl", false)
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range []string{"alice", "bob"} {
		allowed, _, err := evaluator.IsAllowed(ads.RequestContext{
			Subject: &ads.Subject{
				Principals: []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}},
			},
			ServiceName: "mysvc",
			Resource:    "res1",
			Action:      "get",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s allowed=%t\n", user, allowed)
	}
}
```

La importación vacía `_ "github.com/teramoby/speedle-plus/pkg/store/file"` registra el controlador de almacén de archivos. Omitirla provocará un error de tipo `unknown store type "file"`.

`eval.NewFromFile(loc, isWatch)` devuelve un `ads.PolicyEvaluator` y un error. Compruebe el error antes de utilizar el evaluador. Con `isWatch=false`, las políticas se cargan una vez; establecerlo en `true` habilita la vigilancia del archivo para una aplicación de larga ejecución.

Llame a `evaluator.IsAllowed(ctx)` en esa instancia. Su `ads.RequestContext` contiene las entidades principales del sujeto, `ServiceName`, `Resource`, `Action` y atributos opcionales (`Attributes`). Devuelve `(allowed bool, reason ads.Reason, err error)`. Un error de evaluación debe tratarse por separado de una decisión exitosa de `allowed=false`.

## Compilar y ejecutar

Desde `speedle-embedded-demo`, resuelva las dependencias y compile su aplicación:

<!-- embedded: build -->
```bash
go mod tidy
go build -o ./bin/embedded-demo .
```

Ejecútelo desde el mismo directorio para que `./policies.spdl` se resuelva correctamente:

<!-- embedded: run -->
```bash
./bin/embedded-demo
```

La salida de la decisión es:

```text
alice allowed=true
bob allowed=false
```

`alice` coincide con la concesión; `bob` no tiene ninguna concesión coincidente y se le deniega el acceso. No se necesitan servicios de red ni bases de datos para este ejemplo.
