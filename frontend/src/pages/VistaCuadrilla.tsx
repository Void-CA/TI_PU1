import { useCallback, useEffect, useState } from 'react'
import { api, hhmm, ORDER_STATUS_LABEL } from '../api'
import type { ScheduleView } from '../types'

export default function VistaCuadrilla({ crewId }: { crewId: number }) {
  const [schedule, setSchedule] = useState<ScheduleView[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [incidencia, setIncidencia] = useState<number | null>(null)
  const [incidenciaTxt, setIncidenciaTxt] = useState('')

  const load = useCallback(() => {
    api.schedule(crewId).then(setSchedule).catch(e => setError(e.message))
  }, [crewId])
  useEffect(load, [load])

  async function setStatus(orderId: number, status: string) {
    setError('')
    try {
      await api.updateOrderStatus(orderId, status)
      setNotice(`Orden #${orderId}: ${ORDER_STATUS_LABEL[status] ?? status}.`)
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function sendIncidencia() {
    if (incidencia == null || !incidenciaTxt.trim()) return
    try {
      await api.updateOrderStatus(incidencia, 'incident')
      setNotice(`Incidencia registrada en la orden #${incidencia}: ${incidenciaTxt}`)
      setIncidencia(null)
      setIncidenciaTxt('')
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  const active = schedule.filter(sv =>
    sv.assignment.status === 'confirmed' || sv.assignment.status === 'in_progress')
  const completed = schedule.filter(sv => sv.order.status === 'completed').length

  return (
    <div>
      <h2>Mi jornada — cuadrilla #{crewId}</h2>
      <p className="hint">
        El personal de cuadrilla consulta su cronograma, registra avances e incidencias.
        No puede modificar sus asignaciones arbitrariamente: la reasignación la decide el
        administrador siguiendo el proceso.
      </p>
      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="notice" role="status">{notice}</div>}

      <div className="kpi-grid">
        <div className="kpi">
          <b>Órdenes de hoy</b>
          <span className="hint">Programadas para la jornada</span>
          <span className="kpi-value">{schedule.length}</span>
        </div>
        <div className="kpi">
          <b>Activas</b>
          <span className="hint">Confirmadas o en curso</span>
          <span className="kpi-value">{active.length}</span>
        </div>
        <div className="kpi">
          <b>Completadas</b>
          <span className="hint">Cerradas en la jornada</span>
          <span className="kpi-value">{completed}</span>
        </div>
      </div>

      <section className="panel">
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr><th scope="col">Horario</th><th scope="col">Orden</th><th scope="col">Cliente</th><th scope="col">Servicio</th><th scope="col">Requisitos</th>
              <th scope="col">Ubicación</th><th scope="col">Estado</th><th scope="col">Acciones</th></tr>
          </thead>
          <tbody>
            {schedule.map(sv => (
              <tr key={sv.assignment.id}>
                <td>{hhmm(sv.assignment.start)}–{hhmm(sv.assignment.end)}</td>
                <td className="id">#{sv.order.id}</td>
                <td>{sv.order.customer}</td>
                <td>{sv.order.service_type}</td>
                <td>{sv.order.requirements.join(', ')}</td>
                <td>({sv.order.location_x}, {sv.order.location_y})</td>
                <td><span className={`badge os-${sv.order.status}`}>{ORDER_STATUS_LABEL[sv.order.status] ?? sv.order.status}</span></td>
                <td>
                  {sv.order.status === 'assigned' && (
                    <button className="small primary" onClick={() => setStatus(sv.order.id, 'in_progress')}>
                      Iniciar
                    </button>
                  )}
                  {sv.order.status === 'in_progress' && (
                    <>
                      <button className="small primary" onClick={() => setStatus(sv.order.id, 'completed')}>
                        Completar
                      </button>
                      <button className="small danger" onClick={() => setIncidencia(sv.order.id)}>
                        Incidencia…
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {active.length === 0 && schedule.length === 0 && (
              <tr><td colSpan={8} className="hint">Sin órdenes asignadas para hoy.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      </section>

      {incidencia != null && (
        <div className="modal-backdrop" onClick={() => setIncidencia(null)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="incidencia-title"
            onClick={e => e.stopPropagation()}>
            <h3 id="incidencia-title">Registrar incidencia — orden #{incidencia}</h3>
            <label>
              Descripción
              <textarea value={incidenciaTxt} onChange={e => setIncidenciaTxt(e.target.value)}
                rows={3} placeholder="Ej.: acceso bloqueado, material insuficiente…" autoFocus />
            </label>
            <div className="actions">
              <button className="primary" onClick={sendIncidencia} disabled={!incidenciaTxt.trim()}>
                Registrar
              </button>
              <button onClick={() => setIncidencia(null)}>Cancelar</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
