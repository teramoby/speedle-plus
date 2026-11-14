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

Implemente `store.StoreBuilder`, registre un nombre único con `store.Register` e importe el paquete en su ejecutable. `GetStoreParams` relaciona indicadores adicionales con propiedades del almacén. Analice sus indicadores antes del arranque; la biblioteca no analiza argumentos ni instala manejadores de señales.

Reutilice el arranque ADS con `ads.Run(ctx, params, conf, options...)` de `github.com/teramoby/speedle-plus/pkg/ads`. `params` es `*flags.Parameters` y `conf` es `*cfg.Config`; cancelar `ctx` detiene ambos servidores. `ads.WithGate(g)` sustituye la detección de puertas en ambos protocolos; `ads.WithGate(nil)` la desactiva. `ads.WithListeners(rest, rpc)` acepta valores `net.Listener` ya enlazados; nil usa el endpoint configurado. Run asume su propiedad y los cierra incluso si falla el arranque. Se conservan TLS de REST y las credenciales del servidor gRPC. `ads.WithHTTPServer(func(*http.Server))` permite envolver el manejador o añadir rutas. `ads.WithGRPCOptions(...)` añade opciones; para interceptores adicionales use `grpc.ChainUnaryInterceptor(...)`, conservando la recuperación estándar de panic.

`pkg/svcs/gate.Gate` es independiente del protocolo: `Route(serviceName string) gate.Route` selecciona `gate.Local`, `gate.Fallback` o `gate.Unavailable`, y `Forward(context.Context, gate.Operation, *ads.RequestContext) (*gate.Response, error)` devuelve un resultado tipado. Local continúa la evaluación normal. Fallback puede permitir, denegar o devolver roles o permisos; un fallo nunca cambia a evaluación local. Diagnose y Discover se rechazan cuando se elige Fallback. `*gate.UnavailableError` contiene un código público de motivo; otros errores y las respuestas nil se convierten en `fallback_unavailable`. REST devuelve 503 con `authorization_unavailable`, el motivo, `Cache-Control: no-store` y `Retry-After: 30`; gRPC devuelve Unavailable. `ErrorMessage` en la respuesta indica un fallo de evaluación: REST IsAllowed devuelve su cuerpo de error normal con 200, las demás operaciones REST devuelven 500 y gRPC devuelve Internal. Las respuestas alternativas correctas llevan `X-Speedle-Decision-Source: upstream` (clave en minúsculas en metadatos gRPC). El método opcional `gate.Auditor.Audit(ctx, event)` recibe un evento por ruta no local, sin sujetos, tokens ni cuerpos de solicitud. También se escriben los registros de auditoría estándar. Las puertas deben admitir concurrencia y gestionar su autenticación y recursos de respaldo.

Un almacén puede implementar `gate.GateProvider.RequestGate() gate.Gate`; REST y gRPC lo detectan mediante el acceso al almacén del evaluador. Para uso embebido, llame a `eval.NewWithStore` y después a `adsrest.NewRouter(evaluator, adsrest.WithGate(g))` o `adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(g))`. Omitir la opción detecta la puerta del almacén; nil explícito la desactiva. Sin puerta, el comportamiento local no cambia.

`store.WatchRequirement.RequireWatch()` obliga a suscribirse aunque la configuración desactive la vigilancia. El método opcional `ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error)` aporta una instantánea y una confirmación que se ejecuta solo después de instalarla correctamente. `(*eval.PolicyEvalImpl).DistributionStore()` expone el almacén a adaptadores. Cerrar el evaluador detiene su vigilancia y cierra un almacén cerrable; `ads.Run` hace esta limpieza. Una puerta explícita sigue siendo propiedad del llamador.
Consulte el [almacén y la puerta de prueba externos](https://github.com/teramoby/speedle-plus/blob/master/pkg/ads/extensions_test.go), las [interfaces de almacén](https://github.com/teramoby/speedle-plus/tree/master/pkg/store) y el [despliegue embebido](/es/docs/deployment/embedded/).
