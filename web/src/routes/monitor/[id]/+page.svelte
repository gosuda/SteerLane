<script lang="ts">
	import { page } from '$app/state';
	import { apiFetch } from '$lib/api/request';
	import type { AgentSession } from '$lib/api/types';
	import { parseHITLOptions, type HITLQuestion } from '$lib/api/hitl';
	import { createAgentStream, type AgentEvent, type ConnectionStatus } from '$lib/ws/agent';

	let sessionId = $derived(page.params.id);
	let session = $state<AgentSession | null>(null);
	let questions = $state<HITLQuestion[]>([]);
	let answerDrafts = $state<Record<string, string>>({});
	let loading = $state(true);
	let error = $state<string | null>(null);
	let questionError = $state<string | null>(null);
	let submittingQuestionId = $state<string | null>(null);
	let sessionRequestId = 0;
	let questionRequestId = 0;
	let wsGeneration = 0;

	let wsStatus = $state<ConnectionStatus>('disconnected');
	let events = $state<AgentEvent[]>([]);

	let inputTokens = $state(0);
	let outputTokens = $state(0);
	let totalTokens = $derived(inputTokens + outputTokens);

	let now = $state(Date.now());

	let wsConn: { close: () => void } | null = null;

	$effect(() => {
		const interval = setInterval(() => {
			now = Date.now();
		}, 1000);
		return () => clearInterval(interval);
	});

	let elapsedSeconds = $derived.by(() => {
		if (!session?.started_at) return 0;
		const start = new Date(session.started_at).getTime();
		const end = session.completed_at ? new Date(session.completed_at).getTime() : now;
		return Math.max(0, Math.floor((end - start) / 1000));
	});

	function formatTime(sec: number) {
		const h = Math.floor(sec / 3600);
		const m = Math.floor((sec % 3600) / 60);
		const s = sec % 60;
		return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
	}

	$effect(() => {
		if (sessionId) {
			wsGeneration += 1;
			session = null;
			questions = [];
			events = [];
			answerDrafts = {};
			inputTokens = 0;
			outputTokens = 0;
			fetchSession(sessionId);
			fetchQuestions(sessionId);
			connectWs(sessionId, wsGeneration);
		}

		return () => {
			if (wsConn) {
				wsConn.close();
				wsConn = null;
			}
		};
	});

	async function fetchSession(id: string) {
		const requestId = ++sessionRequestId;
		loading = true;
		error = null;
		try {
			const res = await apiFetch(`/api/v1/agent-sessions/${id}`);
			if (!res.ok) {
				throw new Error('Failed to fetch agent session');
			}
			if (requestId !== sessionRequestId) {
				return;
			}
			session = (await res.json()) as AgentSession;
		} catch (err: any) {
			if (requestId !== sessionRequestId) {
				return;
			}
			error = err.message;
		} finally {
			if (requestId !== sessionRequestId) {
				return;
			}
			loading = false;
		}
	}

	async function fetchQuestions(id: string) {
		const requestId = ++questionRequestId;
		questionError = null;
		try {
			const res = await apiFetch(`/api/v1/hitl?session_id=${id}`);
			if (!res.ok) {
				throw new Error('Failed to fetch HITL questions');
			}
			const data = (await res.json()) as { items?: HITLQuestion[] };
			if (requestId !== questionRequestId) {
				return;
			}
			questions = data.items ?? [];
			answerDrafts = questions.reduce<Record<string, string>>((drafts, question) => {
				drafts[question.id] = question.answer ?? drafts[question.id] ?? '';
				return drafts;
			}, { ...answerDrafts });
		} catch (err: any) {
			if (requestId !== questionRequestId) {
				return;
			}
			questionError = err.message;
		}
	}

	async function submitAnswer(questionId: string, answer: string) {
		const trimmed = answer.trim();
		if (!trimmed) {
			questionError = 'Answer text is required.';
			return;
		}

		submittingQuestionId = questionId;
		questionError = null;
		try {
			const res = await apiFetch(`/api/v1/hitl/${questionId}/answer`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ answer: trimmed })
			});
			if (!res.ok) {
				throw new Error('Failed to submit HITL answer');
			}
			if (!sessionId) {
				return;
			}
			await Promise.all([fetchQuestions(sessionId), fetchSession(sessionId)]);
		} catch (err: any) {
			questionError = err.message;
		} finally {
			submittingQuestionId = null;
		}
	}

	function connectWs(id: string, generation: number) {
		if (wsConn) {
			wsConn.close();
		}

		wsConn = createAgentStream(
			id,
			(event) => {
				if (generation !== wsGeneration) {
					return;
				}
				events = [...events, event];
				if (event.type === 'session.ended' || event.payload?.question_id || event.payload?.status === 'waiting_hitl' || event.payload?.event === 'hitl_answered') {
					fetchQuestions(id);
				}
				if ((event.type === 'agent.status' || event.type === 'session.started' || event.type === 'session.ended') && session && event.payload?.status) {
					session = { ...session, status: event.payload.status };
				}
				if (event.type === 'session.started' && event.timestamp && session && !session.started_at) {
					session = { ...session, started_at: event.timestamp };
				}
				if (event.type === 'session.ended' && event.timestamp && session && !session.completed_at) {
					session = { ...session, completed_at: event.timestamp };
				}
				if (event.type === 'token.usage' && event.payload) {
					inputTokens += event.payload.input_tokens || 0;
					outputTokens += event.payload.output_tokens || 0;
				}
			},
			(status) => {
				if (generation !== wsGeneration) {
					return;
				}
				wsStatus = status;
			}
		);
	}
