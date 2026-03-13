import { clearTenantSlug, getTenantSlug, saveTenantSlug } from '$lib/auth/session';

interface LoginForm {
	email: string;
	password: string;
	tenantSlug: string;
}

interface RegisterForm extends LoginForm {
	name: string;
}

export function createAuthStore() {
	let authenticated = $state(false);
	let initialized = $state(false);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let sessionVersion = $state(0);
	let tenantSlug = $state(getTenantSlug());

	function updateSession(isAuthenticated: boolean, nextTenantSlug = tenantSlug) {
		authenticated = isAuthenticated;
		tenantSlug = nextTenantSlug;
		sessionVersion += 1;
		initialized = true;
	}

	async function initialize() {
		const resolveSession = async () => {
			const response = await fetch('/api/v1/auth/session');
			if (!response.ok) {
				return null;
			}
			return (await response.json()) as { authenticated: boolean; tenant_slug?: string };
		};

		try {
			let payload = await resolveSession();
			if (!payload) {
				const refreshed = await fetch('/api/v1/auth/session/refresh', { method: 'POST' });
				if (refreshed.ok) {
					payload = await resolveSession();
				}
			}
			if (!payload) {
				updateSession(false, getTenantSlug());
				return;
			}

			if (payload.tenant_slug) {
				saveTenantSlug(payload.tenant_slug);
			}
			updateSession(Boolean(payload.authenticated), payload.tenant_slug ?? getTenantSlug());
		} catch {
			updateSession(false, getTenantSlug());
		}
	}

	async function login(form: LoginForm) {
		loading = true;
		error = null;
		try {
			const response = await fetch('/api/v1/auth/session/login', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					email: form.email,
					password: form.password,
					tenant_slug: form.tenantSlug
				})
			});

			if (!response.ok) {
				error = 'Unable to sign in with those credentials.';
				return false;
			}

			saveTenantSlug(form.tenantSlug);
			updateSession(true, form.tenantSlug);
			return true;
		} catch {
			error = 'Unable to reach the authentication service.';
			return false;
		} finally {
			loading = false;
		}
	}

	async function register(form: RegisterForm) {
		loading = true;
		error = null;
		try {
			const response = await fetch('/api/v1/auth/session/register', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					email: form.email,
					password: form.password,
					name: form.name,
					tenant_slug: form.tenantSlug
				})
			});

			if (!response.ok) {
				error = 'Unable to create that account.';
				return false;
			}

			saveTenantSlug(form.tenantSlug);
			updateSession(true, form.tenantSlug);
			return true;
		} catch {
			error = 'Unable to reach the registration service.';
			return false;
		} finally {
			loading = false;
		}
	}

	async function logout() {
		await fetch('/api/v1/auth/session/logout', { method: 'POST' });
		clearTenantSlug();
		error = null;
		updateSession(false, 'default');
	}

	return {
		get authenticated() {
			return authenticated;
		},
		get error() {
			return error;
		},
		get initialized() {
			return initialized;
		},
		get loading() {
			return loading;
		},
		get sessionVersion() {
			return sessionVersion;
		},
		get tenantSlug() {
			return tenantSlug;
		},
		initialize,
		login,
		logout,
		register,
		syncFromStorage: initialize
	};
}

export const authStore = createAuthStore();
