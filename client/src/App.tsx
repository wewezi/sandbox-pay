import { Routes, Route, Navigate } from 'react-router-dom'
import Home from './pages/Home'
import Login from './pages/Login'
import Cashier from './pages/Cashier'
import MobilePay from './pages/MobilePay'
import RequireAuth from './components/RequireAuth'
import ErrorBoundary from './components/ErrorBoundary'

// Admin console pages
import DashboardOverview from './pages/admin/DashboardOverview'
import OrdersPage from './pages/admin/OrdersPage'
import MerchantsPage from './pages/admin/MerchantsPage'
import NotificationsPage from './pages/admin/NotificationsPage'
import ReconciliationPage from './pages/admin/ReconciliationPage'
import AuditLogsPage from './pages/admin/AuditLogsPage'
import SettingsPage from './pages/admin/SettingsPage'
import SandboxPage from './pages/admin/SandboxPage'
import IntegrationDocsPage from './pages/admin/IntegrationDocsPage'

export default function App() {
  return (
    <ErrorBoundary>
    <div className="min-h-screen bg-background text-foreground selection:bg-emerald-500 selection:text-white">
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />

        {/* Old dashboard → redirect to new admin console */}
        <Route path="/dashboard" element={<Navigate to="/admin" replace />} />

        {/* Admin console with sidebar layout */}
        <Route path="/admin" element={<RequireAuth><DashboardOverview /></RequireAuth>} />
        <Route path="/admin/orders" element={<RequireAuth><OrdersPage /></RequireAuth>} />
        <Route path="/admin/merchants" element={<RequireAuth><MerchantsPage /></RequireAuth>} />
        <Route path="/admin/notifications" element={<RequireAuth><NotificationsPage /></RequireAuth>} />
        <Route path="/admin/reconciliation" element={<RequireAuth><ReconciliationPage /></RequireAuth>} />
        <Route path="/admin/docs" element={<RequireAuth><IntegrationDocsPage /></RequireAuth>} />
        <Route path="/admin/audit" element={<RequireAuth><AuditLogsPage /></RequireAuth>} />
        <Route path="/admin/settings" element={<RequireAuth><SettingsPage /></RequireAuth>} />
        <Route path="/admin/sandbox" element={<RequireAuth><SandboxPage /></RequireAuth>} />

        {/* Public pages */}
        <Route path="/cashier" element={<Cashier />} />
        <Route path="/mobile-pay" element={<MobilePay />} />

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </div>
    </ErrorBoundary>
  )
}
