<script setup>
const query = ref("");
const reports = ref([]);
const error = ref("");
const status = ref("");
const isLoading = ref(false);

const { token, loadToken } = useAuth();

const fetchReports = async (searchTerm = "") => {
  if (!token.value) {
    status.value = "Sign in to view your reports.";
    reports.value = [];
    return;
  }

  error.value = "";
  status.value = "";
  isLoading.value = true;

  try {
    const response = await $fetch("/api/searchbar_self", {
      params: searchTerm ? { q: searchTerm } : undefined,
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
    });
    reports.value = response?.items || [];
    status.value = response?.count
      ? `${response.count} report${response.count === 1 ? "" : "s"} found`
      : "No reports found";
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Failed to load reports.";
  } finally {
    isLoading.value = false;
  }
};

const handleSearch = () => {
  fetchReports(query.value.trim());
};

onMounted(() => {
  loadToken();
  if (token.value) {
    fetchReports();
  } else {
    status.value = "Sign in to view your reports.";
  }
});

watch(token, (value) => {
  if (value) {
    fetchReports(query.value.trim());
  }
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>My reports</h2>
      <span>{{ reports.length }} total entries</span>
    </div>
    <p>Draft, edit, and review your published notes in one place.</p>
    <form class="search-bar" @submit.prevent="handleSearch">
      <input
        v-model="query"
        type="search"
        placeholder="Search my reports"
        aria-label="Search my reports"
        :disabled="!token"
      />
      <button type="submit" :disabled="isLoading || !token">
        {{ isLoading ? "Searching..." : "Search" }}
      </button>
    </form>
    <p v-if="status" class="helper">{{ status }}</p>
    <div v-if="error" class="panel">
      <p>{{ error }}</p>
    </div>
    <div class="grid-list">
      <NuxtLink
        v-for="(report, index) in reports"
        :key="report.report_id || index"
        class="card"
        :style="{ '--i': index }"
        :to="`/details/${report.report_id}`"
      >
        <div class="meta">
          <span class="pill">{{ report.is_public ? "Public" : "Private" }}</span>
          <span>by {{ report.poster_name }}</span>
        </div>
        <h3>{{ report.report_title }}</h3>
        <p>{{ report.report_description }}</p>
      </NuxtLink>
      <div v-if="!isLoading && token && !reports.length" class="panel empty-state">
        <p>No personal reports match that search yet.</p>
      </div>
    </div>
  </section>
</template>
