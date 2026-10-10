import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { api } from './api'
import type { Crew } from './types'
import AdminSolicitudes from './pages/AdminSolicitudes'
import AdminTablero from './pages/AdminTablero'
import AdminCronograma from './pages/AdminCronograma'
import AdminEvaluacion from './pages/AdminEvaluacion'
import VistaCuadrilla from './pages/VistaCuadrilla'

export default function App() {
  const [role, setRole] = useState(() => localStorage.getItem('pu1-role') || 'admin')
  const [crews, setCrews] = useState<Crew[]>([])

  useEffect(() => {
    api.listCrews().then(views => setCrews(views.map(v => v.crew))).catch(console.error)
  }, [])

  const isAdmin = role === 'admin'
  const crewId = isAdmin ? null : Number(role.split(':')[1])

  function changeRole(next: string) {
    localStorage.setItem('pu1-role', next)
    setRole(next)
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-head">
          <div className="brand">
            <span className="brand-mark">PU1</span>
            <span className="brand-title">Asignación de órdenes de servicio</span>
          </div>
          <div className="role-box">
            <label htmlFor="role">Rol</label>
            <select id="role" value={role} onChange={e => changeRole(e.target.value)}>
              <option value="admin">Administrador de operaciones</option>
              {crews.map(c => (
                <option key={c.id} value={`crew:${c.id}`}>Cuadrilla {c.name}</option>
              ))}
            </select>
          </div>
        </div>

        <nav className="nav">
          {isAdmin ? (
            <>
              <NavLink to="/solicitudes">Solicitudes</NavLink>
              <NavLink to="/tablero">Tablero</NavLink>
              <NavLink to="/cronograma">Cronograma</NavLink>
              <NavLink to="/evaluacion">Evaluación</NavLink>
            </>
          ) : (
            <NavLink to="/cuadrilla">Mi jornada</NavLink>
          )}
        </nav>
      </aside>

      <div className="shell-main">
        <main className="content">
          <Routes>
            {isAdmin ? (
              <>
                <Route path="/solicitudes" element={<AdminSolicitudes />} />
                <Route path="/tablero" element={<AdminTablero />} />
                <Route path="/cronograma" element={<AdminCronograma />} />
                <Route path="/evaluacion" element={<AdminEvaluacion />} />
                <Route path="*" element={<Navigate to="/solicitudes" replace />} />
              </>
            ) : (
              <>
                <Route path="/cuadrilla" element={<VistaCuadrilla crewId={crewId!} />} />
                <Route path="*" element={<Navigate to="/cuadrilla" replace />} />
              </>
            )}
          </Routes>
        </main>

        <footer className="footer">
          PU1 · Plataforma de asignación de órdenes de servicio · © 2026
        </footer>
      </div>
    </div>
  )
}
