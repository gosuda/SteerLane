<script lang="ts">
	import { page } from '$app/state';

	const hasCode = $derived(Boolean(page.url.searchParams.get('code')));
	const hasError = $derived(Boolean(page.url.searchParams.get('error')));
</script>

<svelte:head>
	<title>OAuth Callback - SteerLane</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-6 py-10">
	<div class="w-full max-w-2xl rounded-3xl border border-slate-800 bg-slate-900/90 p-8 shadow-2xl">
		<p class="text-xs font-mono uppercase tracking-[0.22em] text-brand-secondary">OAuth callback</p>
		<h1 class="mt-3 text-3xl font-bold text-white">Provider hand-off received</h1>
		<p class="mt-3 text-sm leading-6 text-slate-400">
			Enterprise OAuth providers are tracked separately in the Phase 3 backlog. This route safely captures callback state so the browser does not land on a missing page.
		</p>

		<div class="mt-6 space-y-3 rounded-2xl border border-slate-800 bg-slate-950 p-5 text-sm text-slate-300">
			<div>
				<span class="text-slate-500">Authorization payload:</span>
				<span class="ml-2 font-mono text-slate-200">{hasCode ? 'received' : 'not provided'}</span>
			</div>
			<div>
				<span class="text-slate-500">Provider error:</span>
				<span class="ml-2 font-mono text-slate-200">{hasError ? 'present' : 'none'}</span>
			</div>
		</div>

		<div class="mt-6 flex gap-3">
			<a class="rounded-xl bg-brand-primary px-4 py-2 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400" href="/login">Back to sign in</a>
			<a class="rounded-xl border border-slate-700 px-4 py-2 text-sm font-semibold text-slate-200 transition-colors hover:border-slate-500 hover:text-white" href="/register">Create account</a>
		</div>
	</div>
</div>
