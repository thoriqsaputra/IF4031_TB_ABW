<script setup>
const form = reactive({
  name: "",
  email: "",
  password: "",
  role_id: 1, // Default to citizen
  department_id: 0
});

const error = ref("");
const status = ref("");
const isSubmitting = ref(false);
const departments = ref([]);
const isLoadingDepartments = ref(false);

const router = useRouter();

const fetchDepartments = async () => {
  isLoadingDepartments.value = true;
  try {
    const response = await $fetch("/api/departments");
    departments.value = response;
  } catch (err) {
    console.error("Failed to fetch departments:", err);
  } finally {
    isLoadingDepartments.value = false;
  }
};

onMounted(() => {
  fetchDepartments();
});

const handleRegister = async () => {
  error.value = "";
  status.value = "";
  isSubmitting.value = true;

  try {
    const body = {
      name: form.name,
      email: form.email,
      password: form.password,
      role_id: form.role_id,
      department_id: form.role_id === 1 ? 0 : form.department_id
    };

    const response = await $fetch("/api/auth/register", {
      method: "POST",
      body,
    });

    status.value = "Registration successful! Redirecting to login...";
    setTimeout(() => {
      router.push("/");
    }, 2000);
  } catch (err) {
    error.value =
      err?.data?.error ||
      err?.data?.message ||
      err?.message ||
      "Registration failed. Please try again.";
  } finally {
    isSubmitting.value = false;
  }
};
</script>

<template>
  <section class="page">
    <div class="hero">
      <div>
        <p class="pill">Join the Archive</p>
        <h1>Register for Agarthan Reports</h1>
        <p>
          Create an account to submit reports, track findings, and contribute to the public archive.
        </p>
        <div class="link-row" style="margin-top: 20px;">
          <NuxtLink class="ghost-link" to="/">Already have an account? Log in</NuxtLink>
          <NuxtLink class="ghost-link" to="/feed">Browse public feed</NuxtLink>
        </div>
      </div>
      <form class="panel" @submit.prevent="handleRegister">
        <label for="name">Full Name</label>
        <input
          id="name"
          v-model="form.name"
          type="text"
          placeholder="Your full name"
          autocomplete="name"
          required
        />

        <label for="email">Email</label>
        <input
          id="email"
          v-model="form.email"
          type="email"
          placeholder="explorer@agarthan.io"
          autocomplete="email"
          required
        />

        <label for="password">Password</label>
        <input
          id="password"
          v-model="form.password"
          type="password"
          placeholder="Choose a strong password"
          autocomplete="new-password"
          required
        />

        <label for="role">Role</label>
        <select id="role" v-model.number="form.role_id">
          <option :value="1">citizen</option>
          <option :value="2">government</option>
          <option :value="3">admin</option>
        </select>

        <label for="department" v-if="form.role_id !== 1">Department</label>
        <select 
          v-if="form.role_id !== 1"
          id="department" 
          v-model.number="form.department_id"
          required
        >
          <option :value="0" disabled>Select a department</option>
          <option v-for="dept in departments" :key="dept.department_id" :value="dept.department_id">
            {{ dept.name }} (ID: {{ dept.department_id }})
          </option>
        </select>
        <p v-if="form.role_id !== 1" class="hint-text">Government officials must select their department</p>

        <button type="submit" :disabled="isSubmitting">
          {{ isSubmitting ? "Creating account..." : "Create account" }}
        </button>
        <p v-if="error" class="form-message error">{{ error }}</p>
        <p v-if="status" class="form-message success">{{ status }}</p>
      </form>
    </div>
  </section>
</template>
