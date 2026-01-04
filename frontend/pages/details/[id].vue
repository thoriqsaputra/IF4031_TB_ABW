<script setup>
import { CheckCircleIcon, ClockIcon, UserIcon, ArrowUpIcon, XMarkIcon } from '@heroicons/vue/24/outline';

definePageMeta({
  middleware: 'auth'
});

const route = useRoute();
const { token, loadToken } = useAuth();
const { userRole, userInfo } = useUser();

const reportId = computed(() => String(route.params.id || ""));
const report = ref(null);
const error = ref("");
const isLoading = ref(false);

// Lightbox state
const lightboxOpen = ref(false);
const lightboxMedia = ref(null);

const isOwner = computed(() => report.value && userInfo.value && report.value.user_id === userInfo.value.user_id);
const isStaff = computed(() => userRole.value === 'government' || userRole.value === 'admin');
const canSeeDetails = computed(() => isOwner.value || isStaff.value);

const statusBadgeClass = computed(() => {
  if (!report.value) return 'badge-gray';
  switch (report.value.status) {
    case 'pending': return 'badge-yellow';
    case 'in_progress': return 'badge-blue';
    case 'resolved': return 'badge-green';
    case 'rejected': return 'badge-red';
    default: return 'badge-gray';
  }
});

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

const formatStatus = (status) => {
  if (!status) return '-';
  return status.split('_').map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ');
};

const openLightbox = (media) => {
  lightboxMedia.value = media;
  lightboxOpen.value = true;
};

