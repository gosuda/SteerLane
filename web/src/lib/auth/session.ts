const tenantSlugKey = 'steerlane.tenant_slug';

function isBrowser() {
	return typeof window !== 'undefined';
}

export function getTenantSlug() {
	if (!isBrowser()) {
		return 'default';
	}
	return window.localStorage.getItem(tenantSlugKey) ?? 'default';
}

export function saveTenantSlug(tenantSlug: string) {
	if (!isBrowser()) {
		return;
	}

	window.localStorage.setItem(tenantSlugKey, tenantSlug);
}

export function clearTenantSlug() {
	if (!isBrowser()) {
		return;
	}

	window.localStorage.removeItem(tenantSlugKey);
}
