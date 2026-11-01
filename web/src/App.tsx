import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes, Link } from 'react-router-dom'
import { ProtectedRoute } from './auth'
import { Layout } from './components/Layout'
import { Loading } from './components/UI'
import { LoginPage } from './pages/Login'
import { PasswordPage } from './pages/Password'

const TodayPage = lazy(() => import('./pages/Today').then((module) => ({ default: module.TodayPage })))
const SchedulePage = lazy(() => import('./pages/Schedule').then((module) => ({ default: module.SchedulePage })))
const JournalPage = lazy(() => import('./pages/Journal').then((module) => ({ default: module.JournalPage })))
const HomeworkPage = lazy(() => import('./pages/Homework').then((module) => ({ default: module.HomeworkPage })))
const GradesPage = lazy(() => import('./pages/Grades').then((module) => ({ default: module.GradesPage })))
const AnnouncementsPage = lazy(() => import('./pages/Announcements').then((module) => ({ default: module.AnnouncementsPage })))
const ReportsPage = lazy(() => import('./pages/Reports').then((module) => ({ default: module.ReportsPage })))
const AdminPage = lazy(() => import('./pages/Admin').then((module) => ({ default: module.AdminPage })))
const SystemPage = lazy(() => import('./pages/SystemPage').then((module) => ({ default: module.SystemPage })))
const RegisterPage = lazy(() => import('./pages/Register').then((module) => ({ default: module.RegisterPage })))
const SettingsPage = lazy(() => import('./pages/Settings').then((module) => ({ default: module.SettingsPage })))

export function App(): React.JSX.Element {
  return <Suspense fallback={<Loading label="Загружаем раздел…" />}><Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route path="/register" element={<RegisterPage />} />
    <Route path="/password" element={<ProtectedRoute><PasswordPage /></ProtectedRoute>} />
    <Route element={<ProtectedRoute><Layout /></ProtectedRoute>}>
      <Route path="/today" element={<TodayPage />} />
      <Route path="/schedule" element={<SchedulePage />} />
      <Route path="/journal" element={<JournalPage />} />
      <Route path="/homework" element={<HomeworkPage />} />
      <Route path="/grades" element={<GradesPage />} />
      <Route path="/announcements" element={<AnnouncementsPage />} />
      <Route path="/reports" element={<ReportsPage />} />
      <Route path="/my-class" element={<ReportsPage classMode />} />
      <Route path="/admin" element={<AdminPage />} />
      <Route path="/system" element={<SystemPage />} />
      <Route path="/settings" element={<SettingsPage />} />
      <Route path="/" element={<Navigate to="/today" replace />} />
      <Route path="*" element={<div className="state-view"><h1>Страница не найдена</h1><p>Проверьте адрес или вернитесь на главную.</p><Link className="button primary" to="/today">На главную</Link></div>} />
    </Route>
  </Routes></Suspense>
}
