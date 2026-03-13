<script lang="ts">
	let {
		label,
		value,
		trend = '',
		urgent = false
	} = $props<{
		label: string;
		value: string | number;
		trend?: string;
		urgent?: boolean;
	}>();
</script>

<div class="flex flex-col justify-between rounded-lg border border-slate-800 bg-brand-surface p-4">
	<span class="text-sm font-medium text-slate-400">{label}</span>
	<div class="mt-2 flex items-baseline gap-2">
		<span class="text-3xl font-bold tracking-tight text-white">{value}</span>
		{#if trend}
			<span class="font-mono text-xs {urgent ? 'text-rose-400' : 'text-emerald-400'}">
				{trend}
			</span>
		{/if}
	</div>
</div>
