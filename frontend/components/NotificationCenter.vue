<template>
  <div class="notification-center">
    <button
      class="notification-bell"
      @click="toggleDropdown"
      aria-label="Notifications"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
        <path d="M13.73 21a2 2 0 0 1-3.46 0" />
      </svg>
      <span v-if="unreadCount > 0" class="badge">{{ unreadCount > 99 ? '99+' : unreadCount }}</span>
    </button>

    <Transition name="dropdown">
      <div v-if="isOpen" class="notification-dropdown">
        <div class="dropdown-header">
          <h3>Notifications</h3>
          <span class="unread-count">{{ unreadCount }} unread</span>
        </div>

        <div class="notification-list" ref="listRef">
          <div
            v-for="notification in filteredNotifications"
            :key="notification.notification_id"
            class="notification-item"
            :class="{ unread: !notification.is_read }"
            @click="handleNotificationClick(notification)"
          >
            <div class="notification-icon" :class="getNotificationClass(notification.type)">
              <span>{{ getNotificationIcon(notification.type) }}</span>
            </div>
            <div class="notification-content">
              <div class="notification-title">{{ notification.title }}</div>
              <div class="notification-message">{{ notification.message }}</div>
              <div class="notification-time">{{ formatTime(notification.created_at) }}</div>
            </div>
            <div v-if="!notification.is_read" class="unread-indicator"></div>
          </div>

          <div v-if="filteredNotifications.length === 0" class="empty-state">
            <p>No notifications yet</p>
          </div>
        </div>

        <div class="dropdown-footer">
          <button @click="viewAll" class="view-all-btn">View All Notifications</button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import type { Notification } from '~/types'
import { onClickOutside } from '@vueuse/core'

const { notifications, unreadCount, fetchNotifications, connectWebSocket, disconnectWebSocket, markAsRead } = useNotifications()
const { userInfo, userRole } = useUser()
const { decodeToken } = useUser()

const isOpen = ref(false)
const listRef = ref<HTMLElement | null>(null)

// Filter notifications based on role
const filteredNotifications = computed(() => {
  if (!userRole.value) return notifications.value

  if (userRole.value === 'citizen') {
    // Citizens see only their own report notifications
    return notifications.value.filter(n => n.user_id === userInfo.value?.user_id)
  }

  if (userRole.value === 'government') {
    // government users see new reports in their department
    // This requires backend to send department-scoped notifications
    return notifications.value
  }

  // admin sees all
  return notifications.value
})

const toggleDropdown = () => {
  isOpen.value = !isOpen.value
}

const handleNotificationClick = async (notification: Notification) => {
  if (!notification.is_read) {
    await markAsRead(notification.notification_id)
  }

  // Navigate to report detail if report_id exists
  if (notification.report_id) {
    await navigateTo(`/details/${notification.report_id}`)
    isOpen.value = false
  }
}

const getNotificationClass = (type: string): string => {
  if (type.includes('success') || type === 'media_completed') return 'success'
  if (type.includes('error') || type.includes('failed')) return 'error'
  if (type === 'new_report') return 'info'
  return 'default'
}

const getNotificationIcon = (type: string): string => {
  if (type.includes('success')) return '✓'
  if (type.includes('error')) return '✕'
  if (type === 'new_report') return '📄'
  if (type.includes('media')) return '🖼'
  return '•'
}

const formatTime = (timestamp: string): string => {
  const now = new Date()
  const time = new Date(timestamp)
  const diff = now.getTime() - time.getTime()

  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return 'Just now'
  if (minutes < 60) return `${minutes}m ago`
  if (hours < 24) return `${hours}h ago`
  if (days < 7) return `${days}d ago`

  return time.toLocaleDateString()
}

const viewAll = () => {
  // Navigate to dedicated notifications page (future feature)
  isOpen.value = false
}

// Initialize
onMounted(async () => {
  const decoded = decodeToken()
  if (decoded?.user_id) {
    await fetchNotifications(decoded.user_id)
    connectWebSocket(decoded.user_id)
  }
})

onUnmounted(() => {
  disconnectWebSocket()
})

// Close dropdown when clicking outside
onClickOutside(listRef, () => {
  isOpen.value = false
})
</script>

<style scoped>
.notification-center {
  position: relative;
}

.notification-bell {
  position: relative;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.5rem;
  color: var(--ink);
  transition: color 0.2s;
}

.notification-bell:hover {
  color: var(--sea);
}

.notification-bell svg {
  width: 24px;
  height: 24px;
  stroke-width: 2;
}

.badge {
  position: absolute;
  top: 0;
  right: 0;
  background: var(--sunset);
  color: white;
  border-radius: 10px;
  padding: 0.125rem 0.375rem;
  font-size: 0.75rem;
  font-weight: 700;
  min-width: 18px;
  text-align: center;
}

.notification-dropdown {
  position: absolute;
  top: calc(100% + 0.5rem);
  right: 0;
  width: 400px;
  max-width: 90vw;
  background: white;
  border: 1px solid #ddd;
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
  z-index: 1000;
  overflow: hidden;
}

.dropdown-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid #eee;
}

.dropdown-header h3 {
  font-family: 'Fraunces', serif;
  font-size: 1.25rem;
  margin: 0;
  color: var(--ink);
}

.unread-count {
  font-size: 0.85rem;
  color: var(--sea);
  font-weight: 600;
}

.notification-list {
  max-height: 400px;
  overflow-y: auto;
}

.notification-item {
  display: flex;
  gap: 1rem;
  padding: 1rem 1.25rem;
  cursor: pointer;
  transition: background 0.2s;
  border-bottom: 1px solid #f5f5f5;
  position: relative;
}

.notification-item:hover {
  background: #fafafa;
}

.notification-item.unread {
  background: rgba(138, 209, 193, 0.05);
}

.notification-icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.25rem;
}

.notification-icon.success {
  background: rgba(138, 209, 193, 0.2);
  color: var(--sea);
}

.notification-icon.error {
  background: rgba(243, 168, 124, 0.2);
  color: var(--sunset);
}

.notification-icon.info {
  background: rgba(247, 213, 138, 0.2);
  color: var(--glow);
}

.notification-icon.default {
  background: #f0f0f0;
  color: #666;
}

.notification-content {
  flex: 1;
  min-width: 0;
}

.notification-title {
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--ink);
  margin-bottom: 0.25rem;
}

.notification-message {
  font-size: 0.85rem;
  color: #666;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.notification-time {
  font-size: 0.75rem;
  color: #999;
  margin-top: 0.25rem;
}

.unread-indicator {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--sea);
}

.empty-state {
  padding: 3rem 2rem;
  text-align: center;
  color: #999;
}

.empty-state p {
  margin: 0;
  font-size: 0.95rem;
}

.dropdown-footer {
  padding: 0.75rem 1.25rem;
  border-top: 1px solid #eee;
  background: #fafafa;
}

.view-all-btn {
  width: 100%;
  padding: 0.5rem;
  background: none;
  border: none;
  color: var(--sea);
  font-weight: 600;
  cursor: pointer;
  transition: color 0.2s;
}

.view-all-btn:hover {
  color: #7ac0b0;
}

/* Transition */
.dropdown-enter-active, .dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from, .dropdown-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

@media (max-width: 768px) {
  .notification-dropdown {
    width: 100vw;
    left: 50%;
    right: auto;
    transform: translateX(-50%);
    border-radius: 0;
  }
}
</style>
