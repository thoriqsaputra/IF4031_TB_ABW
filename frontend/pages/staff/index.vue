<script setup>
const email = ref("");
const password = ref("");
const assignments = ref([]);
const error = ref("");
const authStatus = ref("");
const queueStatus = ref("");
const isSubmitting = ref(false);
const isLoading = ref(false);

const { token, loadToken, setToken, clearToken } = useAuth();

const formatAssignedAt = (value) => {
  if (!value) {
    return "unknown time";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "unknown time";
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
};

const fetchAssignments = async () => {
  if (!token.value) {
    assignments.value = [];
    queueStatus.value = "Sign in to view your assignments.";
    return;
  }

  error.value = "";
  queueStatus.value = "";
  isLoading.value = true;

  try {
    const response = await $fetch("/api/reports/assigned", {
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
    });
    assignments.value = response?.items || [];
    queueStatus.value = response?.count
      ? `${response.count} assignment${response.count === 1 ? "" : "s"} found`
      : "No assignments found";
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Failed to load assignments.";
  } finally {
    isLoading.value = false;
  }
};

const handleLogin = async () => {
  error.value = "";
  authStatus.value = "";
  isSubmitting.value = true;

  try {
    const response = await $fetch("/api/auth/login", {
      method: "POST",
      body: {
        email: email.value,
        password: password.value,
      },
    });

    if (!response?.token) {
      throw new Error("Missing token in response");
    }

    setToken(response.token);
    authStatus.value = "Signed in. Loading assignments...";
    await fetchAssignments();
  } catch (err) {
    error.value =
      err?.data?.error ||
      err?.data?.message ||
      err?.message ||
      "Login failed. Check your credentials.";
  } finally {
    isSubmitting.value = false;
  }
};

const handleRefresh = () => {
  fetchAssignments();
};

const handleSignOut = () => {
  clearToken();
  assignments.value = [];
  authStatus.value = "Signed out.";
  queueStatus.value = "Sign in to view your assignments.";
};

onMounted(() => {
  loadToken();
  if (token.value) {
    fetchAssignments();
  } else {
    queueStatus.value = "Sign in to view your assignments.";
  }
});

watch(token, (value) => {
  if (value) {
    fetchAssignments();
  }
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>Staff desk</h2>
      <span>Assignment queue</span>
    </div>
    <div class="hero staff-hero">
      <div>
        <p class="pill">Internal access</p>
        <h1>Review your assigned reports.</h1>
        <p>
          Sign in to see the reports routed to your department and stay on top
          of status updates.
        </p>
        <div class="stat-row">
          <div class="stat">Assignments: {{ assignments.length }}</div>
          <div class="stat">Secure staff access</div>
        </div>
      </div>
      <form v-if="!token" class="panel" @submit.prevent="handleLogin">
        <label for="staff-email">Email</label>
        <input
          id="staff-email"
          v-model="email"
          type="email"
          placeholder="staff@agarthan.io"
          autocomplete="email"
          required
        />
        <label for="staff-password">Password</label>
        <input
          id="staff-password"
          v-model="password"
          type="password"
          placeholder="Enter your passphrase"
          autocomplete="current-password"
          required
        />
        <button type="submit" :disabled="isSubmitting">
          {{ isSubmitting ? "Signing in..." : "Access assignments" }}
        </button>
        <p v-if="error" class="form-message error">{{ error }}</p>
        <p v-if="authStatus" class="form-message success">{{ authStatus }}</p>
      </form>
      <div v-else class="panel staff-actions">
        <h3>Staff access confirmed</h3>
        <p class="helper">Refresh your queue or sign out.</p>
        <div class="button-row">
          <button type="button" :disabled="isLoading" @click="handleRefresh">
            {{ isLoading ? "Refreshing..." : "Refresh assignments" }}
          </button>
          <button type="button" class="ghost-button" @click="handleSignOut">
            Sign out
          </button>
        </div>
        <p v-if="error" class="form-message error">{{ error }}</p>
        <p v-if="authStatus" class="form-message success">{{ authStatus }}</p>
      </div>
    </div>

    <div class="section-title" style="margin-top: 32px;">
      <h2>Assigned reports</h2>
      <span>{{ assignments.length }} active</span>
    </div>
    <p>These reports are currently assigned to you for review.</p>
    <p v-if="queueStatus" class="helper">{{ queueStatus }}</p>
    <div v-if="error" class="panel">
      <p>{{ error }}</p>
    </div>
    <div class="grid-list">
      <NuxtLink
        v-for="(assignment, index) in assignments"
        :key="assignment.report_id || index"
        class="card"
        :style="{ '--i': index }"
        :to="`/details/${assignment.report_id}`"
      >
        <div class="meta">
          <span class="pill">{{ assignment.status || "Assigned" }}</span>
          <span>Assigned {{ formatAssignedAt(assignment.assigned_at) }}</span>
        </div>
        <h3>{{ assignment.report_title }}</h3>
        <p>{{ assignment.report_description }}</p>
        <div class="meta">
          <span>by {{ assignment.poster_name || "Unknown" }}</span>
          <span v-if="assignment.severity">
            Severity: {{ assignment.severity }}
          </span>
          <span v-if="assignment.location">Location: {{ assignment.location }}</span>
        </div>
      </NuxtLink>
      <div v-if="!isLoading && token && !assignments.length" class="panel empty-state">
        <p>No assigned reports yet.</p>
      </div>
    </div>
  </section>
</template>
