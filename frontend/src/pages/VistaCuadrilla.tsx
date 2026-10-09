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

  return (
    <div>
      <h2>Mi jornada — cuadrilla #{crewId}</h2>
      <p className="hint">
        El personal de cuadrilla consulta su cronograma, registra avances e incidecias.
        No puede modificar sus asignaciones arbitrariamente: la reasignación la decide el
        administrador siguiendo el proceso.
      </p>
      {error && <div className="error">{error}</div>}
      {notice && <div className="notice">{notice}</div>}

      <table className="table">
        <thead>
          <tr><th>Horario</th><th>Orden</th><th>Cliente</th><th>Servicio</th><th>Requisitos</th>
            <th>Ubicación</th><th>Estado</th><th>Acciones</th></tr>
        </thead>
        <tbody>
          {schedule.map(sv => (
            <tr key={sv.assignment.id}>
              <td>{hhmm(sv.assignment.start)}–{hhmm(sv.assignment.end)}</td>
              <td>#{sv.order.id}</td>
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

      {incidencia != null && (
        <div className="modal-backdrop" onClick={() => setIncidencia(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h3>Registrar incidencia — orden #{incidencia}</h3>
            <label>
              Descripción
              <textarea value={incidenciaTxt} onChange={e => setIncidenciaTxt(e.target.value)}
                rows={3} placeholder="Ej.: acceso bloqueado, material insuficiente…" />
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
