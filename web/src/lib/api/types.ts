export interface ADR {
	id: string;
	project_id: string;
	title: string;
	status: 'draft' | 'proposed' | 'accepted' | 'rejected' | 'deprecated';
	context: string;
	decision: string;
	consequences: {
		good?: string[];
		bad?: string[];
		neutral?: string[];
	};
	drivers: string[];
	options?: unknown;
	sequence: number;
	created_at: string;
	updated_at: string;
	created_by?: string;
	agent_session_id?: string;
}

export interface AgentSession {
	id: string;
	project_id: string;
	task_id: string;
	status: 'pending' | 'running' | 'waiting_hitl' | 'completed' | 'failed' | 'cancelled';
	agent_type: string;
	branch_name?: string;
	error?: string;
	created_at: string;
	started_at?: string;
	completed_at?: string;
}

export interface Paginated<T> {
	items: T[];
	next_cursor?: string;
}
