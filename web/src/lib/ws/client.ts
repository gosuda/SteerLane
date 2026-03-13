type WSEventHandler = (event: any) => void;

export class BoardWSClient {
	private ws: WebSocket | null = null;
	private projectId: string;
	private handlers: Set<WSEventHandler> = new Set();
	private reconnectAttempts = 0;
	private maxReconnectAttempts = 5;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private intentionalClose = false;

	public isConnected = false;
	public onConnectionChange?: (connected: boolean) => void;

	constructor(projectId: string) {
		this.projectId = projectId;
	}

	connect() {
		this.intentionalClose = false;

		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws/board/${this.projectId}`;

		this.ws = new WebSocket(wsUrl);

		this.ws.onopen = () => {
			this.isConnected = true;
			this.reconnectAttempts = 0;
			this?.onConnectionChange?.(true);
			console.log(`[WS] Connected to board ${this.projectId}`);
		};

		this.ws.onmessage = (msg) => {
			try {
				const event = JSON.parse(msg.data);
				this.handlers.forEach((h) => h(event));
			} catch (e) {
				console.error('[WS] Failed to parse message', e);
			}
		};

		this.ws.onclose = () => {
			this.isConnected = false;
			this?.onConnectionChange?.(false);
			this.ws = null;

			if (!this.intentionalClose) {
				this.scheduleReconnect();
			}
		};

		this.ws.onerror = (error) => {
			console.error(`[WS] Error in board ${this.projectId}`, error);
		};
	}

	private scheduleReconnect() {
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			console.error('[WS] Max reconnect attempts reached');
			return;
		}

		const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 10000);
		this.reconnectAttempts++;

		console.log(`[WS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
		this.reconnectTimer = setTimeout(() => {
			this.connect();
		}, delay);
	}

	subscribe(handler: WSEventHandler) {
		this.handlers.add(handler);
		return () => {
			this.handlers.delete(handler);
		};
	}

	disconnect() {
		this.intentionalClose = true;
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		if (this.ws) {
			this.ws.close();
		}
	}
}
