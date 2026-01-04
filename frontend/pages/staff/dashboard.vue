<template>
  <div class="dashboard-page">
    <div v-if="isCheckingPermissions" class="loading-state">
      <div class="spinner"></div>
      <p>Checking permissions...</p>
    </div>

    <div v-else-if="!hasRole(['government', 'admin'])" class="access-denied">
      <h1>Access Denied</h1>
      <p>You don't have permission to view this page.</p>
      <NuxtLink to="/" class="btn-primary">Go to Home</NuxtLink>
    </div>

    <div v-else class="dashboard-container">
      <header class="dashboard-header">
        <div class="header-content">
          <div>
            <h1>Analytics Dashboard</h1>
            <p v-if="userRole === 'government'">
              Viewing data for: <strong>{{ userInfo?.department.name }}</strong>
            </p>
            <p v-else-if="userRole === 'admin'">
              Viewing: <strong>All Departments</strong>
            </p>
          </div>
          <div class="observability-actions">
            <a href="http://localhost:9090" target="_blank" class="obs-btn prometheus">
              <ChartBarIcon class="icon-sm" />
              <span>Prometheus</span>
            </a>
            <a href="http://localhost:3100" target="_blank" class="obs-btn grafana">
              <PresentationChartLineIcon class="icon-sm" />
              <span>Grafana</span>
            </a>
            <a href="http://localhost:3100/explore" target="_blank" class="obs-btn loki">
              <DocumentTextIcon class="icon-sm" />
              <span>Logs</span>
            </a>
          </div>
        </div>
      </header>

      <div v-if="isLoading" class="loading-state">
        <div class="spinner"></div>
        <p>Loading analytics...</p>
      </div>

      <div v-else-if="error" class="error-state">
        <p>{{ error }}</p>
        <button @click="fetchAnalytics" class="btn-primary">Retry</button>
      </div>

      <div v-else-if="analyticsData" class="dashboard-content">
        <section class="kpi-section">
          <div class="kpi-card">
            <div class="kpi-icon total">
              <DocumentDuplicateIcon class="icon-lg" />
            </div>
            <div class="kpi-content">
              <span class="kpi-label">Total Reports</span>
              <span class="kpi-value">{{ analyticsData.kpis.total }}</span>
            </div>
          </div>

          <div class="kpi-card">
            <div class="kpi-icon completed">
              <CheckCircleIcon class="icon-lg" />
            </div>
            <div class="kpi-content">
              <span class="kpi-label">Completed</span>
              <span class="kpi-value">{{ analyticsData.kpis.completed }}</span>
            </div>
          </div>

          <div class="kpi-card">
            <div class="kpi-icon pending">
              <ClockIcon class="icon-lg" />
            </div>
            <div class="kpi-content">
              <span class="kpi-label">Pending</span>
              <span class="kpi-value">{{ analyticsData.kpis.pending }}</span>
            </div>
          </div>

          <div class="kpi-card">
            <div class="kpi-icon progress">
              <ArrowPathIcon class="icon-lg" />
            </div>
            <div class="kpi-content">
              <span class="kpi-label">In Progress</span>
              <span class="kpi-value">{{ analyticsData.kpis.inProgress }}</span>
            </div>
          </div>
        </section>

        <section class="charts-section">
          <div class="chart-card">
            <h2>Reports by Category</h2>
            <canvas ref="categoryChartRef"></canvas>
          </div>

          <div class="chart-card">
            <h2>Reports by Severity</h2>
            <canvas ref="severityChartRef"></canvas>
          </div>
        </section>

        <section class="activity-section">
          <h2>Recent Activity</h2>
          <div class="activity-feed">
            <div
              v-for="(activity, index) in analyticsData.recentActivity"
              :key="index"
              class="activity-item"
            >
              <div class="activity-icon">
                <DocumentTextIcon class="icon-md" />
              </div>
              <div class="activity-content">
                <span class="activity-title">{{ activity.title }}</span>
                <span class="activity-time">{{ formatActivityTime(activity.timestamp) }}</span>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { 
  DocumentDuplicateIcon, 
  CheckCircleIcon, 
  ClockIcon, 
  ArrowPathIcon,
  DocumentTextIcon,
  ChartBarIcon,
  PresentationChartLineIcon
} from '@heroicons/vue/24/outline'

