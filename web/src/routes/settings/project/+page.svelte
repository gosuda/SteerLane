<script lang="ts">
	import { apiFetch } from '$lib/api/request';
	import { projectStore } from '$lib/stores/project.svelte';

	interface ProjectResponse {
		id: string;
		name: string;
		repo_url: string;
		branch: string;
		created_at: string;
		settings?: Record<string, unknown>;
	}

	interface MessengerLinkResponse {
		id: string;
		platform: string;
		external_id: string;
		created_at: string;
		status?: string;
		message?: string;
	}

	let project = $state<ProjectResponse | null>(null);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);
	let activeRequestId = 0;
	let activeSaveRequestId = 0;
	let links = $state<MessengerLinkResponse[]>([]);
	let linksLoading = $state(false);
	let linksError = $state<string | null>(null);
	let unlinkingID = $state<string | null>(null);

	let name = $state('');
	let repoUrl = $state('');
	let branch = $state('');
	let mergeStrategy = $state('squash');
	let defaultAgentType = $state('claude');

	$effect(() => {
		const projectId = projectStore.selectedProjectId;
		if (projectId) {
			project = null;
			activeSaveRequestId += 1;
			saving = false;
			void loadProject(projectId);
			void loadMessengerLinks();
		}
	});

	async function loadMessengerLinks() {
		linksLoading = true;
		linksError = null;
		try {
			const response = await apiFetch('/api/v1/messenger-links');
			if (!response.ok) {
				throw new Error('Failed to load messenger connections.');
			}
			const payload = (await response.json()) as { items: MessengerLinkResponse[] };
			links = payload.items ?? [];
		} catch (err: unknown) {
			links = [];
			linksError = err instanceof Error ? err.message : 'Failed to load messenger connections.';
		} finally {
			linksLoading = false;
		}
	}

	async function loadProject(projectId: string) {
		const requestId = ++activeRequestId;
		loading = true;
		error = null;
		success = null;
		try {
			const response = await apiFetch(`/api/v1/projects/${projectId}`);
			if (!response.ok) {
				throw new Error('Failed to load project settings.');
			}

			const data = (await response.json()) as ProjectResponse;
			if (requestId !== activeRequestId) {
				return;
			}
			project = data;
			name = data.name;
			repoUrl = data.repo_url;
			branch = data.branch;
			mergeStrategy = String(data.settings?.merge_strategy ?? 'squash');
			defaultAgentType = String(data.settings?.default_agent_type ?? 'claude');
		} catch (err: unknown) {
			if (requestId !== activeRequestId) {
				return;
			}
			error = err instanceof Error ? err.message : 'Failed to load project settings.';
		} finally {
			if (requestId !== activeRequestId) {
				return;
			}
			loading = false;
		}
	}

	async function saveProject() {
		if (!project?.id) {
			return;
		}
		const requestId = ++activeSaveRequestId;
		const projectID = project.id;
		const selectedProjectID = projectStore.selectedProjectId;

		saving = true;
		error = null;
		success = null;

		const settings = {
			...(project?.settings ?? {}),
			merge_strategy: mergeStrategy,
			default_agent_type: defaultAgentType
		};

		try {
			const response = await apiFetch(`/api/v1/projects/${projectID}`, {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					name,
					repo_url: repoUrl,
					branch,
					settings
				})
			});
			if (!response.ok) {
				throw new Error('Failed to save project settings.');
			}

			const data = (await response.json()) as ProjectResponse;
			if (requestId !== activeSaveRequestId || projectStore.selectedProjectId !== selectedProjectID) {
				return;
			}
			project = data;
			success = 'Project settings saved.';
			await projectStore.loadProjects();
			if (requestId !== activeSaveRequestId || projectStore.selectedProjectId !== selectedProjectID) {
				return;
			}
			projectStore.selectProject(data.id);
		} catch (err: unknown) {
			if (requestId !== activeSaveRequestId) {
				return;
			}
			error = err instanceof Error ? err.message : 'Failed to save project settings.';
		} finally {
			if (requestId !== activeSaveRequestId) {
				return;
			}
			saving = false;
		}
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		void saveProject();
	}

	async function unlinkMessengerLink(linkID: string) {
		unlinkingID = linkID;
		linksError = null;
		try {
			const response = await apiFetch(`/api/v1/messenger-links/${linkID}`, { method: 'DELETE' });
			if (!response.ok) {
				throw new Error('Failed to remove messenger connection.');
			}
			links = links.filter((link) => link.id !== linkID);
		} catch (err: unknown) {
			linksError = err instanceof Error ? err.message : 'Failed to remove messenger connection.';
		} finally {
			unlinkingID = null;
		}
	}
</script>

<svelte:head>
	<title>Project Settings - SteerLane</title>
</svelte:head>

