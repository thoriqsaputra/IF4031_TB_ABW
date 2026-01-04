<script setup>
import { LockClosedIcon, EnvelopeIcon, ArrowRightIcon } from '@heroicons/vue/24/outline'
import { ShieldCheckIcon } from '@heroicons/vue/24/solid'

definePageMeta({
  middleware: 'guest'
});

const email = ref("");
const password = ref("");
const error = ref("");
const status = ref("");
const isSubmitting = ref(false);

const router = useRouter();
const { setToken } = useAuth();
const { decodeToken, fetchUserProfile } = useUser();

const handleLogin = async () => {
  error.value = "";
  status.value = "";
  isSubmitting.value = true;

  try {
    const response = await $fetch("/api/auth/login", {
      method: "POST",
      body: {
        email: email.value,
        password: password.value,
      },
    });

    if (!response?.token) {
      throw new Error("Missing token in response");
    }

    setToken(response.token);
    
    // Fetch user profile to get accurate role
    await fetchUserProfile();
    
    // Decode token to check user role
    const decoded = decodeToken();
    const userRole = decoded?.role;
    
    status.value = "Login successful. Redirecting...";
    
    // Redirect based on role
    if (userRole === 'government' || userRole === 'admin') {
      await router.push("/staff/dashboard");
    } else {
      await router.push("/feed");
    }
  } catch (err) {
    error.value =
      err?.data?.error ||
      err?.data?.message ||
      err?.message ||
      "Login failed. Check your credentials.";
  } finally {
    isSubmitting.value = false;
  }
};
</script>

<template>
  <section class="page">
    <div class="hero">
      <div class="hero-content">
        <div class="feature-badge">
          <ShieldCheckIcon class="badge-icon" />
          <span>Secure Access</span>
        </div>
        <h1>Welcome Back</h1>
        <p class="hero-description">
          Sign in to access your account and manage reports in the Agarthan archive system.
        </p>
        <div class="link-row">
          <NuxtLink class="ghost-link" to="/register">
            <ArrowRightIcon class="icon-sm" />
            <span>Create account</span>
          </NuxtLink>
          <NuxtLink class="ghost-link" to="/feed">
            <ArrowRightIcon class="icon-sm" />
            <span>Browse public feed</span>
          </NuxtLink>
        </div>
      </div>
      <form class="panel login-panel" @submit.prevent="handleLogin">
        <div class="panel-header">
          <h2>Sign In</h2>
          <p>Enter your credentials to continue</p>
        </div>
        
        <div class="form-group">
          <label for="email">Email Address</label>
          <div class="input-wrapper">
            <EnvelopeIcon class="input-icon" />
            <input
              id="email"
              v-model="email"
              type="email"
              placeholder="your.email@example.com"
              autocomplete="email"
              required
            />
          </div>
        </div>
        
        <div class="form-group">
          <label for="password">Password</label>
          <div class="input-wrapper">
            <LockClosedIcon class="input-icon" />
            <input
              id="password"
              v-model="password"
              type="password"
              placeholder="Enter your password"
              autocomplete="current-password"
              required
            />
          </div>
        </div>
        
        <button type="submit" :disabled="isSubmitting" class="btn-submit">
          <span>{{ isSubmitting ? "Signing in..." : "Sign In" }}</span>
          <ArrowRightIcon class="btn-icon" v-if="!isSubmitting" />
        </button>
        
        <p v-if="error" class="form-message error">{{ error }}</p>
        <p v-if="status" class="form-message success">{{ status }}</p>
        
        <div class="form-footer">
          <p>Don't have an account? 
            <NuxtLink to="/register" class="link-primary">Sign up here</NuxtLink>
          </p>
        </div>
      </form>
    </div>
  </section>
</template>

<style scoped>
.page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 3rem 1rem;
}

.hero {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4rem;
  align-items: center;
}

.hero-content {
  padding: 2rem 0;
}

.feature-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  background: var(--primary-100);
  color: var(--primary-700);
  border-radius: var(--radius-full);
  font-size: 0.875rem;
  font-weight: 600;
  margin-bottom: 1.5rem;
}

.badge-icon {
  width: 1.25rem;
  height: 1.25rem;
}

.hero-content h1 {
  font-size: 3rem;
  font-weight: 800;
  line-height: 1.1;
  margin-bottom: 1rem;
  background: linear-gradient(135deg, var(--primary-600), var(--primary-800));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-description {
  font-size: 1.125rem;
  color: var(--text-secondary);
  line-height: 1.7;
  margin-bottom: 2rem;
}

.link-row {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.ghost-link {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1.25rem;
  border-radius: var(--radius-md);
  border: 1px solid var(--border);
  background: var(--background);
  transition: all var(--transition-fast);
  font-weight: 500;
  color: var(--text-secondary);
}

.ghost-link:hover {
  background: var(--primary-50);
  border-color: var(--primary-300);
  color: var(--primary-700);
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.login-panel {
  background: var(--background);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-xl);
  max-width: 480px;
}

.panel-header {
  margin-bottom: 2rem;
  text-align: center;
}

.panel-header h2 {
  font-size: 1.75rem;
  font-weight: 700;
  margin-bottom: 0.5rem;
  color: var(--text-primary);
}

.panel-header p {
  color: var(--text-secondary);
  font-size: 0.9375rem;
}

.form-group {
  margin-bottom: 1.5rem;
}

.input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.input-icon {
  position: absolute;
  left: 1rem;
  width: 1.25rem;
  height: 1.25rem;
  color: var(--text-muted);
  pointer-events: none;
}

.input-wrapper input {
  padding-left: 3rem !important;
}

.btn-submit {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.875rem 1.5rem;
  background: var(--primary-gradient);
  color: white;
  font-weight: 600;
  font-size: 1rem;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  margin-top: 0.5rem;
}

.btn-submit:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.btn-submit:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.form-message {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 1rem;
  padding: 0.75rem 1rem;
  border-radius: var(--radius-md);
  font-size: 0.9375rem;
}

.form-message.error {
  background: var(--error-50);
  color: var(--error-700);
  border: 1px solid var(--error-200);
}

.form-message.success {
  background: var(--success-50);
  color: var(--success-700);
  border: 1px solid var(--success-200);
}

.form-footer {
  margin-top: 2rem;
  padding-top: 1.5rem;
  border-top: 1px solid var(--border-light);
  text-align: center;
}

.form-footer p {
  color: var(--text-secondary);
  font-size: 0.9375rem;
}

.link-primary {
  color: var(--primary-600);
  font-weight: 600;
  transition: color var(--transition-fast);
}

.link-primary:hover {
  color: var(--primary-700);
  text-decoration: underline;
}

@media (max-width: 968px) {
  .hero {
    grid-template-columns: 1fr;
    gap: 3rem;
  }
  
  .hero-content h1 {
    font-size: 2.5rem;
  }
  
  .login-panel {
    max-width: 100%;
  }
}

@media (max-width: 640px) {
  .page {
    padding: 2rem 1rem;
  }
  
  .hero-content h1 {
    font-size: 2rem;
  }
  
  .hero-description {
    font-size: 1rem;
  }
}
</style>
