export interface components {
	schemas: {
		ProjectResponse: {
			created_at: string;
			settings?: Record<string, unknown>;
			name: string;
			repo_url: string;
			branch: string;
			id: string;
		};
		ProjectListResponse: {
			next_cursor?: string;
			items: components['schemas']['ProjectResponse'][];
		};
		TaskResponse: {
			created_at: string;
			updated_at: string;
			adr_id?: string;
			assigned_to?: string;
			agent_session_id?: string;
			title: string;
			description: string;
			status: 'backlog' | 'in_progress' | 'review' | 'done';
			priority: number;
			id: string;
			project_id: string;
		};
		TaskListResponse: {
			next_cursor?: string;
			items: components['schemas']['TaskResponse'][];
		};
	};
}

export interface paths {
	'/projects': {
		get: {
			responses: {
				200: {
					content: {
						'application/json': components['schemas']['ProjectListResponse'];
					};
				};
			};
		};
	};
	'/tasks': {
		get: {
			parameters: {
				query: {
					project_id: string;
					status?: string;
					priority?: number;
					limit?: number;
					cursor?: string;
				};
			};
			responses: {
				200: {
					content: {
						'application/json': components['schemas']['TaskListResponse'];
					};
				};
			};
		};
		post: {
			requestBody: {
				content: {
					'application/json': {
						title: string;
						description?: string;
						status?: string;
						priority?: number;
						project_id: string;
					};
				};
			};
			responses: {
				200: {
					content: {
						'application/json': components['schemas']['TaskResponse'];
					};
				};
			};
		};
	};
	'/tasks/{id}/transition': {
		post: {
			parameters: {
				path: {
					id: string;
				};
			};
			requestBody: {
				content: {
					'application/json': {
						status: 'backlog' | 'in_progress' | 'review' | 'done';
					};
				};
			};
			responses: {
				200: {
					content: {
						'application/json': components['schemas']['TaskResponse'];
					};
				};
			};
		};
	};
}
