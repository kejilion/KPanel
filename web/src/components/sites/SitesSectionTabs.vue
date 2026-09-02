<template>
  <nav class="sites-section-tabs" aria-label="网站功能">
    <RouterLink to="/sites" exact-active-class="is-active">站点管理</RouterLink>
    <RouterLink to="/sites/environment" exact-active-class="is-active">环境管理</RouterLink>
  </nav>
</template>

<style scoped>
.sites-section-tabs {
  display: flex;
  gap: 6px;
  width: fit-content;
  max-width: 100%;
  overflow-x: auto;
  padding: 5px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
}

.sites-section-tabs a {
  display: grid;
  min-width: 108px;
  min-height: 40px;
  place-items: center;
  padding: 9px 15px;
  border-radius: 10px;
  color: var(--muted);
  text-align: center;
  font-weight: 700;
  text-decoration: none;
}

.sites-section-tabs a.is-active {
  color: var(--text);
  background: var(--surface-raised);
  box-shadow: var(--shadow-sm);
}
</style>
