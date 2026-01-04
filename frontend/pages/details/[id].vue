<script setup>
definePageMeta({
  middleware: 'auth'
});

const route = useRoute();
const { token, loadToken } = useAuth();

const reportId = computed(() => String(route.params.id || ""));
const report = ref(null);
const media = ref([]);
const error = ref("");
const isLoading = ref(false);
const isLoadingMedia = ref(false);

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

    // Fetch media for this report
    await fetchMedia();
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Unable to load report details.";
    report.value = null;
  } finally {
    isLoading.value = false;
  }
};

const fetchMedia = async () => {
  if (!reportId.value) return;

  isLoadingMedia.value = true;
  try {
    const response = await $fetch(`/reports/${reportId.value}/media`);
    media.value = response.media || [];
  } catch (err) {
    console.error('Failed to load media:', err);
    media.value = [];
  } finally {
    isLoadingMedia.value = false;
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
    <!-- Media Gallery -->
    <div v-if="report && media.length > 0" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2>Media Attachments</h2>
        <span>{{ media.length }} file(s)</span>
      </div>
      <div class="media-gallery">
        <div v-for="item in media" :key="item.report_media_id" class="media-item">
          <img
            v-if="item.media_type === 'image'"
            :src="item.url"
            :alt="'Report media ' + item.report_media_id"
            loading="lazy"
            style="width: 100%; height: auto; border-radius: 12px; cursor: pointer;"
            @click="window.open(item.url, '_blank')"
          />
          <video
            v-else-if="item.media_type === 'video'"
            :src="item.url"
            controls
            style="width: 100%; height: auto; border-radius: 12px;"
          />
          <p style="font-size: 0.85rem; color: #666; margin-top: 0.5rem;">
            Uploaded: {{ formatTimestamp(item.created_at) }}
          </p>
        </div>
      </div>
    </div>
    <div v-else-if="report && !isLoadingMedia" class="panel" style="margin-top: 18px;">
      <p style="color: #666; text-align: center; padding: 1rem;">No media attachments for this report.</p>
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
