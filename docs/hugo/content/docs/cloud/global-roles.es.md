+++
title = "Roles globales"
linkTitle = "Roles globales"
description = "Asigne un rol una vez para todos los servicios de un espacio de trabajo de Speedle Cloud y conceda sus permisos en cada servicio."
date = 2026-11-17T00:00:00+08:00
lastmod = 2026-11-21T00:00:00+08:00
weight = 25
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Una asignación de rol para todos los servicios del espacio de trabajo."
+++

**Speedle Cloud está en beta pública.** Esta guía describe el flujo de trabajo del servicio en línea.

Una asignación de rol suele pertenecer a un servicio. Cuando varios servicios del mismo espacio de trabajo comparten la misma asignación, defínala una vez en el servicio **Roles globales** del espacio de trabajo y use el rol en las políticas de autorización de cada servicio.

Solo se admiten políticas de rol. Una asignación global no concede ninguna operación por sí misma: cada servicio sigue definiendo los permisos que permite. Los roles globales pertenecen a un espacio de trabajo y nunca cruzan entre espacios de trabajo.

## Cree la asignación una vez

En el selector de servicios de la consola, elija **Roles globales** al principio de la lista y cree una política de rol:

```text
grant user Emma from corp AdminRole
```

La cláusula `from corp` limita la asignación al dominio de identidad `corp`. Sin ella, la concesión coincide con cualquier usuario llamado Emma en cualquier dominio; consulte [Dominios de identidad](/es/docs/idd/).

El rol `AdminRole` está ahora asignado a Emma en todos los servicios del espacio de trabajo.

## Conceda el permiso en un servicio

Cambie a un servicio que necesite el permiso, por ejemplo `library`, y cree una política de autorización:

```text
grant role AdminRole borrow books
```

Repita este paso en cada servicio que deba permitir la operación. Un servicio sin su propia política de autorización no concede nada, aunque tenga el rol asignado.

## Compruebe el resultado

Abra el Playground de `library` y ejecute una prueba con `[{"type":"user","name":"Emma","idd":"corp"}]` como principal, `books` como recurso y `borrow` como acción. Se permite a través de `AdminRole`; la misma petición de `Bob` se deniega.

El Playground de Roles globales es distinto: una comprobación contra el servicio global siempre se deniega. Los roles globales se aplican en el Playground de cada servicio.

## Comportamiento y límites

- Los roles globales solo aceptan políticas de rol. Una política de autorización en el servicio global devuelve `400 {"error":"global_authz_unsupported"}`.
- El servicio global cuenta como uno de los servicios del espacio de trabajo y sus políticas de rol cuentan para el límite combinado de políticas. Consulte [Límites y errores](/es/docs/cloud/limits-and-errors/).
- La consola muestra este servicio especial como **Roles globales**; no se puede crear a mano, renombrar ni eliminar.
- Cada instancia de Speedle Cloud ADS del espacio de trabajo recibe siempre las políticas de rol globales, incluso cuando sincroniza solo un conjunto seleccionado de servicios.
- Los cambios se propagan como las demás políticas: alrededor de un minuto mediante las instantáneas publicadas y de inmediato en el Playground, que lee el estado actual.
