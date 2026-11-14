+++
title = "Extender el servicio de decisiones"
description = "Use un almacén de políticas o un adaptador de servicio propio."
weight = 310
lastmod = 2026-11-14T18:30:00+08:00
draft = false
toc = false
categories = ["docs"]
bref = "Use un almacén de políticas o un adaptador de servicio propio."
+++

Implemente `store.StoreBuilder` y devuelva un almacén que cumpla las interfaces de `pms.PolicyStoreManager` que necesite su aplicación. Registre el constructor con un nombre único mediante `store.Register` en la función `init` de su paquete e importe ese paquete en su ejecutable. Los programas estándar importan los almacenes en `cmd/speedle-ads/stores.go` y `cmd/speedle-pms/stores.go`. `GetStoreParams` relaciona los indicadores de línea de comandos con las propiedades del almacén.

Para un servicio embebido, pase su almacén a `eval.NewWithStore` y el evaluador resultante a `adsrest.NewRouter` o `adsgrpc.NewGRPCService`. Un almacén que necesite suscribirse a eventos de cambio puede implementar `store.WatchRequirement`. `(*eval.PolicyEvalImpl).DistributionStore()` se conserva como acceso al almacén para adaptadores externos; el motor no interpreta interfaces de pasarela propias. Componga la pasarela o el enrutamiento en su ejecutable.

Consulte las [interfaces de almacén](https://github.com/teramoby/speedle-plus/tree/master/pkg/store) y el [despliegue embebido](/es/docs/deployment/embedded/).
