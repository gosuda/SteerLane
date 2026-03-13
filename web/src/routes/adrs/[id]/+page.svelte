<script lang="ts">
	import { page } from '$app/state';
	import { apiFetch } from '$lib/api/request';
	import type { ADR } from '$lib/api/types';

	let adrId = $derived(page.params.id);
	let adr = $state<ADR | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let reviewing = $state(false);

	$effect(() => {
		if (adrId) {
			void fetchAdr(adrId);
		}
	});

	async function fetchAdr(id: string) {
		loading = true;
		error = null;
		try {
			const res = await apiFetch(`/api/v1/adrs/${id}`);
			if (!res.ok) {
				throw new Error('Failed to fetch ADR details');
			}

			adr = (await res.json()) as ADR;
		} catch (err: unknown) {
			error = err instanceof Error ? err.message : 'Failed to fetch ADR details';
		} finally {
			loading = false;
		}
	}

	async function reviewAdr(status: 'accepted' | 'rejected' | 'deprecated') {
		if (!adr) return;
		reviewing = true;
		error = null;
		try {
			const res = await apiFetch(`/api/v1/adrs/${adr.id}/review`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ status })
			});
			if (!res.ok) {
				throw new Error(`Failed to ${status} ADR`);
			}
			adr = (await res.json()) as ADR;
		} catch (err: unknown) {
			error = err instanceof Error ? err.message : `Failed to ${status} ADR`;
		} finally {
			reviewing = false;
		}
	}
</script>

<svelte:head>
	<title>{adr ? `${adr.title} - SteerLane` : 'ADR Details'}</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-4 py-8 text-white">
	<div class="mb-6">
		<a
			href="/adrs"
			class="text-sm font-medium text-slate-400 hover:text-white flex items-center gap-1 transition-colors"
		>
			&larr; Back to timeline
		</a>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-primary"></div>
		</div>
	{:else if error}
		<div class="bg-red-950 border border-red-900 text-red-400 px-4 py-3 rounded-md">
			{error}
		</div>
	{:else if adr}
		<article class="bg-brand-surface rounded-xl shadow-sm border border-slate-800 overflow-hidden">
			<header class="bg-slate-900 border-b border-slate-800 p-6 sm:p-8">
				<div class="flex flex-wrap items-start justify-between gap-4 mb-4">
					<h1 class="text-3xl font-bold font-sans text-white leading-tight">{adr.title}</h1>
					<span
						class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium capitalize
            {adr.status === 'accepted'
							? 'bg-green-900/50 text-green-400 border border-green-800/50'
							: adr.status === 'rejected'
								? 'bg-red-900/50 text-red-400 border border-red-800/50'
								: adr.status === 'draft'
									? 'bg-yellow-900/50 text-yellow-400 border border-yellow-800/50'
									: adr.status === 'proposed'
										? 'bg-blue-900/50 text-blue-400 border border-blue-800/50'
										: 'bg-slate-800 text-slate-300 border border-slate-700'}"
					>
						{adr.status}
					</span>
				</div>
				<div class="flex items-center justify-between gap-4">
					<div class="flex items-center gap-4 text-sm text-slate-400 font-mono">
						<span>Date: {new Date(adr.created_at).toLocaleDateString()}</span>
						<span>ADR #{adr.sequence}</span>
					</div>

					{#if adr.status === 'proposed'}
						<div class="flex items-center gap-3">
							<button
								onclick={() => void reviewAdr('accepted')}
								disabled={reviewing}
								class="px-4 py-1.5 rounded-md text-sm font-medium bg-green-900/30 text-green-400 border border-green-800/50 hover:bg-green-900/50 focus:outline-none focus:ring-2 focus:ring-green-500/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
							>
								Accept
							</button>
							<button
								onclick={() => void reviewAdr('rejected')}
								disabled={reviewing}
								class="px-4 py-1.5 rounded-md text-sm font-medium bg-red-900/30 text-red-400 border border-red-800/50 hover:bg-red-900/50 focus:outline-none focus:ring-2 focus:ring-red-500/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
							>
								Reject
							</button>
							<button
								onclick={() => void reviewAdr('deprecated')}
								disabled={reviewing}
								class="px-4 py-1.5 rounded-md text-sm font-medium bg-amber-900/30 text-amber-400 border border-amber-800/50 hover:bg-amber-900/50 focus:outline-none focus:ring-2 focus:ring-amber-500/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
							>
								Deprecate
							</button>
						</div>
					{/if}
				</div>
			</header>

			<div class="p-6 sm:p-8 space-y-8">
				<section>
					<h2 class="text-xl font-bold text-white mb-3 border-b border-slate-800 pb-2">Context</h2>
					<div class="prose prose-invert max-w-none text-slate-300 whitespace-pre-wrap">
						{adr.context}
					</div>
				</section>

				<section>
					<h2 class="text-xl font-bold text-white mb-3 border-b border-slate-800 pb-2">Decision</h2>
					<div class="prose prose-invert max-w-none text-slate-300 whitespace-pre-wrap">
						{adr.decision}
					</div>
				</section>

				{#if adr.drivers && adr.drivers.length > 0}
					<section>
						<h2 class="text-xl font-bold text-white mb-3 border-b border-slate-800 pb-2">
							Drivers
						</h2>
						<ul class="list-disc list-inside space-y-1 text-slate-300 ml-4">
							{#each adr.drivers as driver}
								<li>{driver}</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if adr.consequences}
					<section>
						<h2 class="text-xl font-bold text-white mb-3 border-b border-slate-800 pb-2">
							Consequences
						</h2>

						{#if adr.consequences.good && adr.consequences.good.length > 0}
							<div class="mb-4">
								<h3 class="mb-2 flex items-center gap-2 font-medium text-green-400">
									<span class="text-lg">+</span> Good
								</h3>
								<ul class="ml-4 list-inside list-disc space-y-1 text-slate-300">
									{#each adr.consequences.good as item}
										<li>{item}</li>
									{/each}
								</ul>
							</div>
						{/if}

						{#if adr.consequences.bad && adr.consequences.bad.length > 0}
							<div class="mb-4">
								<h3 class="mb-2 flex items-center gap-2 font-medium text-red-400">
									<span class="text-lg">-</span> Bad
								</h3>
								<ul class="ml-4 list-inside list-disc space-y-1 text-slate-300">
									{#each adr.consequences.bad as item}
										<li>{item}</li>
									{/each}
								</ul>
							</div>
						{/if}

						{#if adr.consequences.neutral && adr.consequences.neutral.length > 0}
							<div>
								<h3 class="mb-2 flex items-center gap-2 font-medium text-slate-400">
									<span class="text-lg">&middot;</span> Neutral
								</h3>
								<ul class="ml-4 list-inside list-disc space-y-1 text-slate-300">
									{#each adr.consequences.neutral as item}
										<li>{item}</li>
									{/each}
								</ul>
							</div>
						{/if}
					</section>
				{/if}

				{#if adr.options}
					<section>
						<h2 class="mb-3 border-b border-slate-800 pb-2 text-xl font-bold text-white">
							Options
						</h2>
						<pre
							class="overflow-x-auto rounded-lg border border-slate-800 bg-slate-950 p-4 text-xs text-slate-300"
						>{JSON.stringify(adr.options, null, 2)}</pre>
					</section>
				{/if}
			</div>
		</article>
	{/if}
</div>
