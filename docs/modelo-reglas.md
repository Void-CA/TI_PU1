# Entregable 2 — Modelo de reglas

Reglas del motor de asignación, en el orden estricto definido en [`PLAN.md`](../PLAN.md).

## Nivel 1 — Factibilidad (restricciones obligatorias)

Un candidato se descarta si incumple cualquiera de estas reglas. Ninguna puntuación
puede salvar una restricción obligatoria.

| Regla | Código de descarte |
|---|---|
| La cuadrilla está activa | `crew_inactive` |
| Tiene las competencias técnicas requeridas | `missing_skill` |
| Existe un hueco libre dentro de la intersección ventana de la solicitud ∪ disponibilidad de la cuadrilla, sin solapar otras órdenes | `no_free_slot_in_window` |
| La orden no está ya asignada | `order_already_assigned` |

El solapamiento se garantiza además en la base de datos con un constraint
`EXCLUDE USING gist (crew_id, tstzrange)` sobre los estados que ocupan franja
(`confirmed`, `in_progress`, `completed`).

## Nivel 2 y 3 — Eficiencia y equidad (función de costo)

Solo se evalúa sobre candidatos ya factibles:

```
C(q, j) = w_d · D(q, j) + w_t · W(q, j) + w_l · L(q, j)
```

| Símbolo | Significado | Normalización |
|---|---|---|
| `D` | Distancia euclidiana desde el punto de partida de la cuadrilla (su base si no tiene órdenes; si las tiene, la ubicación de su última orden) | ÷ 14.14 (diagonal de la malla 10×10) |
| `W` | Espera estimada: retraso del inicio asignado respecto a la ventana solicitada. **No es conflicto de horario** (eso invalida, no penaliza) | ÷ 240 min |
| `L` | Costo de carga: exceso de la carga resultante sobre la media de las cuadrillas | ÷ 540 min (turno 08:00–17:00) |

Pesos por defecto: `w_d = 0.5`, `w_t = 0.2`, `w_l = 0.3` (configurables).

## Orden de decisión

1. Respetar las restricciones obligatorias (nivel 1).
2. Minimizar desplazamiento y espera del cliente (componentes `D` y `W`).
3. Evitar sobrecargar cuadrillas cuando hay alternativas comparables (componente `L`).

## Selección

- **Método base:** primera cuadrilla factible por ID.
- **Método propuesto:** menor costo; empate resuelto por ID de cuadrilla (determinismo).

El procesamiento de un lote es voraz y secuencial por orden determinista
(prioridad ↓, ventana ↑, ID ↑): **no garantiza el óptimo global de la jornada**.
Las cuadrillas de cada orden se evalúan en paralelo; la confirmación de la reserva
es atómica y recalcula si el estado cambió.

## Reglas de reasignación

- Una cuadrilla no modifica sus asignaciones: registra avances e incidencias.
- La reasignación es **transaccional**: valida la nueva reserva y solo entonces
  reemplaza la anterior (`replaced`). Si algo falla, se revierte todo y la
  asignación vigente se conserva.
- Toda modificación queda registrada con justificación obligatoria.

## Supuestos explícitos

- La distancia euclidiana es un indicador aproximado del desplazamiento.
- La fecha operativa es fija (2026-10-09) para reproducir la demo.
- Los datos de evaluación son sintéticos.
