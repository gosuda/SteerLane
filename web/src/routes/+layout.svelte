<script lang="ts">
	import '../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';

	import { authStore } from '$lib/stores/auth.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { tenantStore } from '$lib/stores/tenant.svelte';

	let { children } = $props();
	let hydratedSessionVersion = $state(-1);

	const publicPaths = ['/login', '/register', '/oauth/callback', '/auth/link'];

	function isPublicPath(pathname: string) {
		return publicPaths.some((path) => pathname === path || pathname.startsWith(`${path}/`));
	}

	function navClass(pathname: string, href: string) {
		return pathname === href || pathname.startsWith(`${href}/`)
			? 'text-white'
			: 'text-slate-400 transition-colors hover:text-white';
	}

	let publicRoute = $derived(isPublicPath(page.url.pathname));

	onMount(() => {
		void authStore.initialize();

		const sync = () => {
			void authStore.syncFromStorage();
		};
		const invalidate = () => {
			void authStore.initialize();
		};
		window.addEventListener('storage', sync);
		window.addEventListener('steerlane:auth-invalid', invalidate as EventListener);

		return () => {
			window.removeEventListener('storage', sync);
			window.removeEventListener('steerlane:auth-invalid', invalidate as EventListener);
		};
	});

	$effect(() => {
		if (!authStore.initialized) {
			return;
		}

		if (!authStore.authenticated) {
			hydratedSessionVersion = -1;
			tenantStore.reset();
			projectStore.reset();
			if (!publicRoute) {
				goto('/login');
			}
			return;
		}

		if ((String(page.url.pathname) === '/login' || String(page.url.pathname) === '/register') && !page.url.searchParams.get('next')) {
			goto('/');
			return;
		}

		if (hydratedSessionVersion === authStore.sessionVersion) {
			return;
		}

		hydratedSessionVersion = authStore.sessionVersion;
		projectStore.reset();
		void tenantStore.loadTenant();
		void projectStore.loadProjects();
	});

	async function handleLogout() {
		await authStore.logout();
		tenantStore.reset();
		projectStore.reset();
		goto('/login');
	}
</script>

{#if !authStore.initialized}
	<div class="flex min-h-screen items-center justify-center bg-slate-950 text-slate-400">
		<div class="flex items-center gap-3 rounded-full border border-slate-800 bg-slate-900 px-5 py-3">
			<div class="h-2.5 w-2.5 animate-pulse rounded-full bg-brand-primary"></div>
			<span class="text-sm font-medium">Loading SteerLane workspace...</span>
		</div>
	</div>
{:else if publicRoute}
	<div class="min-h-screen bg-slate-950 text-slate-300">
		{@render children()}
	</div>
{:else}
	<div class="flex h-screen w-full flex-col overflow-hidden bg-slate-950 font-sans text-slate-300">
		<header class="flex h-16 shrink-0 items-center justify-between border-b border-slate-800 bg-brand-surface px-6">
			<div class="flex items-center gap-5">
				<div class="flex items-center gap-3">
					<div class="h-8 w-8 rounded-md bg-brand-primary"></div>
					<div>
						<div class="text-lg font-bold tracking-tight text-white">SteerLane</div>
						<div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">
							{tenantStore.tenant?.slug ?? 'tenant'}
						</div>
					</div>
				</div>
				<div class="h-5 w-px bg-slate-800"></div>
				<nav class="flex gap-4 text-sm font-medium">
					<a href="/" class={navClass(page.url.pathname, '/')}>Board</a>
					<a href="/adrs" class={navClass(page.url.pathname, '/adrs')}>ADRs</a>
					<a href="/monitor" class={navClass(page.url.pathname, '/monitor')}>Monitor</a>
					<a href="/settings/project" class={navClass(page.url.pathname, '/settings/project')}
						>Settings</a
					>
				</nav>
			</div>

			<div class="flex items-center gap-4 text-sm">
				{#if tenantStore.tenant}
					<div class="hidden rounded-full border border-slate-700 bg-slate-950 px-3 py-1 text-xs text-slate-400 sm:block">
						{tenantStore.tenant.name}
					</div>
				{/if}

				{#if projectStore.projects.length > 0}
					<select
						class="rounded border border-slate-700 bg-slate-950 px-3 py-1.5 text-xs text-slate-200 outline-none focus:border-brand-primary"
						value={projectStore.selectedProjectId}
						onchange={(event) => projectStore.selectProject(event.currentTarget.value)}
					>
						{#each projectStore.projects as project}
							<option value={project.id}>{project.name}</option>
						{/each}
					</select>
				{:else if projectStore.loading}
					<span class="text-xs text-slate-500">Loading projects...</span>
				{/if}

				<button
					type="button"
					class="rounded border border-slate-700 px-3 py-1.5 text-xs font-medium text-slate-300 transition-colors hover:border-slate-500 hover:text-white"
					onclick={handleLogout}
				>
					Sign out
				</button>
			</div>
		</header>

		{#if tenantStore.error || projectStore.error}
			<div class="border-b border-amber-900/60 bg-amber-950/40 px-6 py-2 text-xs text-amber-200">
				{tenantStore.error ?? projectStore.error}
			</div>
		{/if}

		<main class="flex min-h-0 flex-1">{@render children()}</main>
	</div>
{/if}
