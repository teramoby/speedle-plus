+++
title = "Guía de pruebas de rendimiento"
description = "Ejecute una comparación justa y reproducible de motores de autorización en su propia máquina con ayuda de una herramienta de IA."
weight = 15
draft = false
toc = true
singlelinetitle = true
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "performance", "benchmark", "policy"]
categories = ["docs"]
bref = "Mida Speedle y otros motores de autorización en su propio hardware y con sus propias políticas"
lastmod = 2026-11-11T12:00:00+08:00
+++

Esta guía le ayuda a comparar Speedle con otros motores de autorización en su propia máquina. No publica un informe de rendimiento. Los números dependen de su hardware, su conjunto de políticas y las versiones de los motores que fije, así que el resultado que merece confianza es el que usted mismo mida.

La guía ofrece un escenario compartido, políticas equivalentes para cuatro motores, una lista de comprobación metodológica y un prompt que puede pegar en una herramienta de IA como Claude Code, ChatGPT Codex, DeepSeek Harness o similar. El prompt pide a la herramienta que escriba los scripts, los ejecute y deje la salida sin procesar en su máquina. Trate lo que escriba la herramienta como código que debe revisar, no como un resultado publicado.

## Qué obtiene con esta guía

- Reglas RBAC y ABAC equivalentes para Speedle, Open Policy Agent (OPA), Casbin y Cedar, con la decisión esperada para cada solicitud.
- Scripts de benchmark ejecutables, comandos exactos y versiones de motor fijadas, generados y ejecutados en un directorio temporal de su máquina.
- Percentiles de latencia, rendimiento, uso de memoria y escalado, con la carga de políticas medida por separado de la evaluación en estado estable.
- Salida sin procesar, detalles del entorno y una plantilla de resultados que permanecen en local. Esta guía no sube nada.

El conjunto de motores candidatos es un punto de partida, no una clasificación. Confirme que cada versión que elija admite el escenario, registre los casos que excluya y por qué, y no presente un caso excluido como resultado de ese motor.

## Antes de empezar

- Use una máquina que controle y pueda dejar inactiva durante las ejecuciones. Cierre otros procesos pesados y anote el modelo de CPU, el número de núcleos, la memoria y el sistema operativo.
- Instale las herramientas que necesita la comparación: Git y Go 1.26 o posterior para Speedle y los motores basados en Go, y lo que exija el inicio rápido oficial de cada uno de los demás motores. Fije todas las versiones, incluida la etiqueta de versión de Speedle.
- Use una herramienta de IA que pueda crear archivos y ejecutar comandos de shell en un directorio temporal. Revise sus scripts antes de ejecutarlos y conserve el directorio completo, incluida la salida sin procesar, como registro.
- Reserve entre 30 y 60 minutos para una primera comparación con conjuntos de políticas pequeños, y más tiempo si añade recuentos de políticas y concurrencia.

Trabaje en un directorio vacío. No ejecute la comparación dentro del repositorio de Speedle; mantenga los scripts, las políticas, los registros y los resultados separados de cualquier repositorio.

## El escenario compartido

La comparación usa un único conjunto de reglas de negocio:

- Los empleados pueden leer los documentos de su propio departamento.
- Los editores pueden leer y editar los documentos de su propio departamento.
- Bob es editor, pero tiene prohibido explícitamente editar el informe financiero del cuarto trimestre.
- Una solicitud a otro departamento se deniega aunque el usuario tenga el rol correspondiente.

La solicitud lleva el `department` del usuario y el `owner_department` del recurso como atributos que aporta la aplicación. Todos los motores reciben las mismas solicitudes y deben producir la misma decisión.

| Usuario | Rol | Acción | Recurso | department | owner_department | Esperado |
| --- | --- | --- | --- | --- | --- | --- |
| alice | employee | read | `/docs/finance/report` | finance | finance | Permitir |
| alice | employee | read | `/docs/hr/salaries` | finance | hr | Denegar |
| bob | editor | edit | `/docs/finance/report` | finance | finance | Permitir |
| bob | editor | edit | `/docs/finance/q4` | finance | finance | Denegar (denegación explícita) |
| carol | editor | edit | `/docs/finance/q4` | hr | finance | Denegar (condición de departamento) |
| carol | editor | read | `/docs/hr/handbook` | hr | hr | Permitir |

