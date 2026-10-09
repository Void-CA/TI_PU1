# Entregable 1 — Análisis organizacional

> Caso de estudio hipotético basado en un proceso plausible de atención de servicios
> de telecomunicaciones. No se afirma que ninguna empresa real opere exactamente
> así: es una suposición razonable, no validada.

## Proceso actual supuesto

1. El centro de atención recibe la solicitud del cliente y recopila información.
2. Se determina si el problema puede resolverse remotamente.
3. Si requiere intervención presencial, se crea una orden de trabajo.
4. Se selecciona una cuadrilla compatible con el trabajo.
5. Se asigna una fecha y una ventana horaria.
6. El técnico consulta su cronograma, atiende la orden y registra el resultado.

El problema del proyecto se concentra en la **transición entre los pasos 2 y 4**:
decidir quién atiende cada solicitud, cuándo y bajo qué condiciones.

## Actores y decisiones

| Actor | Decisiones que toma |
|---|---|
| Centro de atención | Clasifica la solicitud; decide remoto vs. presencial |
| Administrador de operaciones | Selecciona la cuadrilla; asigna/reasigna con justificación; prioriza |
| Cuadrilla técnica | Ejecuta la orden; registra avances, incidencias y finalización |
| Coordinación (implícita) | Observa carga y cumples ventanas para reequilibrar |

## Problemas potenciales de la asignación manual

- Desplazamientos innecesarios (elegir por cercanía sin considerar el resto del día).
- Sobrecarga de algunas cuadrillas y ociosidad de otras.
- Tiempos de espera elevados por decisión localmente razonable pero globalmente mala.
- Conflictos de horario cuando varias solicitudes se asignan sin coordinación.
- Dificultad para justificar por qué una cuadrilla y no otra.

## Limitaciones de la información disponible

- La ubicación real de las cuadrillas rara vez está actualizada.
- El tiempo de desplazamiento depende del tráfico y no se conoce con precisión.
- Las competencias y recursos disponibles cambian durante la jornada.
- No existe un registro único de cargas acumuladas por cuadrilla.

## Justificación del prototipo

Un sistema de información puede formalizar estas decisiones: filtrar asignaciones
inválidas (factibilidad), ordenar las válidas por criterios explícitos (eficiencia y
equidad) y registrar cada decisión con su justificación, con la base de datos como
autoridad sobre la disponibilidad. La comparación entre un método base y el método
propuesto permite discutir con evidencia si el proceso mejora.
