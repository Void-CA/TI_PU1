import { useCallback, useEffect, useState } from 'react'
import { api, hhmm, PRIORITY_LABEL, REQUEST_STATUS_LABEL } from '../api'
import type { RequestView } from '../types'

const SERVICE_TYPES = ['fiber', 'router', 'splicing', 'cabling', 'coax', 'fiber_splice']

export default function AdminSolicitudes() {
  const [requests, setRequests] = useState<RequestView[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const [form, setForm] = useState({
    customer: '', service_type: 'fiber', priority: 2,
    location_x: 5, location_y: 5, window_start: '09:00', window_end: '12:00', duration_min: 60,
  })

  const load = useCallback(() => {
    api.listRequests().then(setRequests).catch(e => setError(e.message))
  }, [])
  useEffect(load, [load])

  async function create(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.createRequest({
        ...form,
        location_x: Number(form.location_x),
        location_y: Number(form.location_y),
        duration_min: Number(form.duration_min),
        priority: Number(form.priority),
      })
      setForm({ ...form, customer: '' })
      load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function evaluate(id: number, remote: boolean) {
    setError('')
    try {
      await api.evaluateRequest(id, remote)
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div>
      <h2>Solicitudes de servicio</h2>
      <p className="hint">
        Paso 1–3 del proceso: el centro de atención recibe la solicitud y determina si se resuelve
        en remoto o requiere una orden de trabajo (intervención presencial).
      </p>
      {error && <div className="error" role="alert">{error}</div>}

      <div className="grid cols-form">
      <form className="panel form-grid" onSubmit={create}>
        <h3>Nueva solicitud</h3>
        <label>
          Cliente
          <input value={form.customer} onChange={e => setForm({ ...form, customer: e.target.value })} required />
        </label>
        <label>
          Tipo de servicio
          <select value={form.service_type} onChange={e => setForm({ ...form, service_type: e.target.value })}>
            {SERVICE_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
          </select>
        </label>
        <label>
          Prioridad
          <select value={form.priority} onChange={e => setForm({ ...form, priority: Number(e.target.value) })}>
            <option value={1}>Baja</option>
            <option value={2}>Media</option>
            <option value={3}>Alta</option>
          </select>
        </label>
        <label>
          X (malla 0–10)
          <input type="number" min={0} max={10} step={0.5} value={form.location_x}
            onChange={e => setForm({ ...form, location_x: Number(e.target.value) })} />
        </label>
        <label>
          Y (malla 0–10)
          <input type="number" min={0} max={10} step={0.5} value={form.location_y}
            onChange={e => setForm({ ...form, location_y: Number(e.target.value) })} />
        </label>
        <label>
          Ventana inicio (HH:MM)
          <input value={form.window_start} onChange={e => setForm({ ...form, window_start: e.target.value })} />
        </label>
        <label>
          Ventana fin (HH:MM)
          <input value={form.window_end} onChange={e => setForm({ ...form, window_end: e.target.value })} />
        </label>
        <label>
          Duración (min)
          <input type="number" min={15} step={15} value={form.duration_min}
            onChange={e => setForm({ ...form, duration_min: Number(e.target.value) })} />
        </label>
        <button type="submit" disabled={busy || !form.customer}>Registrar solicitud</button>
      </form>

      <section className="panel">
      <h3>Solicitudes registradas</h3>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th scope="col">#</th><th scope="col">Cliente</th><th scope="col">Servicio</th><th scope="col">Prioridad</th>
              <th scope="col">Ubicación</th><th scope="col">Ventana</th><th scope="col">Dur.</th><th scope="col">Estado</th><th scope="col">Acciones</th>
            </tr>
          </thead>
          <tbody>
            {requests.map(rv => (
              <tr key={rv.request.id}>
                <td className="id">{rv.request.id}</td>
                <td>{rv.request.customer}</td>
                <td>{rv.request.service_type}</td>
                <td>{PRIORITY_LABEL[rv.request.priority]}</td>
                <td>({rv.request.location_x}, {rv.request.location_y})</td>
                <td>{hhmm(rv.request.window_start)}–{hhmm(rv.request.window_end)}</td>
                <td>{rv.request.duration_min} min</td>
                <td><span className={`badge st-${rv.request.status}`}>{REQUEST_STATUS_LABEL[rv.request.status] ?? rv.request.status}</span></td>
                <td>
                  {rv.request.status === 'received' && (
                    <>
                      <button className="small" onClick={() => evaluate(rv.request.id, true)}>Resuelta en remoto</button>
                      <button className="small primary" onClick={() => evaluate(rv.request.id, false)}>Generar orden</button>
                    </>
                  )}
                  {rv.order && <span className="hint"> orden #{rv.order.id} ({rv.order.status})</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      </section>
      </div>
    </div>
  )
}
