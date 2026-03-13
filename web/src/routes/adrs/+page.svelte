<script lang="ts">
	import { projectStore } from '$lib/stores/project.svelte';
	import { apiFetch } from '$lib/api/request';
	import type { ADR, Paginated } from '$lib/api/types';

	let adrs = $state<ADR[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	$effect(() => {
		const projectId = projectStore.selectedProjectId;
		if (projectId) {
			void loadADRs(projectId);
		}
	});

	async function loadADRs(projectId: string) {
		loading = true;
		error = null;
		try {
			const res = await apiFetch(`/api/v1/projects/${projectId}/adrs`);
			if (!res.ok) {
				throw new Error('Failed to fetch ADRs');
			}

			const data = (await res.json()) as Paginated<ADR>;
			adrs = data.items || [];
		} catch (err: unknown) {
			error = err instanceof Error ? err.message : 'Failed to load ADRs';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>ADRs - SteerLane</title>
</svelte:head>

<div class="max-w-5xl mx-auto px-4 py-8 text-white">
	<div class="flex items-center justify-between mb-8">
		<h1 class="text-3xl font-bold font-sans text-white">Architecture Decision Records</h1>
		<a
			href="/adrs/new"
			class="px-4 py-2 bg-brand-primary text-slate-950 rounded-md font-medium text-sm hover:bg-amber-400 transition-colors"
		>
			New ADR
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
	{:else if adrs.length === 0}
		<div class="text-center py-12 bg-slate-900 rounded-lg border border-slate-800">
			<h3 class="text-lg font-medium text-slate-200 mb-1">No ADRs yet</h3>
			<p class="text-slate-400">Create an Architectural Decision Record to document a choice.</p>
		</div>
	{:else}
		<div class="grid gap-4">
			{#each adrs as adr}
				<a
					href={`/adrs/${adr.id}`}
					class="block p-6 bg-slate-900 border border-slate-800 rounded-lg shadow-sm hover:border-slate-600 transition-all"
				>
					<div class="flex items-start justify-between gap-4 mb-2">
						<h2 class="text-xl font-bold text-slate-100">{adr.title}</h2>
						<span
							class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium capitalize
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
					<p class="text-slate-400 line-clamp-2 mb-4">{adr.context}</p>
					<div class="text-xs text-slate-500 font-mono">
						{new Date(adr.created_at).toLocaleDateString()} &middot; ID: {adr.id.split('-')[0]}
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>
