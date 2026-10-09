import type {
  AppConfig, Assignment, Candidate, CrewView, DemoResult, Evaluation,
  PlanResponse, Request, RequestView, ScheduleView, WorkOrder,
} from './types'

const BASE = '/api'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const role = localStorage.getItem('pu1-role') || 'admin'
  headers['X-Role'] = role
  const res = await fetch(BASE + path, {
    ...init,
    headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
  })
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new ApiError(res.status, body?.error ?? `HTTP ${res.status}`)
  }
  return body as T
}

export const api = {
  config: () => req<AppConfig>('/config'),

  listRequests: () => req<RequestView[]>('/requests'),
  createRequest: (data: {
    customer: string; service_type: string; priority: number
    location_x: number; location_y: number
    window_start: string; window_end: string; duration_min: number
  }) => req<Request>('/requests', { method: 'POST', body: JSON.stringify(data) }),
  evaluateRequest: (id: number, remote: boolean) =>
    req<RequestView>(`/requests/${id}/evaluate`, { method: 'POST', body: JSON.stringify({ remote }) }),

  listOrders: (status?: string) =>
    req<WorkOrder[]>(`/orders${status ? `?status=${status}` : ''}`),
  candidates: (orderId: number) =>
    req<Candidate[]>(`/orders/${orderId}/candidates`, { method: 'POST' }),
  updateOrderStatus: (orderId: number, status: string) =>
    req<WorkOrder>(`/orders/${orderId}/status`, { method: 'POST', body: JSON.stringify({ status }) }),

  listCrews: () => req<CrewView[]>('/crews'),
  schedule: (crewId: number) => req<ScheduleView[]>(`/crews/${crewId}/schedule`),

  confirm: (orderId: number, crewId: number, justification: string) =>
    req<Assignment>('/assignments/confirm', {
      method: 'POST',
      body: JSON.stringify({ order_id: orderId, crew_id: crewId, justification }),
    }),
  reassign: (assignmentId: number, crewId: number, justification: string) =>
    req<Assignment>(`/assignments/${assignmentId}/reassign`, {
      method: 'POST',
      body: JSON.stringify({ crew_id: crewId, justification }),
    }),
  cancelAssignment: (assignmentId: number, justification: string) =>
    req<{ status: string }>(`/assignments/${assignmentId}/cancel`, {
      method: 'POST',
      body: JSON.stringify({ justification }),
    }),

  plan: () => req<PlanResponse>('/plan', { method: 'POST' }),
  demoConcurrency: (crewId: number, attempts: number) =>
    req<DemoResult>('/demo/concurrency', {
      method: 'POST',
      body: JSON.stringify({ crew_id: crewId, attempts }),
    }),
  evaluation: () => req<Evaluation>('/evaluation'),
}

/** Formats an ISO timestamp as HH:MM in UTC (the prototype's wall clock). */
export function hhmm(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return `${String(d.getUTCHours()).padStart(2, '0')}:${String(d.getUTCMinutes()).padStart(2, '0')}`
}

export const PRIORITY_LABEL: Record<number, string> = {
  1: 'Baja',
  2: 'Media',
  3: 'Alta',
}

export const ORDER_STATUS_LABEL: Record<string, string> = {
  pending: 'Pendiente',
  assigned: 'Asignada',
  in_progress: 'En curso',
  completed: 'Completada',
  incident: 'Incidencia',
  cancelled: 'Cancelada',
}

export const REQUEST_STATUS_LABEL: Record<string, string> = {
  received: 'Recibida',
  resolved_remote: 'Resuelta en remoto',
  with_order: 'Con orden',
  cancelled: 'Cancelada',
}

export const ASSIGNMENT_STATUS_LABEL: Record<string, string> = {
  confirmed: 'Confirmada',
  in_progress: 'En curso',
  completed: 'Completada',
  cancelled: 'Cancelada',
  replaced: 'Reemplazada',
}

export const REASON_LABEL: Record<string, string> = {
  crew_inactive: 'Cuadrilla inactiva',
  missing_skill: 'Competencia faltante',
  no_free_slot_in_window: 'Sin hueco en la ventana',
  order_already_assigned: 'Orden ya asignada',
}
