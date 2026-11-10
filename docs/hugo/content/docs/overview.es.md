+++
title = "Descripción general de Speedle: Motor de autorización para control de acceso detallado"
linktitle = "Descripción general"
description = "Cómo se asigna Speedle a los términos comunes de autorización: PAP, PDP, PEP, PIP, RBAC, ABAC, PBAC, XACML, zero trust, identidad e IAM."
weight = 2
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["core", "pdp", "identity", "spdl"]
categories = ["docs"]
bref = "Dónde encaja Speedle en el vocabulario de autorización y control de acceso"
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle es un motor de autorización de código abierto. Las aplicaciones le envían consultas como "¿puede este usuario realizar esta acción sobre este recurso?", y Speedle responde permitir o denegar de acuerdo con políticas gestionadas centralmente. Esta página relaciona los términos habituales de autorización con los componentes de Speedle que los implementan, y enlaza con las guías correspondientes a cada uno.

Los ejemplos a continuación utilizan un servicio denominado `library`. Cada política se creó mediante la API REST del Servicio de Gestión de Políticas y cada decisión fue devuelta por la API REST del Servicio de Decisiones de Autorización. Los códigos de motivo se detallan en [Decisiones de autorización](../decisions/).

## Motor de autorización

Un motor de autorización evalúa solicitudes de acceso frente a políticas y devuelve una decisión, de forma que la lógica de autorización se mantiene fuera del código de la aplicación. Speedle consta de un [Servicio de Gestión de Políticas (PMS), un repositorio de políticas y un Servicio de Decisiones de Autorización (ADS)](../architecture/). El ADS puede ejecutarse como un servicio independiente o estar [integrado en una aplicación Go](../deployment/embedded/).

## Autorización y control de acceso

La autenticación determina quién es el emisor; la autorización decide qué puede hacer dicho emisor. El control de acceso es la aplicación efectiva de esas decisiones. Speedle gestiona únicamente la autorización: recibe un sujeto previamente autenticado y devuelve una decisión, que la aplicación o pasarela que realiza la llamada hace cumplir. Consulte [Decisiones de autorización](../decisions/).

## Control de acceso detallado (FGA)

El control de acceso detallado decide el acceso por usuario, por recurso y por acción, tomando opcionalmente en consideración el contexto de la solicitud, en lugar de conceder acceso amplio a una aplicación completa. Una política de Speedle nombra a las entidades principales, las acciones y el recurso, y puede incluir una condición. Una política de roles puede asignar un rol únicamente para un recurso determinado. Consulte [SPDL](../spdl/).

## PAP: Punto de administración de políticas

El PAP es donde se crean y gestionan las políticas. En Speedle este es el PMS, utilizado a través de su [API REST](../api/management_api/), API gRPC o la herramienta de línea de comandos `spctl`. Consulte [Gestión de políticas](../pms/policy-mgmt/).

## PDP: Punto de decisión de políticas

El PDP evalúa una solicitud frente a las políticas y devuelve una decisión. En Speedle este es el ADS, o el evaluador integrado cuando Speedle se ejecuta dentro de un proceso Go. Una solicitud de decisión tiene este aspecto:

