+++
title = "Instalación con Homebrew"
description = "Instale speedle-spctl, speedle-pms y speedle-ads con Homebrew y ejecute una demostración local."
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Homebrew instala la herramienta de línea de comandos y ambos servicios, y los ejecuta en 127.0.0.1 con un archivo de políticas compartido."
lastmod = 2026-11-10T13:49:59+08:00
+++

Instale la herramienta de línea de comandos de Speedle con [Homebrew](https://brew.sh/) en macOS o Linux. Homebrew no instala ni inicia una formula de un tap fuera de la organización oficial de Homebrew hasta que ese tap sea de confianza.

```bash
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
```

La formula se llama `spctl`. En macOS el comando es `speedle-spctl`, así que la instalación no sustituye el Gatekeeper de Apple en `/usr/sbin/spctl`. En Linux el mismo programa también está disponible como `spctl`.

El servicio de gestión de políticas y el servicio de decisiones de autorización son formulas independientes:

```bash
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

Instale la herramienta de línea de comandos y ambos servicios a la vez:

```bash
brew install teramoby/tap/speedle
```

Cada uno de `spctl`, `speedle-pms` y `speedle-ads` instala el binario de la versión para macOS o Linux y lo comprueba con el `checksums.txt` de esa versión.

<a id="demo-with-homebrew"></a>

## Demostración con Homebrew

Inicie ambos servicios. Escuchan en `127.0.0.1` y comparten un almacén de políticas en archivo.

```bash
brew services start speedle-pms
brew services start speedle-ads
```

`speedle-pms` escucha en `127.0.0.1:6733`. `speedle-ads` escucha en `127.0.0.1:6734`. Ambos usan `--store-type file` y el archivo de políticas `$(brew --prefix)/var/speedle/policies.json`. Los registros están en `$(brew --prefix)/var/log/`. `--insecure=true` es solo para esta demostración en loopback. Consulte [Seguridad](/es/docs/security/) para TLS.

Cree un servicio y una política. En Linux puede usar `spctl` en lugar de `speedle-spctl`.

```bash
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

Pregunte a ADS si `user1` puede obtener `res1`:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":true`. Los servicios leen los cambios de política del archivo compartido. Deténgalos con:

```bash
brew services stop speedle-ads
brew services stop speedle-pms
```
