+++
title = "Asertor de tokens"
description = "Validar tokens de identidad para proporcionar entidades principales y dominios de identidad de confianza a las solicitudes de autorización de Speedle"
date = 2019-01-21T09:28:30+08:00
weight = 60
draft = false
bref = "Las solicitudes de evaluación pueden contener un token de identidad que representa a un usuario. En este caso, el motor de evaluación puede invocar el servicio de aserción para obtener las identidades explícitas del usuario"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["Identity", "Token asserter"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## ¿Qué hace un asertor de tokens?

Un asertor configurado valida un token y devuelve entidades principales explícitas, incluidos sus dominios de identidad. A continuación, Speedle evalúa las políticas utilizando esas identidades. Una implementación en producción debe validar la firma del token o el resultado de introspección, el emisor, la audiencia, la expiración y cualquier requisito específico del proveedor. La interfaz webhook por sí sola no establece compatibilidad automática con tokens OAuth/OIDC arbitrarios.

La [API del complemento de aserción de tokens](/es/docs/api/asserter_api/) utiliza `errCode` entero y un **objeto** de atributos opcional, no una matriz de atributos tipados. `asserterWebhookConfig.endpoint` es la URL completa; el ejemplo siguiente utiliza `/v1/assert`.

## Ejecutar un simulador local determinista

Este mock reconoce solo tres combinaciones de entrada: `githubtoken`/`github`, `googletoken`/`google` y `gitlabtoken`/`gitlab`. Cada una representa a `user1` en el dominio correspondiente. Los pares token/proveedor incorrectos fallan. **Es un fixture de prueba, no una validación de tokens real.** No registra valores de tokens. Manténgalo únicamente en bucle local y sustitúyalo por una integración real con su proveedor en producción.

Inícielo en un terminal independiente desde la raíz del repositorio:

<!-- docs-check: mock-asserter -->
```bash
go run ./website/tests/examples/asserter
```


<!-- docs-source: website/tests/examples/asserter/main.go -->
```go
// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
```


## Antes de ejecutar los ejemplos

Compile `./bin/spctl`, `./bin/speedle-pms` y `./bin/speedle-ads` utilizando la [guía de inicio rápido](/es/quick-start/). Se requiere Go 1.26.10 o Go 1.27.3 (o un parche posterior en la misma rama), Bash, curl y Python 3. Ejecute cada terminal desde la raíz del repositorio. Utilice un archivo de políticas nuevo para esta página; restablézcalo solo después de detener ambos servicios.

Inicialice un archivo de políticas vacío:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

En el terminal 1, inicie PMS y manténgalo en ejecución:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

En el terminal 2, inicie ADS y manténgalo en ejecución:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true --asserter-endpoint http://127.0.0.1:8080/v1/assert
```

Ejecute los comandos restantes en un tercer terminal. PMS y ADS comparten el mismo archivo; watch propaga los cambios de forma asíncrona. Confirme que la actualización deseada ha llegado a ADS antes de confiar en ella. `--insecure=true` es solo para este ejemplo en bucle local; consulte [seguridad](/es/docs/security/) para conexiones de producción. Las direcciones explícitas de PMS evitan modificar `~/.spctlconfig`. Detenga ambos terminales de servidor con Ctrl-C cuando termine.


## Crear políticas de prueba con dominio de identidad

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/idd.json
```


Este fixture otorga a `user1` de GitHub acceso de lectura, a `user1` de Google acceso de escritura y a todos los dominios acceso de alquiler. Es el mismo [ejemplo de dominio de identidad](/es/docs/idd/).

## Comprobar decisiones basadas en tokens

En orden, espere Allow, Deny, Allow. La segunda solicitud es una identidad válida de **GitLab**, por lo que no puede utilizar el permiso de lectura exclusivo de GitHub.

<!-- docs-check: token-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"githubtoken","tokenType":"github"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


Un token no válido es un error de aserción, no un Deny de política habitual. Esta implementación REST aún puede devolver HTTP 200 con `allowed=false` y un `errorMessage` no vacío; compruebe tanto el cuerpo de la respuesta como el código de estado HTTP. No utilice únicamente `reason` como prueba de que la evaluación se ha realizado correctamente:

<!-- docs-check: token-invalid -->
```bash
curl --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"invalid-token","tokenType":"google"},"serviceName":"booksvc","resource":"book","action":"write"}' \
  -w '\nHTTP %{http_code}\n' http://127.0.0.1:6734/authz-check/v1/is-allowed
```


Detenga el mock y ambos servidores con Ctrl-C cuando termine.

## Configurar la integración de su proveedor

En lugar del indicador local, una configuración JSON completa puede especificar:

```json
{
  "asserterWebhookConfig": {
    "endpoint": "https://identity.example/v1/assert",
    "clientCert": "/absolute/path/client.crt",
    "clientKey": "/absolute/path/client.key",
    "caCert": "/absolute/path/issuer-ca.crt",
    "httpTimeout": 5
  }
}
```

Esta es la sección de aserción de la configuración del servidor; proporcione también los parámetros de almacenamiento y servidor para la instancia desplegada. `caCert` verifica el servidor webhook; `clientCert` y `clientKey` se utilizan si ese servidor requiere mTLS. Obtenga los tokens mediante la integración documentada del proveedor con [OpenID Connect](https://openid.net/connect/) o [OAuth](https://oauth.net/2/), y valide el tipo de token aceptado en la práctica por dicho proveedor. Proteja a los emisores de llamadas a PMS/ADS por separado según se describe en [seguridad](/es/docs/security/).