```bash
curl --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"library","resource":"book","action":"update"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Con las políticas RBAC siguientes, la respuesta es `{"allowed":true,"reason":0}`. Consulte la [API de decisiones](../api/decision_api/).

## PEP: Punto de aplicación de políticas

El PEP intercepta un intento de acceso, solicita una decisión al PDP y la hace cumplir. Speedle no incluye un PEP de propósito general; el PEP es la aplicación o pasarela que invoca al ADS mediante su API REST o gRPC, o que integra el evaluador en Go. Consulte [Integraciones](../../integrations/) para conocer las interfaces compatibles.

## PIP: Punto de información de políticas

El PIP proporciona información adicional necesaria para evaluar una política. En Speedle, esta información proviene de los atributos de la solicitud y de los [atributos integrados](../spdl/#2-2-1-built-in-attributes), del [asertor de tokens](../assertor/), que convierte un token de identidad en un sujeto, y de las [funciones personalizadas](../custom-function/), que invocan endpoints REST externos durante la evaluación.

## XACML

XACML (eXtensible Access Control Markup Language) es un estándar de OASIS que define tanto un lenguaje de políticas como la arquitectura PAP/PDP/PEP/PIP. Speedle sigue la misma división de roles, pero su lenguaje de políticas es SPDL, no XACML, y no admite la importación de políticas XACML.

## RBAC: control de acceso basado en roles

En RBAC, los permisos se conceden a roles y los roles se asignan a usuarios o grupos. Speedle utiliza una política para conceder permisos a un rol y una política de roles para asignar el rol:

```text
grant role librarian read,update book
grant user alice librarian
```

`alice` puede actualizar `book` (motivo 0, grant policy found); a `bob`, que no tiene ningún rol asignado, se le deniega (motivo 3, no applicable policies). Los roles también pueden asignarse solo para un recurso determinado; consulte [Gestión de políticas](../pms/policy-mgmt/) y [Políticas globales](../pms/global-policy/).

## ABAC: control de acceso basado en atributos

En ABAC, la decisión depende de atributos del sujeto, recurso, acción o entorno. En Speedle, cualquier política o política de roles puede incluir una condición `if` sobre atributos:

```text
grant user bob read report if department == 'finance'
```

Una solicitud de `bob` con el atributo de cadena `department` establecido en `finance` se autoriza (motivo 0); con `sales`, o sin el atributo, se deniega (motivo 3). Consulte [Condición](../spdl/#condition).

## PBAC: control de acceso basado en políticas

PBAC gestiona el acceso mediante políticas definidas centralmente en lugar de permisos incrustados en cada aplicación. Todas las políticas de Speedle se mantienen en el PMS y se evalúan mediante dos reglas fijas: la denegación anula la concesión, y una solicitud sin ninguna concesión aplicable es denegada.

```text
grant group staff read,delete journal
deny user carol delete journal
```

A `carol`, miembro de `staff`, se le deniega `delete` sobre `journal` (motivo 1, deny policy found); a `erin`, también miembro de `staff`, se le autoriza (motivo 0).

## Identidad e IAM

La gestión de identidades y accesos (IAM) abarca tanto demostrar quién es un usuario como decidir qué puede hacer ese usuario. Speedle no es un proveedor de identidad y no autentica usuarios. Consume identidades: las entidades principales pueden estar cualificadas mediante un [dominio de identidad](../idd/), y los tokens de identidad pueden resolverse mediante el [asertor de tokens](../assertor/).

```text
grant user dave from github read code
```

`dave` del dominio de identidad `github` puede leer `code` (motivo 0); `dave` de `corp`, o sin dominio de identidad, es denegado (motivo 3).

## Confianza cero (Zero Trust)

Zero Trust no concede acceso en función de la ubicación en la red y comprueba cada solicitud. Speedle lo admite como servicio de decisión: el ADS evalúa cada solicitud que recibe y cualquier elemento no concedido explícitamente se deniega. La autenticación, la confianza del dispositivo y los controles de red quedan fuera del alcance de Speedle.

## SPDL: Lenguaje de definición de políticas Speedle

SPDL corresponde a Speedle Policy Definition Language. Es el lenguaje utilizado para escribir políticas y políticas de roles en Speedle, por ejemplo `grant role librarian read,update book`. Consulte la [referencia de SPDL](../spdl/).

## Integraciones

Otras aplicaciones invocan al ADS a través de sus [APIs REST o gRPC](../api/), o integran el evaluador en Go; consulte [Despliegue](../deployment/). La página de [Integraciones](../../integrations/) enumera las APIs, SDKs, el modo integrado y los almacenes de políticas disponibles en la actualidad.
