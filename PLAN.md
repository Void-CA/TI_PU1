# PLAN.md — Optimización de la asignación de órdenes de servicio

> **Nota de ejecución.** Este documento es un plan de referencia para orientar al equipo. No es una especificación contractual ni una arquitectura definitiva. Su propósito es ayudar a decidir qué implementar y, sobre todo, qué dejar fuera. No hay cronograma formal: basta con seguir el orden de los entregables.

---

## Pregunta central

> ¿Cómo puede un sistema de información mejorar la asignación de órdenes de servicio a cuadrillas técnicas, considerando las restricciones operativas y buscando reducir los desplazamientos y distribuir la carga de trabajo de manera equilibrada?

El proyecto se presenta como **una propuesta para optimizar el proceso de atención de solicitudes de servicio mediante la asignación coordinada de recursos remotos y técnicos de campo**, no como una aplicación para asignar cuadrillas. La segunda formulación es más potente desde la perspectiva organizacional de la asignatura.

## Caso de estudio

Caso hipotético basado en un proceso plausible de atención de servicios de telecomunicaciones (ilustrado con una empresa como Tigo). **No se afirma que Tigo utilice exactamente el proceso modelado**: es un caso de estudio hipotético, y declararlo así es metodológicamente honesto.

El proceso supuesto:

1. El centro de atención recibe la solicitud y recopila información.
2. Se determina si el problema puede resolverse remotamente.
3. Si requiere intervención presencial, se crea una orden de trabajo.
4. Se selecciona una cuadrilla compatible con el trabajo.
5. Se asigna una fecha y una ventana horaria.
6. El técnico consulta su cronograma, atiende la orden y registra el resultado.

El problema del proyecto está en la **transición entre los pasos 2 y 4**: cómo decidir quién debe atender cada solicitud, cuándo y bajo qué condiciones. La asignación manual puede generar desplazamientos innecesarios, sobrecarga de algunas cuadrillas, tiempos de espera elevados y una distribución poco equilibrada del trabajo. Esa es la justificación organizacional concreta.

## Alcance

- **Dominio único: telecomunicaciones.** Riego, ganado, drones y otros dominios se presentan como posibles aplicaciones futuras del enfoque, nunca como objetivos de implementación. Cada dominio tiene restricciones particulares.
- **Separación de capas:** las reglas de negocio se mantienen separadas del transporte HTTP y de la persistencia.
- **Los criterios de asignación son configurables** solo cuando realmente sea útil, no como plataforma universal.
- **No se construye** un sistema completo de atención al cliente: la recepción de solicitudes se simula con un formulario o datos de prueba. El centro de atención es parte del proceso analizado, no un tercer módulo a desarrollar.
- **Sin microservicios, colas distribuidas ni Kubernetes.** Para este alcance añadirían trabajo sin demostrar mejora del proceso.
- Un solo frontend con **dos experiencias de usuario según rol** (administrador / cuadrilla), no dos aplicaciones independientes.

## Supuestos declarados

- El proceso actual descrito es una suposición razonable, no validada con una empresa real.
- La distancia euclidiana es un **indicador aproximado** del costo de desplazamiento, no una representación del tiempo real de viaje (calles, tráfico y barreras geográficas no se modelan en la primera versión).
- Los datos de evaluación son sintéticos: demuestran que el algoritmo funciona bajo los supuestos definidos, no que una empresa real obtendría la misma mejora.
- Las entidades propuestas son un punto de partida para discutir y reducir según el alcance real del trabajo.

---

## Modo de asignación: híbrido

Decisión de diseño asumida:

- **Planificación inicial** de las órdenes conocidas de la jornada.
- **Reasignación controlada** cuando aparezca una solicitud urgente o cambie la disponibilidad de una cuadrilla.

Regla de reasignación: la planificación inicial considera las órdenes conocidas para la jornada. Ante una urgencia o un cambio de disponibilidad, el sistema evalúa alternativas de reasignación, **preservando las asignaciones existentes siempre que sea posible** y respetando las restricciones obligatorias. Toda modificación debe quedar registrada y justificada. «Reasignación» no significa reorganizar arbitrariamente toda la jornada.

Justificación: la asignación puramente reactiva es más simple pero produce decisiones localmente buenas que empeoran el resultado global; la planificada completa se acerca a un problema de optimización logística demasiado ambicioso para el alcance. El híbrido muestra un proceso organizacional completo sin convertir el proyecto en un sistema de optimización de flotas.

---

## Modelo de dominio (propuesta inicial)

