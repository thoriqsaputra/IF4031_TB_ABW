<script setup>
const email = ref("");
const password = ref("");
const error = ref("");
const status = ref("");
const isSubmitting = ref(false);

const router = useRouter();
const { token, loadToken, setToken } = useAuth();

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
    status.value = "Login successful. Redirecting...";
    await router.push("/feed");
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

onMounted(() => {
  loadToken();
  if (token.value) {
    status.value = "You are already signed in.";
  }
});
</script>

<template>
  <section class="page">
    <div class="hero">
      <div>
        <p class="pill">Secure Access</p>
        <h1>Log in to map the public archive.</h1>
        <p>
          Track emerging findings, revisit your own field notes, and open any
          report detail in a single sweep.
        </p>
        <div class="link-row" style="margin-top: 20px;">
          <NuxtLink class="ghost-link" to="/feed">Browse public feed</NuxtLink>
          <NuxtLink class="ghost-link" to="/user_reports">My reports</NuxtLink>
        </div>
      </div>
      <form class="panel" @submit.prevent="handleLogin">
        <label for="email">Email</label>
        <input
          id="email"
          v-model="email"
          type="email"
          placeholder="explorer@agarthan.io"
          autocomplete="email"
          required
        />
        <label for="password">Password</label>
        <input
          id="password"
          v-model="password"
          type="password"
          placeholder="Enter your passphrase"
          autocomplete="current-password"
          required
        />
        <button type="submit" :disabled="isSubmitting">
          {{ isSubmitting ? "Signing in..." : "Enter the archive" }}
        </button>
        <p v-if="error" class="form-message error">{{ error }}</p>
        <p v-if="status" class="form-message success">{{ status }}</p>
      </form>
    </div>
  </section>
</template>
