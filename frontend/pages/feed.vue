<script setup>
const query = ref("");
const reports = ref([]);
const error = ref("");
const status = ref("");
const isLoading = ref(false);

const fetchReports = async (searchTerm = "") => {
  error.value = "";
  status.value = "";
  isLoading.value = true;

  try {
    const response = await $fetch("/api/searchbar_public", {
      params: searchTerm ? { q: searchTerm } : undefined,
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
  fetchReports();
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>Public feed</h2>
      <span>Live, shared field notes</span>
    </div>
    <p>
      Every report here is visible to the wider network, including your own
      published entries.
    </p>
    <form class="search-bar" @submit.prevent="handleSearch">
      <input
        v-model="query"
        type="search"
        placeholder="Search public reports"
        aria-label="Search public reports"
      />
      <button type="submit" :disabled="isLoading">
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
          <span class="pill">Public</span>
          <span>by {{ report.poster_name }}</span>
        </div>
        <h3>{{ report.report_title }}</h3>
        <p>{{ report.report_description }}</p>
      </NuxtLink>
      <div v-if="!isLoading && !reports.length" class="panel empty-state">
        <p>No public reports match that search yet.</p>
      </div>
    </div>
  </section>
</template>
