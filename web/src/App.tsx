import React, { useState, useEffect } from 'react'
import { api, getActiveStoreId, setActiveStoreId } from './api/client'
import type { EngineStatus, Store, AccountStatus, UpdateInfo } from './types'
import { Header } from './components/layout/Header'
import { Sidebar, type TabId } from './components/layout/Sidebar'
import { DashboardTab } from './components/dashboard/DashboardTab'
import { RepricerTab } from './components/repricer/RepricerTab'
import { CatalogTab } from './components/catalog/CatalogTab'
import { SalesTab } from './components/sales/SalesTab'
import { FollowTab } from './components/follow/FollowTab'
import { SettingsTab } from './components/settings/SettingsTab'
import { AddStoreModal } from './components/layout/AddStoreModal'
import { AccountModal } from './components/account/AccountModal'
import { UpdateModal } from './components/updater/UpdateModal'
import { Toaster } from './components/ui/toaster'
import { toast } from './components/ui/use-toast'

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

  // Account State
  const [accountStatus, setAccountStatus] = useState<AccountStatus | null>(null)
  const [isAccountModalOpen, setIsAccountModalOpen] = useState(false)

  // Auto Updater State
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [isUpdateModalOpen, setIsUpdateModalOpen] = useState(false)

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

  // Load account status
  const loadAccountStatus = async () => {
    try {
      const res = await api.getAccountStatus()
      setAccountStatus(res)
      if (!res.eligible) {
        setIsAccountModalOpen(true)
      }
    } catch (err) {
      console.error('Failed to load account status:', err)
    }
  }

  // Fetch version & initial status & stores & account
  useEffect(() => {
    loadAccountStatus()
    const accountTimer = setInterval(() => {
      api.getAccountStatus().then(setAccountStatus).catch(() => setAccountStatus(null))
    }, 5000)

    api.getVersion().then((res) => {
      if (res.version) setVersion(res.version)
    }).catch(() => {})

    loadStores()

    api.getRepriceStatus().then((s) => {
      if (s) setStatus(s)
    }).catch(() => {})
    return () => clearInterval(accountTimer)
  }, [])

  // 检查版本更新
  const checkForUpdate = async (manual = false) => {
    try {
      const res = await api.checkUpdate(manual)
      setUpdateInfo(res)
      if (res.has_update) {
        setIsUpdateModalOpen(true)
        if (manual) {
          toast.info('发现新版本', `检测到新版本 ${res.tag_name}，可立即更新`)
        }
      } else if (manual) {
        toast.success('已是最新版本', `当前版本 v${(res.current_version || version).replace(/^v/, '')} 已是官方最新版本`)
      }
    } catch (err: any) {
      if (manual) {
        toast.error('检查更新失败', err.message || '网络连接超时')
      }
    }
  }

  // 启动 2.5 秒后静默检查更新
  useEffect(() => {
    const timer = setTimeout(() => {
      checkForUpdate(false)
    }, 2500)
    return () => clearTimeout(timer)
  }, [version])

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
    if (!accountStatus?.eligible) {
      setIsAccountModalOpen(true)
      return
    }

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
        toast.error('启动调价失败', res.message || '未知错误')
      }
    } catch (e: any) {
      toast.error('启动调价失败', e.message)
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
      toast.error('暂停操作失败', e.message)
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
      toast.error('停止操作失败', e.message)
    } finally {
      setLoadingAction(false)
    }
  }

  return (
    <div className="h-screen w-screen flex flex-col bg-background text-foreground overflow-hidden">
      {/* Global Toast Notification Viewport (Top Center) */}
      <Toaster />

      {/* Top Header with Store Switcher & Engine Actions */}
      <Header
        version={version}
        status={status}
        onStartReprice={handleStartReprice}
        onPauseReprice={handlePauseReprice}
        onStopReprice={handleStopReprice}
        loadingAction={loadingAction}
        stores={stores}
        currentStoreId={currentStoreId}
        onSelectStore={handleSelectStore}
        onOpenAddStore={() => setIsAddStoreOpen(true)}
        updateInfo={updateInfo}
        onOpenUpdate={() => {
          if (updateInfo?.has_update) {
            setIsUpdateModalOpen(true)
          } else {
            checkForUpdate(true)
          }
        }}
        onStartAllReprice={async () => {
          if (!accountStatus?.eligible) {
            setIsAccountModalOpen(true)
            return
          }
          try {
            const res = await api.startAllReprice()
            await loadStores()
            toast.success('已执行一键启动所有店铺巡检', res.results.join('\n'))
          } catch (e: any) {
            toast.error('一键启动失败', e.message)
          }
        }}
        onStopAllReprice={async () => {
          try {
            const res = await api.stopAllReprice()
            await loadStores()
            toast.warning('已执行一键停止所有店铺巡检', res.results.join('\n'))
          } catch (e: any) {
            toast.error('一键停止失败', e.message)
          }
        }}
      />

      {/* Main Layout: Sidebar + Tab Content */}
      <div className="flex-1 min-h-0 flex flex-col md:flex-row w-full max-w-[1680px] mx-auto overflow-hidden">
        <Sidebar
          activeTab={activeTab}
          onSelectTab={handleSelectTab}
          accountStatus={accountStatus}
          onOpenAccount={() => setIsAccountModalOpen(true)}
          darkMode={darkMode}
          onToggleDarkMode={() => setDarkMode(!darkMode)}
        />

        {/* Tab Content Area: keyed with currentStoreId so switching store resets and reloads views cleanly */}
        <main key={`${activeTab}-${currentStoreId}`} className="flex-1 min-h-0 h-full p-4 sm:p-6 lg:p-8 overflow-y-auto min-w-0">
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
              version={version}
              updateInfo={updateInfo}
              onOpenUpdate={() => setIsUpdateModalOpen(true)}
              onCheckUpdate={() => checkForUpdate(true)}
            />
          )}
        </main>
      </div>

      {/* Global Account Modal */}
      <AccountModal
        open={isAccountModalOpen}
        onOpenChange={setIsAccountModalOpen}
        status={accountStatus}
        onAuthenticated={(newStatus) => {
          setAccountStatus(newStatus)
          if (newStatus.eligible) {
            loadStores()
          }
        }}
        version={version}
        updateInfo={updateInfo}
        onOpenUpdate={() => setIsUpdateModalOpen(true)}
        onCheckUpdate={() => checkForUpdate(true)}
      />

      {/* Global Add Store Modal (accessible from any tab / StoreSwitcher) */}
      <AddStoreModal
        open={isAddStoreOpen}
        onOpenChange={setIsAddStoreOpen}
        onSuccess={async (newStore) => {
          await loadStores()
          handleSelectStore(newStore.id)
        }}
      />

      {/* Global Automatic Update Modal */}
      <UpdateModal
        open={isUpdateModalOpen}
        onOpenChange={setIsUpdateModalOpen}
        updateInfo={updateInfo}
        currentVersion={version}
      />
    </div>
  )
}

export default App
