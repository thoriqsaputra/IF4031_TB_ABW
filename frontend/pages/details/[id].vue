<script setup>
const route = useRoute();
const { token, loadToken } = useAuth();

const reportId = computed(() => String(route.params.id || ""));
const report = ref(null);
const error = ref("");
const isLoading = ref(false);

const formatTimestamp = (value) => {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
};

const fetchReport = async () => {
  if (!reportId.value) {
    error.value = "Invalid report id.";
    report.value = null;
    return;
  }
  if (!token.value) {
    error.value = "Sign in to view report details.";
    report.value = null;
    return;
  }

  error.value = "";
  isLoading.value = true;

  try {
    const response = await $fetch(`/api/reports/${reportId.value}`, {
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
    });
    report.value = response;
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Unable to load report details.";
    report.value = null;
  } finally {
    isLoading.value = false;
  }
};

onMounted(() => {
  loadToken();
  fetchReport();
});

watch([token, reportId], () => {
  fetchReport();
});
</script>

<template>
  <section class="page">
    <div v-if="report" class="detail-hero">
      <p class="pill">{{ report.is_public ? "Public" : "Private" }}</p>
      <h1>{{ report.title }}</h1>
      <div class="stat-row">
        <div class="stat">Location: {{ report.location || "-" }}</div>
        <div class="stat">Severity: {{ report.severity || "-" }}</div>
        <div class="stat">Created: {{ formatTimestamp(report.created_at) }}</div>
      </div>
      <p>{{ report.description }}</p>
    </div>
    <div v-else class="panel">
      <h2>{{ isLoading ? "Loading report..." : "Report unavailable" }}</h2>
      <p>{{ error || "The report you requested is not available yet." }}</p>
    </div>
    <div v-if="report" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2>Report metadata</h2>
        <span>Internal reference</span>
      </div>
      <ul>
        <li>Report ID: {{ report.report_id }}</li>
        <li>User ID: {{ report.user_id }}</li>
        <li>Category ID: {{ report.report_categories_id }}</li>
        <li>Anonymous: {{ report.is_anon ? "Yes" : "No" }}</li>
      </ul>
    </div>
    <div class="link-row" style="margin-top: 18px;">
      <NuxtLink class="ghost-link" to="/feed">Back to feed</NuxtLink>
      <NuxtLink class="ghost-link" to="/user_reports">My reports</NuxtLink>
    </div>
  </section>
</template>
