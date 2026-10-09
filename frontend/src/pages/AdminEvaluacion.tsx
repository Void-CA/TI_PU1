import { useState } from 'react'
import { api } from '../api'
import type { Evaluation } from '../types'

export default function AdminEvaluacion() {
  const [ev, setEv] = useState<Evaluation | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function run() {
    setBusy(true); setError('')
    try {
      setEv(await api.evaluation())
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <h2>Evaluación: método base vs. método propuesto</h2>
      <p className="hint">
        Ambos métodos procesan el mismo conjunto sintético fijo (12 órdenes, 5 cuadrillas, con
        carga inicial). El comparativo es determinista y reproducible.
      </p>
      <div className="actions">
        <button className="primary" onClick={run} disabled={busy}>
          {busy ? 'Ejecutando…' : 'Ejecutar comparación'}
        </button>
      </div>
      {error && <div className="error">{error}</div>}

      {ev && (
        <>
          <table className="table">
            <thead>
              <tr>
                <th>Indicador</th>
                <th>{ev.base.label}</th>
                <th>{ev.proposed.label}</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Órdenes asignadas válidamente</td>
                <td>{ev.base.assigned} / {ev.orders_total} ({ev.base.valid_percent.toFixed(1)}%)</td>
                <td>{ev.proposed.assigned} / {ev.orders_total} ({ev.proposed.valid_percent.toFixed(1)}%)</td>
              </tr>
              <tr>
                <td>Distancia total de desplazamiento (aprox.)</td>
                <td>{ev.base.total_distance.toFixed(2)}</td>
                <td className="better">{ev.proposed.total_distance.toFixed(2)}</td>
              </tr>
              <tr>
                <td>Conflicto de horario</td>
                <td>{ev.base.conflicts}</td>
                <td>{ev.proposed.conflicts}</td>
              </tr>
              <tr>
                <td>Espera total del cliente (min)</td>
                <td>{ev.base.total_wait_min.toFixed(0)}</td>
                <td>{ev.proposed.total_wait_min.toFixed(0)}</td>
              </tr>
            </tbody>
          </table>

          <h3>Distribución de horas por cuadrilla</h3>
          <table className="table">
            <thead>
              <tr><th>Cuadrilla</th><th>Antes</th><th>Después (base)</th><th>Después (propuesto)</th></tr>
            </thead>
            <tbody>
              {ev.crews.map(name => (
                <tr key={name}>
                  <td>{name}</td>
                  <td>{(ev.base.hours_before[name] ?? 0).toFixed(2)} h</td>
                  <td>{(ev.base.hours_after[name] ?? 0).toFixed(2)} h</td>
                  <td>{(ev.proposed.hours_after[name] ?? 0).toFixed(2)} h</td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>Supuestos del experimento</h3>
          <ul className="assumptions">
            {ev.assumptions.map((a, i) => <li key={i}>{a}</li>)}
          </ul>
          <p className="hint">
            Pesos utilizados: w_d={ev.weights.w_d}, w_t={ev.weights.w_t}, w_l={ev.weights.w_l}.
            No todos los indicadores mejoran a la vez: la comparación permite observar las
            compensaciones entre desplazamiento y distribución de carga.
          </p>
        </>
      )}
    </div>
  )
}
