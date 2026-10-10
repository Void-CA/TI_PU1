import { useCallback, useEffect, useState } from 'react'
import {
  api, hhmm, PRIORITY_LABEL, REASON_LABEL, ORDER_STATUS_LABEL,
} from '../api'
import type { Candidate, DemoResult, PlanResponse, WorkOrder } from '../types'

export default function AdminTablero() {
  const [orders, setOrders] = useState<WorkOrder[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [candidates, setCandidates] = useState<{ order: WorkOrder; list: Candidate[] } | null>(null)
  const [planResult, setPlanResult] = useState<PlanResponse | null>(null)
  const [demo, setDemo] = useState<DemoResult | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.listOrders().then(setOrders).catch(e => setError(e.message))
  }, [])
  useEffect(load, [load])

  const pending = orders.filter(o => o.status === 'pending')
  const kpi = {
    pending: pending.length,
    assigned: orders.filter(o => o.status === 'assigned').length,
    progress: orders.filter(o => o.status === 'in_progress').length,
    incident: orders.filter(o => o.status === 'incident').length,
  }

  async function openCandidates(order: WorkOrder) {
    setError('')
    try {
      const list = await api.candidates(order.id)
      setCandidates({ order, list })
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function confirm(orderId: number, crewId: number) {
    setError('')
    try {
      await api.confirm(orderId, crewId, 'asignación desde el tablero')
      setCandidates(null)
      setNotice(`Orden #${orderId} confirmada.`)
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function runPlan() {
    setBusy(true); setError(''); setNotice('')
    try {
      const res = await api.plan()
      setPlanResult(res)
      setNotice(`Planificación: ${res.orders_assigned}/${res.orders_total} órdenes asignadas.`)
      load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function runDemo() {
    setBusy(true); setError(''); setNotice('')
    try {
      const res = await api.demoConcurrency(3, 3)
      setDemo(res)
      load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <h2>Tablero de órdenes</h2>
      <p className="hint">
        Órdenes pendientes de asignar. El motor evalúa factibilidad (competencias, disponibilidad,
        solapamiento) y, solo entre las factibles, calcula el costo D·w_d + W·w_t + L·w_l.
      </p>

      <div className="kpi-grid">
        <div className="kpi">
          <b>Pendientes</b>
          <span className="hint">Esperando planificación</span>
          <span className="kpi-value">{kpi.pending}</span>
        </div>
        <div className="kpi">
          <b>Asignadas</b>
          <span className="hint">Confirmadas hoy</span>
          <span className="kpi-value">{kpi.assigned}</span>
        </div>
        <div className="kpi">
          <b>En curso</b>
          <span className="hint">En ejecución</span>
          <span className="kpi-value">{kpi.progress}</span>
        </div>
        <div className="kpi">
          <b>Incidencias</b>
          <span className="hint">Requieren atención</span>
          <span className="kpi-value">{kpi.incident}</span>
        </div>
      </div>

      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="notice" role="status">{notice}</div>}

      <div className="grid cols-board">
        <section className="panel">
          <h3>Órdenes pendientes ({pending.length})</h3>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr><th scope="col">#</th><th scope="col">Cliente</th><th scope="col">Servicio</th><th scope="col">Requisitos</th><th scope="col">Prioridad</th>
                  <th scope="col">Ventana</th><th scope="col">Dur.</th><th scope="col"><span className="sr-only">Acciones</span></th></tr>
              </thead>
              <tbody>
                {pending.map(o => (
                  <tr key={o.id}>
                    <td className="id">{o.id}</td>
                    <td>{o.customer}</td>
                    <td>{o.service_type}</td>
                    <td>{o.requirements.join(', ')}</td>
                    <td>{PRIORITY_LABEL[o.priority]}</td>
                    <td>{hhmm(o.window_start)}–{hhmm(o.window_end)}</td>
                    <td>{o.duration_min} min</td>
                    <td><button className="small primary" onClick={() => openCandidates(o)}>Asignar…</button></td>
                  </tr>
                ))}
                {pending.length === 0 && (
                  <tr><td colSpan={8} className="hint">No hay órdenes pendientes.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel">
          <h3>Planificación</h3>
          <div className="actions">
            <button className="primary" onClick={runPlan} disabled={busy || pending.length === 0}>
              Planificar jornada (lote)
            </button>
            <button onClick={runDemo} disabled={busy}>
              Probar concurrencia (3 confirmaciones simultáneas)
            </button>
          </div>

          {planResult && (
            <>
              <h3>Resultado de la planificación</h3>
              <div className="table-wrap">
                <table className="table">
                  <thead><tr><th scope="col">Orden</th><th scope="col">Resultado</th><th scope="col">Cuadrilla</th><th scope="col">Horario</th><th scope="col">Costo</th><th scope="col">Reintentos</th></tr></thead>
                  <tbody>
                    {planResult.results.map(r => (
                      <tr key={r.order_id}>
                        <td className="id">#{r.order_id}</td>
                        <td>{r.assigned
                          ? <span className="badge ok">Asignada</span>
                          : <span className="badge bad">{r.reason}</span>}</td>
                        <td>{r.crew ?? '—'}</td>
                        <td>{r.start && r.end ? `${hhmm(r.start)}–${hhmm(r.end)}` : '—'}</td>
                        <td>{r.cost != null ? r.cost.toFixed(3) : '—'}</td>
                        <td>{r.retries ?? 0}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {demo && (
            <>
              <h3>Prueba de concurrencia</h3>
              <p>
                Franja <b>{demo.window_start}–{demo.window_end}</b> para la cuadrilla #{demo.crew_id}:
                <b> {demo.succeeded} confirmación exitosa</b> de {demo.attempts} intentos simultáneos
                ({demo.rejected} rechazados con 409).
              </p>
              <p className="hint">
                La base de datos garantiza, con el constraint EXCLUDE, que dos asignaciones
                bloqueantes de la misma cuadrilla nunca solapen, sin importar cuántas confirmaciones
                ocurran a la vez.
              </p>
            </>
          )}

          {!planResult && !demo && (
            <p className="hint">
              Ejecuta la planificación por lote o la prueba de concurrencia para ver el resultado aquí.
            </p>
          )}
        </section>
      </div>

      <h3>Todas las órdenes</h3>
      <div className="table-wrap">
        <table className="table">
          <thead><tr><th scope="col">#</th><th scope="col">Cliente</th><th scope="col">Estado</th><th scope="col">Ventana</th></tr></thead>
          <tbody>
            {orders.map(o => (
              <tr key={o.id}>
                <td className="id">{o.id}</td>
                <td>{o.customer}</td>
                <td><span className={`badge os-${o.status}`}>{ORDER_STATUS_LABEL[o.status] ?? o.status}</span></td>
                <td>{hhmm(o.window_start)}–{hhmm(o.window_end)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {candidates && (
        <div className="modal-backdrop" onClick={() => setCandidates(null)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="candidates-title"
            onClick={e => e.stopPropagation()}>
            <h3 id="candidates-title">Orden #{candidates.order.id} — {candidates.order.customer}</h3>
            <p className="hint">
              Ventana {hhmm(candidates.order.window_start)}–{hhmm(candidates.order.window_end)} ·
              duración {candidates.order.duration_min} min · requisitos {candidates.order.requirements.join(', ')}
            </p>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr><th scope="col">Cuadrilla</th><th scope="col">Factible</th><th scope="col">Horario</th><th scope="col">Distan.</th><th scope="col">Espera</th>
                    <th scope="col">Carga result.</th><th scope="col">Costo</th><th scope="col"><span className="sr-only">Acciones</span></th></tr>
                </thead>
                <tbody>
                  {candidates.list.map(c => (
                    <tr key={c.crew_id} className={c.feasible ? '' : 'row-rejected'}>
                      <td>{c.name}</td>
                      <td>
                        {c.feasible
                          ? <span className="badge ok">Sí</span>
                          : <span className="badge bad">{(c.reasons ?? []).map(r => REASON_LABEL[r] ?? r).join(', ')}</span>}
                      </td>
                      <td>{c.feasible ? `${hhmm(c.start)}–${hhmm(c.end)}` : '—'}</td>
                      <td>{c.feasible ? c.distance.toFixed(2) : '—'}</td>
                      <td>{c.feasible ? `${c.wait_min.toFixed(0)} min` : '—'}</td>
                      <td>{c.feasible ? `${c.load_hours.toFixed(2)} h` : '—'}</td>
                      <td>{c.feasible ? c.cost.toFixed(3) : '—'}</td>
                      <td>
                        {c.feasible && (
                          <button className="small primary"
                            onClick={() => confirm(candidates.order.id, c.crew_id)}>
                            Asignar
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <button onClick={() => setCandidates(null)}>Cerrar</button>
          </div>
        </div>
      )}
    </div>
  )
}
