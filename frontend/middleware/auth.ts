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

  // Define public routes (accessible without authentication)
  const publicRoutes = ['/', '/register', '/feed']
  
  // Define protected routes with role requirements
  const citizenRoutes = ['/report', '/user_reports']
  const staffRoutes = ['/staff', '/staff/dashboard']
  const staffRoutePattern = /^\/staff/

  // If authenticated and trying to access login/register, redirect to appropriate dashboard
  if (isAuthenticated && (to.path === '/' || to.path === '/register')) {
    if (role === 'government' || role === 'admin') {
      return navigateTo('/staff/dashboard')
    } else {
      return navigateTo('/feed')
    }
  }

  // If not authenticated and trying to access protected route, redirect to login
  if (!isAuthenticated && !publicRoutes.includes(to.path)) {
    return navigateTo('/')
  }

  // Role-based access control
  if (isAuthenticated && role) {
    // Citizens trying to access staff routes
    if (role === 'citizen' && staffRoutePattern.test(to.path)) {
      return navigateTo('/403')
    }

    // Staff trying to access citizen-only routes
    if ((role === 'government' || role === 'admin') && citizenRoutes.includes(to.path)) {
      return navigateTo('/403')
    }
  }
})
