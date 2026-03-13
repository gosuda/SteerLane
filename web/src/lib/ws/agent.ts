export interface AgentEvent {
	type: string;
	payload: any;
	timestamp: string;
}

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error';

export function createAgentStream(
	sessionId: string,
	onEvent: (event: AgentEvent) => void,
	onStatus: (status: ConnectionStatus) => void
) {
	let ws: WebSocket | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let isIntentionallyClosed = false;

	const connect = () => {
		if (isIntentionallyClosed) return;

		onStatus('connecting');

		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws/agent/${sessionId}`;

		ws = new WebSocket(wsUrl);

		ws.onopen = () => {
			onStatus('connected');
		};

		ws.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				onEvent(data);
			} catch (e) {
				console.error('Failed to parse agent WS message:', e);
			}
		};

		ws.onclose = () => {
			onStatus('disconnected');
			if (!isIntentionallyClosed) {
				// Attempt to reconnect after delay
				reconnectTimer = setTimeout(connect, 3000);
			}
		};

		ws.onerror = (err) => {
			console.error('Agent WS error:', err);
			onStatus('error');
		};
	};

	connect();

	return {
		close: () => {
			isIntentionallyClosed = true;
			if (reconnectTimer) clearTimeout(reconnectTimer);
			if (ws) {
				ws.close();
				ws = null;
			}
		}
	};
}
