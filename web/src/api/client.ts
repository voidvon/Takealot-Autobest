import type {
  SystemConfig,
  Store,
  OfferViewModel,
  EngineStatus,
  LogEntry,
  OfficialOffersResponse,
  SalesResponse,
  SalesSummaryItem,
  SalesOrdersResponse,
  InvoiceDocument,
  StockCounts,
  StockHealthStats,
  FollowItem,
  TargetConfig,
  AccountStatus,
  UpdateInfo,
  UpdateProgress,
} from '../types'

let currentStoreId = localStorage.getItem('takealot_active_store_id') || ''

export function getActiveStoreId(): string {
  return currentStoreId
}

export function setActiveStoreId(id: string) {
  currentStoreId = id
  if (id) {
    localStorage.setItem('takealot_active_store_id', id)
  } else {
    localStorage.removeItem('takealot_active_store_id')
  }
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const customHeaders: Record<string, string> = {}
  if (currentStoreId) {
    customHeaders['X-Store-Id'] = currentStoreId
  }

  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...customHeaders,
      ...(options?.headers || {}),
    },
    ...options,
  })
  if (!res.ok) {
    let errorMsg = `HTTP ${res.status}`
    try {
      const data = await res.json()
      if (data.error) errorMsg = data.error
      else if (data.message) errorMsg = data.message
    } catch {
      // ignore
    }
    throw new Error(errorMsg)
  }
  return res.json()
}

