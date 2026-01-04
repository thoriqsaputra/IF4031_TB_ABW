<script setup>
definePageMeta({
  middleware: 'auth'
});

const route = useRoute();
const { token, loadToken } = useAuth();

const reportId = computed(() => String(route.params.id || ""));
const report = ref(null);
const latestResponse = ref(null);
const reportStatus = ref("");

const loadError = ref("");
const submitError = ref("");
const submitStatus = ref("");
const isLoading = ref(false);
const isSubmitting = ref(false);

const editorRef = ref(null);
const quillInstance = ref(null);
const editorHtml = ref("");
const editorText = ref("");

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

const syncEditor = () => {
  const quill = quillInstance.value;
  if (!quill) {
    editorHtml.value = "";
    editorText.value = "";
    return;
  }

  editorHtml.value = quill.root?.innerHTML || "";
  editorText.value = quill.getText() || "";
};

const cleanText = computed(() => editorText.value.replace(/\s+/g, " ").trim());

const wordCount = computed(() => {
  if (!cleanText.value) {
    return 0;
  }
  return cleanText.value.split(" ").length;
});

const charCount = computed(() => editorText.value.trim().length);

const clearEditor = () => {
  const quill = quillInstance.value;
  if (!quill) {
    return;
  }
  quill.setText("");
  syncEditor();
};

const initEditor = async () => {
  if (!process.client || quillInstance.value || !editorRef.value) {
    return;
  }

  const quillModule = await import("quill");
  const Quill = quillModule.default || quillModule;

  quillInstance.value = new Quill(editorRef.value, {
    theme: "snow",
    placeholder: "Write a clear, structured response...",
    modules: {
      toolbar: [
        [{ header: [1, 2, 3, false] }],
        ["bold", "italic", "underline", "strike"],
        [{ list: "ordered" }, { list: "bullet" }],
        ["link", "blockquote"],
        [{ align: [] }],
        ["clean"],
      ],
    },
  });

  quillInstance.value.on("text-change", syncEditor);
  syncEditor();
};

const fetchReportStatus = async () => {
  const id = Number(reportId.value);
  if (!token.value || !Number.isFinite(id) || id <= 0) {
    return;
  }

  try {
    const response = await $fetch(`/api/reports/${id}/status`, {
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
    });
    reportStatus.value = response?.status || "";
    latestResponse.value = response?.latest_response || null;
  } catch {
    reportStatus.value = "";
    latestResponse.value = null;
  }
};

const fetchReport = async () => {
  const id = Number(reportId.value);
  if (!Number.isFinite(id) || id <= 0) {
    loadError.value = "Invalid report id.";
    report.value = null;
    return;
  }
  if (!token.value) {
    loadError.value = "Sign in to view assigned report details.";
    report.value = null;
    return;
  }

  loadError.value = "";
  isLoading.value = true;

  try {
    const response = await $fetch(`/api/reports/${id}`, {
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
    });
    report.value = response;
    await fetchReportStatus();
  } catch (err) {
    loadError.value =
      err?.data?.error || err?.data?.message || "Unable to load report details.";
    report.value = null;
  } finally {
    isLoading.value = false;
  }
};

const handleSubmit = async () => {
  submitError.value = "";
  submitStatus.value = "";

  if (!token.value) {
    submitError.value = "Sign in to respond to this report.";
    return;
  }
  const id = Number(reportId.value);
  if (!Number.isFinite(id) || id <= 0) {
    submitError.value = "Invalid report id.";
    return;
  }

  if (!cleanText.value) {
    submitError.value = "Response cannot be empty.";
    return;
  }

  isSubmitting.value = true;

  try {
    const response = await $fetch("/api/report_response", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token.value}`,
      },
      body: {
        report_id: id,
        message: editorHtml.value,
      },
    });
    const requestId = response?.request_id ? ` (request ${response.request_id})` : "";
    submitStatus.value = `Response queued${requestId}.`;
    clearEditor();
    await fetchReportStatus();
  } catch (err) {
    submitError.value =
      err?.data?.error || err?.data?.message || "Failed to send response.";
  } finally {
    isSubmitting.value = false;
  }
};

onMounted(() => {
  loadToken();
  fetchReport();
});

watch([token, reportId], () => {
  fetchReport();
});

watch(report, async (value) => {
  if (!value) {
    return;
  }
  await nextTick();
  initEditor();
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>Assigned report</h2>
      <span>Staff response desk</span>
    </div>

    <div v-if="report" class="response-layout">
      <div class="panel">
        <div class="detail-hero" style="margin-bottom: 0;">
          <p class="pill">Assigned</p>
          <h1>{{ report.title }}</h1>
          <div class="stat-row">
            <div class="stat">Location: {{ report.location || "-" }}</div>
            <div class="stat">Severity: {{ report.severity || "-" }}</div>
            <div class="stat">Created: {{ formatTimestamp(report.created_at) }}</div>
          </div>
          <p>{{ report.description }}</p>
        </div>
      </div>

      <div class="panel editor-shell">
        <div class="section-title">
          <h2>Draft response</h2>
          <span>Rich text editor</span>
        </div>
        <p class="helper">
          Compose a response for the reporter. Formatting will be preserved in the stored message.
        </p>
        <div
          ref="editorRef"
          class="editor-area"
        ></div>
        <div class="editor-meta">
          <span>{{ wordCount }} word{{ wordCount === 1 ? "" : "s" }}</span>
          <span>{{ charCount }} characters</span>
        </div>
        <div class="button-row" style="margin-top: 16px;">
          <button type="button" :disabled="isSubmitting" @click="handleSubmit">
            {{ isSubmitting ? "Sending..." : "Send response" }}
          </button>
          <button type="button" class="ghost-button" :disabled="isSubmitting" @click="clearEditor">
            Clear draft
          </button>
        </div>
        <p v-if="submitError" class="form-message error">{{ submitError }}</p>
        <p v-if="submitStatus" class="form-message success">{{ submitStatus }}</p>
      </div>
    </div>

    <div v-else class="panel" style="margin-top: 18px;">
      <h2>{{ isLoading ? "Loading report..." : "Report unavailable" }}</h2>
      <p>{{ loadError || "The report you requested is not available yet." }}</p>
    </div>

    <div v-if="report" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2>Latest response</h2>
        <span>{{ reportStatus || "Pending" }}</span>
      </div>
      <div v-if="latestResponse" class="response-preview" v-html="latestResponse.message"></div>
      <p v-else class="helper">No response has been logged yet for this report.</p>
    </div>

    <div class="link-row" style="margin-top: 18px;">
      <NuxtLink class="ghost-link" to="/staff">Back to staff desk</NuxtLink>
      <NuxtLink class="ghost-link" :to="`/details/${reportId}`">View public details</NuxtLink>
    </div>
  </section>
</template>