</script>

<svelte:head>
	<title>Agent Monitor - SteerLane</title>
</svelte:head>

<div class="max-w-6xl mx-auto px-4 py-8 text-white">
	<div class="mb-6">
		<a
			href="/"
			class="text-sm font-medium text-slate-400 hover:text-white flex items-center gap-1 transition-colors"
		>
			&larr; Back to board
		</a>
	</div>

	{#if loading && !session}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-primary"></div>
		</div>
	{:else if error && !session}
		<div class="bg-red-950 border border-red-900 text-red-400 px-4 py-3 rounded-md">
			{error}
		</div>
	{:else if session}
		<div class="bg-brand-surface rounded-xl shadow-sm border border-slate-800 p-5 mb-6 flex flex-wrap gap-8 items-center">
			<div class="flex flex-col">
				<span class="text-xs text-slate-500 uppercase font-semibold tracking-wider mb-1">Elapsed Time</span>
				<span class="text-xl font-mono font-bold text-white">{formatTime(elapsedSeconds)}</span>
			</div>

			<div class="h-10 w-px bg-slate-800 hidden sm:block"></div>

			<div class="flex flex-col">
				<span class="text-xs text-slate-500 uppercase font-semibold tracking-wider mb-1">Total Tokens</span>
				<span class="text-xl font-mono font-bold text-brand-secondary">{totalTokens.toLocaleString()}</span>
			</div>

			<div class="flex gap-6">
				<div class="flex flex-col">
					<span class="text-[10px] text-slate-500 uppercase font-semibold tracking-wider mb-1">Input</span>
					<span class="text-sm font-mono text-slate-300">{inputTokens.toLocaleString()}</span>
				</div>
				<div class="flex flex-col">
					<span class="text-[10px] text-slate-500 uppercase font-semibold tracking-wider mb-1">Output</span>
					<span class="text-sm font-mono text-slate-300">{outputTokens.toLocaleString()}</span>
				</div>
			</div>
		</div>

		<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
			<!-- Left sidebar: Session Details -->
			<div class="lg:col-span-1 space-y-6">
				<div class="bg-brand-surface rounded-xl shadow-sm border border-slate-800 overflow-hidden">
					<div class="p-5 border-b border-slate-800 bg-slate-900">
						<div class="flex items-center justify-between mb-2">
							<h2 class="text-lg font-bold text-white font-sans">Session Details</h2>
							<span
								class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium capitalize
                {session.status === 'completed'
									? 'bg-green-900/50 text-green-400 border border-green-800/50'
									: session.status === 'failed'
										? 'bg-red-900/50 text-red-400 border border-red-800/50'
										: session.status === 'running'
											? 'bg-blue-900/50 text-blue-400 border border-blue-800/50'
											: 'bg-slate-800 text-slate-300 border border-slate-700'}"
							>
								{session.status}
							</span>
						</div>
						<div class="text-xs font-mono text-slate-400 break-all">{session.id}</div>
					</div>

					<div class="p-5 space-y-4">
						<div>
							<div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-1">
								Agent Type
							</div>
							<div class="text-slate-200 font-medium">{session.agent_type || 'Unknown'}</div>
						</div>

						{#if session.branch_name}
							<div>
								<div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-1">
									Branch
								</div>
								<div
									class="text-slate-200 font-mono text-sm bg-slate-900 px-2 py-1 rounded inline-block border border-slate-800"
								>
									{session.branch_name}
								</div>
							</div>
						{/if}

						<div>
							<div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-1">
								Created At
							</div>
							<div class="text-slate-300 text-sm">
								{new Date(session.created_at).toLocaleString()}
							</div>
						</div>

						{#if session.started_at}
							<div>
								<div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-1">
									Started At
								</div>
								<div class="text-slate-300 text-sm">
									{new Date(session.started_at).toLocaleString()}
								</div>
							</div>
						{/if}

						{#if session.completed_at}
							<div>
								<div class="text-xs font-semibold text-slate-500 uppercase tracking-wider mb-1">
									Completed At
								</div>
								<div class="text-slate-300 text-sm">
									{new Date(session.completed_at).toLocaleString()}
								</div>
							</div>
						{/if}

						{#if session.error}
							<div>
								<div class="text-xs font-semibold text-red-500 uppercase tracking-wider mb-1">
									Error
								</div>
								<div
									class="text-red-400 text-sm bg-red-950/50 p-2 rounded border border-red-900/50"
								>
									{session.error}
								</div>
							</div>
						{/if}
					</div>
				</div>

				<div class="bg-brand-surface rounded-xl shadow-sm border border-slate-800 p-5">
					<div class="flex items-center justify-between mb-4">
						<h2 class="text-sm font-bold text-white uppercase tracking-wider">Connection Status</h2>
						<div class="flex items-center gap-2">
							<span class="relative flex h-3 w-3">
								{#if wsStatus === 'connected'}
									<span
										class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
									></span>
									<span class="relative inline-flex rounded-full h-3 w-3 bg-green-500"></span>
								{:else if wsStatus === 'connecting'}
									<span
										class="animate-ping absolute inline-flex h-full w-full rounded-full bg-yellow-400 opacity-75"
									></span>
									<span class="relative inline-flex rounded-full h-3 w-3 bg-yellow-500"></span>
								{:else}
									<span class="relative inline-flex rounded-full h-3 w-3 bg-red-500"></span>
								{/if}
							</span>
							<span class="text-xs font-medium text-slate-300 capitalize">{wsStatus}</span>
						</div>
					</div>
					<p class="text-xs text-slate-400">
						Live stream of agent activity. Events will appear automatically when connected.
					</p>
				</div>

				<div class="bg-brand-surface rounded-xl shadow-sm border border-slate-800 p-5 space-y-4">
					<div class="flex items-center justify-between gap-4">
						<div>
							<h2 class="text-sm font-bold text-white uppercase tracking-wider">HITL Questions</h2>
							<p class="text-xs text-slate-400 mt-1">
								Pending human questions for this session.
							</p>
						</div>
						<button
							type="button"
							class="rounded border border-slate-700 px-2.5 py-1 text-xs text-slate-300 transition-colors hover:border-slate-500 hover:text-white"
							onclick={() => sessionId && fetchQuestions(sessionId)}
						>
							Refresh
						</button>
					</div>

					{#if questionError}
						<div class="rounded-lg border border-red-900/60 bg-red-950/40 px-3 py-2 text-xs text-red-300">
							{questionError}
						</div>
					{/if}

					{#if questions.length === 0}
						<div class="rounded-lg border border-dashed border-slate-800 bg-slate-900/60 px-3 py-4 text-sm text-slate-500">
							No HITL questions yet.
						</div>
					{:else}
						<div class="space-y-4">
							{#each questions as question}
								<div class="rounded-xl border border-slate-800 bg-slate-950/70 p-4 space-y-3">
									<div class="flex items-start justify-between gap-3">
										<div>
											<div class="text-sm font-semibold text-white">{question.question}</div>
											<div class="mt-1 text-xs text-slate-500">
												Asked {new Date(question.created_at).toLocaleString()}
												{#if question.timeout_at}
													• timeout {new Date(question.timeout_at).toLocaleString()}
												{/if}
											</div>
										</div>
										<span class="rounded-full border border-slate-700 px-2.5 py-0.5 text-xs capitalize text-slate-300">
											{question.status}
										</span>
									</div>

									{#if parseHITLOptions(question.options).length > 0 && question.status === 'pending'}
										<div class="flex flex-wrap gap-2">
											{#each parseHITLOptions(question.options) as option}
												<button
													type="button"
													class="rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 transition-colors hover:border-brand-primary hover:text-white"
													onclick={() => {
														answerDrafts[question.id] = option.value;
														submitAnswer(question.id, option.value);
													}}
													disabled={submittingQuestionId === question.id}
												>
													{option.label}
												</button>
											{/each}
										</div>
									{/if}

									{#if question.status === 'pending'}
										<div class="space-y-2">
											<textarea
												class="min-h-24 w-full rounded-xl border border-slate-800 bg-slate-900 px-3 py-2 text-sm text-slate-200 outline-none transition-colors focus:border-brand-primary"
												placeholder="Reply to the agent"
												bind:value={answerDrafts[question.id]}
											></textarea>
											<div class="flex justify-end">
												<button
													type="button"
													class="rounded-lg bg-brand-primary px-3 py-2 text-xs font-semibold text-slate-950 transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
													onclick={() => submitAnswer(question.id, answerDrafts[question.id] ?? '')}
													disabled={submittingQuestionId === question.id}
												>
													{submittingQuestionId === question.id ? 'Submitting...' : 'Send answer'}
												</button>
											</div>
										</div>
									{:else if question.answer}
										<div class="rounded-lg border border-emerald-900/40 bg-emerald-950/30 px-3 py-2 text-sm text-emerald-200">
											{question.answer}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>

			<!-- Right column: Event Stream -->
			<div class="lg:col-span-2 flex flex-col h-[700px]">
				<div
					class="bg-brand-surface rounded-t-xl border-x border-t border-slate-800 p-4 flex items-center justify-between"
				>
					<h2 class="text-white font-mono font-bold flex items-center gap-2">
						<svg
							class="w-5 h-5 text-brand-secondary"
							fill="none"
							stroke="currentColor"
							viewBox="0 0 24 24"
							xmlns="http://www.w3.org/2000/svg"
							><path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M8 9l3 3-3 3m5 0h3M4 18h16a2 2 0 002-2V6a2 2 0 00-2-2H4a2 2 0 00-2 2v10a2 2 0 002 2z"
							></path></svg
						>
						Terminal Output
					</h2>
					<span class="text-slate-400 text-xs font-mono">{events.length} events</span>
				</div>

				<div
					class="flex-1 bg-slate-950 border border-slate-800 rounded-b-xl p-4 overflow-y-auto font-mono text-sm shadow-inner relative"
				>
					{#if events.length === 0}
						<div class="absolute inset-0 flex items-center justify-center text-slate-500">
							Waiting for agent activity...
						</div>
					{:else}
						<div class="space-y-3">
							{#each events as event}
								<div
									class="border-l-2 border-slate-700 pl-3 py-1 group hover:bg-slate-900 transition-colors rounded-r"
								>
									<div
										class="flex items-start justify-between mb-1 opacity-70 group-hover:opacity-100 transition-opacity"
									>
										<span class="text-brand-secondary font-bold text-xs">{event.type}</span>
										<span class="text-slate-500 text-xs">
											{event.timestamp
												? new Date(event.timestamp).toLocaleTimeString()
												: new Date().toLocaleTimeString()}
										</span>
									</div>
									<div class="text-slate-300 whitespace-pre-wrap break-words">
										{#if typeof event.payload === 'string'}
											{event.payload}
										{:else if event.payload}
											<pre
												class="text-xs bg-slate-900 p-2 rounded mt-1 overflow-x-auto border border-slate-800">{JSON.stringify(
													event.payload,
													null,
													2
												)}</pre>
										{:else}
											<span class="italic text-slate-500">No payload</span>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>
	{/if}
</div>
