<script lang="ts">
	import ActivityRail from '$components/agent/ActivityRail.svelte';
	import Board from '$components/board/Board.svelte';
	import StatCard from '$components/StatCard.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
</script>

<div class="flex flex-1 overflow-hidden">
	<!-- Main Workspace -->
	<div class="flex min-w-0 flex-1 flex-col overflow-y-auto bg-slate-950 p-6">
		<header class="mb-8 flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-bold tracking-tight text-white">Project Overlook</h1>
				<p class="text-sm text-slate-400">Human + Agent Orchestration</p>
			</div>
			<button
				class="rounded bg-brand-primary px-4 py-2 text-sm font-semibold text-slate-950 hover:bg-amber-400 transition-colors"
			>
				New Task
			</button>
		</header>

		<!-- Stats Row -->
		<div class="mb-8 grid grid-cols-4 gap-4">
			<StatCard label="Active Agents" value="3" trend="+1" />
			<StatCard label="Tasks In Progress" value="12" />
			<StatCard label="Blocked Tasks" value="1" trend="Needs Human" urgent={true} />
			<StatCard label="Recent ADRs" value="5" />
		</div>

		<!-- Kanban Board -->
		<div
			class="flex-1 rounded-lg border border-slate-800 bg-brand-surface p-4 flex flex-col min-h-0"
		>
			{#if projectStore.selectedProjectId}
				<Board projectId={projectStore.selectedProjectId} />
			{:else}
				<div class="flex h-64 items-center justify-center">
					<div class="text-slate-500">Please select a project</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- Activity Rail -->
	<div class="w-80 shrink-0 border-l border-slate-800 bg-brand-surface p-4 flex flex-col">
		<h2 class="mb-4 text-sm font-bold uppercase tracking-wider text-slate-500">System Activity</h2>
		<ActivityRail />
	</div>
</div>
