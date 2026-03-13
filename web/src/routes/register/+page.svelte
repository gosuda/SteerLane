<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { authStore } from '$lib/stores/auth.svelte';

	let name = $state('');
	let email = $state('');
	let password = $state('');
	let tenantSlug = $state(authStore.tenantSlug || 'default');
	const nextPath = $derived.by(() => {
		const next = page.url.searchParams.get('next') ?? '/';
		return next.startsWith('/') ? next : '/';
	});

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		const ok = await authStore.register({ name, email, password, tenantSlug });
		if (ok) {
			goto(nextPath);
		}
	}
</script>

<svelte:head>
	<title>Create Account - SteerLane</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-[radial-gradient(circle_at_top,_rgba(34,211,238,0.14),_transparent_40%),linear-gradient(180deg,_#020617_0%,_#111827_100%)] px-6 py-10">
	<form class="w-full max-w-lg rounded-3xl border border-slate-800 bg-slate-900/90 p-8 shadow-2xl" onsubmit={submit}>
		<p class="text-xs font-mono uppercase tracking-[0.22em] text-brand-secondary">Register</p>
		<h1 class="mt-3 text-3xl font-bold text-white">Create an operator account</h1>
		<p class="mt-2 text-sm leading-6 text-slate-400">
			Use the tenant slug for your SteerLane workspace. Self-hosted environments typically use <span class="font-mono text-slate-200">default</span>.
		</p>

		<div class="mt-8 grid gap-4 sm:grid-cols-2">
			<label class="block sm:col-span-2">
				<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Full name</span>
				<input bind:value={name} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" placeholder="Avery Operator" required />
			</label>

			<label class="block">
				<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Tenant slug</span>
				<input bind:value={tenantSlug} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" placeholder="default" required />
			</label>

			<label class="block">
				<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Email</span>
				<input bind:value={email} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" placeholder="operator@example.com" required type="email" />
			</label>

			<label class="block sm:col-span-2">
				<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Password</span>
				<input bind:value={password} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" minlength="8" required type="password" />
			</label>
		</div>

		{#if authStore.error}
			<div class="mt-4 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300">
				{authStore.error}
			</div>
		{/if}

		<button class="mt-6 w-full rounded-xl bg-brand-primary px-4 py-3 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400 disabled:cursor-not-allowed disabled:opacity-60" disabled={authStore.loading} type="submit">
			{authStore.loading ? 'Creating account...' : 'Create account'}
		</button>

		<p class="mt-5 text-sm text-slate-400">
			Already have an account?
			<a class="font-medium text-white underline-offset-4 hover:underline" href={`/login?next=${encodeURIComponent(nextPath)}`}>Sign in</a>
		</p>
	</form>
</div>
