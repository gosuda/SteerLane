import { apiClient } from '$lib/api/client';
import type { components } from '$lib/api/schema';
import { projectStore } from './project.svelte';

type Task = components['schemas']['TaskResponse'];
type TaskStatus = Task['status'];

export function createBoardStore() {
	let tasks = $state<Task[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function loadTasks(projectId: string) {
		loading = true;
		error = null;
		try {
			const { data, error: apiError } = await apiClient.GET('/tasks', {
				params: {
					query: { project_id: projectId }
				}
			});
			if (apiError) {
				error = (apiError as any).message || 'Failed to load tasks';
				return;
			}
			if (data && data.items) {
				tasks = data.items;
			} else {
				tasks = [];
			}
		} catch (e: any) {
			error = e.message || 'Error fetching tasks';
		} finally {
			loading = false;
		}
	}

	async function transitionTask(taskId: string, newStatus: TaskStatus) {
		// Optimistic update
		const taskIndex = tasks.findIndex((t) => t.id === taskId);
		if (taskIndex === -1) return;

		const oldStatus = tasks[taskIndex].status;
		tasks[taskIndex].status = newStatus;

		try {
			const { error: apiError } = await apiClient.POST('/tasks/{id}/transition', {
				params: { path: { id: taskId } },
				body: { status: newStatus }
			});

			if (apiError) {
				// Revert
				tasks[taskIndex].status = oldStatus;
				error = (apiError as any).message || 'Failed to transition task';
			}
		} catch (e: any) {
			tasks[taskIndex].status = oldStatus;
			error = e.message || 'Error transitioning task';
		}
	}

	function handleWebSocketEvent(event: { type: string; payload?: Task | { id?: string; project_id?: string } }) {
		if (!event.payload || event.payload.project_id !== projectStore.selectedProjectId) {
			return;
		}

		switch (event.type) {
			case 'task.created':
			case 'task.updated':
			case 'task.transition': {
				const payload = event.payload as Task;
				const existingIndex = tasks.findIndex((t) => t.id === payload.id);
				if (existingIndex >= 0) {
					tasks[existingIndex] = { ...tasks[existingIndex], ...payload };
				} else {
					tasks.push(payload);
				}
				break;
			}
			case 'task.deleted': {
				const payload = event.payload as { id?: string };
				if (payload.id) {
					tasks = tasks.filter((t) => t.id !== payload.id);
				}
				break;
			}
		}
	}

	return {
		get tasks() {
			return tasks;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		loadTasks,
		transitionTask,
		handleWebSocketEvent
	};
}

export const boardStore = createBoardStore();