> Las cinco entidades siguientes son **propuestas iniciales**. No implica que deban implementarse todas como entidades independientes: son un punto de partida para discutir y reducir según el alcance.

| Entidad | Contenido |
|---|---|
| **Solicitud** | cliente, tipo de servicio, prioridad, ubicación, duración estimada, estado |
| **Cuadrilla** | integrantes, competencias, zona de trabajo, disponibilidad |
| **Orden de trabajo** | solicitud que requiere intervención, con duración y requisitos |
| **Asignación** | relación entre orden y cuadrilla, fecha, horario, estado |
| **Disponibilidad** | intervalos ocupados y libres de cada cuadrilla |

Flujo asociado: solicitud → evaluación de atención (remota o presencial) → motor de asignación (filtrado de restricciones, evaluación de candidatos, selección) → administrador (asigna, revisa, reorganiza) → cuadrilla (consulta cronograma, registra avances).

---

## Reglas de negocio

### Los tres niveles de decisión, en orden estricto

**1. Factibilidad (filtro obligatorio).** ¿Puede esta cuadrilla realizar el trabajo?

- Tiene las competencias técnicas requeridas.
- Está disponible en la ventana horaria.
- No tiene conflicto con otra orden.
- Dispone de los recursos necesarios.

**2. Eficiencia operativa.** ¿Qué asignación permite atender mejor las solicitudes?

- Menor tiempo o distancia de desplazamiento.
- Menor tiempo de espera del cliente.
- Menos viajes innecesarios entre órdenes.
- Mejor utilización de las horas disponibles.

**3. Equidad en la carga.** ¿Se distribuye el trabajo razonablemente?

- Horas de trabajo acumuladas.
- Número y complejidad de las órdenes.
- Carga restante del día.
- Oportunidades de atención equivalentes cuando las condiciones lo permiten.

La distinción es fundamental: una cuadrilla puede estar cerca y no ser válida por falta de especialidad; otra puede ser válida pero terminar el día sobrecargada. **Primero se descartan las asignaciones inválidas y después se optimiza entre las que quedan.** Resolver ambas cosas con una sola puntuación arbitraria produce resultados difíciles de justificar.

### Restricciones temporales frente a costo temporal

Se separan dos conceptos que no deben confundirse:

- **Restricciones temporales** (factibilidad, filtro obligatorio): disponibilidad, duración, ventanas de atención y ausencia de solapamientos. Un conflicto horario **hace inválida la asignación**; nunca es una penalización. De lo contrario, un peso bajo podría permitir seleccionar una cuadrilla que ya tiene otra orden en ese horario.
- **Costo temporal:** tiempo estimado de espera o retraso respecto a la ventana solicitada.

### Función de costo (propuesta inicial)

```
C(q, j) = w_d · D(q, j) + w_t · W(q, j) + w_l · L(q, j)
```

- `D`: distancia o costo de desplazamiento.
- `W`: espera estimada (retraso respecto a la ventana solicitada), **no conflictos de horario**.
- `L`: costo de carga, es decir, el efecto de la nueva orden sobre la distribución del trabajo.
- `w_d, w_t, w_l`: pesos que expresan la importancia relativa de cada criterio.

La función solo se evalúa sobre candidatos ya validados contra las restricciones obligatorias. Queda pendiente de justificar la normalización de cada variable y el significado de cada penalización. **Regla inquebrantable: ninguna puntuación puede violar una restricción obligatoria.** Una heurística sencilla que explique por qué elige una cuadrilla es más defendible en un proyecto académico que un algoritmo sofisticado cuyo comportamiento no se pueda justificar.

### Definición de equidad

Es probablemente la decisión de negocio más importante que justificar. Ejemplo: tres cuadrillas con cargas de 2, 5 y 7 horas; una solicitud nueva de 2 horas. Solo minimizando distancia podría asignarse a la de 7 horas. Priorizando equidad, conviene una de las otras dos. Pero tampoco es correcto enviar una cuadrilla lejana solo para igualar números.

Orden de decisión adoptado:

1. Respetar las restricciones obligatorias.
2. Minimizar el tiempo de desplazamiento y la espera del cliente.
3. Evitar sobrecargar cuadrillas cuando existan alternativas comparables.

### Tratamiento de la distancia

- **Primera versión:** distancia euclidiana sobre coordenadas ficticias o una cuadrícula de zonas. Documentar que es un indicador aproximado.
- **Versión mejorada (opcional):** distancia por carretera o tiempos de viaje estimados, si hay datos adecuados.
- Distinguir siempre la **distancia desde la ubicación actual** de la cuadrilla y la **distancia desde su última orden programada**: si ya tiene tareas, su punto de partida depende de su cronograma.

