# PU1 · Optimización de la asignación de órdenes de servicio

Prototipo funcional de la propuesta descrita en [`PLAN.md`](PLAN.md): un sistema de
información que mejora la **asignación de órdenes de servicio a cuadrillas técnicas**
considerando restricciones operativas, reduciendo desplazamientos y distribuyendo la
carga de trabajo de manera equilibrada.

Caso de estudio **hipotético** de telecomunicaciones (no es un proceso validado con
una empresa real). Los datos son sintéticos.

## Levantarlo

```bash
docker compose up --build
# abrir http://localhost:8080
```

PostgreSQL queda **solo en la red interna** de Docker (sin puertos publicados al
host); el frontend expone el puerto 8080 y hace de proxy inverso hacia la API.

```bash
make down      # detener
make test      # unit tests (reglas + simulación) y vet
make test-db   # tests de integración con PostgreSQL efímero
make eval      # comparación base vs. propuesto por CLI
```

Desarrollo local (vite + api en docker):

```bash
make dev
cd frontend && VITE_API_TARGET=http://localhost:8080 npm run dev
```

## Qué demuestra el prototipo

| Criterio | Dónde verlo |
|---|---|
| Flujo solicitud → evaluación → orden → asignación → cronograma | pestañas *Solicitudes*, *Tablero*, *Cronograma* |
| Restricciones obligatorias (competencias, disponibilidad, solapamiento) | modal de candidatos del tablero: motivos de descarte |
| Concurrencia: nunca reservar la misma franja de forma incompatible | botón *Probar concurrencia* (1 éxito, N−1 rechazos) + test `TestConcurrentConfirmExactlyOneWins` |
| Reasignación transaccional con justificación | pestaña *Cronograma* → *Reasignar…* |
| Evaluación reproducible base vs. propuesto | pestaña *Evaluación* (o `make eval`) |

### Resultado de la evaluación (dataset fijo)

Método base (primera cuadrilla factible) vs. método propuesto
(`C = w_d·D + w_t·W + w_l·L`), mismos 12 datos para ambos:

| Indicador | Base | Propuesto |
|---|---|---|
| Órdenes asignadas válidamente | 10/12 (83.3%) | 10/12 (83.3%) |
| Distancia total (euclidiana, malla 10×10) | 46.93 | **34.02** |
| Conflicto de horario | 0 | 0 |
| Distribución final de horas | {5.25, 6.25, 2.5, 2, 4} | {5.25, 4.25, 1, 5.5, 4} |

Ambos métodos dejan 2 órdenes sin asignar por restricciones obligatorias (una
requiere una competencia que ninguna cuadrilla tiene; otra satura la ventana de la
única cuadrilla con la competencia). El método propuesto reduce el desplazamiento
≈27% y distribuye la carga de forma distinta: la comparación permite discutir las
compensaciones entre distancia y equidad. **No implica que una empresa real obtenga
esta mejora**: es un conjunto sintético bajo los supuestos documentados.

## Arquitectura

```
frontend/            React + TypeScript (Vite), nginx con proxy /api → api:8080
backend/             Go + chi + pgx, organizado por capability
  cmd/api            servidor HTTP (migra y siembra al arrancar)
  cmd/evaluate       CLI de evaluación (make eval)
  internal/
    domain/          entidades, enums y invariantes (transiciones, Interval.Overlaps)
    engine/          algoritmo puro de asignación (factibilidad → costo D+W+L)
    fixture/         dataset sintético compartido (evaluación y seed); solo depende de domain
    platform/
      db/            pool, migraciones embebidas, seed, errores sentinel
      dbtest/        BD privada por test (los features corren en paralelo)
      httplib/       JSON, mapeo de errores a HTTP, CORS, X-Role simulado
    features/
      requests/      solicitudes: listar, crear, evaluar remoto/presencial
      orders/        órdenes: listar, transiciones de estado
      crews/         cuadrillas: carga, cronograma
      assignments/   confirmación atómica, reasignación transaccional, demo de concurrencia
      planning/      planificación por lote (ordena el día y coordina assignments)
      evaluation/    comparación base vs. propuesto + método base
    httpapi/         solo ensamblaje de rutas
docker-compose.yml   db (red interna) + api + web
```

Reglas de dependencia: `domain` y `engine` no importan features; `fixture` solo
depende de `domain`; `planning` coordina servicios de `orders` y `assignments`
(sin HTTP ni SQL propio); nadie importa a `httpapi`.

**Integridad de asignaciones:** la tabla `assignments` tiene un constraint
`EXCLUDE USING gist (crew_id, tstzrange)` parcial a los estados que ocupan
(`confirmed`, `in_progress`, `completed`). Dos goroutines que confirmen la misma
franja obtienen una sola fila: la otra recibe `exclusion_violation` (23P01) y la API
responde `409`. La confirmación también re-verifica la disponibilidad dentro de la
transacción, con un advisory lock por cuadrilla.

**Planificación híbrida:** `POST /api/plan` procesa las órdenes pendientes en orden
determinista (prioridad ↓, ventana ↑, id ↑), evalúa las cuadrillas de cada orden **en
paralelo** (goroutines) y confirma de forma atómica; si la reserva es rechazada por un
cambio concurrente, recalcula. Es una heurística voraz, no el óptimo global.

## Limitaciones (declaradas)

- **Selector de roles simulado**: el encabezado `X-Role` cambia la experiencia de
  usuario (administrador / cuadrilla). **No es autenticación ni autorización**: el
  prototipo no está pensado para producción.
- **Distancia euclidiana** sobre una malla ficticia: aproximación del costo de
  desplazamiento, no tiempo real de viaje.
- **Fecha operativa fija** (2026-10-09) para que la demo sea reproducible.
- **Datos sintéticos**: demuestran comportamiento bajo supuestos definidos, no
  mejoras reales en una empresa.
- **Sin centro de atención completo**: la recepción de solicitudes es un formulario;
  el resto del proceso de atención está fuera de alcance.