const closeLightbox = () => {
  lightboxOpen.value = false;
  lightboxMedia.value = null;
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
      <div class="hero-header">
        <div>
          <p class="pill">{{ report.is_public ? "Public" : "Private" }}</p>
          <h1>{{ report.title }}</h1>
        </div>
        <div :class="['status-badge', statusBadgeClass]">
          {{ formatStatus(report.status) }}
        </div>
      </div>
      <div class="stat-row">
        <div class="stat">
          <span class="stat-label">Location:</span>
          <span class="stat-value">{{ report.location || "-" }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">Severity:</span>
          <span class="stat-value">{{ report.severity || "-" }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">Created:</span>
          <span class="stat-value">{{ formatTimestamp(report.created_at) }}</span>
        </div>
      </div>
      <p class="description">{{ report.description }}</p>
    </div>
    <div v-else class="panel">
      <h2>{{ isLoading ? "Loading report..." : "Report unavailable" }}</h2>
      <p>{{ error || "The report you requested is not available yet." }}</p>
    </div>

    <!-- Assignment & Status Info (Owner & Staff Only) -->
    <div v-if="report && canSeeDetails && report.assignment" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2><UserIcon class="icon" /> Assignment Information</h2>
      </div>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">Assigned To:</span>
          <span class="info-value">{{ report.assignment.assigned_to_email }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Assigned At:</span>
          <span class="info-value">{{ formatTimestamp(report.assignment.assigned_at) }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Assignment Status:</span>
          <span class="info-value">{{ formatStatus(report.assignment.status) }}</span>
        </div>
        <div v-if="report.assignment.responded_at" class="info-item">
          <span class="info-label">Responded At:</span>
          <span class="info-value">{{ formatTimestamp(report.assignment.responded_at) }}</span>
        </div>
      </div>
      <div v-if="report.assignment.response" class="response-box">
        <h3>Government Response:</h3>
        <div class="response-content" v-html="report.assignment.response"></div>
      </div>
    </div>

    <!-- Status History (Owner & Staff Only) -->
    <div v-if="report && canSeeDetails && report.status_history && report.status_history.length > 0" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2><ClockIcon class="icon" /> Status History</h2>
        <span>{{ report.status_history.length }} change(s)</span>
      </div>
      <div class="timeline">
        <div v-for="(change, index) in report.status_history" :key="index" class="timeline-item">
          <div class="timeline-marker"></div>
          <div class="timeline-content">
            <div class="timeline-header">
              <span class="status-change">
                <span class="old-status">{{ formatStatus(change.old_status) }}</span>
                →
                <span class="new-status">{{ formatStatus(change.new_status) }}</span>
              </span>
              <span class="timeline-date">{{ formatTimestamp(change.changed_at) }}</span>
            </div>
            <p v-if="change.notes" class="timeline-notes">{{ change.notes }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Escalations (Owner & Staff Only) -->
    <div v-if="report && canSeeDetails && report.escalations && report.escalations.length > 0" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2><ArrowUpIcon class="icon" /> Escalation History</h2>
        <span>{{ report.escalations.length }} escalation(s)</span>
      </div>
      <div class="escalation-list">
        <div v-for="(esc, index) in report.escalations" :key="index" class="escalation-item">
          <div class="escalation-header">
            <span>Department {{ esc.from_department_id }} → Department {{ esc.to_department_id }}</span>
            <span class="escalation-date">{{ formatTimestamp(esc.escalated_at) }}</span>
          </div>
          <p class="escalation-reason">{{ esc.reason }}</p>
        </div>
      </div>
    </div>

    <!-- Report Responses (Owner & Staff Only) -->
    <div v-if="report && canSeeDetails && report.responses && report.responses.length > 0" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2>📝 Official Responses</h2>
        <span>{{ report.responses.length }} response(s)</span>
      </div>
      <div class="response-list">
        <div v-for="(resp, index) in report.responses" :key="index" class="response-item">
          <div class="response-header">
            <span class="response-date">{{ formatTimestamp(resp.created_at) }}</span>
          </div>
          <div class="response-message" v-html="resp.message"></div>
        </div>
      </div>
    </div>

    <!-- Media Gallery -->
    <div v-if="report && report.media && report.media.length > 0" class="panel" style="margin-top: 18px;">
      <div class="section-title">
        <h2>📎 Media Attachments</h2>
        <span>{{ report.media.length }} file(s)</span>
      </div>
      <div class="media-gallery">
        <div v-for="item in report.media" :key="item.media_id" class="media-item" @click="openLightbox(item)">
          <div class="media-wrapper">
            <img
              v-if="item.media_type === 'image'"
              :src="item.url"
              :alt="item.filename || 'Report media'"
              loading="lazy"
              class="media-image"
            />
            <video
              v-else-if="item.media_type === 'video'"
              :src="item.url"
              controls
              preload="metadata"
              class="media-video"
            >
              Your browser does not support the video tag.
            </video>
            <div v-else class="media-placeholder">
              <span>{{ item.media_type || 'Unknown' }}</span>
            </div>
          </div>
          <div class="media-info">
            <span class="media-filename">{{ item.filename || 'Unnamed file' }}</span>
            <span class="media-type">{{ item.media_type || 'Unknown type' }}</span>
            <span class="media-date">{{ formatTimestamp(item.created_at) }}</span>
          </div>
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
<style scoped>
.hero-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1rem;
}

.status-badge {
  padding: 0.5rem 1rem;
  border-radius: 8px;
  font-weight: 600;
  font-size: 0.9rem;
  white-space: nowrap;
}

.badge-yellow {
  background: #fef3c7;
  color: #92400e;
}

.badge-blue {
  background: #dbeafe;
  color: #1e40af;
}

.badge-green {
  background: #d1fae5;
  color: #065f46;
}

.badge-red {
  background: #fee2e2;
  color: #991b1b;
}

.badge-gray {
  background: #f3f4f6;
  color: #374151;
}

.stat-row {
  display: flex;
  gap: 2rem;
  margin: 1rem 0;
  flex-wrap: wrap;
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.stat-label {
  font-size: 0.85rem;
  color: #6b7280;
  font-weight: 500;
}

.stat-value {
  font-size: 0.95rem;
  color: #111827;
  font-weight: 600;
}

.description {
  margin-top: 1rem;
  line-height: 1.6;
  color: #374151;
}

.icon {
  width: 20px;
  height: 20px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 0.5rem;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
  margin: 1rem 0;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.info-label {
  font-size: 0.85rem;
  color: #6b7280;
  font-weight: 500;
}

.info-value {
  font-size: 0.95rem;
  color: #111827;
}

.response-box {
  margin-top: 1.5rem;
  padding: 1rem;
  background: #f9fafb;
  border-left: 4px solid #3b82f6;
  border-radius: 8px;
}

.response-box h3 {
  margin: 0 0 0.75rem 0;
  font-size: 1rem;
  color: #374151;
}

.response-content {
  line-height: 1.6;
  color: #111827;
}

.timeline {
  position: relative;
  padding-left: 2rem;
}

.timeline-item {
  position: relative;
  padding-bottom: 1.5rem;
}

.timeline-item:last-child {
  padding-bottom: 0;
}

.timeline-marker {
  position: absolute;
  left: -2rem;
  top: 0.25rem;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #3b82f6;
  border: 3px solid #dbeafe;
}

.timeline-item::before {
  content: '';
  position: absolute;
  left: -1.5rem;
  top: 1rem;
  bottom: -1rem;
  width: 2px;
  background: #e5e7eb;
}

.timeline-item:last-child::before {
  display: none;
}

.timeline-content {
  background: #f9fafb;
  padding: 1rem;
  border-radius: 8px;
}

.timeline-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  flex-wrap: wrap;
}

.status-change {
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.old-status {
  color: #6b7280;
  text-decoration: line-through;
}

.new-status {
  color: #3b82f6;
}

.timeline-date {
  font-size: 0.85rem;
  color: #6b7280;
}

.timeline-notes {
  margin-top: 0.5rem;
  color: #374151;
  font-size: 0.9rem;
}

.escalation-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.escalation-item {
  background: #fef3c7;
  border-left: 4px solid #f59e0b;
  padding: 1rem;
  border-radius: 8px;
}

.escalation-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  font-weight: 600;
  color: #92400e;
  margin-bottom: 0.5rem;
}

.escalation-date {
  font-size: 0.85rem;
  font-weight: normal;
  color: #78350f;
}

.escalation-reason {
  margin: 0;
  color: #78350f;
  font-size: 0.9rem;
}

.response-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.response-item {
  background: #eff6ff;
  border-left: 4px solid #3b82f6;
  padding: 1rem;
  border-radius: 8px;
}

.response-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.response-date {
  font-size: 0.85rem;
  color: #1e40af;
  font-weight: 600;
}

.response-message {
  color: #1e3a8a;
  line-height: 1.6;
}

.response-message :deep(p) {
  margin: 0.5rem 0;
}

.response-message :deep(p:first-child) {
  margin-top: 0;
}

.response-message :deep(p:last-child) {
  margin-bottom: 0;
}

.response-message :deep(strong) {
  font-weight: 600;
}

.response-message :deep(em) {
  font-style: italic;
}

.response-message :deep(ul),
.response-message :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.5rem;
}

.response-message :deep(li) {
  margin: 0.25rem 0;
}

.media-gallery {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1.5rem;
  margin-top: 1rem;
}

.media-item {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  overflow: hidden;
  transition: transform 0.2s, box-shadow 0.2s;
}

.media-item:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.media-wrapper {
  position: relative;
  width: 100%;
  background: #000;
  aspect-ratio: 16/9;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.media-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  cursor: pointer;
  transition: transform 0.3s;
}

.media-image:hover {
  transform: scale(1.05);
}

.media-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.media-info {
  padding: 0.75rem;
  background: #f9fafb;
}

.media-info p {
  margin: 0;
  font-size: 0.85rem;
  color: #374151;
}

.media-info p:first-child {
  font-weight: 600;
  text-transform: capitalize;
  margin-bottom: 0.25rem;
}

.media-date {
  font-size: 0.75rem;
  color: #6b7280;
}

.media-item {
  cursor: pointer;
}

.media-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
  font-size: 0.875rem;
}

.media-filename {
  font-size: 0.875rem;
  font-weight: 500;
  color: #111827;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-type {
  font-size: 0.75rem;
  color: #6b7280;
  display: block;
  margin-top: 0.25rem;
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