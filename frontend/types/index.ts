export interface User {
  user_id: number
  email: string
  name: string
  role_id: number
  department_id: number
  role: {
    role_id: number
    name: string  // "citizen" | "government" | "admin"
  }
  department: {
    department_id: number
    name: string
  }
}

export interface DecodedToken {
  user_id: number
  role: string
  exp: number
}

export interface MediaUploadResponse {
  report_media_id: number
  object_key: string
  media_type: string
  url: string
  job_id?: number
}

export interface Notification {
  notification_id: number
  user_id: number
  type: string
  title: string
  message: string
  is_read: boolean
  created_at: string
  report_id?: number
}

export interface NotificationStats {
  notifications: Notification[]
  total_notifications: number
  unread_notifications: number
}

export interface AnalyticsData {
  reportsByCategory: { category: string; count: number }[]
  reportsBySeverity: { severity: string; count: number }[]
  kpis: {
    total: number
    completed: number
    pending: number
    inProgress: number
  }
  recentActivity: {
    title: string
    timestamp: string
    type: string
  }[]
}
