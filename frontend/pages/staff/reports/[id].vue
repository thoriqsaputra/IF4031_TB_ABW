<script setup>
import { CheckCircleIcon, ArrowUpIcon, XMarkIcon } from '@heroicons/vue/24/outline';

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

// Lightbox state
const lightboxOpen = ref(false);
const lightboxMedia = ref(null);

// Status update
const isUpdatingStatus = ref(false);
const statusUpdateError = ref("");
const statusUpdateSuccess = ref("");
const selectedStatus = ref("");

// Escalation
const showEscalationModal = ref(false);
const isEscalating = ref(false);
const escalationError = ref("");
const escalationSuccess = ref("");
const escalationDepartmentId = ref("");
const escalationReason = ref("");
const departments = ref([]);

const editorRef = ref(null);
const quillInstance = ref(null);
const editorHtml = ref("");
const editorText = ref("");

const openLightbox = (media) => {
  lightboxMedia.value = media;
  lightboxOpen.value = true;
};

const closeLightbox = () => {
  lightboxOpen.value = false;
  lightboxMedia.value = null;
};

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

  try {
    editorHtml.value = quill.root?.innerHTML || "";
    editorText.value = quill.getText() || "";
  } catch (err) {
    console.error("Error syncing editor:", err);
    editorHtml.value = "";
    editorText.value = "";
  }
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

const fetchDepartments = async () => {
  if (!token.value) return;
  
  try {
    const response = await $fetch('/api/departments', {
      headers: {
        Authorization: `Bearer ${token.value}`
      }
    });
    departments.value = response || [];
  } catch (err) {
    console.error('Failed to fetch departments:', err);
  }
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
    selectedStatus.value = response.status || "pending";
    await fetchReportStatus();
  } catch (err) {
    loadError.value =
      err?.data?.error || err?.data?.message || "Unable to load report details.";
    report.value = null;
  } finally {
    isLoading.value = false;
  }
};

const updateStatus = async () => {
  statusUpdateError.value = "";
  statusUpdateSuccess.value = "";
  
  if (!token.value) {
    statusUpdateError.value = "Not authenticated";
    return;
  }
  
  const id = Number(reportId.value);
  if (!Number.isFinite(id) || id <= 0) {
    statusUpdateError.value = "Invalid report id";
    return;
  }
  
  if (!selectedStatus.value) {
    statusUpdateError.value = "Please select a status";
    return;
  }
  
  isUpdatingStatus.value = true;
  
  try {
    await $fetch(`/api/reports/${id}/status`, {
      method: 'PATCH',
      headers: {
        Authorization: `Bearer ${token.value}`
      },
      body: {
        status: selectedStatus.value
      }
    });
    
    statusUpdateSuccess.value = "Status updated successfully";
    await fetchReport();
    
    setTimeout(() => {
      statusUpdateSuccess.value = "";
    }, 3000);
  } catch (err) {
    statusUpdateError.value = err?.data?.error || err?.data?.message || "Failed to update status";
  } finally {
    isUpdatingStatus.value = false;
  }
};

const openEscalationModal = () => {
  escalationError.value = "";
  escalationSuccess.value = "";
  escalationDepartmentId.value = "";
  escalationReason.value = "";
  showEscalationModal.value = true;
};

const closeEscalationModal = () => {
  showEscalationModal.value = false;
};

