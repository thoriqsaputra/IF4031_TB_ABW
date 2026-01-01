<template>
  <div class="app">
    <div class="bg">
      <span class="orb orb-one"></span>
      <span class="orb orb-two"></span>
      <span class="grid"></span>
    </div>
    <header class="topbar">
      <NuxtLink class="logo" to="/">Agarthan Reports</NuxtLink>
      <nav class="nav">
        <NuxtLink to="/feed">Feed</NuxtLink>
        <NuxtLink to="/user_reports">My Reports</NuxtLink>
        <NuxtLink to="/report">Submit Report</NuxtLink>
        <NuxtLink to="/staff">Staff Desk</NuxtLink>
      </nav>
    </header>
    <main class="content">
      <NuxtPage />
    </main>
  </div>
</template>
