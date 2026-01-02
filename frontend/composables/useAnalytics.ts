import type { AnalyticsData } from '~/types'

export const useAnalytics = () => {
  const { token } = useAuth()
  const { userDepartment, userRole } = useUser()

  const analyticsData = useState<AnalyticsData | null>('analytics', () => null)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  const fetchAnalytics = async () => {
    console.log('[useAnalytics] fetchAnalytics called')
    console.log('[useAnalytics] token:', token.value ? 'Present' : 'Missing')
    console.log('[useAnalytics] userRole:', userRole.value)
    console.log('[useAnalytics] userDepartment:', userDepartment.value)
    
    isLoading.value = true
    error.value = null

    try {
      // Construct URL with department filter for government users
      let url = '/api/analytics'
      if (userRole.value === 'government' && userDepartment.value) {
        url += `?department_id=${userDepartment.value}`
      }
      console.log('[useAnalytics] Fetching URL:', url)

      const response = await $fetch<AnalyticsData>(url, {
        headers: {
          Authorization: `Bearer ${token.value}`
        }
      })

      console.log('[useAnalytics] Response received:', response)
      analyticsData.value = response
    } catch (err: any) {
      console.error('[useAnalytics] ERROR:', err)
      console.error('[useAnalytics] Error details:', err.data)
      error.value = err.data?.error || err.message || 'Failed to fetch analytics'
    } finally {
      isLoading.value = false
      console.log('[useAnalytics] isLoading set to false')
    }
  }

  return {
    analyticsData,
    isLoading,
    error,
    fetchAnalytics
  }
}