const escalateReport = async () => {
  escalationError.value = "";
  escalationSuccess.value = "";
  
  if (!token.value) {
    escalationError.value = "Not authenticated";
    return;
  }
  
  const id = Number(reportId.value);
  if (!Number.isFinite(id) || id <= 0) {
    escalationError.value = "Invalid report id";
    return;
  }
  
  if (!escalationDepartmentId.value) {
    escalationError.value = "Please select a department";
    return;
  }
  
  if (!escalationReason.value.trim()) {
    escalationError.value = "Please provide a reason for escalation";
    return;
  }
  
  isEscalating.value = true;
  
  try {
    await $fetch(`/api/reports/${id}/escalate`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token.value}`
      },
      body: {
        to_department_id: Number(escalationDepartmentId.value),
        reason: escalationReason.value
      }
    });
    
    escalationSuccess.value = "Report escalated successfully";
    await fetchReport();
    
    setTimeout(() => {
      closeEscalationModal();
    }, 2000);
  } catch (err) {
    escalationError.value = err?.data?.error || err?.data?.message || "Failed to escalate report";
  } finally {
    isEscalating.value = false;
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

  // Capture content before submission to avoid Quill state issues
  const quill = quillInstance.value;
  if (!quill) {
    submitError.value = "Editor not initialized.";
    return;
  }

  let messageContent;
  try {
    messageContent = quill.root?.innerHTML || "";
    if (!messageContent || messageContent.trim() === "<p><br></p>") {
      submitError.value = "Response cannot be empty.";
      return;
    }
  } catch (err) {
    console.error("Error getting editor content:", err);
    submitError.value = "Failed to get editor content.";
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
        message: messageContent,
      },
      // Accept 202 (Accepted) as a successful response
      onResponseError({ response }) {
        // Don't throw for 202 status
        if (response.status === 202) {
          return;
        }
      }
    });
    const requestId = response?.request_id ? ` (request ${response.request_id})` : "";
    submitError.value = "";
    submitStatus.value = `Response queued${requestId}!`;
    clearEditor();
    
    // Refresh responses after a short delay to allow processing
    setTimeout(async () => {
      await fetchReportStatus();
      await fetchReport();
    }, 1500);
  } catch (err) {
    submitStatus.value = "";
    submitError.value = err?.data?.error || err?.data?.message || "Failed to send response.";
  } finally {
    isSubmitting.value = false;
  }
};

onMounted(() => {
  loadToken();
  fetchReport();
  fetchDepartments();
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

        <!-- Media Attachments -->
        <div v-if="report.media && report.media.length > 0" class="panel-section" style="margin-top: 24px; padding-top: 24px; border-top: 1px solid var(--cream-200);">
          <div class="section-title" style="margin-bottom: 16px;">
            <h3 style="font-size: 1.1rem; margin: 0;">Media Attachments</h3>
            <span style="font-size: 0.875rem;">{{ report.media.length }} file{{ report.media.length === 1 ? '' : 's' }}</span>
          </div>
          <div class="media-gallery">
            <div v-for="media in report.media" :key="media.media_id" class="media-item" @click="openLightbox(media)">
              <div class="media-wrapper">
                <img 
                  v-if="media.media_type === 'image'" 
                  :src="media.url" 
                  :alt="media.filename || 'Report media'"
                  class="media-image"
                />
                <video 
                  v-else-if="media.media_type === 'video'" 
                  :src="media.url" 
                  controls
                  preload="metadata"
                  class="media-video"
                >
                  Your browser does not support video playback.
                </video>
                <div v-else class="media-placeholder">
                  <span>{{ media.media_type || 'Unknown type' }}</span>
                </div>
              </div>
              <div class="media-info">
                <span class="media-filename">{{ media.filename || 'Unnamed file' }}</span>
                <span class="media-type">{{ media.media_type || 'Unknown' }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Status Update Section -->
        <div class="panel-section" style="margin-top: 24px; padding-top: 24px; border-top: 1px solid var(--cream-200);">
          <div class="section-title" style="margin-bottom: 12px;">
            <h3 style="font-size: 1.1rem; margin: 0;">Update Status</h3>
            <span style="font-size: 0.875rem;">Change report status</span>
          </div>
          
          <div style="display: flex; gap: 12px; align-items: flex-start; flex-wrap: wrap;">
            <div style="flex: 1; min-width: 200px;">
              <select 
                v-model="selectedStatus" 
                :disabled="isUpdatingStatus"
                style="width: 100%; padding: 10px 12px; border: 1px solid var(--cream-300); border-radius: 8px; background: white; font-size: 0.95rem;"
              >
                <option value="pending">Pending</option>
                <option value="in_progress">In Progress</option>
                <option value="resolved">Resolved</option>
                <option value="rejected">Rejected</option>
              </select>
            </div>
            
            <button 
              @click="updateStatus" 
              :disabled="isUpdatingStatus || selectedStatus === report.status"
              style="padding: 10px 20px; display: flex; align-items: center; gap: 8px;"
              class="status-update-btn"
            >
              <CheckCircleIcon class="icon" style="width: 18px; height: 18px;" />
              {{ isUpdatingStatus ? 'Updating...' : 'Update Status' }}
            </button>

            <button 
              @click="openEscalationModal" 
              :disabled="isUpdatingStatus"
              style="padding: 10px 20px; display: flex; align-items: center; gap: 8px; background: var(--primary-600); border: none;"
              class="escalate-btn"
            >
              <ArrowUpIcon class="icon" style="width: 18px; height: 18px;" />
              Escalate
            </button>
          </div>

          <p v-if="statusUpdateError" class="form-message error" style="margin-top: 12px;">{{ statusUpdateError }}</p>
          <p v-if="statusUpdateSuccess" class="form-message success" style="margin-top: 12px;">{{ statusUpdateSuccess }}</p>
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
      </div>
      <div v-if="latestResponse" class="response-preview" v-html="latestResponse.message"></div>
      <p v-else class="helper">No response has been logged yet for this report.</p>
    </div>

    <div class="link-row" style="margin-top: 18px;">
      <NuxtLink class="ghost-link" to="/staff">Back to staff desk</NuxtLink>
      <NuxtLink class="ghost-link" :to="`/details/${reportId}`">View public details</NuxtLink>
    </div>

    <!-- Escalation Modal -->
    <div v-if="showEscalationModal" class="modal-overlay" @click="closeEscalationModal">
      <div class="modal-content" @click.stop>
        <div class="modal-header">
          <h2 style="margin: 0; display: flex; align-items: center; gap: 10px;">
            <ArrowUpIcon class="icon" style="width: 24px; height: 24px; color: var(--primary-600);" />
            Escalate Report
          </h2>
          <button @click="closeEscalationModal" class="modal-close">&times;</button>
        </div>
        
        <div class="modal-body">
          <p style="color: var(--text-secondary); margin-bottom: 20px;">
            Escalate this report to a higher department for specialized handling.
          </p>
          
          <div class="form-group">
            <label for="escalation-dept">Target Department</label>
            <select 
              id="escalation-dept"
              v-model="escalationDepartmentId"
              :disabled="isEscalating"
              style="width: 100%; padding: 10px 12px; border: 1px solid var(--cream-300); border-radius: 8px; background: white;"
            >
              <option value="">Select department...</option>
              <option v-for="dept in departments" :key="dept.department_id" :value="dept.department_id">
                {{ dept.name }}
              </option>
            </select>
          </div>

          <div class="form-group" style="margin-top: 16px;">
            <label for="escalation-reason">Reason for Escalation</label>
            <textarea 
              id="escalation-reason"
              v-model="escalationReason"
              :disabled="isEscalating"
              placeholder="Explain why this report needs to be escalated..."
              rows="4"
              style="width: 100%; padding: 10px 12px; border: 1px solid var(--cream-300); border-radius: 8px; resize: vertical; font-family: inherit;"
            ></textarea>
          </div>

          <p v-if="escalationError" class="form-message error" style="margin-top: 12px;">{{ escalationError }}</p>
          <p v-if="escalationSuccess" class="form-message success" style="margin-top: 12px;">{{ escalationSuccess }}</p>
        </div>

        <div class="modal-footer">
          <button @click="closeEscalationModal" :disabled="isEscalating" class="ghost-button">
            Cancel
          </button>
          <button @click="escalateReport" :disabled="isEscalating" style="background: var(--primary-600);">
            {{ isEscalating ? 'Escalating...' : 'Escalate Report' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Lightbox Modal -->
    <div v-if="lightboxOpen" class="lightbox-overlay" @click="closeLightbox">
      <button class="lightbox-close" @click="closeLightbox">
        <XMarkIcon class="icon" />
      </button>
      <div class="lightbox-content" @click.stop>
        <img
          v-if="lightboxMedia?.media_type === 'image'"
          :src="lightboxMedia.url"
          :alt="lightboxMedia.filename"
          class="lightbox-image"
        />
        <video
          v-else-if="lightboxMedia?.media_type === 'video'"
          :src="lightboxMedia.url"
          controls
          autoplay
          controlslist="nodownload"
          class="lightbox-video"
        >
          Your browser does not support the video tag.
        </video>
      </div>
    </div>
  </section>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal-content {
  background: white;
  border-radius: 12px;
  max-width: 500px;
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 24px;
  border-bottom: 1px solid var(--cream-200);
}

.modal-close {
  background: none;
  border: none;
  font-size: 32px;
  line-height: 1;
  cursor: pointer;
  color: var(--text-secondary);
  padding: 0;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  transition: all 0.2s;
}

.modal-close:hover {
  background: var(--cream-100);
  color: var(--text-primary);
}

.modal-body {
  padding: 24px;
}

.modal-footer {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  padding: 24px;
  border-top: 1px solid var(--cream-200);
}

.form-group label {
  display: block;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--text-primary);
  font-size: 0.95rem;
}

.status-update-btn {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  color: white;
  border: none;
  transition: all 0.2s;
}

.status-update-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 8px 12px rgba(16, 185, 129, 0.3);
}

.status-update-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.escalate-btn {
  transition: all 0.2s;
}

.escalate-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 8px 12px rgba(249, 115, 22, 0.3);
}

.escalate-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.media-gallery {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.media-item {
  border: 1px solid var(--cream-200);
  border-radius: 12px;
  overflow: hidden;
  background: white;
  transition: all 0.2s;
}

.media-item:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.media-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  background: var(--cream-100);
  overflow: hidden;
}

.media-image,
.media-video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.media-video {
  background: #000;
}

.media-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  font-size: 0.875rem;
}

.media-info {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-top: 1px solid var(--cream-200);
}

.media-filename {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-type {
  font-size: 0.8rem;
  color: var(--text-secondary);
}

.media-item {
  cursor: pointer;
}

.response-preview {
  padding: 1rem;
  background: #eff6ff;
  border-left: 4px solid #3b82f6;
  border-radius: 8px;
  color: #1e3a8a;
  line-height: 1.6;
}

.response-preview :deep(p) {
  margin: 0.5rem 0;
}

.response-preview :deep(p:first-child) {
  margin-top: 0;
}

.response-preview :deep(p:last-child) {
  margin-bottom: 0;
}

.response-preview :deep(strong) {
  font-weight: 600;
}

.response-preview :deep(em) {
  font-style: italic;
}

.response-preview :deep(ul),
.response-preview :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.5rem;
}

.response-preview :deep(li) {
  margin: 0.25rem 0;
}

/* Lightbox Styles */
.lightbox-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.95);
  z-index: 99999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  animation: fadeIn 0.2s;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.lightbox-close {
  position: absolute;
  top: 20px;
  right: 20px;
  width: 48px;
  height: 48px;
  border: none;
  background: rgba(255, 255, 255, 0.1);
  color: white;
  border-radius: 50%;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
  z-index: 100000;
}

.lightbox-close:hover {
  background: rgba(255, 255, 255, 0.2);
  transform: scale(1.1);
}

.lightbox-close .icon {
  width: 28px;
  height: 28px;
}

.lightbox-content {
  max-width: 95vw;
  max-height: 95vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

.lightbox-image {
  max-width: 100%;
  max-height: 95vh;
  object-fit: contain;
  border-radius: 8px;
  animation: zoomIn 0.3s;
}

.lightbox-video {
  max-width: 100%;
  max-height: 95vh;
  border-radius: 8px;
  animation: zoomIn 0.3s;
}

@keyframes zoomIn {
  from {
    transform: scale(0.9);
    opacity: 0;
  }
  to {
    transform: scale(1);
    opacity: 1;
  }
}
</style>