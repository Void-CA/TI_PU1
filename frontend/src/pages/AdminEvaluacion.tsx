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
        Ambos métodos procesan el mismo conjunto fijo de órdenes y cuadrillas, con las mismas
        restricciones operativas. El comparativo es determinista y reproducible.
      </p>
      <div className="actions">
        <button className="primary" onClick={run} disabled={busy}>
          {busy ? 'Ejecutando…' : 'Ejecutar comparación'}
        </button>
      </div>
      {error && <div className="error" role="alert">{error}</div>}

      {ev && (
        <>
          <div className="grid cols-2">
            <section className="panel method-card">
              <header>
                <h3>{ev.base.label}</h3>
                <span className="method-tag">Base</span>
              </header>
              <div className="stat-row">
                <span className="stat-label">Órdenes asignadas válidamente</span>
                <span className="stat-value">{ev.base.assigned} / {ev.orders_total} ({ev.base.valid_percent.toFixed(1)}%)</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Distancia total de desplazamiento (aprox.)</span>
                <span className="stat-value">{ev.base.total_distance.toFixed(2)}</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Conflictos de horario</span>
                <span className="stat-value">{ev.base.conflicts}</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Espera total del cliente</span>
                <span className="stat-value">{ev.base.total_wait_min.toFixed(0)} min</span>
              </div>
            </section>

            <section className="panel method-card method-card--proposed">
              <header>
                <h3>{ev.proposed.label}</h3>
                <span className="method-tag">Propuesto</span>
              </header>
              <div className="stat-row">
                <span className="stat-label">Órdenes asignadas válidamente</span>
                <span className="stat-value">{ev.proposed.assigned} / {ev.orders_total} ({ev.proposed.valid_percent.toFixed(1)}%)</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Distancia total de desplazamiento (aprox.)</span>
                <span className="stat-value better">{ev.proposed.total_distance.toFixed(2)}</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Conflictos de horario</span>
                <span className="stat-value">{ev.proposed.conflicts}</span>
              </div>
              <div className="stat-row">
                <span className="stat-label">Espera total del cliente</span>
                <span className="stat-value">{ev.proposed.total_wait_min.toFixed(0)} min</span>
              </div>
            </section>
          </div>

          <h3>Distribución de horas por cuadrilla</h3>
          <div className="table-wrap">
          <table className="table">
            <thead>
              <tr><th scope="col">Cuadrilla</th><th scope="col">Antes</th><th scope="col">Después (base)</th><th scope="col">Después (propuesto)</th></tr>
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
          </div>

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
