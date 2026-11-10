+++
title = "Función personalizada"
description = "Escriba condiciones de políticas con sus propias funciones"
date = 2019-01-21T10:43:02+08:00
weight = 600
draft = false
bref = "Cuando la condición en una política o política de rol es demasiado compleja para redactarla mediante una expresión y funciones integradas, puede escribir sus propias funciones para incluirlas en la condición."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-11-07T01:07:23+08:00
+++

## ¿Por qué utilizar una función personalizada?

Las condiciones en SPDL pueden invocar una función configurada cuando los operadores o funciones integradas no permiten expresar la regla requerida. Speedle invoca su endpoint REST mediante `POST {"params":[...]}` y espera recibir `{"result":...}` o `{"error":"..."}`. El orden/tipo de los parámetros y el tipo del resultado deben coincidir con la condición. Los fallos en las funciones constituyen errores de evaluación; las aplicaciones deben fallar de forma restrictiva (fail closed).

## Iniciar el ejemplo local

Este ejemplo acepta valores numéricos entre 100 y 50000. Los tipos de parámetros incorrectos o una lista de parámetros vacía devuelven un error HTTP. Utiliza texto en plano únicamente en una dirección IP de loopback; utilice TLS para otras interfaces.

Inícielo en una terminal independiente:

<!-- docs-check: function-server -->
```bash
go run ./website/tests/examples/custom-function
```


<!-- docs-source: website/tests/examples/custom-function/main.go -->
```go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"time"
)

type request struct {
	Params []interface{} `json:"params"`
}
type response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func isValid(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		send(405, response{Error: "POST required"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil || len(input.Params) == 0 {
		send(400, response{Error: "params must contain at least one number"})
		return
	}
	valid := true
	for _, param := range input.Params {
		value, ok := param.(float64)
		if !ok {
			send(400, response{Error: "all params must be numbers"})
			return
		}
		if value < 100 || value > 50000 {
			valid = false
		}
	}
	send(200, response{Result: valid})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:23456", "Function listen address")
	cert := flag.String("cert", "", "TLS server certificate")
	key := flag.String("key", "", "TLS server key")
	flag.Parse()
	if (*cert == "") != (*key == "") {
		log.Fatal("provide both --cert and --key")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/func/isValid", isValid)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if *cert != "" {
		log.Fatal(server.ListenAndServeTLS(*cert, *key))
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("plaintext examples require a loopback IP; provide TLS certificates for other addresses")
	}
	log.Fatal(server.ListenAndServe())
}
```


## Antes de ejecutar los ejemplos

Compile `./bin/spctl`, `./bin/speedle-pms` y `./bin/speedle-ads` mediante el [inicio rápido](/es/quick-start/). Utilice Go 1.26.10 o Go 1.27.3 (o un parche más reciente de la misma rama de lanzamiento), Bash, curl y Python 3. Ejecute cada terminal desde la raíz del repositorio. Utilice un archivo de políticas nuevo para esta página; reinícielo únicamente tras detener ambos servicios.

Inicialice un archivo de políticas vacío:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

En la primera terminal, inicie PMS y déjelo en ejecución:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

En la segunda terminal, inicie ADS y déjelo en ejecución:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

Ejecute los comandos restantes en una tercera terminal. PMS y ADS comparten el archivo; watch propaga los cambios de forma asíncrona. Confirme que la actualización deseada ha llegado a ADS antes de depender de ella. `--insecure=true` es solo para este ejemplo en loopback; consulte [seguridad](/es/docs/security/) para conexiones de producción. Indicar direcciones explícitas de PMS evita modificar `~/.spctlconfig`. Detenga ambas terminales de servidor con Ctrl-C al terminar.

## Registrar e invocar la función

La siguiente definición deshabilita el almacenamiento en caché de resultados para que las decisiones reflejen cada solicitud. Crea el servicio antes de su política.

<!-- docs-check: function-fixture -->
```bash
cat > function.json <<'JSON'
{"name":"isValid","funcURL":"http://127.0.0.1:23456/func/isValid","resultCachable":false}
JSON
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create function --json-file function.json
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service service1
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user Ally access library if isValid(attr1)" --service-name service1
```

Las siguientes solicitudes devuelven Conceder (`200` está dentro del rango) y Denegar (`50` está fuera de él):

<!-- docs-check: function-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":200}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: function-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":50}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

## TLS y almacenamiento en caché

Para un endpoint TLS, genere [certificados de prueba locales](/es/docs/security/) nuevos y luego inicie la función con `go run ./website/tests/examples/custom-function --cert ./tls/server.crt --key ./tls/server.key`. Registre `funcURL` como `https://localhost:23456/func/isValid` y establezca `ca` con el **contenido del certificado PEM** de `tls/ca.crt`, no con su ruta de archivo. Los certificados de producción deben cubrir el host real. No se requiere ningún certificado vencido codificado de forma fija o clave privada.

`resultCachable` es false por defecto; `resultTTL` es el tiempo de vida en segundos cuando la caché está habilitada. Habilite la caché solo si el resultado sigue siendo correcto para ese período y conjunto de parámetros. El estado externo, como el estado de una cuenta, puede cambiar antes de que expire el TTL. Utilice la [gestión de funciones de PMS](/es/docs/api/management_api/) para administrar definiciones; la invocación de funciones externas no está disponible en el Playground del navegador.

## Ejemplo: privilegios permanentes cero (ZSP)

Las condiciones SPDL y las funciones personalizadas pueden admitir un patrón de privilegios permanentes cero (ZSP) con credenciales de corta duración y alcance limitado, junto con la verificación de tokens y el consumo atómico en la aplicación. El [ejemplo ejecutable de ZSP](https://github.com/teramoby/ai.speedle.io/tree/main/samples/zsp) para Speedle autohospedado combina una función personalizada con comprobaciones de políticas relativas a la vida útil de un token, la ventana de validez y su estado no utilizado. La emisión, verificación y consumo atómico de tokens continúan siendo responsabilidad de la aplicación.