Los ejemplos de esta guía se validaron con Speedle v1.6.0, OPA v1.22.3, Casbin v2.123.0 y Cedar CLI v4.14.1. Fije las versiones que instale; son ejemplos, no una afirmación de que sean las únicas versiones compatibles.

### Speedle (SPDL)

```spdl
[service.documents]

[policy]
grant role employee read expr:/docs/.* if department == owner_department
grant role editor read,edit expr:/docs/.* if department == owner_department
deny user bob edit /docs/finance/q4

[rolepolicy]
grant user alice role employee
grant user bob role editor
grant user carol role editor
```

En Speedle, una denegación aplicable prevalece sobre un permiso. Una condición que hace referencia a un atributo ausente de la solicitud no coincide, así que la aplicación debe enviar `department` y `owner_department` en cada solicitud. El archivo de políticas se valida con `spctl check`; consulte [SPDL](../spdl/) para la gramática y [Gestión de políticas](../pms/policy-mgmt/) para los servicios y las políticas de rol.

### OPA (Rego)

Política:

```rego
package documents

import rego.v1

default allow := false

allow if {
	not denied
	input.action == "read"
	"employee" in roles
	input.attributes.department == input.attributes.owner_department
}

allow if {
	not denied
	input.action in {"read", "edit"}
	"editor" in roles
	input.attributes.department == input.attributes.owner_department
}

roles contains role if {
	some role in data.roles[input.user]
}

denied if {
	input.user == "bob"
	input.action == "edit"
	input.resource == "/docs/finance/q4"
}
```

Datos (`data.json`):

```json
{
  "roles": {
    "alice": ["employee"],
    "bob": ["editor"],
    "carol": ["editor"]
  }
}
```

Rego no trae una regla integrada de prioridad de denegación, así que las políticas anteriores restan la denegación explícita de las dos reglas de permiso. Los datos de roles forman parte del documento de datos de OPA. Use la sintaxis de Rego de la versión de OPA que fije (el ejemplo usa la sintaxis Rego v1 de OPA 1.x).

### Casbin

Modelo (`model.conf`):

```ini
[request_definition]
r = sub, obj, act, dept, ownerDept

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (r.sub == p.sub || g(r.sub, p.sub)) && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act) && r.dept == r.ownerDept
```

Política (`policy.csv`):

```csv
p, employee, /docs/*, read, allow
p, editor, /docs/*, read, allow
p, editor, /docs/*, edit, allow
p, bob, /docs/finance/q4, edit, deny
g, alice, employee
g, bob, editor
g, carol, editor
```

Casbin divide el modelo, las filas de política y las asignaciones de rol en tres entradas. La comprobación ABAC de departamento vive en la expresión de coincidencia, así que también se aplica a la fila de denegación; la solicitud denegada de este escenario lleva departamentos coincidentes, lo que mantiene las decisiones esperadas. Registre cualquier cambio que haga en la expresión de coincidencia para una denegación incondicional.

### Cedar

Esquema (`schema.cedar`):

```cedar
entity Role;
entity User in [Role];
entity Folder;
entity Document in [Folder];

action "read" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
action "edit" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
```

Política (`policies.cedar`):

```cedar
permit (
  principal in Role::"employee",
  action == Action::"read",
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

permit (
  principal in Role::"editor",
  action in [Action::"read", Action::"edit"],
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

forbid (
  principal == User::"bob",
  action == Action::"edit",
  resource == Document::"/docs/finance/q4"
);
```

Entidades (`entities.json`):

```json
[
  {"uid": {"type": "User", "id": "alice"}, "attrs": {}, "parents": [{"type": "Role", "id": "employee"}]},
  {"uid": {"type": "User", "id": "bob"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "User", "id": "carol"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "Folder", "id": "/docs"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/docs/finance/report"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/finance/q4"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/salaries"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/handbook"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]}
]
```

En Cedar, un forbid prevalece sobre un permit y toda solicitud empieza denegada. Los roles y la jerarquía de documentos son datos del almacén de entidades, no texto de la política, así que el esquema y las entidades forman parte de la comparación. La lista de entidades del ejemplo debe incluir todos los documentos que usa la matriz de solicitudes.

### Diferencias semánticas que debe registrar

La misma regla de negocio no siempre se traduce en los mismos conceptos. Antes de medir nada, haga que las cuatro implementaciones coincidan en las decisiones esperadas y anote:

