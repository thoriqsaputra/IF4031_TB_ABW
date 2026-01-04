import type { DecodedToken, User } from '~/types'

export const useUser = () => {
  const { token } = useAuth()
  const userInfo = useState<User | null>('userInfo', () => null)
  const isLoadingProfile = useState<boolean>('isLoadingProfile', () => false)
  const userRole = computed(() => userInfo.value?.role.name ?? null)
  const userDepartment = computed(() => userInfo.value?.department_id ?? null)

  // Decode JWT token (client-side only, not cryptographically verified)
  const decodeToken = (): DecodedToken | null => {
    if (!token.value) return null

    try {
      const [, payload] = token.value.split('.')
      const decoded = JSON.parse(
        atob(payload.replace(/-/g, '+').replace(/_/g, '/'))
      )
      return decoded as DecodedToken
    } catch {
      return null
    }
  }

  // Fetch full user profile from backend
  const fetchUserProfile = async () => {
    if (!token.value) {
      console.log('[useUser] No token found, skipping profile fetch')
      return
    }

    isLoadingProfile.value = true
    try {
      console.log('[useUser] Fetching profile from /api/auth/profile')
      const response = await $fetch<User>('/api/auth/profile', {
        headers: {
          Authorization: `Bearer ${token.value}`
        }
      })
      console.log('[useUser] Profile response:', response)
      console.log('[useUser] Role from response:', response?.role)
      userInfo.value = response
      console.log('[useUser] userInfo set to:', userInfo.value)
    } catch (error) {
      console.error('[useUser] Failed to fetch user profile:', error)
      throw error
    } finally {
      isLoadingProfile.value = false
    }
  }

  // Check if user has specific role
  const hasRole = (role: string | string[]) => {
    const roles = Array.isArray(role) ? role : [role]
    return userRole.value ? roles.includes(userRole.value) : false
  }

  return {
    userInfo,
    userRole,
    userDepartment,
    isLoadingProfile,
    decodeToken,
    fetchUserProfile,
    hasRole
  }
}
