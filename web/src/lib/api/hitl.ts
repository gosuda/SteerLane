export interface HITLOption {
	label: string;
	value: string;
}

export interface HITLQuestion {
	id: string;
	agent_session_id: string;
	question: string;
	status: 'pending' | 'answered' | 'timeout' | 'cancelled';
	created_at: string;
	answered_at?: string;
	timeout_at?: string;
	answer?: string;
	answered_by?: string;
	options?: unknown;
}

export function parseHITLOptions(raw: unknown): HITLOption[] {
	if (!Array.isArray(raw)) {
		return [];
	}

	return raw.flatMap((option) => {
		if (typeof option === 'string' && option.trim()) {
			return [{ label: option, value: option }];
		}
		if (
			typeof option === 'object' &&
			option !== null &&
			'type' in option === false &&
			'label' in option &&
			'value' in option &&
			typeof (option as { label: unknown }).label === 'string' &&
			typeof (option as { value: unknown }).value === 'string'
		) {
			const typed = option as { label: string; value: string };
			if (typed.label.trim() && typed.value.trim()) {
				return [{ label: typed.label, value: typed.value }];
			}
		}
		return [];
	});
}