export const api = {
  // Store Management
  getStores: () => request<{ success: boolean; stores: Store[]; active_store_id: string }>('/api/stores'),
  createStore: (store: Partial<Store>) =>
    request<{ success: boolean; store: Store; message?: string }>('/api/stores', {
      method: 'POST',
      body: JSON.stringify(store),
    }),
  updateStore: (store: Partial<Store>) =>
    request<{ success: boolean; store: Store; message?: string }>('/api/stores', {
      method: 'PUT',
      body: JSON.stringify(store),
    }),
  deleteStore: (id: string) =>
    request<{ success: boolean; message?: string }>(`/api/stores?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  testStore: (authorization: string, proxyUrl?: string) =>
    request<{ success: boolean; message: string; display_name?: string; total_offers?: number }>('/api/stores/test', {
      method: 'POST',
      body: JSON.stringify({ authorization, proxy_url: proxyUrl }),
    }),
  syncStoreName: () => request<{ success: boolean; name: string }>('/api/stores/sync', { method: 'POST' }),
  startAllReprice: () => request<{ success: boolean; results: string[] }>('/api/reprice/start_all', { method: 'POST' }),
  stopAllReprice: () => request<{ success: boolean; results: string[] }>('/api/reprice/stop_all', { method: 'POST' }),

  // System & Config
  getVersion: () => request<{ version: string }>('/api/version'),
  getConfig: () => request<SystemConfig>('/api/config'),
  updateConfig: (config: SystemConfig) =>
    request<{ success: boolean; message: string }>('/api/config', {
      method: 'POST',
      body: JSON.stringify(config),
    }),
  testAuth: () =>
    request<{ success: boolean; message: string; total_offers: number }>('/api/test_auth', {
      method: 'POST',
    }),

  // Repricing Engine & Offers
  getOffers: (sync = false) =>
    request<{ success: boolean; total: number; offers: OfferViewModel[]; source?: string }>(
      sync ? '/api/offers?sync=true' : '/api/offers'
    ),
  saveTargets: (targets: Record<string, TargetConfig>) =>
    request<{ success: boolean; message: string }>('/api/targets', {
      method: 'POST',
      body: JSON.stringify(targets),
    }),
  startReprice: () => request<{ success: boolean; message: string }>('/api/reprice/start', { method: 'POST' }),
  pauseReprice: () => request<{ success: boolean; message: string }>('/api/reprice/pause', { method: 'POST' }),
  stopReprice: () => request<{ success: boolean; message: string }>('/api/reprice/stop', { method: 'POST' }),
  getRepriceStatus: () => request<EngineStatus>('/api/reprice/status'),

  // Follow Selling
  uploadFollowExcel: async (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    const customHeaders: Record<string, string> = {}
    if (currentStoreId) {
      customHeaders['X-Store-Id'] = currentStoreId
    }
    const res = await fetch('/api/follow/upload', {
      method: 'POST',
      headers: customHeaders,
      body: formData,
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      throw new Error(data.error || `HTTP ${res.status}`)
    }
    return res.json() as Promise<{ success: boolean; total: number; items: FollowItem[] }>
  },
  startFollowBatch: (items: FollowItem[]) =>
    request<{ success: boolean; message: string }>('/api/follow/start', {
      method: 'POST',
      body: JSON.stringify({ items }),
    }),

  // Logs
  getLogsHistory: () => request<LogEntry[]>('/api/logs/history'),

  // Official Seller API
  getOfficialOffers: (page = 1, pageSize = 50, filter = '') =>
    request<OfficialOffersResponse>(
      `/api/official/offers?page=${page}&page_size=${pageSize}${filter ? `&filter=${encodeURIComponent(filter)}` : ''}`
    ),
  getOfficialOffersCount: () => request<{ count: number }>('/api/official/offers/count'),
  updateOfficialOffer: (
    offerId: string | number,
    payload: {
      selling_price?: number
      rrp?: number
      leadtime_days?: number
      status?: string
      store_id?: string
    },
    storeId?: string
  ) => {
    const customHeaders: Record<string, string> = {}
    const targetStoreId = storeId || payload.store_id
    if (targetStoreId) {
      customHeaders['X-Store-Id'] = targetStoreId
    }
    return request<{ success: boolean; message: string }>('/api/official/offers/update', {
      method: 'POST',
      headers: customHeaders,
      body: JSON.stringify({ offer_id: String(offerId), store_id: targetStoreId, ...payload }),
    })
  },
  getOfficialSales: (page = 1, pageSize = 50, startDate = '', endDate = '') =>
    request<SalesResponse>(
      `/api/official/sales?page=${page}&page_size=${pageSize}${startDate ? `&start_date=${startDate}` : ''}${endDate ? `&end_date=${endDate}` : ''}`
    ),
  getOfficialSalesSummary: () => request<SalesSummaryItem[]>('/api/official/sales/summary'),
  getOfficialSalesOrders: (startDate = '', endDate = '', page = 1, pageSize = 50) =>
    request<SalesOrdersResponse>(
      `/api/official/sales/orders?page=${page}&page_size=${pageSize}${startDate ? `&start_date=${startDate}` : ''}${endDate ? `&end_date=${endDate}` : ''}`
    ),
  getOfficialCustomerInvoices: (orderId: number) =>
    request<{ documents: InvoiceDocument[] }>(`/api/official/sales/orders/invoices?order_id=${orderId}`),
  getOfficialStockCounts: () => request<StockCounts>('/api/official/stock/counts'),
  getOfficialStockHealth: () => request<StockHealthStats>('/api/official/stock/health'),

  // Official Single Offer & Batch Operations
  getOfficialSingleOffer: (identifier: string) =>
    request<any>(`/api/official/offers/single?identifier=${encodeURIComponent(identifier)}`),
  createOfficialOffer: (payload: {
    barcode: string
    sku?: string
    selling_price: number
    rrp?: number
    leadtime_days?: number
    leadtime_stock?: any[]
  }) =>
    request<{ success: boolean; result?: any }>('/api/official/offers/create', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  setOfficialOfferStatus: (identifier: string, action: 'Disable' | 'Re-enable') =>
    request<{ success: boolean; identifier: string; action: string }>('/api/official/offers/status', {
      method: 'POST',
      body: JSON.stringify({ identifier, action }),
    }),
  createOfficialBatch: (offers: any[], actionType = 'batch_update') =>
    request<{ batch_id: number | string; status: { id: number; description: string } }>(
      '/api/official/offers/batch',
      {
        method: 'POST',
        body: JSON.stringify({ offers, action_type: actionType }),
      }
    ),
  getOfficialBatchStatus: (batchId: string | number) =>
    request<import('../types').BatchStatusResponse>(
      `/api/official/offers/batch/status?batch_id=${encodeURIComponent(batchId)}`
    ),
  getOfficialBatchList: (limit = 50) =>
    request<import('../types').BatchJobRecord[]>(`/api/official/offers/batch/list?limit=${limit}`),

  // SQLite Database Histories
  getRepriceHistory: (limit = 50) =>
    request<import('../types').RepriceHistoryRecord[]>(`/api/reprice/history?limit=${limit}`),
  getFollowHistory: (limit = 50) =>
    request<import('../types').FollowHistoryRecord[]>(`/api/follow/history?limit=${limit}`),

  // Inbound & Order Fulfillment
  getLeadtimeOrders: () =>
    request<{ total: number; items: import('../types').LeadtimeOrderItem[] }>('/api/fulfillment/leadtime-orders'),
  getShipments: (status?: string) =>
    request<import('../types').ShipmentRecord[]>(`/api/fulfillment/shipments${status ? `?status=${status}` : ''}`),
  createShipment: (payload: {
    shipment_number?: string
    destination_dc?: string
    notes?: string
    status?: string
    items: import('../types').ShipmentItemRecord[]
  }) =>
    request<import('../types').ShipmentRecord>('/api/fulfillment/shipments/create', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateShipmentStatus: (id: number, status: string) =>
    request<{ success: boolean }>('/api/fulfillment/shipments/status', {
      method: 'POST',
      body: JSON.stringify({ id, status }),
    }),
  deleteShipment: (id: number) =>
    request<{ success: boolean }>(`/api/fulfillment/shipments/delete?id=${id}`, {
      method: 'POST',
    }),
  updateShipmentItem: (payload: {
    item_id: number
    ship_qty: number
    leadtime_stock: number
    actual_weight: number
    volumetric_weight: number
    weigh_status: string
  }) =>
    request<{ success: boolean }>('/api/fulfillment/shipments/item/update', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  quickUpdateOffer: (payload: {
    offer_id?: string
    tsin?: string
    selling_price?: number
    rrp?: number
    weight_kg?: number
    leadtime_stock?: number
  }) =>
    request<{ success: boolean; message: string }>('/api/fulfillment/offer/quick-update', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  getBookings: () => request<import('../types').BookingRecord[]>('/api/fulfillment/bookings'),
  createBooking: (payload: {
    shipment_id?: number
    dc: string
    booking_date: string
    time_slot: string
    carrier: string
    vehicle_reg: string
    notes?: string
  }) =>
    request<import('../types').BookingRecord>('/api/fulfillment/bookings/create', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateBookingStatus: (id: number, status: string) =>
    request<{ success: boolean }>('/api/fulfillment/bookings/status', {
      method: 'POST',
      body: JSON.stringify({ id, status }),
    }),
  deleteBooking: (id: number) =>
    request<{ success: boolean }>(`/api/fulfillment/bookings/delete?id=${id}`, {
      method: 'POST',
    }),
  // Online member account; remote session stays in the Go process.
  getAccountStatus: () => request<AccountStatus>('/api/account/status'),
  refreshAccount: () => request<AccountStatus>('/api/account/refresh', { method: 'POST', body: '{}' }),
  loginAccount: (identifier: string, password: string) =>
    request<AccountStatus>('/api/account/login', { method: 'POST', body: JSON.stringify({ identifier, password }) }),
  registerAccount: (username: string, email: string, password: string) =>
    request<{ ok: boolean }>('/api/account/register', { method: 'POST', body: JSON.stringify({ username, email, password }) }),
  logoutAccount: () => request<AccountStatus>('/api/account/logout', { method: 'POST', body: '{}' }),

  // Automatic Updater
  checkUpdate: (force = false) =>
    request<UpdateInfo>(`/api/updater/check${force ? '?force=true' : ''}`),
  applyUpdate: (payload?: { proxy?: boolean; download_url?: string }) =>
    request<{ success: boolean; message: string }>('/api/updater/apply', {
      method: 'POST',
      body: JSON.stringify(payload || {}),
    }),
  getUpdateProgress: () => request<UpdateProgress>('/api/updater/progress'),
}
