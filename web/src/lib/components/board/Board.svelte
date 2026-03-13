<script lang="ts">
	import { boardStore } from '$lib/stores/board.svelte';
	import type { components } from '$lib/api/schema';
	import { onMount, onDestroy } from 'svelte';
	import { BoardWSClient } from '$lib/ws/client';

	type Task = components['schemas']['TaskResponse'];
	type TaskStatus = Task['status'];

	let { projectId } = $props<{ projectId: string }>();

	const columns: { id: TaskStatus; title: string }[] = [
		{ id: 'backlog', title: 'Backlog' },
		{ id: 'in_progress', title: 'In Progress' },
		{ id: 'review', title: 'Review' },
		{ id: 'done', title: 'Done' }
	];

	let draggedTask = $state<Task | null>(null);
	let wsClient: BoardWSClient | null = null;
	let wsConnected = $state(false);

	$effect(() => {
		if (projectId) {
			boardStore.loadTasks(projectId);

			if (wsClient) {
				wsClient.disconnect();
			}

			wsClient = new BoardWSClient(projectId);
			wsClient.onConnectionChange = (connected) => {
				wsConnected = connected;
			};

			wsClient.subscribe((event) => {
				boardStore.handleWebSocketEvent(event);
			});

			wsClient.connect();
		}
	});

	onDestroy(() => {
		if (wsClient) {
			wsClient.disconnect();
		}
	});

	function handleDragStart(e: DragEvent, task: Task) {
		draggedTask = task;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', task.id);
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		if (e.dataTransfer) {
			e.dataTransfer.dropEffect = 'move';
		}
	}

	function handleDrop(e: DragEvent, targetStatus: TaskStatus) {
		e.preventDefault();
		if (draggedTask && draggedTask.status !== targetStatus) {
			boardStore.transitionTask(draggedTask.id, targetStatus);
		}
		draggedTask = null;
	}

	function handleDragEnd() {
		draggedTask = null;
	}
</script>

<div class="flex h-full flex-col">
	<div class="mb-4 flex items-center justify-between">
		<h2 class="text-lg font-semibold text-white">Active Sprint</h2>
		<div class="flex items-center gap-4">
			<div
				class="flex items-center gap-2 rounded-full border border-slate-700 bg-slate-950 px-3 py-1 font-mono text-xs"
			>
				<div
					class="h-2 w-2 rounded-full {wsConnected
						? 'bg-emerald-500'
						: 'bg-rose-500'} transition-colors"
				></div>
				<span class="text-slate-300">WS {wsConnected ? 'Connected' : 'Disconnected'}</span>
			</div>
			<div class="flex items-center gap-2">
				<span class="font-mono text-xs text-slate-500">Filter:</span>
				<select
					class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-xs text-slate-300"
				>
					<option>All Tasks</option>
					<option>My Tasks</option>
					<option>Agent Tasks</option>
				</select>
			</div>
		</div>
	</div>

	{#if boardStore.loading && boardStore.tasks.length === 0}
		<div class="flex h-64 items-center justify-center">
			<div class="text-slate-500">Loading tasks...</div>
		</div>
	{:else if boardStore.error}
		<div class="flex h-64 items-center justify-center">
			<div class="rounded bg-rose-500/10 px-4 py-3 text-sm text-rose-400 border border-rose-500/20">
				{boardStore.error}
			</div>
		</div>
	{:else}
		<div class="flex flex-1 gap-4 overflow-x-auto pb-2 min-h-0">
			{#each columns as column}
				{@const columnTasks = boardStore.tasks
					.filter((t) => t.status === column.id)
					.sort((a, b) => b.priority - a.priority || a.title.localeCompare(b.title))}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="flex w-72 shrink-0 flex-col rounded-md bg-slate-900 border border-slate-800 p-3"
					ondragover={handleDragOver}
					ondrop={(e) => handleDrop(e, column.id)}
				>
					<div class="mb-3 flex items-center justify-between px-1">
						<h3 class="text-sm font-semibold text-slate-300">{column.title}</h3>
						<span class="rounded bg-slate-800 px-2 py-0.5 font-mono text-xs text-slate-400">
							{columnTasks.length}
						</span>
					</div>

					<div class="flex flex-1 flex-col gap-2 overflow-y-auto min-h-0">
						{#each columnTasks as task (task.id)}
							<div
								class="flex cursor-grab flex-col gap-2 rounded border border-slate-700 bg-brand-surface-elevated p-3 shadow-sm hover:border-slate-500 transition-colors"
								class:opacity-50={draggedTask?.id === task.id}
								draggable="true"
								ondragstart={(e) => handleDragStart(e, task)}
								ondragend={handleDragEnd}
							>
								<div class="flex items-start justify-between gap-2">
									<span class="font-mono text-xs font-medium text-brand-secondary"
										>{task.id.split('-')[0]}-{task.id.slice(-4)}</span
									>
									{#if task.priority === 0}
										<span class="h-2 w-2 rounded-full bg-rose-500 mt-1" title="Critical"></span>
									{:else if task.priority === 1}
										<span class="h-2 w-2 rounded-full bg-amber-500 mt-1" title="High"></span>
									{/if}
								</div>
								<p class="text-sm text-slate-200 line-clamp-2">{task.title}</p>
								<div class="mt-2 flex items-center justify-between text-xs">
									<span class="text-slate-500 truncate"
										>{task.assigned_to
											? 'Human'
											: task.agent_session_id
												? 'Agent'
												: 'Unassigned'}</span
									>
								{#if task.adr_id}
									<a
										href={`/adrs/${task.adr_id}`}
										class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-400"
										onclick={(e) => e.stopPropagation()}
										>ADR</a
									>
								{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
