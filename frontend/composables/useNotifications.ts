import type { Notification, NotificationStats } from '~/types'

export const useNotifications = () => {
  const { token } = useAuth()
  const { decodeToken } = useUser()

  const notifications = useState<Notification[]>('notifications', () => [])
  const unreadCount = computed(() =>
    notifications.value.filter(n => !n.is_read).length
  )

  const ws = ref<WebSocket | null>(null)
  const isConnected = ref(false)

  // Fetch notifications from REST API
  const fetchNotifications = async (userId: number) => {
    try {
      const response = await $fetch<NotificationStats>(
        `/api/notifications?user_id=${userId}&limit=50`
      )
      notifications.value = response.notifications
    } catch (error) {
      console.error('Failed to fetch notifications:', error)
    }
  }

  // Connect to WebSocket for real-time updates
  const connectWebSocket = (userId: number) => {
    if (ws.value?.readyState === WebSocket.OPEN) return

    // Construct WebSocket URL based on current location
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws?user_id=${userId}`

    ws.value = new WebSocket(wsUrl)

    ws.value.onopen = () => {
      isConnected.value = true
      console.log('WebSocket connected')
    }

    ws.value.onmessage = (event) => {
      try {
        const notification = JSON.parse(event.data) as Notification
        notifications.value.unshift(notification)
      } catch (error) {
        console.error('Failed to parse notification:', error)
      }
    }

    ws.value.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    ws.value.onclose = () => {
      isConnected.value = false
      console.log('WebSocket disconnected')
      // Reconnect after 5 seconds
      setTimeout(() => connectWebSocket(userId), 5000)
    }
  }

  // Disconnect WebSocket
  const disconnectWebSocket = () => {
    if (ws.value) {
      ws.value.close()
      ws.value = null
      isConnected.value = false
    }
  }

  // Mark notification as read
  const markAsRead = async (notificationId: number) => {
    try {
      await $fetch(
        `/api/notifications/${notificationId}/read`,
        {
          method: 'PUT',
          headers: token.value ? {
            Authorization: `Bearer ${token.value}`
          } : undefined
        }
      )

      const index = notifications.value.findIndex(
        n => n.notification_id === notificationId
      )
      if (index !== -1) {
        notifications.value[index].is_read = true
      }
    } catch (error) {
      console.error('Failed to mark as read:', error)
    }
  }

  return {
    notifications,
    unreadCount,
    isConnected,
    fetchNotifications,
    connectWebSocket,
    disconnectWebSocket,
    markAsRead
  }
}
