let refreshPromise: Promise<string | null> | null = null;

function notifyAuthInvalid() {
	if (typeof window === 'undefined') {
		return;
	}
	window.dispatchEvent(new CustomEvent('steerlane:auth-invalid'));
}

function isAuthRoute(url: string) {
	return url.includes('/api/v1/auth/session/login') || url.includes('/api/v1/auth/session/register') || url.includes('/api/v1/auth/session/refresh');
}

async function execute(request: Request) {
	return fetch(request.clone());
}

async function refreshAccessToken() {
	if (refreshPromise) {
		return refreshPromise;
	}

	refreshPromise = (async () => {
		try {
			const response = await fetch('/api/v1/auth/session/refresh', { method: 'POST' });
			if (!response.ok) {
				return null;
			}
			return 'ok';
		} catch {
			return null;
		}
	})();

	try {
		return await refreshPromise;
	} finally {
		refreshPromise = null;
	}
}

export async function apiFetch(input: RequestInfo | URL, init?: RequestInit) {
	const request = input instanceof Request && init === undefined ? input : new Request(input, init);
	let response = await execute(request);
	if (response.status !== 401 || isAuthRoute(request.url)) {
		return response;
	}

	const refreshed = await refreshAccessToken();
	if (!refreshed) {
		notifyAuthInvalid();
		return response;
	}

	response = await execute(request);
	if (response.status === 401) {
		notifyAuthInvalid();
	}
	return response;
}
