export type PriorityStatus = 'winning' | 'losing' | 'solo'

export interface OfferViewModel {
  key: string
  tsin_id: string
  sku?: string
  plid: string
  title: string
  selling_price: number
  rrp: number
  stock: number
  date_modified: string
  selected: boolean
  min_price: number
  max_price: number
  best_price: number
  competing_offers: number
  priority_status: PriorityStatus
  price_diff: number
  image_url?: string
  image_large_url?: string
}

export interface WarehouseStockDetail {
  warehouse: {
    warehouse_id: number
    name: string
  }
  quantity_available: number
}

export interface OfficialOfferItem {
  offer_id: number
  tsin_id: number
  sku: string
  barcode: string
  product_label_number: string
  selling_price: number
  rrp: number
  leadtime_days: number
  status: string
  title: string
  offer_url: string
  image_url: string
  image_large_url: string
  stock_at_takealot: WarehouseStockDetail[]
  stock_on_way: WarehouseStockDetail[]
  total_stock_on_way: number
  stock_at_takealot_total: number
  catalogue_quality_score: number
  date_created: string
  discount: string
  discount_shown: boolean
}

export interface OfficialOffersResponse {
  page_size: number
  page_number: number
  total_results: number
  offers: OfficialOfferItem[]
}

export interface SaleItem {
  order_item_id: number
  order_id: number
  order_date: string
  sale_status: string
  offer_id: number
  tsin: number
  sku: string
  product_title: string
  selling_price: number
  success_fee: number
  fulfillment_fee: number
  courier_collection_fee: number
  customer: string
  dc: string
}

export interface SalesResponse {
  page_summary: {
    total: number
    page_size: number
    page_number: number
  }
  sales: SaleItem[]
}

export interface SalesSummaryItem {
  date_range: string
  total: number
  quantity: number
}

export interface OrderItemInfo {
  order_item_id: number
  merchant_sku: string
  title: string
  tsin_id: number
  takealot_url: string
}

export interface OrderRecord {
  order_id: number
  date_authed: string
  order_items: OrderItemInfo[]
}

export interface SalesOrdersResponse {
  page_summary: {
    total: number
    page_size: number
    page_number: number
  }
  orders: OrderRecord[]
}

export interface InvoiceDocument {
  document_id: number
  document_date: string
  document_reason: string
  file_name: string
  document_type: string
  downloadable: boolean
}

export interface StockCounts {
  total_stock_count: number
  unbalanced_stock_count: number
}

export interface StockHealthStats {
  storage_fee_enabled_offer_count: number
  recommended_for_replenishment_offer_count: number
}

export interface EngineStatus {
  is_running: boolean
  is_paused: boolean
  total_checked: number
  total_repriced: number
  countdown_seconds: number
  last_run_time: string
}

export interface LogEntry {
  time: string
  level: 'INFO' | 'WARN' | 'ERROR' | 'SUCCESS'
  message: string
}

export interface TargetConfig {
  selected: boolean
  min_price: number
  max_price: number
}

export interface SystemConfig {
  authorization: string
  price_decrease_step: number
  price_increase_step: number
  rrp_percentage: number
  interval_minutes: number
  bulk_stock: number
  max_fetch_offers: number
  targets: Record<string, TargetConfig>
}

export interface FollowItem {
  url: string
  stock: number
  min_price: number
}

export interface RepriceHistoryRecord {
  id: number
  target_key: string
  tsin_id: string
  sku?: string
  image_url?: string
  title: string
  store_name?: string
  action?: string
  old_price: number
  new_price: number
  competitor_price: number
  reason: string
  created_at: string
}

export interface FollowHistoryRecord {
  id: number
  source_url: string
  tsin_id: string
  plid: string
  title: string
  stock: number
  min_price: number
  status: string
  message: string
  created_at: string
}

export interface BatchJobRecord {
  id: number
  batch_id: string
  action_type: string
  item_count: number
  status: string
  result_summary: string
  created_at: string
  updated_at: string
}

export interface BatchStatusResponse {
  batch_id: number | string
  status: {
    id: number
    description: string
  }
  results: Array<{
    offer?: {
      offer_id: number
      sku: string
      selling_price: number
      rrp: number
      status: string
    }
    validation_errors?: Array<{
      code: string
      message: string
    }>
  }>
}

export interface LeadtimeOrderItem {
  id: number
  order_id: number
  order_item_id: number
  order_date: string
  due_date: string
  remaining_seconds: number
  countdown_str: string
  is_overdue: boolean
  image_url: string
  title: string
  selling_price: number
  actual_weight: number
  volumetric_weight: number
  weigh_status: 'pending' | 'done' | string
  sku: string
  store_name: string
  tsin: string
  offer_id: string
  dc: string
  leadtime_stock: number
  demand_qty: number
  ship_qty: number
  status: string
}

export interface ShipmentItemRecord {
  id?: number
  shipment_id?: number
  order_id: number
  order_item_id: number
  order_date: string
  due_date: string
  tsin: string
  sku: string
  title: string
  image_url: string
  selling_price: number
  dc: string
  leadtime_stock: number
  demand_qty: number
  ship_qty: number
  actual_weight: number
  volumetric_weight: number
  weigh_status: 'pending' | 'done' | string
  created_at?: string
}

export interface ShipmentRecord {
  id: number
  shipment_number: string
  status: 'draft' | 'confirmed' | 'shipped' | 'delivered' | 'cancelled'
  destination_dc: string
  total_items: number
  total_units: number
  total_value: number
  notes: string
  items?: ShipmentItemRecord[]
  created_at: string
  updated_at: string
}

export interface BookingRecord {
  id: number
  booking_number: string
  shipment_id?: number
  shipment_no?: string
  destination_dc: string
  booking_date: string
  time_slot: string
  carrier_name: string
  vehicle_reg: string
  status: 'scheduled' | 'completed' | 'cancelled'
  notes: string
  created_at: string
}

