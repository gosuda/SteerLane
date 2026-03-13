<script lang="ts">
	const mockActivities = [
		{ id: 1, type: 'agent_start', message: 'Agent Odin started task T-104', time: 'Just now' },
		{ id: 2, type: 'human_comment', message: 'Alice commented on T-101', time: '5m ago' },
		{ id: 3, type: 'adr_create', message: 'ADR-004 created: API Error Handling', time: '1h ago' },
		{ id: 4, type: 'agent_complete', message: 'Agent Thor completed T-099', time: '2h ago' },
		{ id: 5, type: 'system', message: 'Deployment successful', time: '3h ago' }
	];

	function getIconColor(type: string) {
		switch (type) {
			case 'agent_start':
				return 'text-brand-secondary bg-brand-secondary/10';
			case 'agent_complete':
				return 'text-emerald-400 bg-emerald-400/10';
			case 'human_comment':
				return 'text-amber-400 bg-amber-400/10';
			case 'adr_create':
				return 'text-purple-400 bg-purple-400/10';
			default:
				return 'text-slate-400 bg-slate-800';
		}
	}
</script>

<div class="flex flex-col gap-4 flex-1 overflow-y-auto min-h-0 pr-2">
	{#each mockActivities as activity}
		<div class="flex gap-3 text-sm">
			<div
				class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full {getIconColor(
					activity.type
				)}"
			>
				<!-- Placeholder icon dot -->
				<div class="h-2 w-2 rounded-full bg-current"></div>
			</div>
			<div class="flex flex-col gap-1">
				<p class="text-slate-300">{activity.message}</p>
				<span class="font-mono text-xs text-slate-500">{activity.time}</span>
			</div>
		</div>
	{/each}
</div>

<div class="mt-4 border-t border-slate-800 pt-4">
	<div class="rounded border border-slate-700 bg-slate-900 p-3">
		<h3 class="mb-2 text-xs font-semibold uppercase text-slate-500">Live Agent Feed</h3>
		<div class="font-mono text-[10px] leading-relaxed text-emerald-400">
			> Analyzing codebase...<br />
			> Found 14 files matching criteria<br />
			> Generating response layout...<br />
			<span class="animate-pulse">_</span>
		</div>
	</div>
</div>
