<script setup>
import { ShieldExclamationIcon, ArrowLeftIcon } from '@heroicons/vue/24/outline'

const { token } = useAuth()
const { userRole } = useUser()

const goBack = () => {
  if (userRole.value === 'government' || userRole.value === 'admin') {
    navigateTo('/staff/dashboard')
  } else if (token.value) {
    navigateTo('/feed')
  } else {
    navigateTo('/')
  }
}
</script>

<template>
  <div class="error-page">
    <div class="error-content">
      <div class="error-icon-wrapper">
        <ShieldExclamationIcon class="error-icon" />
      </div>
      <h1 class="error-title">403 - Access Denied</h1>
      <p class="error-message">
        You don't have permission to access this resource.
      </p>
      <p class="error-details">
        This page is restricted to specific user roles. Please contact your administrator if you believe this is an error.
      </p>
      <button @click="goBack" class="btn-primary error-btn">
        <ArrowLeftIcon class="btn-icon" />
        <span>Go to Dashboard</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.error-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 70vh;
  padding: 2rem;
}

.error-content {
  text-align: center;
  max-width: 500px;
}

.error-icon-wrapper {
  display: inline-flex;
  padding: 1.5rem;
  background: var(--error-100);
  border-radius: var(--radius-full);
  margin-bottom: 1.5rem;
}

.error-icon {
  width: 4rem;
  height: 4rem;
  color: var(--error-600);
  animation: pulse 2s ease-in-out infinite;
}

.error-title {
  font-size: 2.5rem;
  font-weight: 700;
  margin-bottom: 1rem;
  color: var(--error-600);
}

.error-message {
  font-size: 1.25rem;
  margin-bottom: 1rem;
  color: var(--text-secondary);
}

.error-details {
  font-size: 1rem;
  margin-bottom: 2rem;
  color: var(--text-muted);
}

.error-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

@keyframes pulse {
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.05);
    opacity: 0.8;
  }
}
</style>
