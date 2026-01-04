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

const categories = ref([]);
const error = ref("");
const status = ref("");
const isSubmitting = ref(false);
const isWaiting = ref(false);
const createdReportId = ref(null);
const selectedFiles = ref([]);
const isUploadingMedia = ref(false);

// Fetch categories from API
const fetchCategories = async () => {
  try {
    const response = await $fetch("/api/categories");
    categories.value = response;
    if (categories.value.length > 0) {
      form.report_categories_id = categories.value[0].report_categories_id;
    }
  } catch (err) {
    console.error("Failed to fetch categories:", err);
    error.value = "Failed to load categories";
  }
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

    console.log('Report API Response:', response);

    // Report submitted successfully - get report_id from response
    const reportId = response?.report_id || response?.data?.report_id;
    console.log('Extracted reportId:', reportId);
    console.log('Selected files count:', selectedFiles.value.length);

    if (reportId) {
      createdReportId.value = reportId;
      status.value = "Report submitted successfully!";

      // Auto-upload selected media files if any
      if (selectedFiles.value.length > 0) {
        console.log('Starting media upload for report:', reportId);
        await uploadSelectedMedia(reportId);
      }
    } else {
      status.value = "Report submitted and queued for processing.";
      console.warn('No report_id in response, cannot upload media');
    }

    isWaiting.value = false;
    isSubmitting.value = false;
  } catch (err) {
    error.value =
      err?.data?.error || err?.data?.message || "Failed to submit report.";
    isWaiting.value = false;
    isSubmitting.value = false;
  }
};

const handleFileSelect = (event) => {
  const files = Array.from(event.target.files || []);
  const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'video/mp4', 'video/quicktime', 'video/x-msvideo'];

  const validFiles = files.filter(file => {
    if (!allowedTypes.includes(file.type)) {
      error.value = `File type ${file.type} not allowed`;
      return false;
    }
    if (file.size > 50 * 1024 * 1024) {
      error.value = 'File size exceeds 50MB limit';
      return false;
    }
    return true;
  });

  selectedFiles.value = validFiles.slice(0, 5); // Max 5 files
};

const removeFile = (index) => {
  selectedFiles.value.splice(index, 1);
};

const uploadSelectedMedia = async (reportId) => {
  if (selectedFiles.value.length === 0) return;

  console.log('uploadSelectedMedia called with reportId:', reportId);
  isUploadingMedia.value = true;
  const { uploadFile } = useMedia();
  let uploadedCount = 0;

  for (const file of selectedFiles.value) {
    try {
      console.log('Uploading file:', file.name, 'for report:', reportId);
      const result = await uploadFile(file, reportId);
      console.log('Upload result:', result);
      if (result) {
        uploadedCount++;
      }
    } catch (err) {
      console.error('Upload error:', err);
      error.value = `Failed to upload ${file.name}: ${err.message || 'Unknown error'}`;
    }
  }

  isUploadingMedia.value = false;
  if (uploadedCount > 0) {
    status.value = `Report created and ${uploadedCount} media file(s) uploaded successfully!`;
    selectedFiles.value = []; // Clear selected files
  } else {
    error.value = 'Failed to upload media files. Check console for details.';
  }
};

const formatFileSize = (bytes) => {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
};

onMounted(() => {
  loadToken();
  fetchCategories();
});
</script>

<template>
  <section class="page">
    <div class="section-title">
      <h2>Submit report</h2>
      <span>Send a new field entry</span>
    </div>
    <p>
      Submit a report with optional media attachments. Reports are processed and stored in the database.
    </p>
    <form class="panel report-form" @submit.prevent="submitReport">
      <div class="form-grid">
        <div>
          <label for="title">Title</label>
          <input id="title" v-model="form.title" type="text" required />
        </div>
        <div>
          <label for="category">Category</label>
          <select
            id="category"
            v-model="form.report_categories_id"
            required
          >
            <option v-for="cat in categories" :key="cat.report_categories_id" :value="cat.report_categories_id">
              {{ cat.name }}
            </option>
          </select>
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

      <!-- Media Upload Section (Optional) -->
      <div style="margin: 1.5rem 0; padding: 1.5rem; background: rgba(138, 209, 193, 0.05); border-radius: 12px; border: 1px dashed var(--sea);">
        <label style="display: block; font-size: 0.95rem; font-weight: 600; margin-bottom: 0.5rem; color: var(--ink);">
          📎 Attach Media (Optional)
        </label>
        <p style="font-size: 0.85rem; color: #666; margin: 0 0 1rem 0;">
          Upload photos or videos to support your report. Max 5 files, 50MB each.
        </p>

        <input
          type="file"
          id="media-files"
          multiple
          accept="image/jpeg,image/png,image/gif,image/webp,video/mp4,video/quicktime,video/x-msvideo"
          @change="handleFileSelect"
          style="display: block; margin-bottom: 1rem; padding: 0.5rem; border: 1px solid var(--line); border-radius: 8px; background: white; width: 100%;"
        />

        <!-- Selected Files Preview -->
        <div v-if="selectedFiles.length > 0" style="margin-top: 1rem;">
          <p style="font-size: 0.9rem; font-weight: 600; margin-bottom: 0.5rem;">Selected Files:</p>
          <div style="display: flex; flex-direction: column; gap: 0.5rem;">
            <div
              v-for="(file, index) in selectedFiles"
              :key="index"
              style="display: flex; justify-content: space-between; align-items: center; padding: 0.5rem; background: white; border-radius: 6px; font-size: 0.85rem;"
            >
              <span style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                {{ file.name }} ({{ formatFileSize(file.size) }})
              </span>
              <button
                type="button"
                @click="removeFile(index)"
                style="background: var(--sunset); color: white; border: none; border-radius: 4px; padding: 0.25rem 0.5rem; cursor: pointer; font-size: 0.8rem;"
              >
                Remove
              </button>
            </div>
          </div>
        </div>
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

    <div v-if="isWaiting || isUploadingMedia" class="overlay">
      <div class="spinner"></div>
      <p v-if="isUploadingMedia">Uploading media files...</p>
      <p v-else>Submitting report...</p>
    </div>
  </section>
</template>