<div class="mx-auto flex w-full max-w-4xl flex-1 flex-col gap-6 px-6 py-8 text-white">
	<div>
		<p class="text-xs font-mono uppercase tracking-[0.22em] text-brand-secondary">Settings</p>
		<h1 class="mt-2 text-3xl font-bold text-white">Project configuration</h1>
		<p class="mt-2 text-sm leading-6 text-slate-400">
			Tune repository defaults and execution preferences for the currently selected project.
		</p>
	</div>

	{#if loading}
		<div class="rounded-2xl border border-slate-800 bg-brand-surface p-8 text-sm text-slate-400">Loading project settings...</div>
	{:else if !projectStore.selectedProjectId}
		<div class="rounded-2xl border border-dashed border-slate-800 bg-brand-surface p-8 text-sm text-slate-500">Select a project in the header before editing settings.</div>
	{:else}
		<div class="grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
			<form class="rounded-2xl border border-slate-800 bg-brand-surface p-6 shadow-lg" onsubmit={handleSubmit}>
				<div class="grid gap-5">
					<label class="block">
						<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Project name</span>
						<input bind:value={name} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" required />
					</label>

					<label class="block">
						<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Repository URL</span>
						<input bind:value={repoUrl} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" required />
					</label>

					<label class="block">
						<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Default branch</span>
						<input bind:value={branch} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary" required />
					</label>

					<div class="grid gap-5 sm:grid-cols-2">
						<label class="block">
							<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Merge strategy</span>
							<select bind:value={mergeStrategy} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary">
								<option value="squash">Squash</option>
								<option value="merge">Merge commit</option>
								<option value="rebase">Rebase</option>
							</select>
						</label>

						<label class="block">
							<span class="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Default agent</span>
							<select bind:value={defaultAgentType} class="w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-slate-100 outline-none transition-colors focus:border-brand-primary">
								<option value="claude">Claude</option>
								<option value="opencode">OpenCode</option>
								<option value="codex">Codex</option>
								<option value="gemini">Gemini CLI</option>
							</select>
						</label>
					</div>

					{#if error}
						<div class="rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300">{error}</div>
					{/if}

					{#if success}
						<div class="rounded-xl border border-emerald-900/60 bg-emerald-950/40 px-4 py-3 text-sm text-emerald-300">{success}</div>
					{/if}

					<button class="rounded-xl bg-brand-primary px-4 py-3 text-sm font-semibold text-slate-950 transition-colors hover:bg-amber-400 disabled:cursor-not-allowed disabled:opacity-60" disabled={saving} type="submit">
						{saving ? 'Saving...' : 'Save project settings'}
					</button>
				</div>
			</form>

			<aside class="space-y-4">
				<div class="rounded-2xl border border-slate-800 bg-brand-surface p-5">
					<p class="text-xs font-mono uppercase tracking-[0.18em] text-slate-500">Current project</p>
					<h2 class="mt-3 text-xl font-bold text-white">{project?.name ?? 'Selected project'}</h2>
					<p class="mt-2 text-sm leading-6 text-slate-400">
						Repository defaults live on the project object so dispatch, branch isolation, and review flows share one source of truth.
					</p>
				</div>

				<div class="rounded-2xl border border-slate-800 bg-brand-surface p-5">
					<p class="text-xs font-mono uppercase tracking-[0.18em] text-slate-500">Messenger connections</p>
					<p class="mt-3 text-sm leading-6 text-slate-400">
						Message the bot from Slack to start linking, then finish the secure browser hand-off when it opens <span class="font-mono text-slate-200">/auth/link</span>.
					</p>

					{#if linksError}
						<div class="mt-4 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300">{linksError}</div>
					{/if}

					<div class="mt-4 space-y-3">
						{#if linksLoading}
							<div class="rounded-xl border border-slate-800 bg-slate-950 px-4 py-3 text-sm text-slate-400">Loading messenger connections...</div>
						{:else if links.length === 0}
							<div class="rounded-xl border border-dashed border-slate-800 bg-slate-950 px-4 py-3 text-sm text-slate-500">No messenger accounts linked yet.</div>
						{:else}
							{#each links as link}
								<div class="rounded-xl border border-slate-800 bg-slate-950 px-4 py-3">
									<div class="flex items-start justify-between gap-3">
										<div>
											<div class="text-sm font-semibold uppercase tracking-[0.18em] text-slate-300">{link.platform}</div>
											<div class="mt-1 font-mono text-xs text-slate-500">{link.external_id}</div>
											<div class="mt-2 text-xs text-slate-500">Linked {new Date(link.created_at).toLocaleString()}</div>
										</div>
										<button class="rounded-lg border border-slate-700 px-3 py-1.5 text-xs font-semibold text-slate-200 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50" disabled={unlinkingID === link.id} onclick={() => unlinkMessengerLink(link.id)} type="button">
											{unlinkingID === link.id ? 'Removing...' : 'Unlink'}
										</button>
									</div>
								</div>
							{/each}
						{/if}
					</div>
				</div>
			</aside>
		</div>
	{/if}
</div>
