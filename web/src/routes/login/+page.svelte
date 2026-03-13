<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { authStore } from '$lib/stores/auth.svelte';

	let email = $state('');
	let password = $state('');
	let tenantSlug = $state(authStore.tenantSlug || 'default');
	const nextPath = $derived.by(() => {
		const next = page.url.searchParams.get('next') ?? '/';
		return next.startsWith('/') ? next : '/';
	});

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		const ok = await authStore.login({ email, password, tenantSlug });
		if (ok) {
			goto(nextPath);
		}
	}
</script>

<svelte:head>
	<title>Sign In - SteerLane</title>
</svelte:head>

<div class="grid min-h-screen lg:grid-cols-[1.1fr_0.9fr]">
	<section class="hidden border-r border-slate-800 bg-[radial-gradient(circle_at_top_left,_rgba(245,158,11,0.18),_transparent_45%),linear-gradient(180deg,_#020617_0%,_#0f172a_100%)] px-12 py-14 lg:flex lg:flex-col lg:justify-between">
		<div>
			<div class="mb-10 flex items-center gap-3">
				<div class="h-9 w-9 rounded-md bg-brand-primary"></div>
				<div>
					<div class="text-xl font-bold text-white">SteerLane</div>
					<div class="text-xs uppercase tracking-[0.24em] text-slate-500">Operator Console</div>
				</div>
			</div>
			<h1 class="max-w-xl text-5xl font-bold leading-tight text-white">
				Resume work without losing the board.
			</h1>
			<p class="mt-5 max-w-lg text-base leading-7 text-slate-300">
				Sign in to manage tasks, answer HITL prompts, and monitor active agent sessions from one tenant-aware dashboard.
			</p>
		</div>
		<div class="grid gap-4 text-sm text-slate-400">
			<div class="rounded-2xl border border-slate-800 bg-slate-900/70 p-5">
				Default self-hosted tenant slug: <span class="font-mono text-slate-200">default</span>
			</div>
			<div class="rounded-2xl border border-slate-800 bg-slate-900/70 p-5">
				Slack account links return through <span class="font-mono text-slate-200">/auth/link</span>.
			</div>
		</div>
	</section>

	<section class="flex items-center justify-center px-6 py-10 sm:px-10">
		<form class="w-full max-w-md rounded-3xl border border-slate-800 bg-slate-900/90 p-8 shadow-2xl" onsubmit={submit}>
			<p class="text-xs font-mono uppercase tracking-[0.22em] text-brand-secondary">Sign in</p>
			<h2 class="mt-3 text-3xl font-bold text-white">Welcome back</h2>
			<p class="mt-2 text-sm leading-6 text-slate-400">
				Use your tenant slug, email, and password to unlock the dashboard.
			</p>

			<div class="mt-8 space-y-4">
				<label class="block">
					<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Tenant slug</span>
					<input bind:value={tenantSlug} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" placeholder="default" required />
				</label>

				<label class="block">
					<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Email</span>
					<input bind:value={email} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" placeholder="operator@example.com" required type="email" />
				</label>

				<label class="block">
					<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Password</span>
					<input bind:value={password} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" required type="password" />
				</label>
			</div>

			{#if authStore.error}
				<div class="mt-4 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300">
					{authStore.error}
				</div>
			{/if}

			<button class="mt-6 w-full rounded-xl bg-brand-primary px-4 py-3 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400 disabled:cursor-not-allowed disabled:opacity-60" disabled={authStore.loading} type="submit">
				{authStore.loading ? 'Signing in...' : 'Enter dashboard'}
			</button>

			<p class="mt-5 text-sm text-slate-400">
				Need an account?
				<a class="font-medium text-white underline-offset-4 hover:underline" href={`/register?next=${encodeURIComponent(nextPath)}`}>Create one</a>
			</p>
		</form>
	</section>
</div>