- Cómo resuelve cada motor una denegación que coincide con un permiso, y si la denegación es incondicional o comparte la condición ABAC.
- Qué hace cada motor cuando una solicitud omite `department` u `owner_department`.
- Cómo se expresan los patrones de recurso (expresiones regulares `expr:` de SPDL, expresiones de coincidencia de Casbin, reglas auxiliares de OPA, jerarquía de entidades de Cedar).
- Qué partes son código de aplicación o configuración obligatoria fuera del texto de la política (esquema y entidades de Cedar, modelo y datos de roles de Casbin, documentos de datos de OPA, servicio y políticas de rol de Speedle).
- Qué caso no puede expresar un motor en la versión elegida; exclúyalo de los casos clasificados y dígalo.

## Medir lo correcto

Use la misma máquina, los mismos datos de solicitud y los mismos tamaños de conjunto de políticas para todos los motores. Fije las versiones y conserve el conjunto de datos generado, para que una nueva ejecución mida el mismo trabajo.

- **Separe la evaluación integrada de las pruebas de servicio o API.** Una llamada a función en proceso y una solicitud HTTP miden cosas distintas. Preséntelas en tablas separadas y nunca clasifique una frente a la otra.
- **Caliente y repita.** Ejecute cada motor las veces necesarias para alcanzar un resultado estable, descarte las ejecuciones de calentamiento, repita la medición completa al menos tres veces e informe la variación entre ejecuciones.
- **Informe latencia y rendimiento.** Incluya la latencia p50, p95 y p99 y las solicitudes por segundo cuando las herramientas del motor lo permitan. Registre los fallos y los tiempos de espera.
- **Mida la memoria.** Registre la memoria residente máxima o la métrica de memoria propia del motor que pueda medir de forma consistente.
- **Varíe el número de políticas y la concurrencia.** Empiece con un conjunto pequeño y aumente después el número de políticas y de llamadas concurrentes. Registre todos los puntos de datos, no solo el mejor.
- **Mida la carga de políticas y el arranque por separado.** Mida cuánto tarda cada motor en analizar y cargar el conjunto de políticas antes del bucle en estado estable; no oculte ese coste en la primera solicitud.
- **Declare las cachés y la preparación.** Anote los índices, las políticas compiladas, las cachés de decisiones y cualquier cálculo previo, y si estaban calientes o fríos.
- **Conserve la salida sin procesar.** Guarde el comando exacto, la configuración, los archivos de política, las versiones de los motores, el entorno y la salida completa de cada ejecución.

## Ejecutar la comparación con una herramienta de IA

Abra un directorio vacío en Claude Code, ChatGPT Codex, DeepSeek Harness o una herramienta similar que pueda escribir archivos y ejecutar comandos. Pegue el prompt siguiente tal cual. Pide a la herramienta que registre el entorno, fije las versiones, construya el escenario compartido, valide las decisiones esperadas, ejecute las mediciones y produzca una plantilla de resultados local.

```text
Ayúdame a ejecutar una comparación rápida de rendimiento de Speedle con OPA, Casbin y Cedar en mi propia máquina. Primero registra mi sistema operativo, mi hardware y mis límites de recursos; después selecciona y fija versiones de motor compatibles. Crea ejemplos equivalentes de RBAC y ABAC con un conjunto de datos compartido y decisiones de permitir/denegar esperadas. Explica las diferencias semánticas y excluye de las clasificaciones directas los casos no compatibles. Valida la corrección antes de medir el rendimiento.

Crea scripts de benchmark ejecutables y comandos exactos de instalación y ejecución. Mantén separadas las pruebas integradas de las de servicio/API. Usa cargas de trabajo idénticas, varía el número de políticas y la concurrencia, declara las cachés y la preparación, calienta cada motor y repite las mediciones. Informa latencia, rendimiento, uso de memoria y variación entre ejecuciones cuando sea posible, con la salida sin procesar y los detalles del entorno. Informa solo los números que hayas medido realmente; etiqueta todo lo que no hayas podido ejecutar.

Muestra la política equivalente y cualquier modelo, esquema o código de aplicación necesario para cada motor. Compara legibilidad, conceptos que hay que aprender, esfuerzo de autoría, validación, depuración y facilidad para cambiar una política. Explica las ventajas y desventajas y qué motor encaja en cada carga de trabajo probada sin suponer que Speedle gana.
```

