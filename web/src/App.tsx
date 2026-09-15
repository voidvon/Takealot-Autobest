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

const VALID_TABS: TabId[] = ['dashboard', 'repricer', 'catalog', 'sales', 'follow', 'logs', 'settings']

const getTabFromLocation = (): TabId => {
  // 优先从 Hash 解析 (如 #/repricer 或 #repricer)
  const hash = window.location.hash.replace(/^#\/?/, '').toLowerCase()
  if (VALID_TABS.includes(hash as TabId)) {
    return hash as TabId
  }
  // 备选从 Pathname 解析 (如 /repricer)
  const path = window.location.pathname.replace(/^\//, '').toLowerCase()
  if (VALID_TABS.includes(path as TabId)) {
    return path as TabId
  }
  return 'dashboard'
}

export const App: React.FC = () => {
  const [activeTab, setActiveTabState] = useState<TabId>(getTabFromLocation)
  const [version, setVersion] = useState('0.2.0')
  const [loadingAction, setLoadingAction] = useState(false)

  const handleSelectTab = (tab: TabId) => {
    setActiveTabState(tab)
    if (window.location.hash !== `#/${tab}`) {
      window.location.hash = `/${tab}`
    }
  }

  // 监听浏览器 URL 路由变化（前进/后退/手动修改地址栏）
  useEffect(() => {
    const handleUrlChange = () => {
      const tab = getTabFromLocation()
      setActiveTabState(tab)
    }

    // 默认补全路由 Hash，便于收藏与刷新定位
    if (!window.location.hash) {
      window.location.hash = `/${getTabFromLocation()}`
    }

    window.addEventListener('hashchange', handleUrlChange)
    window.addEventListener('popstate', handleUrlChange)
    return () => {
      window.removeEventListener('hashchange', handleUrlChange)
      window.removeEventListener('popstate', handleUrlChange)
    }
  }, [])

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
          onSelectTab={handleSelectTab}
        />

        {/* Tab Content Area */}
        <main className="flex-1 p-4 sm:p-6 lg:p-8 overflow-y-auto min-w-0">
          {activeTab === 'dashboard' && (
            <DashboardTab
              onNavigate={handleSelectTab}
              onStartReprice={handleStartReprice}
              isRunning={status.is_running}
            />
          )}

          {activeTab === 'repricer' && <RepricerTab />}

          {activeTab === 'catalog' && <CatalogTab />}

          {activeTab === 'sales' && <SalesTab />}

          {activeTab === 'follow' && <FollowTab onNavigateLogs={() => handleSelectTab('logs')} />}

          {activeTab === 'logs' && <LogsTab />}

          {activeTab === 'settings' && <SettingsTab />}
        </main>
      </div>
    </div>
  )
}
export default App
