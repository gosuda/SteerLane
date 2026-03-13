import { apiClient } from '$lib/api/client';
import type { components } from '$lib/api/schema';

type Project = components['schemas']['ProjectResponse'];

export function createProjectStore() {
	let projects = $state<Project[]>([]);
	let selectedProjectId = $state<string | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let activeLoadRequest = 0;

	async function loadProjects() {
		const requestId = ++activeLoadRequest;
		loading = true;
		error = null;
		try {
			const { data, error: apiError } = await apiClient.GET('/projects');
			if (apiError) {
				if (requestId !== activeLoadRequest) {
					return;
				}
				error = (apiError as any).message || 'Failed to load projects';
				return;
			}
			if (requestId !== activeLoadRequest) {
				return;
			}
			if (data && data.items) {
				projects = data.items;
				// If no project selected, select the first one
				if (!selectedProjectId && projects.length > 0) {
					selectedProjectId = projects[0].id;
				}
			}
		} catch (e: any) {
			if (requestId !== activeLoadRequest) {
				return;
			}
			error = e.message || 'Error fetching projects';
		} finally {
			if (requestId !== activeLoadRequest) {
				return;
			}
			loading = false;
		}
	}

	function selectProject(id: string) {
		selectedProjectId = id;
	}

	function reset() {
		activeLoadRequest += 1;
		projects = [];
		selectedProjectId = null;
		loading = false;
		error = null;
	}

	return {
		get projects() {
			return projects;
		},
		get selectedProjectId() {
			return selectedProjectId;
		},
		get selectedProject() {
			return projects.find((p) => p.id === selectedProjectId) || null;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		loadProjects,
		reset,
		selectProject
	};
}

export const projectStore = createProjectStore();
