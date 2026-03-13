<script lang="ts">
	let sessionId = $state('');
</script>

<svelte:head>
	<title>Agent Monitor - SteerLane</title>
</svelte:head>

<div class="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center px-6 py-12 text-white">
	<div class="rounded-2xl border border-slate-800 bg-brand-surface p-8 shadow-lg">
		<p class="mb-2 text-xs font-mono uppercase tracking-[0.2em] text-brand-secondary">Agent Monitor</p>
		<h1 class="mb-3 text-3xl font-bold tracking-tight text-white">Attach to a live session</h1>
		<p class="mb-6 max-w-2xl text-sm leading-6 text-slate-400">
			Paste an agent session ID to open the live monitor stream. This route stays client-side so
			static SPA fallback continues to work for embedded builds.
		</p>

		<form class="flex flex-col gap-3 sm:flex-row" action={`/monitor/${sessionId || ''}`}>
			<input
				bind:value={sessionId}
				class="flex-1 rounded-lg border border-slate-700 bg-slate-950 px-4 py-3 font-mono text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary"
				placeholder="agent-session-uuid"
				type="text"
			/>
			<button
				class="rounded-lg bg-brand-primary px-5 py-3 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400"
				type="submit"
			>
				Open Monitor
			</button>
		</form>
	</div>
</div>
