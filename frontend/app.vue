<script setup>
const { token, loadToken, clearToken } = useAuth();
const router = useRouter();
const showLogoutDialog = ref(false);
const isLoggingOut = ref(false);

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

onMounted(() => {
  loadToken();
});
</script>

<template>
  <div class="app">
    <div class="bg">
      <span class="orb orb-one"></span>
      <span class="orb orb-two"></span>
      <span class="grid"></span>
    </div>
    <header class="topbar">
      <NuxtLink class="logo" to="/">Agarthan Reports</NuxtLink>
      <nav class="nav">
        <NuxtLink to="/feed">Feed</NuxtLink>
        <NuxtLink to="/user_reports">My Reports</NuxtLink>
        <NuxtLink to="/report">Submit Report</NuxtLink>
      </nav>
      <div class="header-actions">
        <NotificationCenter />
        <button v-if="token" @click="openLogoutDialog" class="logout-btn">Logout</button>
      </div>
    </header>
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
