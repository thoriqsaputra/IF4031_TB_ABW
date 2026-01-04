<script setup>
import { Squares2X2Icon, BellIcon, ArrowRightOnRectangleIcon } from '@heroicons/vue/24/outline'

const { token, loadToken, clearToken } = useAuth();
const { userRole, fetchUserProfile, isLoadingProfile } = useUser();
const router = useRouter();
const showLogoutDialog = ref(false);
const isLoggingOut = ref(false);

const isStaff = computed(() => userRole.value === 'government' || userRole.value === 'admin');
const isCitizen = computed(() => userRole.value === 'citizen');

// Determine theme class based on role
const themeClass = computed(() => {
  if (userRole.value === 'government' || userRole.value === 'admin') {
    return 'staff-theme';
  } else if (userRole.value === 'citizen') {
    return 'citizen-theme';
  }
  return '';
});

const openLogoutDialog = () => {
  showLogoutDialog.value = true;
};

const cancelLogout = () => {
  showLogoutDialog.value = false;
};

const confirmLogout = async () => {
  isLoggingOut.value = true;
  try {
    if (token.value) {
      await $fetch("/api/auth/logout", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token.value}`
        }
      });
    }
  } catch (err) {
    console.error("Logout error:", err);
  } finally {
    clearToken();
    showLogoutDialog.value = false;
    isLoggingOut.value = false;
    await router.push("/");
  }
};

</script>

<template>
  <div class="app" :class="themeClass">
    <!-- Loading Overlay During Profile Fetch -->
    <Transition name="fade">
      <div v-if="token && isLoadingProfile" class="loading-overlay">
        <div class="loading-spinner"></div>
      </div>
    </Transition>

    <!-- Modern Navigation Bar -->
    <header class="topbar" :class="themeClass">
      <div class="container">
        <NuxtLink class="logo" to="/">
          <Squares2X2Icon class="logo-icon" />
          <span>Agarthan Reports</span>
        </NuxtLink>
        
        <nav class="nav">
          <!-- Public / Guest Navigation -->
          <template v-if="!token">
            <NuxtLink to="/feed">Public Feed</NuxtLink>
          </template>
          
          <!-- Citizen Navigation -->
          <template v-if="isCitizen">
            <NuxtLink to="/feed">Feed</NuxtLink>
            <NuxtLink to="/report">Submit Report</NuxtLink>
            <NuxtLink to="/user_reports">My Reports</NuxtLink>
          </template>
          
          <!-- Staff Navigation (Government & Admin) -->
          <template v-if="isStaff">
            <NuxtLink to="/staff">Assigned Reports</NuxtLink>
            <NuxtLink to="/staff/dashboard">Dashboard</NuxtLink>
            <NuxtLink to="/feed">Public Feed</NuxtLink>
          </template>
        </nav>
        
        <div class="header-actions">
          <NotificationCenter v-if="token" />
          <button v-if="token" @click="openLogoutDialog" class="logout-btn">
            <ArrowRightOnRectangleIcon class="btn-icon" />
            <span>Logout</span>
          </button>
          <NuxtLink v-if="!token" to="/" class="btn-ghost login-btn">
            Login
          </NuxtLink>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <main class="content">
      <NuxtPage />
    </main>

    <!-- Logout Confirmation Dialog -->
    <Teleport to="body">
      <Transition name="dialog-fade">
        <div v-if="showLogoutDialog" class="dialog-overlay" @click.self="cancelLogout">
          <div class="dialog-box">
            <div class="dialog-header">
              <h3>Confirm Logout</h3>
            </div>
            <div class="dialog-body">
              <p>Are you sure you want to log out of your account?</p>
            </div>
            <div class="dialog-actions">
              <button 
                @click="cancelLogout" 
                class="dialog-btn dialog-btn-secondary"
                :disabled="isLoggingOut"
              >
                Cancel
              </button>
              <button 
                @click="confirmLogout" 
                class="dialog-btn dialog-btn-primary"
                :disabled="isLoggingOut"
              >
                {{ isLoggingOut ? "Logging out..." : "Logout" }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style>
/* Dialog Transition Animations */
.dialog-fade-enter-active,
.dialog-fade-leave-active {
  transition: opacity 0.25s ease;
}

.dialog-fade-enter-from,
.dialog-fade-leave-to {
  opacity: 0;
}

.dialog-fade-enter-active .dialog-box,
.dialog-fade-enter-active .modal {
  animation: dialog-bounce 0.3s ease;
}

.dialog-fade-leave-active .dialog-box,
.dialog-fade-leave-active .modal {
  animation: dialog-bounce-out 0.2s ease;
}

@keyframes dialog-bounce {
  0% {
    transform: scale(0.9);
    opacity: 0;
  }
  50% {
    transform: scale(1.02);
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

@keyframes dialog-bounce-out {
  0% {
    transform: scale(1);
    opacity: 1;
  }
  100% {
    transform: scale(0.95);
    opacity: 0;
  }
}

@keyframes fade-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes rise {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Loading Overlay */
.loading-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(255, 255, 255, 0.9);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 500;
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #e0e0e0;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>

