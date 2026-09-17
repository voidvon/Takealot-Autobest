import React, { useState, useEffect } from 'react'
import { api, getActiveStoreId, setActiveStoreId } from './api/client'
import type { EngineStatus, Store } from './types'
import { Header } from './components/layout/Header'
import { Sidebar, type TabId } from './components/layout/Sidebar'
import { DashboardTab } from './components/dashboard/DashboardTab'
import { RepricerTab } from './components/repricer/RepricerTab'
import { CatalogTab } from './components/catalog/CatalogTab'
import { SalesTab } from './components/sales/SalesTab'
import { FollowTab } from './components/follow/FollowTab'
import { SettingsTab } from './components/settings/SettingsTab'
import { AddStoreModal } from './components/layout/AddStoreModal'

const VALID_TABS: TabId[] = ['dashboard', 'repricer', 'catalog', 'sales', 'follow', 'settings']

const getTabFromLocation = (): TabId => {
  const hash = window.location.hash.replace(/^#\/?/, '').toLowerCase()
  if (VALID_TABS.includes(hash as TabId)) {
    return hash as TabId
  }
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

  // Multi-Store State
  const [stores, setStores] = useState<Store[]>([])
  const [currentStoreId, setCurrentStoreId] = useState<string>(() => getActiveStoreId())
  const [isAddStoreOpen, setIsAddStoreOpen] = useState(false)

  const handleSelectTab = (tab: TabId) => {
    setActiveTabState(tab)
    if (window.location.hash !== `#/${tab}`) {
      window.location.hash = `/${tab}`
    }
  }

  // 监听浏览器 URL 路由变化
  useEffect(() => {
    const handleUrlChange = () => {
      const tab = getTabFromLocation()
      setActiveTabState(tab)
    }

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

  // Load stores list
  const loadStores = async () => {
    try {
      const res = await api.getStores()
      if (res.stores && res.stores.length > 0) {
        setStores(res.stores)
        if (currentStoreId !== 'all') {
          const exists = res.stores.find((s) => s.id === currentStoreId)
          if (!exists) {
            const nextId = res.active_store_id || res.stores[0].id
            setCurrentStoreId(nextId)
            setActiveStoreId(nextId)
          }
        }
      }
    } catch (err) {
      console.error('Failed to load stores:', err)
    }
  }

  // Fetch version & initial status & stores
  useEffect(() => {
    api.getVersion().then((res) => {
      if (res.version) setVersion(res.version)
    }).catch(() => {})

    loadStores()

    api.getRepriceStatus().then((s) => {
      if (s) setStatus(s)
    }).catch(() => {})
  }, [])

  // When currentStoreId changes, refresh status
  useEffect(() => {
    if (currentStoreId) {
      api.getRepriceStatus().then((s) => {
        if (s) setStatus(s)
      }).catch(() => {})
    }
  }, [currentStoreId])

  const handleSelectStore = (storeId: string) => {
    setCurrentStoreId(storeId)
    setActiveStoreId(storeId)
    api.getRepriceStatus().then((s) => {
      if (s) setStatus(s)
    }).catch(() => {})
  }

  // 本地实时每秒倒计时
  useEffect(() => {
    if (!status.is_running || status.is_paused || status.countdown_seconds <= 0) return

    const tickTimer = setInterval(() => {
      setStatus((prev) => {
        if (!prev.is_running || prev.is_paused || prev.countdown_seconds <= 0) return prev
        return {
          ...prev,
          countdown_seconds: Math.max(0, prev.countdown_seconds - 1),
        }
      })
    }, 1000)

    return () => clearInterval(tickTimer)
  }, [status.is_running, status.is_paused, status.countdown_seconds])

  // 智能校准
  useEffect(() => {
    if (!status.is_running) return

    const syncCalibrate = () => {
      if (document.hidden) return
      api.getRepriceStatus().then((s) => {
        if (s) setStatus(s)
      }).catch(() => {})
    }

    const intervalTimer = setInterval(syncCalibrate, 15000)

    const handleVisibility = () => {
      if (!document.hidden) syncCalibrate()
    }
    document.addEventListener('visibilitychange', handleVisibility)

    return () => {
      clearInterval(intervalTimer)
      document.removeEventListener('visibilitychange', handleVisibility)
    }
  }, [status.is_running, currentStoreId])

  // Engine controls for active store
  const handleStartReprice = async () => {
    setLoadingAction(true)
    try {
      const res = await api.startReprice()
      if (res.success) {
        setStatus((prev) => ({ ...prev, is_running: true, is_paused: false }))
        api.getRepriceStatus().then((s) => {
          if (s) setStatus(s)
        }).catch(() => {})
        await loadStores()
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
        api.getRepriceStatus().then((s) => {
          if (s) setStatus(s)
        }).catch(() => {})
        await loadStores()
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
        setStatus((prev) => ({ ...prev, is_running: false, is_paused: false, countdown_seconds: 0 }))
        await loadStores()
      }
    } catch (e: any) {
      alert(`停止操作失败: ${e.message}`)
    } finally {
      setLoadingAction(false)
    }
  }

  return (
    <div className="min-h-screen flex flex-col bg-background text-foreground">
      {/* Top Header with Store Switcher */}
      <Header
        version={version}
        status={status}
        onStartReprice={handleStartReprice}
        onPauseReprice={handlePauseReprice}
        onStopReprice={handleStopReprice}
        darkMode={darkMode}
        onToggleDarkMode={() => setDarkMode(!darkMode)}
        loadingAction={loadingAction}
        stores={stores}
        currentStoreId={currentStoreId}
        onSelectStore={handleSelectStore}
        onOpenAddStore={() => setIsAddStoreOpen(true)}
        onStartAllReprice={async () => {
          try {
            const res = await api.startAllReprice()
            await loadStores()
            alert(`已执行一键启动所有店铺巡检！\n\n${res.results.join('\n')}`)
          } catch (e: any) {
            alert(`操作失败: ${e.message}`)
          }
        }}
        onStopAllReprice={async () => {
          try {
            const res = await api.stopAllReprice()
            await loadStores()
            alert(`已执行一键停止所有店铺巡检！\n\n${res.results.join('\n')}`)
          } catch (e: any) {
            alert(`操作失败: ${e.message}`)
          }
        }}
      />

      {/* Main Layout: Sidebar + Tab Content */}
      <div className="flex-1 flex flex-col md:flex-row w-full max-w-[1680px] mx-auto">
        <Sidebar
          activeTab={activeTab}
          onSelectTab={handleSelectTab}
        />

        {/* Tab Content Area: keyed with currentStoreId so switching store resets and reloads views cleanly */}
        <main key={`${activeTab}-${currentStoreId}`} className="flex-1 p-4 sm:p-6 lg:p-8 overflow-y-auto min-w-0">
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

          {activeTab === 'follow' && <FollowTab onNavigateLogs={() => handleSelectTab('repricer')} />}

          {activeTab === 'settings' && (
            <SettingsTab
              stores={stores}
              currentStoreId={currentStoreId}
              onRefreshStores={loadStores}
              onSelectStore={handleSelectStore}
              onOpenAddStore={() => setIsAddStoreOpen(true)}
            />
          )}
        </main>
      </div>

      {/* Global Add Store Modal (accessible from any tab / StoreSwitcher) */}
      <AddStoreModal
        open={isAddStoreOpen}
        onOpenChange={setIsAddStoreOpen}
        onSuccess={async (newStore) => {
          await loadStores()
          handleSelectStore(newStore.id)
        }}
      />
    </div>
  )
}

export default App