### Reglas de reasignación

- Una cuadrilla **no modifica arbitrariamente** sus asignaciones: registra avances y solicita cambios.
- La reasignación sigue las reglas definidas por el proceso organizacional y siempre queda **justificada**.

### Concurrencia como requisito demostrable y acotado

La concurrencia no hace el algoritmo mejor ni más correcto; el problema real es que dos solicitudes simultáneas no reserven la misma franja horaria de la misma cuadrilla de manera incompatible. Se separan dos responsabilidades:

1. **Calcular candidatos:** evaluar cuadrillas y costos, potencialmente en paralelo.
2. **Confirmar la asignación:** no basta con calcular y luego guardar, porque el estado puede cambiar entre ambas operaciones. La confirmación debe **verificar la disponibilidad vigente en el momento de confirmar** y garantizar que la reserva sea **atómica**, con la base de datos como autoridad sobre la asignación confirmada. Si otra operación ha ocupado el intervalo, el sistema **rechaza la confirmación** y recalcula o solicita una nueva decisión.

Requisito demostrable del proyecto: **un caso de asignaciones simultáneas que permita comprobar que no se reserva una misma franja horaria de manera incompatible.** No se añaden goroutines sin un caso de uso claro y medible.

---

## Actores y responsabilidades

**Administrador de operaciones**

- Consultar solicitudes pendientes.
- Ver disponibilidad y carga.
- Ejecutar o revisar asignaciones.
- Reasignar órdenes con justificación.
- Consultar el estado general de la operación.

**Personal de cuadrilla**

- Consultar su cronograma.
- Ver detalles de cada orden.
- Actualizar el estado de atención.
- Registrar incidencias o finalización.

---

## Entregables (en orden de prioridad)

1. **Análisis organizacional.** Documentar el proceso actual supuesto, sus actores, sus decisiones, los problemas potenciales y las limitaciones de la información disponible.
2. **Modelo de reglas.** Formalizar restricciones obligatorias, criterios de optimización y reglas de reasignación. Dejar explícitas las suposiciones que no pudieron validarse con la empresa.
3. **Prototipo funcional.** Implementar solicitudes, cuadrillas, asignación, cronograma y seguimiento del trabajo, con el caso concurrente acotado descrito arriba.
4. **Evaluación proporcionada a un proyecto de tres días.** Comparar el método base (primera cuadrilla disponible que cumpla requisitos) y el método propuesto (distancia, carga y disponibilidad temporal) con un **conjunto pequeño de datos sintéticos** y **unos pocos indicadores representativos**. No hace falta diseñar un experimento formal.

No se añaden fechas ni responsables: el orden mismo de los entregables es la priorización.

---

## Evaluación (proporcional al alcance)

Conjunto pequeño de datos sintéticos: pocas cuadrillas con competencias y disponibilidades distintas, algunas solicitudes con prioridades y ubicaciones variadas, algunos casos no asignables. Ejecutar ambos métodos y comparar unos pocos indicadores representativos:

| Indicador | Qué demuestra |
|---|---|
| Porcentaje de órdenes asignadas válidamente | Factibilidad de la planificación |
| Distancia o tiempo total de desplazamiento estimado | Eficiencia operativa |
| Distribución de horas entre cuadrillas | Equilibrio de carga |

Para la equidad no hace falta una métrica sofisticada: basta con **medir las horas asignadas a cada cuadrilla y mostrar su distribución antes y después de aplicar cada método**.

Tres indicadores bastan. No es necesario mejorar todos simultáneamente: mostrar las compensaciones entre distancia y equidad hace el análisis más interesante. Las conclusiones deben ser cuidadosas: el conjunto sintético demuestra que el algoritmo funciona bajo los supuestos definidos, no que una empresa real obtendría la misma mejora.

## Criterios de éxito

- Cualquier integrante del equipo entiende, leyendo este documento: el problema organizacional, las reglas de asignación, los supuestos y cómo se demuestra que la solución funciona.
- El prototipo cubre el flujo completo: solicitud → evaluación → asignación → cronograma → seguimiento.
- El caso concurrente se puede ejecutar y observar que no genera reservas incompatibles.
- La comparación base vs. propuesta es reproducible con los mismos datos de prueba, y la distribución de horas por cuadrilla se muestra antes y después de aplicar cada método.
- El alcance se mantiene: cualquier característica fuera de lo anterior se documenta como futura, no se implementa.
