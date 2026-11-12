+++
title = "Python SDK"
description = "Añada el SDK de Speedle para Python y compruebe autorizaciones frente a un Speedle local o Speedle Cloud."
weight = 20
aliases = ["/es/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Instale speedle 0.2.0 con pip o uv, y luego llame a is_allowed y check frente a un Speedle local o Speedle Cloud."
lastmod = 2026-11-07T19:43:15+08:00
+++

El SDK de Python requiere Python 3.10 o posterior; su única dependencia de tiempo de ejecución es `httpx`. Cuenta con un `Client` síncrono y un `AsyncClient` asíncrono con los mismos argumentos. Paquete: [`speedle`](https://pypi.org/project/speedle/) 0.2.0 en PyPI. Referencia: [speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

Con pip, en un entorno virtual:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.2.0
```

O con uv, en un proyecto uv:

```bash
uv add speedle==0.2.0
```

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Guarde este programa como `main.py`:

```python
import sys

from speedle import Auth, Client, SpeedleError

try:
    with Client(
        backend="speedle",
        endpoint="http://localhost:6734",
        auth=Auth.none(),
        service_name="docs",
    ) as speedle:
        allowed = speedle.is_allowed("alice", "get", "/docs/public")
        print(f"alice get /docs/public: allowed={allowed}")

        decision = speedle.check("alice", "delete", "/docs/public")
        print(f"alice delete /docs/public: allowed={decision.allowed} reason={decision.reason!r}")
except SpeedleError as e:
    print(f"Speedle error: {e}", file=sys.stderr)
    sys.exit(1)
```

Ejecútelo con `python main.py` en el entorno virtual, o con uv:

```bash
uv run main.py
```

Salida:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```

## Llamar a Speedle Cloud

> **Speedle Cloud estará disponible próximamente.**

El programa lee el nombre del servicio desde `SPEEDLE_SERVICE` y una clave API de cuenta con ámbito `ads` desde `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` anula el endpoint predeterminado `https://cloud.speedle.io`. Sustituya `main.py` por:

```python
import os
import sys

from speedle import Auth, Client, SpeedleError

try:
    with Client(
        backend="speedle-cloud",
        endpoint=os.environ.get("SPEEDLE_ENDPOINT", "https://cloud.speedle.io"),
        auth=Auth.bearer(os.environ["SPEEDLE_TOKEN"]),
        service_name=os.environ["SPEEDLE_SERVICE"],
    ) as speedle:
        allowed = speedle.is_allowed("alice", "get", "/docs/public")
        print(f"alice get /docs/public: allowed={allowed}")

        decision = speedle.check("alice", "delete", "/docs/public")
        print(f"alice delete /docs/public: allowed={decision.allowed} reason={decision.reason!r}")
except SpeedleError as e:
    print(f"Speedle error: {e}", file=sys.stderr)
    sys.exit(1)
```

Configure las variables de entorno y ejecútelo. `read -rs` evita que la clave API quede registrada en el historial de la terminal o en pantalla:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
python main.py
```

Salida, cuando el servicio cuenta con la política de ejemplo:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```

La clave API es un secreto del lado del servidor. Nunca lo distribuya a navegadores, aplicaciones móviles o clientes de escritorio.

## Gestión de errores

Una solicitud denegada no constituye un error: `is_allowed` devuelve `False`, y `check` devuelve una decisión con `allowed` en false y el motivo, tal como se muestra en la llamada a `delete` anterior. Con la configuración predeterminada, cuando una llamada no puede devolver una decisión, el SDK genera una excepción `SpeedleError`. Los ejemplos la capturan, imprimen `Speedle error: <mensaje>` en la salida de error estándar y finalizan con el código de salida 1.
