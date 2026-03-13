import createClient from 'openapi-fetch';
import type { paths } from './schema';
import { apiFetch } from './request';

/**
 * Shared API client instance configured to talk to the Go backend.
 * Uses openapi-fetch for type-safe requests based on the OpenAPI schema.
 */
export const apiClient = createClient<paths>({
	baseUrl: '/api/v1',
	fetch: (request) => apiFetch(request)
});
