const API_BASE = import.meta.env.VITE_API_URL || '';

type MessageType = 'subscribe' | 'unsubscribe' | 'command' | 'proxy_command' | 'server_status' | 'player_join' | 'player_leave' | 'log' | 'resources' | 'crash';

interface Message {
	type: MessageType;
	channel?: string;
	server_id?: number;
	proxy_id?: number;
	command?: string;
	status?: string;
	player_name?: string;
	player_uuid?: string;
	log_line?: string;
	cpu_percent?: number;
	memory_mb?: number;
	crash_log_id?: number;
}

type MessageHandler = (message: Message) => void;

import { getToken } from '$lib/api';

class WebSocketClient {
	private ws: WebSocket | null = null;
	private reconnectAttempts = 0;
	private maxReconnectAttempts = 5;
	private reconnectDelay = 2000;
	private handlers: Map<string, Set<MessageHandler>> = new Map();
	private connectionPromise: Promise<void> | null = null;

	constructor() {
	}

	connect(): Promise<void> {
		if (this.connectionPromise) {
			return this.connectionPromise;
		}

		this.connectionPromise = new Promise((resolve, reject) => {
			const token = getToken();
			if (!token) {
				reject(new Error('No authentication token'));
				return;
			}

			const wsProtocol = typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'wss:' : 'ws:';
			const wsHost = API_BASE || `${window.location.host}`;
			const wsUrl = `${wsProtocol}//${wsHost}/api/ws?token=${token}`;

			this.ws = new WebSocket(wsUrl);

			this.ws.onopen = () => {
				console.log('WebSocket connected');
				this.reconnectAttempts = 0;
				resolve();
			};

			this.ws.onerror = (error) => {
				console.error('WebSocket error:', error);
				reject(error);
			};

			this.ws.onclose = () => {
				console.log('WebSocket disconnected');
				this.handleReconnect();
			};

			this.ws.onmessage = (event) => {
				try {
					const message: Message = JSON.parse(event.data);
					this.handleMessage(message);
				} catch (error) {
					console.error('Failed to parse WebSocket message:', error);
				}
			};
		});

		return this.connectionPromise;
	}

	private handleReconnect() {
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			console.error('Max reconnect attempts reached');
			return;
		}

		this.reconnectAttempts++;
		const delay = this.reconnectDelay * this.reconnectAttempts;

		setTimeout(() => {
			console.log(`Reconnecting... Attempt ${this.reconnectAttempts}`);
			this.connectionPromise = null;
			this.connect().catch(console.error);
		}, delay);
	}

	private handleMessage(message: Message) {
		const handlers = this.handlers.get(message.type);
		if (handlers) {
			handlers.forEach(handler => handler(message));
		}

		const allHandlers = this.handlers.get('*');
		if (allHandlers) {
			allHandlers.forEach(handler => handler(message));
		}
	}

	on(type: MessageType | '*', handler: MessageHandler) {
		if (!this.handlers.has(type)) {
			this.handlers.set(type, new Set());
		}
		this.handlers.get(type)!.add(handler);
	}

	off(type: MessageType | '*', handler: MessageHandler) {
		const handlers = this.handlers.get(type);
		if (handlers) {
			handlers.delete(handler);
		}
	}

	subscribe(channel: string) {
		this.send({
			type: 'subscribe',
			channel
		});
	}

	unsubscribe(channel: string) {
		this.send({
			type: 'unsubscribe',
			channel
		});
	}

	sendCommand(serverId: number, command: string) {
		console.log(`[WebSocket] Preparing to send command: serverId=${serverId}, command="${command}"`);
		this.send({
			type: 'command',
			server_id: serverId,
			command
		});
	}

	sendProxyCommand(proxyId: number, command: string) {
		console.log(`[WebSocket] Preparing to send proxy command: proxyId=${proxyId}, command="${command}"`);
		this.send({
			type: 'proxy_command',
			proxy_id: proxyId,
			command
		});
	}

	private send(message: Message) {
		console.log(`[WebSocket] Sending message:`, message);
		console.log(`[WebSocket] WebSocket state: ${this.ws ? 'exists' : 'null'}, readyState: ${this.ws?.readyState}`);

		if (this.ws && this.ws.readyState === WebSocket.OPEN) {
			try {
				this.ws.send(JSON.stringify(message));
				console.log(`[WebSocket] Message sent successfully`);
			} catch (err) {
				console.error(`[WebSocket] Error sending message:`, err);
				throw err;
			}
		} else {
			console.error(`[WebSocket] Cannot send message - WebSocket is not connected`);
			console.error(`[WebSocket] ws exists: ${!!this.ws}, readyState: ${this.ws?.readyState}, OPEN: ${WebSocket.OPEN}`);
			throw new Error('WebSocket is not connected');
		}
	}

	disconnect() {
		if (this.ws) {
			this.ws.close();
			this.ws = null;
		}
		this.connectionPromise = null;
	}

	isConnected(): boolean {
		return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
	}
}

export const ws = new WebSocketClient();
