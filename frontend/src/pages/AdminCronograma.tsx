import { useCallback, useEffect, useState } from 'react'
import { api, hhmm, ASSIGNMENT_STATUS_LABEL, ORDER_STATUS_LABEL } from '../api'
import type { CrewView, ScheduleView } from '../types'

export default function AdminCronograma() {
  const [crews, setCrews] = useState<CrewView[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [schedule, setSchedule] = useState<ScheduleView[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [reassigning, setReassigning] = useState<{ assignmentId: number; crewNames: string } | null>(null)
  const [targetCrew, setTargetCrew] = useState<number>(1)
  const [justification, setJustification] = useState('')

  const loadCrews = useCallback(() => {
    api.listCrews().then(views => {
      setCrews(views)
      setSelected(prev => prev ?? views[0]?.crew.id ?? null)
    }).catch(e => setError(e.message))
  }, [])

  const loadSchedule = useCallback(() => {
    if (selected == null) return
    api.schedule(selected).then(setSchedule).catch(e => setError(e.message))
  }, [selected])

  useEffect(loadCrews, [loadCrews])
  useEffect(loadSchedule, [loadSchedule])

  async function doReassign() {
    if (!reassigning) return
    setError('')
    try {
      await api.reassign(reassigning.assignmentId, targetCrew, justification)
      setNotice(`Asignación #${reassigning.assignmentId} reasignada a la cuadrilla #${targetCrew}.`)
      setReassigning(null)
      setJustification('')
      loadSchedule()
      loadCrews()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function doCancel(assignmentId: number) {
    const just = window.prompt('Justificación para cancelar la asignación:')
    if (!just) return
    setError('')
    try {
      await api.cancelAssignment(assignmentId, just)
      setNotice(`Asignación #${assignmentId} cancelada; la orden volvió a pendiente.`)
      loadSchedule()
      loadCrews()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  const crewNames = crews.map(c => `#${c.crew.id} ${c.crew.name}`).join(', ')
  const selectedCrew = crews.find(cv => cv.crew.id === selected)?.crew

  return (
    <div>
      <h2>Cronograma y carga</h2>
      <p className="hint">
        Distribución del día operativo. La reasignación es transaccional: si la nueva reserva no es
        posible, la asignación vigente se conserva. Toda modificación queda registrada y justificada.
      </p>
      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="notice" role="status">{notice}</div>}

      <div className="crew-strip">
        {crews.map(cv => (
          <button key={cv.crew.id}
            className={`crew-card ${selected === cv.crew.id ? 'selected' : ''}`}
            onClick={() => setSelected(cv.crew.id)}>
            <b>{cv.crew.name}</b>
            <span className="hint">{cv.crew.zone} · {cv.crew.skills.join('/')}</span>
            <span>Carga: <b>{cv.load_hours.toFixed(2)} h</b></span>
            <div className="load-bar">
              <div className="load-fill" style={{ width: `${Math.min(100, (cv.load_hours / 9) * 100)}%` }} />
            </div>
          </button>
        ))}
      </div>

      {selected != null && (
        <section className="panel">
          <h3>Cronograma · {selectedCrew?.name ?? `#${selected}`}</h3>
          <div className="table-wrap">
          <table className="table">
            <thead>
              <tr><th scope="col">Inicio</th><th scope="col">Fin</th><th scope="col">Orden</th><th scope="col">Cliente</th><th scope="col">Requisitos</th>
                <th scope="col">Estado orden</th><th scope="col">Estado asign.</th><th scope="col">Justificación</th><th scope="col"><span className="sr-only">Acciones</span></th></tr>
            </thead>
            <tbody>
              {schedule.map(sv => (
                <tr key={sv.assignment.id}>
                  <td>{hhmm(sv.assignment.start)}</td>
                  <td>{hhmm(sv.assignment.end)}</td>
                  <td className="id">#{sv.order.id}</td>
                  <td>{sv.order.customer}</td>
                  <td>{sv.order.requirements.join(', ')}</td>
                  <td><span className={`badge os-${sv.order.status}`}>{ORDER_STATUS_LABEL[sv.order.status] ?? sv.order.status}</span></td>
                  <td><span className={`badge as-${sv.assignment.status}`}>{ASSIGNMENT_STATUS_LABEL[sv.assignment.status] ?? sv.assignment.status}</span></td>
                  <td className="hint">{sv.assignment.justification}</td>
                  <td>
                    {(sv.assignment.status === 'confirmed' || sv.assignment.status === 'in_progress') && (
                      <>
                        <button className="small" title={crewNames}
                          onClick={() => { setReassigning({ assignmentId: sv.assignment.id, crewNames }); setJustification('') }}>
                          Reasignar…
                        </button>
                        <button className="small danger" onClick={() => doCancel(sv.assignment.id)}>Cancelar</button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
              {schedule.length === 0 && (
                <tr><td colSpan={9} className="hint">Sin asignaciones para esta cuadrilla.</td></tr>
              )}
            </tbody>
          </table>
          </div>
        </section>
      )}

      {reassigning && (
        <div className="modal-backdrop" onClick={() => setReassigning(null)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="reassign-title"
            onClick={e => e.stopPropagation()}>
            <h3 id="reassign-title">Reasignar asignación #{reassigning.assignmentId}</h3>
            <p className="hint">Cuadrillas disponibles: {reassigning.crewNames}</p>
            <label>
              Nueva cuadrilla
              <select value={targetCrew} onChange={e => setTargetCrew(Number(e.target.value))}>
                {crews.map(cv => (
                  <option key={cv.crew.id} value={cv.crew.id}>
                    #{cv.crew.id} {cv.crew.name} ({cv.crew.skills.join('/')}, carga {cv.load_hours.toFixed(1)} h)
                  </option>
                ))}
              </select>
            </label>
            <label>
              Justificación (obligatoria)
              <input value={justification} onChange={e => setJustification(e.target.value)}
                placeholder="Ej.: menor desplazamiento para la siguiente orden" autoFocus />
            </label>
            <div className="actions">
              <button className="primary" onClick={doReassign} disabled={!justification.trim()}>Confirmar reasignación</button>
              <button onClick={() => setReassigning(null)}>Cancelar</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
