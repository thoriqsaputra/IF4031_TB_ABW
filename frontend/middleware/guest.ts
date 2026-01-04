export default defineNuxtRouteMiddleware(async (to, from) => {
  const { token, loadToken } = useAuth()
  const { userRole, fetchUserProfile } = useUser()

  // Load token if not already loaded
  if (process.client && !token.value) {
    loadToken()
  }

  // Fetch user profile if token exists but no user info
  if (token.value && !userRole.value) {
    await fetchUserProfile()
  }

  const isAuthenticated = !!token.value
  const role = userRole.value

  // If authenticated, redirect to appropriate dashboard
  if (isAuthenticated) {
    if (role === 'government' || role === 'admin') {
      return navigateTo('/staff/dashboard')
    } else {
      return navigateTo('/feed')
    }
  }
})
