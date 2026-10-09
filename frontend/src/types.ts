// API DTOs mirroring the Go backend JSON responses.

export interface Crew {
  id: number
  name: string
  members: string[]
  skills: string[]
  zone: string
  base_x: number
  base_y: number
  available_from_min: number
  available_to_min: number
  active: boolean
}

export interface CrewView {
  crew: Crew
  load_hours: number
  busy: { start: string; end: string }[]
  last_x: number
  last_y: number
}

export interface Request {
  id: number
  customer: string
  service_type: string
  description: string
  priority: number
  location_x: number
  location_y: number
  window_start: string
  window_end: string
  duration_min: number
  status: string
  created_at: string
}

export interface WorkOrder {
  id: number
  request_id: number
  requirements: string[]
  duration_min: number
  status: string
  created_at: string
  customer: string
  priority: number
  location_x: number
  location_y: number
  window_start: string
  window_end: string
  service_type: string
}

export interface RequestView {
  request: Request
  order?: WorkOrder
}

export interface Candidate {
  crew_id: number
  name: string
  start: string
  end: string
  feasible: boolean
  reasons?: string[]
  distance: number
  wait_min: number
  load_hours: number
  mean_load: number
  cost: number
}

export interface Assignment {
  id: number
  order_id: number
  crew_id: number
  start: string
  end: string
  status: string
  justification: string
  created_at: string
}

export interface ScheduleView {
  assignment: Assignment
  order: WorkOrder
}

export interface PlanResult {
  order_id: number
  crew_id?: number
  crew?: string
  start?: string
  end?: string
  cost?: number
  assigned: boolean
  reason?: string
  retries?: number
}

export interface PlanResponse {
  orders_total: number
  orders_assigned: number
  results: PlanResult[]
}

export interface DemoResult {
  crew_id: number
  window_start: string
  window_end: string
  attempts: number
  succeeded: number
  rejected: number
  order_ids: number[]
  errors: string[]
}

export interface MethodResult {
  method: string
  label: string
  assigned: number
  unassigned: number
  valid_percent: number
  total_distance: number
  total_wait_min: number
  conflicts: number
  hours_before: Record<string, number>
  hours_after: Record<string, number>
  unassigned_order_ids: number[]
}

export interface Evaluation {
  orders_total: number
  day: string
  weights: { w_d: number; w_t: number; w_l: number }
  base: MethodResult
  proposed: MethodResult
  assumptions: string[]
  crews: string[]
}

export interface AppConfig {
  operating_day: string
  weights: { w_d: number; w_t: number; w_l: number }
  service_types: string[]
  roles_note: string
}
