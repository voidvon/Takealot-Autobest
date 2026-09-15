import React, { useState, useEffect } from 'react'
import { api } from './api/client'
import type { EngineStatus } from './types'
import { Header } from './components/layout/Header'
import { Sidebar, type TabId } from './components/layout/Sidebar'
import { DashboardTab } from './components/dashboard/DashboardTab'
import { RepricerTab } from './components/repricer/RepricerTab'
import { CatalogTab } from './components/catalog/CatalogTab'
import { SalesTab } from './components/sales/SalesTab'
import { FollowTab } from './components/follow/FollowTab'
import { LogsTab } from './components/logs/LogsTab'
import { SettingsTab } from './components/settings/SettingsTab'

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<TabId>('dashboard')
  const [version, setVersion] = useState('0.2.0')
  const [loadingAction, setLoadingAction] = useState(false)

  // Dark mode
  const [darkMode, setDarkMode] = useState(() => {
    const saved = localStorage.getItem('takealot_theme')
    if (saved) return saved === 'dark'
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  })

  // Engine status
  const [status, setStatus] = useState<EngineStatus>({
    is_running: false,
    is_paused: false,
    total_checked: 0,
    total_repriced: 0,
    countdown_seconds: 0,
    last_run_time: '',
  })

  // Apply dark mode class to document
  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('takealot_theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('takealot_theme', 'light')
    }
  }, [darkMode])

  // Fetch version & poll status
  useEffect(() => {
    api.getVersion().then((res) => {
      if (res.version) setVersion(res.version)
    }).catch(() => {})

    const fetchStatus = () => {
      api.getRepriceStatus().then((s) => {
        if (s) setStatus(s)
      }).catch(() => {})
    }

    fetchStatus()
    const timer = setInterval(fetchStatus, 3000)
    return () => clearInterval(timer)
  }, [])

  // Engine controls
  const handleStartReprice = async () => {
    setLoadingAction(true)
    try {
      const res = await api.startReprice()
      if (res.success) {
        setStatus((prev) => ({ ...prev, is_running: true, is_paused: false }))
      } else {
        alert(res.message || '启动失败')
      }
    } catch (e: any) {
      alert(`启动失败: ${e.message}`)
    } finally {
      setLoadingAction(false)
    }
  }

  const handlePauseReprice = async () => {
    setLoadingAction(true)
    try {
      const res = await api.pauseReprice()
      if (res.success) {
        setStatus((prev) => ({ ...prev, is_paused: !prev.is_paused }))
      }
    } catch (e: any) {
      alert(`暂停操作失败: ${e.message}`)
    } finally {
      setLoadingAction(false)
    }
  }

  const handleStopReprice = async () => {
    setLoadingAction(true)
    try {
      const res = await api.stopReprice()
      if (res.success) {
        setStatus((prev) => ({ ...prev, is_running: false, is_paused: false }))
      }
    } catch (e: any) {
      alert(`停止操作失败: ${e.message}`)
    } finally {
      setLoadingAction(false)
    }
  }

  return (
    <div className="min-h-screen flex flex-col bg-background text-foreground">
      {/* Top Header */}
      <Header
        version={version}
        status={status}
        onStartReprice={handleStartReprice}
        onPauseReprice={handlePauseReprice}
        onStopReprice={handleStopReprice}
        darkMode={darkMode}
        onToggleDarkMode={() => setDarkMode(!darkMode)}
        loadingAction={loadingAction}
      />

      {/* Main Layout: Sidebar + Tab Content */}
      <div className="flex-1 flex flex-col md:flex-row w-full max-w-[1680px] mx-auto">
        <Sidebar
          activeTab={activeTab}
          onSelectTab={setActiveTab}
        />

        {/* Tab Content Area */}
        <main className="flex-1 p-4 sm:p-6 lg:p-8 overflow-y-auto">
          {activeTab === 'dashboard' && (
            <DashboardTab
              onNavigate={setActiveTab}
              onStartReprice={handleStartReprice}
              isRunning={status.is_running}
            />
          )}

          {activeTab === 'repricer' && <RepricerTab />}

          {activeTab === 'catalog' && <CatalogTab />}

          {activeTab === 'sales' && <SalesTab />}

          {activeTab === 'follow' && <FollowTab onNavigateLogs={() => setActiveTab('logs')} />}

          {activeTab === 'logs' && <LogsTab />}

          {activeTab === 'settings' && <SettingsTab />}
        </main>
      </div>
    </div>
  )
}
export default App
