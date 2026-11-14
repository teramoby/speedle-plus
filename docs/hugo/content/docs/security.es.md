+++
title = "Seguridad"
description = "Proteja sus instancias de Speedle"
weight = 310
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["security", "guide", "tls", "https"]
categories = ["docs"]
bref = ""
lastmod = 2026-10-29T11:53:01+08:00
+++

<a id="protect-both-api-transports"></a>
## Proteger ambos transportes de API

Los procesos independientes de PMS y ADS de Speedle exponen **REST y gRPC**. Ambos utilizan ahora los mismos parámetros de transporte: `--insecure`, `--cert`, `--key`, `--client-cert` y `--force-client-cert`. Certificados no válidos o una configuración de confianza incorrecta provocarán un fallo de inicio; el modo seguro nunca se degrada a texto en plano.

TLS cifra la conexión y autentica al servidor. Para requerir clientes autenticados, proporcione una CA de cliente con `--client-cert` **y** configure `--force-client-cert=true`. Sin ese indicador, una CA configurada valida el certificado de cliente únicamente si el cliente proporciona uno. TLS por sí solo no autoriza a un administrador ni proporciona validación de tokens de portador (bearer tokens). Utilice una pasarela autenticada o controles de acceso equivalentes para los endpoints de gestión, diagnóstico y descubrimiento. El indicador `--enable-authz` no proporciona una política de autorización de endpoints integrada.

## Direcciones de escucha y compatibilidad

| Servicio | Dirección REST predeterminada | Dirección gRPC predeterminada |
| --- | --- | --- |
| PMS | `0.0.0.0:6733` | deshabilitada hasta definir `--grpc-endpoint` |
| ADS | `0.0.0.0:6734` | deshabilitada hasta definir `--grpc-endpoint` |

Las direcciones de escucha de REST y gRPC se configuran por separado. Establecer solo `--endpoint` no habilita gRPC. Utilice explícitamente `--grpc-endpoint 127.0.0.1:50001` para PMS o `127.0.0.1:50002` para ADS. `127.0.0.1:0` solicita un puerto efímero para pruebas aisladas; especifique una dirección IPv6 de forma explícita cuando sea necesario.

La precedencia de configuración es la línea de comandos, luego las variables de entorno `SPDL_*`, luego JSON y finalmente los valores predeterminados. Los reemplazos para gRPC son `--grpc-endpoint`, `SPDL_GRPC_ENDPOINT` y `serverConfig.grpcEndpoint`. Consulte los [contratos proto actuales](/es/docs/api/) para los espacios de nombres RPC y la generación de clientes.

## Generar certificados para una prueba local

Compile los binarios mediante el [inicio rápido](/es/quick-start/). Desde la raíz del repositorio, utilice Go 1.27.4 (o un parche más reciente de la misma rama de lanzamiento) para generar una CA local y certificados independientes para servidor y cliente. El certificado de servidor contiene SANs para `localhost`, `127.0.0.1` y `::1`; las claves privadas utilizan permisos 0600. Estos **certificados de demostración de 24 horas** no son credenciales para producción. El generador rechaza sobrescribir archivos existentes: seleccione un nuevo directorio de salida si vuelve a generar.

<!-- docs-check: certificates -->
```bash
go run ./website/tests/examples/certificates --out ./tls
```

## Iniciar servicios autenticados mutuamente

Inicialice un archivo de políticas nuevo una sola vez, con ambos servicios detenidos:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

Inicie PMS en una terminal:

<!-- docs-check: secure-pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:50001 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```

Inicie ADS en una segunda terminal:

<!-- docs-check: secure-ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:50002 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```

Ejecute ambas terminales desde la misma raíz del repositorio. Manténgalas abiertas; detenga cada una con Ctrl-C al terminar. Los certificados de producción deben cubrir los nombres DNS/IPs reales y proceder de la CA de confianza de su organización.

## Conectar con spctl y curl

<!-- docs-check: secure-cli -->
```bash
./bin/spctl --pms-endpoint https://127.0.0.1:6733/policy-mgmt/v1/ \
  --skipverify=false --cacert ./tls/ca.crt --cert ./tls/client.crt --key ./tls/client.key \
  get service --all
```

<!-- docs-check: secure-curl -->
```bash
curl --fail --silent --show-error --cacert ./tls/ca.crt \
  --cert ./tls/client.crt --key ./tls/client.key \
  https://127.0.0.1:6733/policy-mgmt/v1/service
```

Un almacén vacío devuelve `[]`. Omitir el certificado de cliente produce un fallo en el protocolo de enlace TLS. `spctl` utiliza `--cacert` para la confianza del servidor y `--cert`/`--key` para su propia identidad; `--client-cert` y `--force-client-cert` son opciones del servidor. Mantenga `--skipverify=false` (el valor predeterminado).

Para gRPC, configure el canal del cliente generado con la misma CA, un nombre de servidor coincidente y el certificado/clave del cliente. PMS utiliza `pb.PolicyManager`; ADS utiliza `pb.Evaluator`. Un canal no seguro debe fallar contra estos endpoints TLS.

## Desarrollo local en texto plano

Solo un parámetro explícito `--insecure=true` habilita texto en plano para ambos transportes. Vincule REST y gRPC a loopback para ese modo. No publique estos endpoints sin autenticación ni confíe en nombres de entidad principal o atributos suministrados por emisores no confiables. El [asertor de tokens](/es/docs/assertor/) valida un token de identidad configurado; no autentica a cada cliente de PMS/ADS ni sustituye los controles de acceso a los endpoints.