La herramienta debe trabajar en el directorio actual y dejar allí los scripts, las políticas y la salida. Revise los scripts antes de la primera ejecución, especialmente cualquier paso de descarga o instalación. Si la herramienta no puede instalar un motor en su entorno, pídale que marque ese motor como no ejecutado en lugar de estimar un número.

## Revisar el trabajo de la herramienta de IA

- Confirme que las cuatro implementaciones devuelven la decisión de la tabla para cada solicitud antes de empezar a medir. Una respuesta rápida pero incorrecta no es un resultado.
- Confirme que las versiones instaladas coinciden con las que informa y que se usaron la misma máquina, los mismos datos de solicitud y los mismos tamaños de conjunto de políticas para todos los motores.
- Confirme que la carga y el arranque se informan por separado del bucle en estado estable, y que las mediciones integradas y las de servicio/API están en tablas separadas.
- Confirme que existe salida sin procesar para cada número del resumen; un número sin comando ni salida detrás se trata como no medido.
- Lea la sección de limitaciones. Un motor que no pudo ejecutar un caso debe aparecer como no ejecutado, no clasificado.

## Plantilla de resultados

Pida a la herramienta que complete una tabla local como esta y consérvela junto a la salida sin procesar. No la publique como un informe de rendimiento de Speedle.

| Motor | Versión | Modo | Políticas | Concurrencia | p50 | p95 | p99 | Rendimiento | Memoria máxima | Tiempo de carga |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Speedle | | integrado | | | | | | | | |
| OPA | | integrado | | | | | | | | |
| Casbin | | integrado | | | | | | | | |
| Cedar | | integrado | | | | | | | | |

Conserve también una tabla de ejecuciones:

| Ejecución | Motor | Comando | Salida sin procesar | Notas |
| --- | --- | --- | --- | --- |
| 1 | | | | |

## Comparación de facilidad de uso

Las mediciones y la facilidad de uso son observaciones separadas. Compare las mismas reglas de negocio y registre cómo las expresa cada motor:

- **Speedle (SPDL).** Cada regla es una sentencia, las asignaciones de rol están en una sección de políticas de rol aparte y la condición es una expresión booleana sobre atributos de la solicitud. Las políticas se pueden validar con `spctl check`.
- **OPA (Rego).** Las reglas se escriben como cuerpos bajo una denegación por defecto, los datos de roles viven en un documento aparte y una denegación explícita necesita su propia regla y exclusiones en las reglas de permiso.
- **Casbin.** El modelo, las filas de política y las asignaciones de rol son entradas separadas. La comprobación ABAC forma parte de la expresión de coincidencia, así que cambiar la condición implica cambiar el modelo.
- **Cedar.** Un esquema declara las entidades y las acciones, las políticas son sentencias permit y forbid, y los roles y la jerarquía de recursos viven en el almacén de entidades. Forbid prevalece sobre permit.

Para cada motor, registre la legibilidad (cuán directamente la regla se lee como la frase de negocio), los conceptos que tuvo que aprender, el código repetitivo y de aplicación, la validación y los mensajes de error, las pruebas y la depuración, y lo que cuesta añadir o cambiar una regla. El número de líneas de política puede dar contexto, pero no lo use como única medida de facilidad de uso.

## Benchmarks solo de Speedle

Los [benchmarks](https://github.com/teramoby/speedle-plus/blob/master/benchmarks/README.md) del repositorio cubren una evaluación solo de Speedle, incluido un caso opcional de un millón de políticas. Ejecútelos en el checkout con `go test -bench .` y siga el README para el caso de un millón de políticas. Esas mediciones son solo de Speedle y se tomaron en hardware concreto; no las presente como una comparación actual entre motores.

## Informar con justicia

- Use el mismo escenario, los mismos datos y los mismos tamaños de conjunto de políticas para todos los motores, y diga qué casos no pudo ejecutar cada uno.
- Informe todo el rango de sus resultados, incluidos los casos en que otro motor sea más rápido.
- Explique qué motor encaja en cada carga de trabajo probada, con sus limitaciones. No declare un motor más rápido de forma universal a partir de un escenario en una máquina.
- Conserve la salida sin procesar y los comandos exactos junto a cualquier resumen que comparta.
