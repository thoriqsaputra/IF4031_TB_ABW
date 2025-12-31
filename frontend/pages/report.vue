<script setup>
const { token, loadToken } = useAuth();

const form = reactive({
  title: "",
  description: "",
  location: "",
  severity: "medium",
  report_categories_id: 1,
  is_public: true,
  is_anon: false,
});

const error = ref("");
const status = ref("");
const isSubmitting = ref(false);
const isWaiting = ref(false);
const requestId = ref("");
const eventSource = ref(null);

const closeStream = () => {
  if (eventSource.value) {
    eventSource.value.close();
    eventSource.value = null;
  }
};

const listenForCompletion = (id) => {
  if (!process.client) {
    return;
  }
  closeStream();
  const source = new EventSource(
    `/api/notifications/stream?request_id=${encodeURIComponent(id)}`
  );
  eventSource.value = source;

  source.addEventListener("report", (event) => {
    try {
      const payload = JSON.parse(event.data || "{}");
      const result = payload.event?.status || "success";
      const message = payload.event?.message || "Report processed.";
      if (result === "error") {
        error.value = message;
        status.value = "";
      } else {
        status.value = message;
      }
    } catch (err) {
      status.value = "Report processed.";
    }
    isWaiting.value = false;
    isSubmitting.value = false;
    closeStream();
  });

  source.addEventListener("timeout", () => {
    status.value = "Still queued. Refresh later to confirm status.";
    isWaiting.value = false;
    isSubmitting.value = false;
    closeStream();
  });

  source.onerror = () => {
    error.value = "Connection lost while waiting for confirmation.";
    isWaiting.value = false;
    isSubmitting.value = false;
    closeStream();
  };
};

const submitReport = async () => {
  error.value = "";
  status.value = "";

  if (!token.value) {
    error.value = "Sign in to submit a report.";
    return;
  }

  if (!form.title.trim() || !form.description.trim()) {
    error.value = "Title and description are required.";
    return;
  }

  isSubmitting.value = true;
  isWaiting.value = true;

  try {
    const response = await $fetch("/api/reports", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
      body: {
        title: form.title,
        description: form.description,
        location: form.location,
        severity: form.severity,
        report_categories_id: Number(form.report_categories_id) || 1,
        is_public: form.is_public,
        is_anon: form.is_anon,
      },
    });

    requestId.value = response?.request_id || "";
    if (!requestId.value) {
      throw new Error("Missing request id");
    }
    status.value = "Report submitted. Waiting for confirmation...";
    listenForCompletion(requestId.value);
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Failed to submit report.";
    isWaiting.value = false;
    isSubmitting.value = false;
  }
};

onMounted(() => {
  loadToken();
});

onBeforeUnmount(() => {
  closeStream();
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>Submit report</h2>
      <span>Send a new field entry</span>
    </div>
    <p>
      Reports are queued and stored asynchronously. You will see a confirmation
      as soon as the report is written to the database.
    </p>
    <form class="panel report-form" @submit.prevent="submitReport">
      <div class="form-grid">
        <div>
          <label for="title">Title</label>
          <input id="title" v-model="form.title" type="text" required />
        </div>
        <div>
          <label for="category">Category ID</label>
          <input
            id="category"
            v-model="form.report_categories_id"
            type="number"
            min="1"
          />
        </div>
        <div>
          <label for="location">Location</label>
          <input id="location" v-model="form.location" type="text" />
        </div>
        <div>
          <label for="severity">Severity</label>
          <select id="severity" v-model="form.severity">
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
          </select>
        </div>
      </div>
      <label for="description">Description</label>
      <textarea
        id="description"
        v-model="form.description"
        rows="5"
        required
      ></textarea>
      <div class="toggle-row">
        <label class="toggle">
          <input v-model="form.is_public" type="checkbox" />
          <span>Public report</span>
        </label>
        <label class="toggle">
          <input v-model="form.is_anon" type="checkbox" />
          <span>Anonymous</span>
        </label>
      </div>
      <button type="submit" :disabled="isSubmitting || !token">
        {{ isSubmitting ? "Submitting..." : "Send report" }}
      </button>
      <p v-if="error" class="form-message error">{{ error }}</p>
      <p v-if="status" class="form-message success">{{ status }}</p>
      <p v-if="!token" class="form-message error">
        Please sign in before submitting a report.
      </p>
    </form>

    <div v-if="isWaiting" class="overlay">
      <div class="spinner"></div>
      <p>Waiting for confirmation...</p>
    </div>
  </section>
</template>
