export default defineNuxtPlugin(async (nuxtApp) => {
  const { token, loadToken } = useAuth();
  const { fetchUserProfile } = useUser();

  // Load token immediately before app renders
  loadToken();

  // If token exists, fetch user profile
  if (token.value) {
    try {
      await fetchUserProfile();
    } catch (error) {
      console.error('Failed to load user profile:', error);
      // Clear invalid token
      const { clearToken } = useAuth();
      clearToken();
    }
  }
});