definePageMeta({
  middleware: 'auth'
});

import Chart from 'chart.js/auto'

const { token, loadToken } = useAuth()
const { hasRole, userRole, userInfo, fetchUserProfile } = useUser()
const { analyticsData, isLoading, error, fetchAnalytics } = useAnalytics()

const isCheckingPermissions = ref(true)
const categoryChartRef = ref<HTMLCanvasElement | null>(null)
const severityChartRef = ref<HTMLCanvasElement | null>(null)
let categoryChart: Chart | null = null
let severityChart: Chart | null = null

const initCharts = () => {
  if (!analyticsData.value) return

  if (categoryChartRef.value) {
    categoryChart = new Chart(categoryChartRef.value, {
      type: 'bar',
      data: {
        labels: analyticsData.value.reportsByCategory.map(d => d.category),
        datasets: [{
          label: 'Reports',
          data: analyticsData.value.reportsByCategory.map(d => d.count),
          backgroundColor: 'rgba(138, 209, 193, 0.8)',
          borderColor: '#8ad1c1',
          borderWidth: 2
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        plugins: {
          legend: {
            display: false
          }
        },
        scales: {
          y: {
            beginAtZero: true,
            ticks: {
              precision: 0
            }
          }
        }
      }
    })
  }

  if (severityChartRef.value) {
    const severityColors = {
      low: 'rgba(138, 209, 193, 0.8)',
      medium: 'rgba(247, 213, 138, 0.8)',
      high: 'rgba(243, 168, 124, 0.8)'
    }

    severityChart = new Chart(severityChartRef.value, {
      type: 'pie',
      data: {
        labels: analyticsData.value.reportsBySeverity.map(d =>
          d.severity.charAt(0).toUpperCase() + d.severity.slice(1)
        ),
        datasets: [{
          data: analyticsData.value.reportsBySeverity.map(d => d.count),
          backgroundColor: analyticsData.value.reportsBySeverity.map(d =>
            severityColors[d.severity as keyof typeof severityColors] || '#ccc'
          ),
          borderWidth: 2,
          borderColor: '#fff'
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        plugins: {
          legend: {
            position: 'bottom'
          }
        }
      }
    })
  }
}

const formatActivityTime = (timestamp: string): string => {
  const date = new Date(timestamp)
  return date.toLocaleString('id-ID', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit'
  })
}

// Fetch data on mount
onMounted(async () => {
  console.log('=== Dashboard onMounted ===')

  isCheckingPermissions.value = true

  loadToken()

  console.log('Token:', token.value ? 'Present' : 'Missing')

  await fetchUserProfile()

  console.log('After fetchUserProfile:')
  console.log('- userInfo:', userInfo.value)
  console.log('- userRole:', userRole.value)
  console.log('- hasRole([government, admin]):', hasRole(['government', 'admin']))

  isCheckingPermissions.value = false

  if (hasRole(['government', 'admin'])) {
    console.log('Access granted - fetching analytics')
    await fetchAnalytics()

    nextTick(() => {
      initCharts()
    })
  } else {
    console.log('Access denied - user role does not match government or admin')
  }
})

onUnmounted(() => {
  if (categoryChart) categoryChart.destroy()
  if (severityChart) severityChart.destroy()
})

watch(() => analyticsData.value, () => {
  if (categoryChart) categoryChart.destroy()
  if (severityChart) severityChart.destroy()
  nextTick(() => initCharts())
})
</script>

<style scoped>
.dashboard-page {
  min-height: 100vh;
  background: transparent;
  padding: 2rem 1rem;
}

.access-denied {
  max-width: 600px;
  margin: 4rem auto;
  text-align: center;
  padding: 3rem 2rem;
  background: white;
  border-radius: 12px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

.access-denied h1 {
  font-family: 'Fraunces', serif;
  color: var(--sunset);
  margin-bottom: 1rem;
}

.access-denied p {
  color: #666;
  margin-bottom: 2rem;
}

.dashboard-container {
  max-width: 1400px;
  margin: 0 auto;
}

.dashboard-header {
  margin-bottom: 2rem;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 1.5rem;
}

.dashboard-header h1 {
  font-family: 'Fraunces', serif;
  font-size: 2.5rem;
  color: var(--text-primary);
  margin-bottom: 0.5rem;
}

.dashboard-header p {
  color: var(--text-secondary);
  font-size: 1.1rem;
}

.observability-actions {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.obs-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  border-radius: var(--radius-lg);
  font-size: 0.875rem;
  font-weight: 600;
  text-decoration: none;
  transition: all var(--transition-base);
  border: 1px solid transparent;
}

.obs-btn.prometheus {
  background: linear-gradient(135deg, #e6572e, #d94727);
  color: white;
}

.obs-btn.prometheus:hover {
  background: linear-gradient(135deg, #d94727, #c03f23);
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.obs-btn.grafana {
  background: linear-gradient(135deg, #f79520, #e67e00);
  color: white;
}

.obs-btn.grafana:hover {
  background: linear-gradient(135deg, #e67e00, #d17100);
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.obs-btn.loki {
  background: linear-gradient(135deg, var(--primary-500), var(--primary-600));
  color: white;
}

.obs-btn.loki:hover {
  background: linear-gradient(135deg, var(--primary-600), var(--primary-700));
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.loading-state, .error-state {
  text-align: center;
  padding: 4rem 2rem;
}

.spinner {
  width: 50px;
  height: 50px;
  border: 4px solid #f3f3f3;
  border-top: 4px solid var(--sea);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.dashboard-content {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.kpi-section {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 1.5rem;
}

.kpi-card {
  background: white;
  padding: 1.5rem;
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  display: flex;
  align-items: center;
  gap: 1rem;
  transition: transform 0.2s;
}

.kpi-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
}

.kpi-icon {
  width: 60px;
  height: 60px;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.kpi-icon.total { 
  background: linear-gradient(135deg, var(--primary-100), var(--primary-200)); 
  color: var(--primary-600);
}
.kpi-icon.completed { 
  background: linear-gradient(135deg, var(--success-100), var(--success-200)); 
  color: var(--success-600);
}
.kpi-icon.pending { 
  background: linear-gradient(135deg, var(--warning-100), var(--warning-200)); 
  color: var(--warning-600);
}
.kpi-icon.progress { 
  background: linear-gradient(135deg, var(--primary-200), var(--primary-300)); 
  color: var(--primary-700);
}

.kpi-content {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.kpi-label {
  font-size: 0.9rem;
  color: var(--text-secondary);
  font-weight: 500;
}

.kpi-value {
  font-size: 2rem;
  font-weight: 700;
  color: var(--text-primary);
  font-family: 'Fraunces', serif;
}

.charts-section {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 2rem;
}

.chart-card {
  background: white;
  padding: 2rem;
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.chart-card h2 {
  font-family: 'Fraunces', serif;
  font-size: 1.5rem;
  color: var(--text-primary);
  margin-bottom: 1.5rem;
}

.activity-section {
  background: white;
  padding: 2rem;
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-md);
}

.activity-section h2 {
  font-family: 'Fraunces', serif;
  font-size: 1.5rem;
  color: var(--text-primary);
  margin-bottom: 1.5rem;
}

.activity-feed {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.activity-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: #fafafa;
  border-radius: 8px;
  transition: background 0.2s;
}

.activity-item:hover {
  background: #f0f0f0;
}

.activity-icon {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--primary-100), var(--primary-200));
  color: var(--primary-600);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.activity-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.activity-title {
  font-weight: 600;
  color: var(--text-primary);
}

.activity-time {
  font-size: 0.85rem;
  color: var(--text-tertiary);
}

.btn-primary {
  display: inline-block;
  padding: 0.75rem 1.5rem;
  background: var(--sea);
  color: white;
  border-radius: 8px;
  text-decoration: none;
  font-weight: 600;
  transition: all 0.2s;
  border: none;
  cursor: pointer;
}

.btn-primary:hover {
  background: #7ac0b0;
  transform: translateY(-2px);
}

@media (max-width: 768px) {
  .kpi-section {
    grid-template-columns: 1fr;
  }

  .charts-section {
    grid-template-columns: 1fr;
  }

  .dashboard-header h1 {
    font-size: 2rem;
  }
}
</style>
