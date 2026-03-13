import { apiFetch } from '$lib/api/request';

export interface TenantSummary {
	id: string;
	name: string;
	slug: string;
	settings?: Record<string, unknown>;
	created_at: string;
	updated_at: string;
}

export function createTenantStore() {
	let tenant = $state<TenantSummary | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let activeLoadRequest = 0;

	async function loadTenant() {
		const requestId = ++activeLoadRequest;
		loading = true;
		error = null;
		try {
			const response = await apiFetch('/api/v1/tenants/me');
			if (!response.ok) {
				throw new Error('Failed to load tenant');
			}
			if (requestId !== activeLoadRequest) {
				return;
			}
			tenant = (await response.json()) as TenantSummary;
		} catch (err: unknown) {
			if (requestId !== activeLoadRequest) {
				return;
			}
			tenant = null;
			error = err instanceof Error ? err.message : 'Failed to load tenant';
		} finally {
			if (requestId !== activeLoadRequest) {
				return;
			}
			loading = false;
		}
	}

	function reset() {
		activeLoadRequest += 1;
		tenant = null;
		loading = false;
		error = null;
	}

	return {
		get error() {
			return error;
		},
		get loading() {
			return loading;
		},
		get tenant() {
			return tenant;
		},
		loadTenant,
		reset
	};
}

export const tenantStore = createTenantStore();
