<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { apiFetch } from '$lib/api/request';
	import { saveTenantSlug } from '$lib/auth/session';
	import { authStore } from '$lib/stores/auth.svelte';

	const pendingLinkTokenKey = 'steerlane.pending_link_token';

	let linkStatus = $state<'idle' | 'linking' | 'linked' | 'error'>('idle');
	let linkMessage = $state('');
	let tenantHint = $state('');
	let platformHint = $state('');
	let linkToken = $state(page.url.searchParams.get('token') ?? '');
	const hasToken = $derived(linkToken.length > 0);
	const nextAuthHref = $derived(`/login?next=${encodeURIComponent(page.url.pathname)}`);
	const nextRegisterHref = $derived(`/register?next=${encodeURIComponent(page.url.pathname)}`);

	onMount(() => {
		const loadMetadata = async (rawToken: string) => {
			const response = await fetch(`/api/v1/auth/link/metadata?token=${encodeURIComponent(rawToken)}`);
			if (!response.ok) {
				return;
			}
			const payload = (await response.json()) as { tenant_slug: string; platform: string };
			tenantHint = payload.tenant_slug;
			platformHint = payload.platform;
			saveTenantSlug(payload.tenant_slug);
		};

		const queryToken = page.url.searchParams.get('token');
		if (queryToken) {
			sessionStorage.setItem(pendingLinkTokenKey, queryToken);
			linkToken = queryToken;
			history.replaceState({}, '', page.url.pathname);
			void loadMetadata(queryToken);
			return;
		}

		linkToken = sessionStorage.getItem(pendingLinkTokenKey) ?? '';
		if (linkToken) {
			void loadMetadata(linkToken);
		}
	});

	$effect(() => {
		if (!authStore.authenticated || !hasToken || linkStatus !== 'idle') {
			return;
		}

		linkStatus = 'linking';
		linkMessage = 'Connecting your messenger account...';

		void (async () => {
			const response = await apiFetch('/api/v1/auth/link/complete', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ token: linkToken })
			});

			if (!response.ok) {
				linkStatus = 'error';
				linkMessage = 'We could not complete this messenger link. The token may be expired or already claimed.';
				return;
			}

			const payload = (await response.json()) as { message?: string };
			sessionStorage.removeItem(pendingLinkTokenKey);
			linkStatus = 'linked';
			linkMessage = payload.message ?? 'Messenger account linked.';
		})();
	});
</script>

<svelte:head>
	<title>Link Messenger Account - SteerLane</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-6 py-10">
	<div class="w-full max-w-2xl rounded-3xl border border-slate-800 bg-slate-900/90 p-8 shadow-2xl">
		<p class="text-xs font-mono uppercase tracking-[0.22em] text-brand-secondary">Messenger linking</p>
		<h1 class="mt-3 text-3xl font-bold text-white">Continue account linking</h1>
		<p class="mt-3 text-sm leading-6 text-slate-400">
			Slack and future messenger integrations send you here after the first bot interaction. Keep this tab open until the browser hand-off finishes.
		</p>

		{#if tenantHint || platformHint}
			<div class="mt-4 rounded-2xl border border-slate-800 bg-slate-950 px-4 py-3 text-sm text-slate-300">
				Linking <span class="font-semibold text-white">{platformHint || 'messenger'}</span>
				{#if tenantHint}
					for tenant <span class="font-mono text-white">{tenantHint}</span>
				{/if}
			</div>
		{/if}

		<div class="mt-6 rounded-2xl border border-slate-800 bg-slate-950 p-5 text-sm text-slate-300">
			<div class="text-slate-500">Link request</div>
			<div class="mt-2 font-mono text-xs text-slate-200">{hasToken ? 'Secure hand-off received.' : 'Missing hand-off token.'}</div>
		</div>

		<div class="mt-6 space-y-3 text-sm text-slate-400">
			<p>
				{#if authStore.authenticated && hasToken}
					Finish this browser hand-off to connect the messenger identity that opened the link.
				{:else if authStore.authenticated}
					Your dashboard session is active, but this hand-off is missing a link token.
				{:else}
					Sign in or register with the same tenant, then this page will finish the browser hand-off automatically.
				{/if}
			</p>

			{#if authStore.authenticated && hasToken}
				<div class="rounded-2xl border border-slate-800 bg-slate-950 px-4 py-3 text-sm {linkStatus === 'error' ? 'text-red-300' : 'text-slate-300'}">
					{linkMessage}
				</div>
			{/if}
		</div>

		<div class="mt-6 flex gap-3">
			{#if authStore.authenticated}
				<button class="rounded-xl bg-brand-primary px-4 py-2 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400" onclick={() => goto('/settings/project')} type="button">
					Open project settings
				</button>
			{:else}
				<a class="rounded-xl bg-brand-primary px-4 py-2 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400" href={nextAuthHref}>Sign in</a>
				<a class="rounded-xl border border-slate-700 px-4 py-2 text-sm font-semibold text-slate-200 transition-colors hover:border-slate-500 hover:text-white" href={nextRegisterHref}>Create account</a>
			{/if}
		</div>
	</div>
</div>
