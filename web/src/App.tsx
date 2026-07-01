import { useEffect, useState } from 'react'
import type { ButtonHTMLAttributes, Dispatch, FormEvent, ReactNode, SetStateAction } from 'react'
import {
  Activity,
  Archive,
  Bell,
  Bot,
  Boxes,
  CalendarClock,
  Check,
  ChevronRight,
  CircleStop,
  ClipboardList,
  Cloud,
  Database,
  GitPullRequest,
  History,
  LayoutList,
  Loader2,
  LogOut,
  Play,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
} from 'lucide-react'

type Json = Record<string, any>

type Session = {
  authRequired?: boolean
  authenticated?: boolean
  user?: { login?: string; avatarUrl?: string }
}

type AgentInfo = {
  Name: string
  Description?: string
  Version?: string
  Author?: string
  RequiredTools?: string[]
  Domains?: string[]
  TriggerKeywords?: string[]
  TriggerFiles?: string[]
  ArchitectureGuidance?: string[]
  OutputExpectations?: string[]
}

type RepositorySummary = {
  id?: number
  name?: string
  full_name?: string
  private?: boolean
  html_url?: string
  default_branch?: string
  updated_at?: string
}

type Orchestration = {
  id: string
  repo?: string
  repoPath?: string
  baseBranch?: string
  task?: string
  status?: string
  strategy?: string
  llmPreset?: string
  outputLanguage?: string
  agents?: string[]
  customAgents?: Json[]
  scenarioTemplate?: Json
  github?: Json
  limits?: Json
  usage?: Json
  plan?: { subtasks?: Json[] }
  subtasks?: Json[]
  results?: Json[]
  events?: Json[]
  summary?: string
  error?: string
  memoryUsed?: Json[]
  memoryProposals?: Json[]
  guidelinesUsed?: Json[]
  missedRequiredGuidelines?: Json[]
  createdAt?: string
  updatedAt?: string
}

type Schedule = {
  id: string
  name?: string
  status?: string
  repo?: string
  baseBranch?: string
  task?: string
  agents?: string[]
  strategy?: string
  llmPreset?: string
  outputLanguage?: string
  schedule?: Json
  concurrencyPolicy?: string
  notification?: Json
  limits?: Json
  nextRunAt?: string
  lastRunAt?: string
  lastRunId?: string
  lastRunStatus?: string
  executions?: Json[]
}

type NotificationRecord = {
  id: string
  scheduleId?: string
  runId?: string
  trigger?: string
  title?: string
  message?: string
  status?: string
  repo?: string
  runUrl?: string
  destinations?: string[]
  deliveries?: Json[]
  createdAt?: string
}

type ScheduleTemplate = {
  id: string
  name: string
  description?: string
  category?: string
  task?: string
  agents?: string[]
  strategy?: string
  schedule?: Json
  concurrencyPolicy?: string
  outputLanguage?: string
  github?: Json
  expectedOutputs?: string[]
  requiredPermissions?: string[]
}

type PageName = 'orchestrates' | 'schedules' | 'agents' | 'audit'
type OrchPanel = 'new' | 'list' | 'detail'
type DetailTab = 'overview' | 'runs' | 'memory' | 'guidelines' | 'search' | 'github'

const detailTabs: DetailTab[] = ['overview', 'runs', 'memory', 'guidelines', 'search', 'github']

const defaultForm = {
  repo: '',
  baseBranch: 'main',
  task: '',
  strategy: 'sequential',
  llmPreset: '',
  outputLanguage: '',
  scenarioTemplate: '',
  createIssue: false,
  createPullRequest: false,
  branchName: '',
  prBase: '',
  issueTitle: '',
  prTitle: '',
  issueTemplate: '',
  prTemplate: '',
  maxDuration: '30m',
  maxSubtasks: '12',
  maxRetries: '',
  maxLlmTokens: '',
  maxGitHubRequests: '',
  maxConcurrentRepoRuns: '1',
  maxConcurrentOrgRuns: '',
}

const defaultScheduleForm = {
  templateId: '',
  name: '',
  repo: '',
  baseBranch: 'main',
  task: '',
  agents: 'analyst, reporter',
  strategy: 'sequential',
  llmPreset: '',
  outputLanguage: '',
  scheduleType: 'interval',
  interval: '24h',
  cron: '0 9 * * 1',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  concurrencyPolicy: 'forbid',
  createIssue: false,
  createPullRequest: false,
  issueTemplate: '',
  prTemplate: '',
  notifyEnabled: true,
  notifyTriggers: 'failed, quality_gate_failed, manual_intervention',
  notifyDestinations: 'inbox',
  webhookUrl: '',
  maxDuration: '30m',
  maxSubtasks: '12',
  maxRetries: '',
  maxLlmTokens: '',
  maxGitHubRequests: '',
  maxConcurrentRepoRuns: '1',
  maxConcurrentOrgRuns: '',
}

async function api<T = any>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) throw new Error(await res.text())
  if (res.status === 204) return null as T
  return res.json()
}

function cx(...values: Array<string | false | null | undefined>) {
  return values.filter(Boolean).join(' ')
}

function shortText(value: unknown, size = 120) {
  const text = String(value ?? '')
  return text.length > size ? `${text.slice(0, size - 1)}...` : text
}

function readableTask(value: unknown) {
  return String(value ?? '')
    .replace(/\s+(Source issue:)/g, '\n$1')
    .replace(/\s+(Source PR:)/g, '\n$1')
    .replace(/\s+(Issue body:)/g, '\n$1')
    .replace(/\s+(PR body:)/g, '\n$1')
    .replace(/\s+(Labels?:)/g, '\n$1')
    .trim()
}

function numberOrUndefined(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

function formatTime(value?: string) {
  if (!value || value === '0001-01-01T00:00:00Z') return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

function ago(value?: string) {
  if (!value) return '-'
  const ms = Date.now() - new Date(value).getTime()
  if (Number.isNaN(ms)) return '-'
  if (ms < 60_000) return `${Math.max(0, Math.floor(ms / 1000))}s`
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`
  return `${Math.floor(ms / 86_400_000)}d`
}

function repoForGitHub(repo = '') {
  if (/^[^/]+\/[^/]+$/.test(repo) && !repo.includes(':')) return repo
  return repo.match(/github\.com[:/]([^/]+\/[^/.]+)(?:\.git)?/)?.[1] ?? ''
}

function Status({ value }: { value?: string | boolean }) {
  const text = String(value ?? 'pending')
  const tone = text.toLowerCase()
  const color =
    tone.includes('fail') || tone.includes('denied') || tone.includes('reject')
      ? 'border-red-os/50 bg-red-os/10 text-red-os'
      : tone.includes('complete') || tone.includes('success') || tone.includes('pass') || tone === 'open'
        ? 'border-green-os/50 bg-green-os/10 text-green-os'
        : tone.includes('run') || tone.includes('plan') || tone.includes('pending') || tone.includes('queue')
          ? 'border-amber-os/50 bg-amber-os/10 text-amber-os'
          : 'border-line bg-panel-2 text-soft'
  return <span className={cx('inline-flex rounded-full border px-2 py-0.5 text-[11px] font-medium', color)}>{text}</span>
}

function Tag({ children }: { children: ReactNode }) {
  return <span className="inline-flex max-w-full rounded border border-line bg-panel-2 px-2 py-0.5 text-[11px] text-soft">{children}</span>
}

function IconButton({
  children,
  icon,
  tone = 'primary',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { icon?: ReactNode; tone?: 'primary' | 'secondary' | 'danger' }) {
  const cls =
    tone === 'danger'
      ? 'border-red-os/60 bg-red-os/15 text-red-os hover:bg-red-os/20'
      : tone === 'secondary'
        ? 'border-line bg-panel-2 text-ink hover:bg-line/40'
        : 'border-cyan-os/60 bg-cyan-os/15 text-cyan-os hover:bg-cyan-os/20'
  return (
    <button
      {...props}
      className={cx(
        'inline-flex min-h-9 items-center justify-center gap-2 rounded-os border px-3 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50',
        cls,
        props.className,
      )}
    >
      {icon}
      {children}
    </button>
  )
}

function Field({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className="grid gap-1.5 text-xs text-soft">
      <span>{label}</span>
      {children}
    </label>
  )
}

const inputClass =
  'min-h-10 w-full rounded-os border border-line bg-void px-3 text-sm text-ink outline-none placeholder:text-soft/60 focus:border-cyan-os/70'
const textareaClass = `${inputClass} min-h-28 resize-y py-2`

function Panel({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={cx('min-w-0 overflow-hidden rounded-os border border-line bg-panel/95 p-4 shadow-[0_16px_60px_rgb(0_0_0/0.24)]', className)}>{children}</section>
}

function App() {
  const [session, setSession] = useState<Session>({ authRequired: false, authenticated: true })
  const [page, setPage] = useState<PageName>('orchestrates')
  const [orchPanel, setOrchPanel] = useState<OrchPanel>('new')
  const [detailTab, setDetailTab] = useState<DetailTab>('overview')
  const [agents, setAgents] = useState<AgentInfo[]>([])
  const [customAgents, setCustomAgents] = useState<Json[]>([])
  const [repositories, setRepositories] = useState<RepositorySummary[]>([])
  const [templates, setTemplates] = useState<Json[]>([])
  const [llm, setLLM] = useState<Json>({ defaultPreset: '', presets: [] })
  const [records, setRecords] = useState<Orchestration[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [scheduleTemplates, setScheduleTemplates] = useState<ScheduleTemplate[]>([])
  const [notifications, setNotifications] = useState<NotificationRecord[]>([])
  const [selectedID, setSelectedID] = useState('')
  const [current, setCurrent] = useState<Orchestration | null>(null)
  const [audit, setAudit] = useState<Json[]>([])
  const [form, setForm] = useState(defaultForm)
  const [scheduleForm, setScheduleForm] = useState(defaultScheduleForm)
  const [selectedAgents, setSelectedAgents] = useState<Set<string>>(new Set())
  const [status, setStatus] = useState('')
  const [scheduleStatus, setScheduleStatus] = useState('')

  const canUseApp = !session.authRequired || session.authenticated

  useEffect(() => {
    api<Session>('/api/auth/session')
      .then(setSession)
      .catch(() => setSession({ authRequired: false, authenticated: true }))
  }, [])

  useEffect(() => {
    if (!canUseApp) return
    void Promise.all([loadAgents(), loadRepositories(), loadLLM(), loadRecords(), loadSchedules(), loadScheduleTemplates(), loadNotifications()])
  }, [canUseApp])

  useEffect(() => {
    if (!selectedID) return
    void refreshCurrent(selectedID)
    const timer = window.setInterval(() => {
      void refreshCurrent(selectedID)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [selectedID])

  async function loadAgents() {
    const data = await api<AgentInfo[]>('/api/agents')
    setAgents(data)
  }

  async function loadRepositories() {
    try {
      const data = await api<RepositorySummary[]>('/api/github/repositories')
      setRepositories(data)
    } catch {
      setRepositories([])
    }
  }

  async function loadLLM() {
    try {
      const data = await api<Json>('/api/settings/llm')
      setLLM(data)
      setForm((f) => ({ ...f, llmPreset: f.llmPreset || data.defaultPreset || '' }))
    } catch {
      setLLM({ defaultPreset: '', presets: [] })
    }
  }

  async function loadTemplates() {
    try {
      const data = await api<Json[]>('/api/orchestrate/templates', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ repo: form.repo, baseBranch: form.baseBranch || 'main' }),
      })
      setTemplates(data)
    } catch {
      setTemplates([])
    }
  }

  async function loadRecords() {
    const data = await api<Orchestration[]>('/api/orchestrates')
    setRecords(data)
  }

  async function loadSchedules() {
    const data = await api<Schedule[]>('/api/schedules')
    setSchedules(data)
  }

  async function loadScheduleTemplates() {
    const data = await api<ScheduleTemplate[]>('/api/schedules/templates')
    setScheduleTemplates(data)
  }

  async function loadNotifications() {
    const data = await api<NotificationRecord[]>('/api/notifications')
    setNotifications(data)
  }

  async function refreshCurrent(id = selectedID) {
    if (!id) return
    const data = await api<Orchestration>(`/api/orchestrates/${encodeURIComponent(id)}`)
    setCurrent(data)
    await loadRecords()
  }

  async function loadAudit() {
    const data = await api<Json[]>('/api/audit')
    setAudit(data)
  }

  function navTo(next: PageName) {
    setPage(next)
    if (next === 'agents') void loadAgents()
    if (next === 'audit') void loadAudit()
    if (next === 'orchestrates') void loadRecords()
    if (next === 'schedules') void Promise.all([loadSchedules(), loadScheduleTemplates(), loadNotifications()])
  }

  if (session.authRequired && !session.authenticated) {
    return (
      <main className="grid min-h-dvh place-items-center px-5">
        <Panel className="w-full max-w-md">
          <div className="mb-5 flex items-center gap-3">
            <div className="grid size-10 place-items-center rounded-os border border-cyan-os/40 bg-cyan-os/10">
              <ShieldCheck className="size-5 text-cyan-os" />
            </div>
            <div>
              <h1 className="text-lg font-semibold">AgentOS</h1>
              <p className="text-sm text-soft">GitHub sign in required.</p>
            </div>
          </div>
          <a className="inline-flex min-h-10 w-full items-center justify-center rounded-os border border-cyan-os/60 bg-cyan-os/15 text-sm font-semibold text-cyan-os" href="/auth/login">
            Sign in with GitHub
          </a>
        </Panel>
      </main>
    )
  }

  const agentChoices = [
    ...agents.map((a) => ({ name: a.Name, label: a.Description ?? '', custom: false })),
    ...customAgents.map((a) => ({ name: a.metadata?.name, label: a.metadata?.labels?.role ?? 'repository custom agent', custom: true })).filter((a) => a.name),
  ]

  return (
    <div className="min-h-dvh pb-20 md:pb-0">
      <header className="sticky top-0 z-20 border-b border-line bg-void/85 px-4 py-3 backdrop-blur md:px-6">
        <div className="mx-auto flex max-w-7xl items-center gap-4">
          <button className="flex min-w-0 items-center gap-3" onClick={() => navTo('orchestrates')} type="button">
            <div className="grid size-9 shrink-0 place-items-center rounded-os border border-cyan-os/40 bg-cyan-os/10">
              <Boxes className="size-5 text-cyan-os" />
            </div>
            <div className="hidden min-w-0 text-left sm:block">
              <div className="text-sm font-semibold tracking-wide text-ink">AgentOS</div>
              <div className="text-[11px] text-soft">v1.3 workspace</div>
            </div>
          </button>
          <nav className="hidden gap-1 md:flex">
            <NavButton active={page === 'orchestrates'} icon={<ClipboardList className="size-4" />} onClick={() => navTo('orchestrates')}>Orchestrate</NavButton>
            <NavButton active={page === 'schedules'} icon={<CalendarClock className="size-4" />} onClick={() => navTo('schedules')}>Schedules</NavButton>
            <NavButton active={page === 'agents'} icon={<Bot className="size-4" />} onClick={() => navTo('agents')}>Agents</NavButton>
            <NavButton active={page === 'audit'} icon={<History className="size-4" />} onClick={() => navTo('audit')}>Audit</NavButton>
          </nav>
          <div className="ml-auto flex min-w-0 items-center gap-3 text-sm text-soft">
            {session.user?.avatarUrl ? <img className="size-7 rounded-full" src={session.user.avatarUrl} alt="" /> : null}
            {session.user?.login ? <span className="hidden max-w-40 truncate sm:inline">{session.user.login}</span> : null}
            {session.authenticated && session.user ? (
              <a className="inline-flex items-center gap-1 text-soft hover:text-ink" href="/auth/logout">
                <LogOut className="size-4" />
                <span className="hidden sm:inline">Sign out</span>
              </a>
            ) : null}
          </div>
        </div>
      </header>

      <main className="mx-auto grid max-w-7xl min-w-0 gap-4 overflow-hidden px-4 py-4 md:px-6">
        {page === 'orchestrates' ? (
          <OrchestratesPage
            panel={orchPanel}
            setPanel={setOrchPanel}
            detailTab={detailTab}
            setDetailTab={setDetailTab}
            records={records}
            current={current}
            selectedID={selectedID}
            setSelectedID={(id) => {
              setSelectedID(id)
              setOrchPanel('detail')
            }}
            refreshCurrent={() => void refreshCurrent()}
            loadRecords={() => void loadRecords()}
            form={form}
            setForm={setForm}
            agents={agentChoices}
            selectedAgents={selectedAgents}
            setSelectedAgents={setSelectedAgents}
            customAgents={customAgents}
            setCustomAgents={setCustomAgents}
            repositories={repositories}
            loadRepositories={() => void loadRepositories()}
            templates={templates}
            loadTemplates={() => void loadTemplates()}
            llm={llm}
            status={status}
            setStatus={setStatus}
            submit={submitOrchestration}
          />
        ) : null}
        {page === 'schedules' ? (
          <SchedulesPage
            schedules={schedules}
            notifications={notifications}
            reload={() => void Promise.all([loadSchedules(), loadNotifications()])}
            form={scheduleForm}
            setForm={setScheduleForm}
            templates={scheduleTemplates}
            repositories={repositories}
            llm={llm}
            status={scheduleStatus}
            setStatus={setScheduleStatus}
            submit={submitSchedule}
          />
        ) : null}
        {page === 'agents' ? <AgentsPage agents={agents} reload={() => void loadAgents()} /> : null}
        {page === 'audit' ? <AuditPage audit={audit} reload={() => void loadAudit()} /> : null}
      </main>

      <nav className="fixed inset-x-0 bottom-0 z-30 grid grid-cols-4 border-t border-line bg-void/95 px-2 py-2 backdrop-blur md:hidden">
        <BottomNav active={page === 'orchestrates'} icon={<ClipboardList className="size-5" />} onClick={() => navTo('orchestrates')}>Orchestrate</BottomNav>
        <BottomNav active={page === 'schedules'} icon={<CalendarClock className="size-5" />} onClick={() => navTo('schedules')}>Schedules</BottomNav>
        <BottomNav active={page === 'agents'} icon={<Bot className="size-5" />} onClick={() => navTo('agents')}>Agents</BottomNav>
        <BottomNav active={page === 'audit'} icon={<History className="size-5" />} onClick={() => navTo('audit')}>Audit</BottomNav>
      </nav>
    </div>
  )

  async function submitOrchestration(event: FormEvent) {
    event.preventDefault()
    if (selectedAgents.size === 0) {
      setStatus('Select at least one agent.')
      return
    }
    setStatus('Starting...')
    try {
      const template = templates.find((t) => t.id === form.scenarioTemplate)
      const result = await api<Orchestration>('/api/orchestrate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          agents: [...selectedAgents],
          customAgents,
          scenarioTemplate: template ? { id: template.id, name: template.name, source: template.source } : null,
          repo: form.repo.trim(),
          baseBranch: form.baseBranch.trim() || 'main',
          task: form.task,
          strategy: form.strategy,
          llmPreset: form.llmPreset,
          outputLanguage: form.outputLanguage,
          github: {
            createIssue: form.createIssue,
            createPullRequest: form.createPullRequest,
            branchName: form.branchName.trim(),
            prBase: form.prBase.trim(),
            issueTitle: form.issueTitle.trim(),
            prTitle: form.prTitle.trim(),
            issueTemplate: form.issueTemplate,
            prTemplate: form.prTemplate,
          },
          limits: {
            maxDuration: form.maxDuration.trim(),
            maxSubtasks: numberOrUndefined(form.maxSubtasks),
            maxRetries: numberOrUndefined(form.maxRetries),
            maxLlmTokens: numberOrUndefined(form.maxLlmTokens),
            maxGitHubRequests: numberOrUndefined(form.maxGitHubRequests),
            maxConcurrentRepoRuns: numberOrUndefined(form.maxConcurrentRepoRuns),
            maxConcurrentOrgRuns: numberOrUndefined(form.maxConcurrentOrgRuns),
          },
        }),
      })
      setStatus(`Started: ${result.id}`)
      setSelectedID(result.id)
      setOrchPanel('detail')
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error))
    }
  }

  async function submitSchedule(event: FormEvent) {
    event.preventDefault()
    setScheduleStatus('Creating...')
    const agentNames = scheduleForm.agents.split(',').map((agent) => agent.trim()).filter(Boolean)
    try {
      const created = await api<Schedule>('/api/schedules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: scheduleForm.name,
          templateId: scheduleForm.templateId,
          repo: scheduleForm.repo.trim(),
          baseBranch: scheduleForm.baseBranch.trim() || 'main',
          task: scheduleForm.task,
          agents: agentNames,
          strategy: scheduleForm.strategy,
          llmPreset: scheduleForm.llmPreset,
          outputLanguage: scheduleForm.outputLanguage,
          schedule: {
            type: scheduleForm.scheduleType,
            interval: scheduleForm.interval,
            cron: scheduleForm.cron,
            timezone: scheduleForm.timezone,
          },
          concurrencyPolicy: scheduleForm.concurrencyPolicy,
          github: {
            createIssue: scheduleForm.createIssue,
            createPullRequest: scheduleForm.createPullRequest,
            issueTemplate: scheduleForm.issueTemplate,
            prTemplate: scheduleForm.prTemplate,
          },
          notification: {
            enabled: scheduleForm.notifyEnabled,
            triggers: scheduleForm.notifyTriggers.split(',').map((item) => item.trim()).filter(Boolean),
            destinations: scheduleForm.notifyDestinations.split(',').map((item) => item.trim()).filter(Boolean),
            webhookUrl: scheduleForm.webhookUrl.trim(),
          },
          limits: {
            maxDuration: scheduleForm.maxDuration.trim(),
            maxSubtasks: numberOrUndefined(scheduleForm.maxSubtasks),
            maxRetries: numberOrUndefined(scheduleForm.maxRetries),
            maxLlmTokens: numberOrUndefined(scheduleForm.maxLlmTokens),
            maxGitHubRequests: numberOrUndefined(scheduleForm.maxGitHubRequests),
            maxConcurrentRepoRuns: numberOrUndefined(scheduleForm.maxConcurrentRepoRuns),
            maxConcurrentOrgRuns: numberOrUndefined(scheduleForm.maxConcurrentOrgRuns),
          },
        }),
      })
      setScheduleStatus(`Created: ${created.id}`)
      await loadSchedules()
    } catch (error) {
      setScheduleStatus(error instanceof Error ? error.message : String(error))
    }
  }
}

function NavButton({ active, icon, children, onClick }: { active: boolean; icon: ReactNode; children: ReactNode; onClick: () => void }) {
  return (
    <button className={cx('inline-flex items-center gap-2 rounded-os px-3 py-2 text-sm', active ? 'bg-panel-2 text-ink' : 'text-soft hover:bg-panel hover:text-ink')} onClick={onClick} type="button">
      {icon}
      {children}
    </button>
  )
}

function BottomNav({ active, icon, children, onClick }: { active: boolean; icon: ReactNode; children: ReactNode; onClick: () => void }) {
  return (
    <button className={cx('grid justify-items-center gap-1 rounded-os py-1.5 text-[11px]', active ? 'bg-panel-2 text-cyan-os' : 'text-soft')} onClick={onClick} type="button">
      {icon}
      {children}
    </button>
  )
}

function SchedulesPage(props: {
  schedules: Schedule[]
  notifications: NotificationRecord[]
  reload: () => void
  form: typeof defaultScheduleForm
  setForm: Dispatch<SetStateAction<typeof defaultScheduleForm>>
  templates: ScheduleTemplate[]
  repositories: RepositorySummary[]
  llm: Json
  status: string
  setStatus: (value: string) => void
  submit: (event: FormEvent) => void
}) {
  const { form, setForm } = props
  const update = (patch: Partial<typeof defaultScheduleForm>) => setForm((current) => ({ ...current, ...patch }))

  function selectRepository(repo: string) {
    const selected = props.repositories.find((item) => item.full_name === repo)
    update({ repo, baseBranch: selected?.default_branch || form.baseBranch || 'main' })
  }

  function applyTemplate(id: string) {
    const template = props.templates.find((item) => item.id === id)
    if (!template) {
      update({ templateId: id })
      return
    }
    update({
      templateId: id,
      name: template.name,
      task: template.task ?? form.task,
      agents: (template.agents ?? []).join(', '),
      strategy: template.strategy ?? form.strategy,
      outputLanguage: template.outputLanguage ?? form.outputLanguage,
      scheduleType: template.schedule?.type ?? form.scheduleType,
      interval: template.schedule?.interval ?? form.interval,
      cron: template.schedule?.cron ?? form.cron,
      timezone: template.schedule?.timezone ?? form.timezone,
      concurrencyPolicy: template.concurrencyPolicy ?? form.concurrencyPolicy,
      createIssue: Boolean(template.github?.createIssue),
      createPullRequest: Boolean(template.github?.createPullRequest),
      issueTemplate: template.github?.issueTemplate ?? form.issueTemplate,
      prTemplate: template.github?.prTemplate ?? form.prTemplate,
    })
  }

  async function scheduleAction(schedule: Schedule, action: 'pause' | 'resume' | 'run') {
    props.setStatus(`${action}...`)
    try {
      await api(`/api/schedules/${encodeURIComponent(schedule.id)}/${action}`, { method: 'POST' })
      props.setStatus(`${action} requested.`)
      props.reload()
    } catch (error) {
      props.setStatus(error instanceof Error ? error.message : String(error))
    }
  }

  const repositoryOptions = [
    ...props.repositories,
    ...(form.repo && !props.repositories.some((repo) => repo.full_name === form.repo) ? [{ full_name: form.repo, default_branch: form.baseBranch }] : []),
  ]
  const selectedTemplate = props.templates.find((item) => item.id === form.templateId)

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="grid gap-4">
        <Panel className="p-0">
          <div className="flex items-center justify-between gap-3 border-b border-line p-4">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink"><CalendarClock className="size-4 text-cyan-os" /> Schedules</div>
            <IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={props.reload}>Refresh</IconButton>
          </div>
          <div className="divide-y divide-line">
            {props.schedules.length === 0 ? <div className="p-4 text-sm text-soft">No schedules.</div> : null}
            {props.schedules.map((schedule) => (
              <div key={schedule.id} className="grid gap-3 p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="mb-2 flex flex-wrap items-center gap-2"><h2 className="break-words text-sm font-semibold text-ink">{schedule.name || schedule.id}</h2><Status value={schedule.status} /></div>
                    <p className="break-words text-sm text-soft">{shortText(schedule.task, 220)}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <IconButton tone="secondary" icon={<Play className="size-4" />} onClick={() => scheduleAction(schedule, 'run')}>Run Now</IconButton>
                    {schedule.status === 'paused'
                      ? <IconButton tone="secondary" icon={<Check className="size-4" />} onClick={() => scheduleAction(schedule, 'resume')}>Resume</IconButton>
                      : <IconButton tone="secondary" icon={<CircleStop className="size-4" />} onClick={() => scheduleAction(schedule, 'pause')}>Pause</IconButton>}
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Tag>{schedule.repo || '-'}</Tag>
                  <Tag>{schedule.baseBranch || 'main'}</Tag>
                  <Tag>{schedule.schedule?.type === 'cron' ? schedule.schedule?.cron : schedule.schedule?.interval}</Tag>
                  <Tag>{schedule.schedule?.timezone || 'UTC'}</Tag>
                  <Tag>{schedule.concurrencyPolicy || 'forbid'}</Tag>
                </div>
                <div className="grid gap-2 text-xs text-soft sm:grid-cols-3">
                  <span>Next: {formatTime(schedule.nextRunAt)}</span>
                  <span>Last: {formatTime(schedule.lastRunAt)}</span>
                  <span>Status: {schedule.lastRunStatus || '-'}</span>
                </div>
                {(schedule.executions ?? []).length ? (
                  <div className="overflow-x-auto rounded-os border border-line">
                    <table className="w-full min-w-[620px] table-fixed text-left text-xs">
                      <thead className="text-soft"><tr><th className="px-3 py-2">Time</th><th className="px-3 py-2">Status</th><th className="px-3 py-2">Reason</th><th className="px-3 py-2">Run</th></tr></thead>
                      <tbody className="divide-y divide-line">
                        {(schedule.executions ?? []).slice().reverse().slice(0, 5).map((execution) => (
                          <tr key={execution.id}>
                            <td className="px-3 py-2 text-soft">{formatTime(execution.startedAt)}</td>
                            <td className="px-3 py-2"><Status value={execution.status} /></td>
                            <td className="px-3 py-2 text-soft">{execution.reason || '-'}</td>
                            <td className="px-3 py-2 break-all text-soft">{execution.runId || '-'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </Panel>
        <Panel className="p-0">
          <div className="flex items-center justify-between gap-3 border-b border-line p-4">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink"><Bell className="size-4 text-cyan-os" /> Notifications</div>
            <IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={props.reload}>Refresh</IconButton>
          </div>
          <div className="divide-y divide-line">
            {props.notifications.length === 0 ? <div className="p-4 text-sm text-soft">No notifications.</div> : null}
            {props.notifications.slice(0, 12).map((notification) => (
              <div key={notification.id} className="grid gap-2 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <h2 className="break-words text-sm font-semibold text-ink">{notification.title || notification.id}</h2>
                    <p className="text-xs text-soft">{formatTime(notification.createdAt)}</p>
                  </div>
                  <Status value={notification.trigger} />
                </div>
                <p className="break-words text-sm text-soft">{shortText(notification.message, 260)}</p>
                <div className="flex flex-wrap gap-2">
                  <Tag>{notification.repo || '-'}</Tag>
                  <Tag>{notification.status || '-'}</Tag>
                  {(notification.destinations ?? []).map((destination) => <Tag key={destination}>{destination}</Tag>)}
                  {notification.runUrl ? <a className="text-xs text-cyan-os hover:underline" href={notification.runUrl}>Run</a> : null}
                </div>
                {(notification.deliveries ?? []).length ? (
                  <div className="flex flex-wrap gap-2 text-xs text-soft">
                    {(notification.deliveries ?? []).map((delivery, index) => (
                      <span key={`${delivery.destination}-${index}`}>{delivery.destination}: {delivery.status}{delivery.attempts ? ` (${delivery.attempts})` : ''}</span>
                    ))}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </Panel>
      </div>

      <Panel>
        <form className="grid gap-3" onSubmit={props.submit}>
          <div className="flex items-center gap-2 text-sm font-semibold text-ink"><Sparkles className="size-4 text-cyan-os" /> New Schedule</div>
          <Field label="Template">
            <select className={inputClass} value={form.templateId} onChange={(e) => applyTemplate(e.target.value)}>
              <option value="">Custom schedule</option>
              {props.templates.map((template) => <option key={template.id} value={template.id}>{template.name}</option>)}
            </select>
          </Field>
          {selectedTemplate ? (
            <div className="grid gap-2 rounded-os border border-line bg-void p-3 text-xs text-soft">
              <div className="flex flex-wrap gap-2"><Tag>{selectedTemplate.category || 'template'}</Tag>{(selectedTemplate.agents ?? []).map((agent) => <Tag key={agent}>{agent}</Tag>)}</div>
              <p className="break-words">{selectedTemplate.description}</p>
              {(selectedTemplate.expectedOutputs ?? []).length ? <ul className="grid gap-1">{(selectedTemplate.expectedOutputs ?? []).map((item) => <li key={item} className="break-words">- {item}</li>)}</ul> : null}
            </div>
          ) : null}
          <Field label="Name">
            <input className={inputClass} value={form.name} onChange={(e) => update({ name: e.target.value })} placeholder="Weekly repository health report" />
          </Field>
          <Field label="Repository">
            <select className={inputClass} required value={form.repo} onChange={(e) => selectRepository(e.target.value)}>
              <option value="">{repositoryOptions.length ? 'Select repository' : 'No GitHub repositories available'}</option>
              {repositoryOptions.map((repo) => <option key={repo.full_name} value={repo.full_name}>{repo.full_name}</option>)}
            </select>
          </Field>
          <Field label="Base Branch">
            <input className={inputClass} value={form.baseBranch} onChange={(e) => update({ baseBranch: e.target.value })} placeholder="main" />
          </Field>
          <Field label="Task">
            <textarea className={textareaClass} required value={form.task} onChange={(e) => update({ task: e.target.value })} placeholder="Create a scheduled operations report." />
          </Field>
          <Field label="Agents">
            <input className={inputClass} value={form.agents} onChange={(e) => update({ agents: e.target.value })} placeholder="analyst, reporter" />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
            <Field label="Strategy">
              <select className={inputClass} value={form.strategy} onChange={(e) => update({ strategy: e.target.value })}><option value="sequential">Sequential</option><option value="parallel">Parallel</option></select>
            </Field>
            <Field label="LLM Preset">
              <select className={inputClass} value={form.llmPreset} onChange={(e) => update({ llmPreset: e.target.value })}>
                {(props.llm.presets ?? []).length ? (props.llm.presets ?? []).map((p: Json) => <option key={p.id} value={p.id}>{p.name ?? p.id}</option>) : <option value="">Default</option>}
              </select>
            </Field>
          </div>
          <Field label="Output Language">
            <select className={inputClass} value={form.outputLanguage} onChange={(e) => update({ outputLanguage: e.target.value })}>
              <option value="">Repository default / English</option>
              <option value="en">English</option>
              <option value="ja">Japanese</option>
            </select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
            <Field label="Schedule">
              <select className={inputClass} value={form.scheduleType} onChange={(e) => update({ scheduleType: e.target.value })}><option value="interval">Interval</option><option value="cron">Cron</option></select>
            </Field>
            {form.scheduleType === 'cron' ? (
              <Field label="Cron">
                <input className={inputClass} value={form.cron} onChange={(e) => update({ cron: e.target.value })} placeholder="0 9 * * 1" />
              </Field>
            ) : (
              <Field label="Interval">
                <input className={inputClass} value={form.interval} onChange={(e) => update({ interval: e.target.value })} placeholder="24h" />
              </Field>
            )}
          </div>
          <Field label="Timezone">
            <input className={inputClass} value={form.timezone} onChange={(e) => update({ timezone: e.target.value })} placeholder="UTC" />
          </Field>
          <Field label="Concurrency">
            <select className={inputClass} value={form.concurrencyPolicy} onChange={(e) => update({ concurrencyPolicy: e.target.value })}><option value="forbid">Forbid overlap</option><option value="allow">Allow overlap</option></select>
          </Field>
          <div className="grid gap-3 rounded-os border border-line bg-void p-3">
            <div className="text-sm font-semibold text-ink">Limits</div>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
              <Field label="Max Duration">
                <input className={inputClass} value={form.maxDuration} onChange={(e) => update({ maxDuration: e.target.value })} placeholder="30m" />
              </Field>
              <Field label="Max Subtasks">
                <input className={inputClass} inputMode="numeric" value={form.maxSubtasks} onChange={(e) => update({ maxSubtasks: e.target.value })} placeholder="12" />
              </Field>
              <Field label="Max Retries">
                <input className={inputClass} inputMode="numeric" value={form.maxRetries} onChange={(e) => update({ maxRetries: e.target.value })} placeholder="agent default" />
              </Field>
              <Field label="Repo Concurrency">
                <input className={inputClass} inputMode="numeric" value={form.maxConcurrentRepoRuns} onChange={(e) => update({ maxConcurrentRepoRuns: e.target.value })} placeholder="1" />
              </Field>
              <Field label="Org Concurrency">
                <input className={inputClass} inputMode="numeric" value={form.maxConcurrentOrgRuns} onChange={(e) => update({ maxConcurrentOrgRuns: e.target.value })} placeholder="optional" />
              </Field>
            </div>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
              <Field label="LLM Token Budget">
                <input className={inputClass} inputMode="numeric" value={form.maxLlmTokens} onChange={(e) => update({ maxLlmTokens: e.target.value })} placeholder="optional" />
              </Field>
              <Field label="GitHub Request Budget">
                <input className={inputClass} inputMode="numeric" value={form.maxGitHubRequests} onChange={(e) => update({ maxGitHubRequests: e.target.value })} placeholder="optional" />
              </Field>
            </div>
          </div>
          <div className="grid gap-3 rounded-os border border-line bg-void p-3">
            <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={form.notifyEnabled} onChange={(e) => update({ notifyEnabled: e.target.checked })} />Notifications</label>
            <Field label="Notification Triggers">
              <input className={inputClass} value={form.notifyTriggers} onChange={(e) => update({ notifyTriggers: e.target.value })} placeholder="completed, failed, quality_gate_failed" />
            </Field>
            <Field label="Notification Destinations">
              <input className={inputClass} value={form.notifyDestinations} onChange={(e) => update({ notifyDestinations: e.target.value })} placeholder="inbox, webhook, github_issue" />
            </Field>
            <Field label="Webhook URL">
              <input className={inputClass} value={form.webhookUrl} onChange={(e) => update({ webhookUrl: e.target.value })} placeholder="https://example.com/agentos-hook" />
            </Field>
          </div>
          <div className="grid gap-2">
            <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={form.createIssue} onChange={(e) => update({ createIssue: e.target.checked })} />Create Issue</label>
            <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={form.createPullRequest} onChange={(e) => update({ createPullRequest: e.target.checked })} />Create PR</label>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
            <Field label="Issue Template">
              <select className={inputClass} value={form.issueTemplate} onChange={(e) => update({ issueTemplate: e.target.value })}><option value="">Default</option><option value="repository">Repository</option></select>
            </Field>
            <Field label="PR Template">
              <select className={inputClass} value={form.prTemplate} onChange={(e) => update({ prTemplate: e.target.value })}><option value="">Default</option><option value="repository">Repository</option></select>
            </Field>
          </div>
          <IconButton icon={<Check className="size-4" />}>Create Schedule</IconButton>
          {props.status ? <p className="break-words text-sm text-soft">{props.status}</p> : null}
        </form>
      </Panel>
    </div>
  )
}

function OrchestratesPage(props: {
  panel: OrchPanel
  setPanel: (value: OrchPanel) => void
  detailTab: DetailTab
  setDetailTab: (value: DetailTab) => void
  records: Orchestration[]
  current: Orchestration | null
  selectedID: string
  setSelectedID: (id: string) => void
  refreshCurrent: () => void
  loadRecords: () => void
  form: typeof defaultForm
  setForm: Dispatch<SetStateAction<typeof defaultForm>>
  agents: { name: string; label: string; custom: boolean }[]
  selectedAgents: Set<string>
  setSelectedAgents: Dispatch<SetStateAction<Set<string>>>
  customAgents: Json[]
  setCustomAgents: (agents: Json[]) => void
  repositories: RepositorySummary[]
  loadRepositories: () => void
  templates: Json[]
  loadTemplates: () => void
  llm: Json
  status: string
  setStatus: (value: string) => void
  submit: (event: FormEvent) => void
}) {
  return (
    <>
      <Panel className="p-3">
        <div className="flex items-center gap-2 overflow-x-auto">
          <Segment active={props.panel === 'new'} icon={<Sparkles className="size-4" />} onClick={() => props.setPanel('new')}>New</Segment>
          <Segment active={props.panel === 'list'} icon={<LayoutList className="size-4" />} onClick={() => { props.setPanel('list'); props.loadRecords() }}>List</Segment>
          <Segment active={props.panel === 'detail'} icon={<Activity className="size-4" />} onClick={() => props.setPanel('detail')}>Detail</Segment>
          <IconButton className="ml-auto" tone="secondary" icon={<RefreshCw className="size-4" />} onClick={props.panel === 'detail' ? props.refreshCurrent : props.loadRecords}>Refresh</IconButton>
        </div>
      </Panel>
      {props.panel === 'new' ? <NewOrchestration {...props} /> : null}
      {props.panel === 'list' ? <OrchestrationList records={props.records} open={props.setSelectedID} /> : null}
      {props.panel === 'detail' ? <OrchestrationDetail current={props.current} selectedID={props.selectedID} tab={props.detailTab} setTab={props.setDetailTab} refresh={props.refreshCurrent} /> : null}
    </>
  )
}

function Segment({ active, icon, children, onClick }: { active: boolean; icon?: ReactNode; children: ReactNode; onClick: () => void }) {
  return (
    <button className={cx('inline-flex min-h-9 shrink-0 items-center gap-2 rounded-os px-3 text-sm', active ? 'bg-cyan-os/15 text-cyan-os' : 'text-soft hover:bg-panel-2 hover:text-ink')} type="button" onClick={onClick}>
      {icon}
      {children}
    </button>
  )
}

function NewOrchestration(props: Parameters<typeof OrchestratesPage>[0]) {
  const { form, setForm } = props
  const update = (patch: Partial<typeof defaultForm>) => setForm((current) => ({ ...current, ...patch }))
  const selectedTemplate = props.templates.find((t) => t.id === form.scenarioTemplate)

  async function loadRepositoryAgents() {
    props.setStatus('Loading repository agents...')
    try {
      const result = await api<Json>('/api/agents/repository', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ repo: form.repo, baseBranch: form.baseBranch || 'main' }),
      })
      props.setCustomAgents(result.agents ?? [])
      props.setStatus((result.agents ?? []).length ? `Loaded ${(result.agents ?? []).length} repository agent(s).` : 'No repository agents found.')
    } catch (error) {
      props.setCustomAgents([])
      props.setStatus(error instanceof Error ? error.message : String(error))
    }
  }

  async function recommend() {
    props.setStatus('Recommending...')
    try {
      const rec = await api<Json>('/api/orchestrate/recommend', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ repo: form.repo, baseBranch: form.baseBranch || 'main', task: form.task || 'Recommend agents from repository context.' }),
      })
      props.setSelectedAgents(new Set(rec.agents ?? []))
      update({ strategy: rec.strategy ?? form.strategy, createPullRequest: Boolean(rec.createPullRequest) })
      props.setStatus(`${rec.preset ?? 'general'} ${Math.round((rec.confidence ?? 0) * 100)}%`)
    } catch (error) {
      props.setStatus(error instanceof Error ? error.message : String(error))
    }
  }

  function applyTemplate() {
    if (!selectedTemplate) return
    let task = String(selectedTemplate.taskTemplate ?? '')
    task = task.replaceAll('{{ repo }}', form.repo).replaceAll('{{ baseBranch }}', form.baseBranch || 'main')
    update({ task: task.trim() || form.task, strategy: selectedTemplate.strategy ?? form.strategy, createPullRequest: Boolean(selectedTemplate.createPullRequest) })
    props.setSelectedAgents(new Set(selectedTemplate.agents ?? []))
  }

  function selectRepository(repo: string) {
    const selected = props.repositories.find((r) => r.full_name === repo)
    update({ repo, baseBranch: selected?.default_branch || form.baseBranch || 'main' })
  }

  const repositoryOptions = [
    ...props.repositories,
    ...(form.repo && !props.repositories.some((repo) => repo.full_name === form.repo) ? [{ full_name: form.repo, default_branch: form.baseBranch }] : []),
  ]

  return (
    <form className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1fr)_22rem]" onSubmit={props.submit}>
      <div className="grid min-w-0 gap-4">
        <Panel>
          <div className="mb-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink"><Cloud className="size-4 text-cyan-os" /> Repository</div>
            <IconButton type="button" tone="secondary" icon={<RefreshCw className="size-4" />} onClick={props.loadRepositories}>Refresh</IconButton>
          </div>
          <div className="grid gap-3 sm:grid-cols-[1fr_11rem]">
            <Field label="Repository">
              <select className={inputClass} required value={form.repo} onChange={(e) => selectRepository(e.target.value)}>
                <option value="">{repositoryOptions.length ? 'Select repository' : 'No GitHub repositories available'}</option>
                {repositoryOptions.map((repo) => <option key={repo.full_name} value={repo.full_name}>{repo.full_name}{repo.private ? ' private' : ''}</option>)}
              </select>
            </Field>
            <Field label="Base Branch">
              <input className={inputClass} value={form.baseBranch} onChange={(e) => update({ baseBranch: e.target.value })} placeholder="main" />
            </Field>
          </div>
        </Panel>

        <Panel>
          <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink"><TerminalSquare className="size-4 text-cyan-os" /> Task</div>
          <div className="grid gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Scenario Template">
                <select className={inputClass} value={form.scenarioTemplate} onChange={(e) => update({ scenarioTemplate: e.target.value })} onFocus={props.loadTemplates}>
                  <option value="">No template</option>
                  {props.templates.map((t) => <option key={t.id} value={t.id}>{t.name}{t.source === 'repository' ? ' (repository)' : ''}</option>)}
                </select>
              </Field>
              <Field label="Strategy">
                <select className={inputClass} value={form.strategy} onChange={(e) => update({ strategy: e.target.value })}>
                  <option value="sequential">Sequential</option>
                  <option value="parallel">Parallel</option>
                </select>
              </Field>
            </div>
            {selectedTemplate ? (
              <div className="rounded-os border border-line bg-void p-3">
                <div className="mb-2 flex flex-wrap gap-2">{(selectedTemplate.agents ?? []).map((a: string) => <Tag key={a}>{a}</Tag>)}</div>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words text-xs text-soft">{selectedTemplate.taskTemplate}</pre>
                <IconButton type="button" className="mt-3" tone="secondary" icon={<Check className="size-4" />} onClick={applyTemplate}>Apply</IconButton>
              </div>
            ) : null}
            <textarea className={textareaClass} required value={form.task} onChange={(e) => update({ task: e.target.value })} placeholder="Describe the repository work." />
          </div>
        </Panel>

        <Panel>
          <div className="mb-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink"><Bot className="size-4 text-cyan-os" /> Agents</div>
            <div className="flex gap-2">
              <IconButton type="button" tone="secondary" icon={<Database className="size-4" />} onClick={loadRepositoryAgents}>Load</IconButton>
              <IconButton type="button" tone="secondary" icon={<Sparkles className="size-4" />} onClick={recommend}>Suggest</IconButton>
            </div>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {props.agents.map((agent) => (
              <label key={agent.name} className={cx('flex min-w-0 items-center gap-3 rounded-os border p-3 text-sm', props.selectedAgents.has(agent.name) ? 'border-cyan-os/70 bg-cyan-os/10' : 'border-line bg-void')}>
                <input
                  className="size-4 accent-cyan-os"
                  type="checkbox"
                  checked={props.selectedAgents.has(agent.name)}
                  onChange={(e) => {
                    props.setSelectedAgents((current) => {
                      const next = new Set(current)
                      if (e.target.checked) next.add(agent.name)
                      else next.delete(agent.name)
                      return next
                    })
                  }}
                />
                <span className="min-w-0">
                  <span className="block truncate text-ink">{agent.name}</span>
                  <span className="block truncate text-xs text-soft">{agent.custom ? 'custom' : agent.label}</span>
                </span>
              </label>
            ))}
          </div>
        </Panel>

        <Panel>
          <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink"><GitPullRequest className="size-4 text-cyan-os" /> GitHub</div>
          <div className="grid gap-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={form.createIssue} onChange={(e) => update({ createIssue: e.target.checked })} />Create Issue</label>
              <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={form.createPullRequest} onChange={(e) => update({ createPullRequest: e.target.checked })} />Create PR</label>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Branch name (optional)">
                <input className={inputClass} value={form.branchName} onChange={(e) => update({ branchName: e.target.value })} placeholder="agentos/<run-id>" />
              </Field>
              <Field label="PR base branch">
                <input className={inputClass} value={form.prBase} onChange={(e) => update({ prBase: e.target.value })} placeholder="main" />
              </Field>
              <Field label="Issue title">
                <input className={inputClass} value={form.issueTitle} onChange={(e) => update({ issueTitle: e.target.value })} placeholder="Issue title" />
              </Field>
              <Field label="PR title">
                <input className={inputClass} value={form.prTitle} onChange={(e) => update({ prTitle: e.target.value })} placeholder="PR title" />
              </Field>
            </div>
          </div>
        </Panel>
      </div>

      <aside className="grid min-w-0 content-start gap-4">
        <Panel>
          <div className="mb-3 text-sm font-semibold text-ink">Runtime</div>
          <div className="grid gap-3">
            <Field label="LLM Preset">
              <select className={inputClass} value={form.llmPreset} onChange={(e) => update({ llmPreset: e.target.value })}>
                {(props.llm.presets ?? []).length ? (props.llm.presets ?? []).map((p: Json) => <option key={p.id} value={p.id}>{p.name ?? p.id} / {p.model}</option>) : <option value="">Default</option>}
              </select>
            </Field>
            <Field label="Output Language">
              <select className={inputClass} value={form.outputLanguage} onChange={(e) => update({ outputLanguage: e.target.value })}>
                <option value="">Repository default / English</option>
                <option value="en">English</option>
                <option value="ja">Japanese</option>
              </select>
            </Field>
            <div className="grid gap-3 rounded-os border border-line bg-void p-3">
              <div className="text-sm font-semibold text-ink">Limits</div>
              <Field label="Max Duration">
                <input className={inputClass} value={form.maxDuration} onChange={(e) => update({ maxDuration: e.target.value })} placeholder="30m" />
              </Field>
              <Field label="Max Subtasks">
                <input className={inputClass} inputMode="numeric" value={form.maxSubtasks} onChange={(e) => update({ maxSubtasks: e.target.value })} placeholder="12" />
              </Field>
              <Field label="Max Retries">
                <input className={inputClass} inputMode="numeric" value={form.maxRetries} onChange={(e) => update({ maxRetries: e.target.value })} placeholder="agent default" />
              </Field>
              <Field label="Repo Concurrency">
                <input className={inputClass} inputMode="numeric" value={form.maxConcurrentRepoRuns} onChange={(e) => update({ maxConcurrentRepoRuns: e.target.value })} placeholder="1" />
              </Field>
              <Field label="Org Concurrency">
                <input className={inputClass} inputMode="numeric" value={form.maxConcurrentOrgRuns} onChange={(e) => update({ maxConcurrentOrgRuns: e.target.value })} placeholder="optional" />
              </Field>
              <Field label="LLM Token Budget">
                <input className={inputClass} inputMode="numeric" value={form.maxLlmTokens} onChange={(e) => update({ maxLlmTokens: e.target.value })} placeholder="optional" />
              </Field>
              <Field label="GitHub Request Budget">
                <input className={inputClass} inputMode="numeric" value={form.maxGitHubRequests} onChange={(e) => update({ maxGitHubRequests: e.target.value })} placeholder="optional" />
              </Field>
            </div>
          </div>
        </Panel>
        <Panel>
          <IconButton type="submit" className="w-full" icon={<Play className="size-4" />}>Start Orchestration</IconButton>
          {props.status ? <p className="mt-3 break-words text-sm text-soft">{props.status}</p> : null}
        </Panel>
      </aside>
    </form>
  )
}

function OrchestrationList({ records, open }: { records: Orchestration[]; open: (id: string) => void }) {
  return (
    <Panel className="p-0">
      {records.length === 0 ? <div className="p-4 text-sm text-soft">No orchestrations.</div> : null}
      <div className="divide-y divide-line">
        {records.map((record) => (
          <button key={record.id} className="grid w-full min-w-0 gap-2 p-4 text-left hover:bg-panel-2 sm:grid-cols-[11rem_1fr_auto]" onClick={() => open(record.id)} type="button">
            <div className="flex items-center gap-2"><Status value={record.status} /><span className="text-xs text-soft">{ago(record.updatedAt ?? record.createdAt)}</span></div>
            <div className="min-w-0">
              <div className="truncate text-sm font-medium text-ink">{record.id}</div>
              <div className="break-words text-sm text-soft">{shortText(record.task, 150)}</div>
              <div className="mt-2 flex flex-wrap gap-2"><Tag>{record.repo || '-'}</Tag><Tag>{record.baseBranch || '-'}</Tag></div>
            </div>
            <ChevronRight className="hidden size-5 self-center text-soft sm:block" />
          </button>
        ))}
      </div>
    </Panel>
  )
}

function OrchestrationDetail({ current, selectedID, tab, setTab, refresh }: { current: Orchestration | null; selectedID: string; tab: DetailTab; setTab: (tab: DetailTab) => void; refresh: () => void }) {
  if (!selectedID) return <Panel><p className="text-sm text-soft">Select an orchestration.</p></Panel>
  if (!current) return <Panel><Loader2 className="size-5 animate-spin text-cyan-os" /></Panel>
  return (
    <div className="grid gap-4">
      <Panel>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap items-center gap-2"><h1 className="break-all text-lg font-semibold text-ink">{current.id}</h1><Status value={current.status} /></div>
            <div className="flex flex-wrap gap-2"><Tag>{current.repo || '-'}</Tag><Tag>{current.baseBranch || 'main'}</Tag><Tag>{current.strategy || '-'}</Tag><Tag>{current.llmPreset || '-'}</Tag></div>
          </div>
          <div className="flex gap-2">
            {current.status === 'pending_approval' && current.github?.approvalStatus === 'pending' ? <ApprovalActions id={current.id} refresh={refresh} /> : null}
            {(current.status === 'planning' || current.status === 'running') ? <CancelButton id={current.id} refresh={refresh} /> : null}
            <IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={refresh}>Refresh</IconButton>
          </div>
        </div>
        <pre className="mt-3 whitespace-pre-wrap break-words font-sans text-sm leading-6 text-soft">{readableTask(current.task)}</pre>
        {current.error ? <p className="mt-3 break-words text-sm text-red-os">{current.error}</p> : null}
      </Panel>
      <Panel className="p-2">
        <div className="flex gap-1 overflow-x-auto">
          {detailTabs.map((value) => <Segment key={value} active={tab === value} onClick={() => setTab(value)}>{value[0].toUpperCase() + value.slice(1)}</Segment>)}
        </div>
      </Panel>
      {tab === 'overview' ? <OverviewTab record={current} /> : null}
      {tab === 'runs' ? <RunsTab record={current} refresh={refresh} /> : null}
      {tab === 'memory' ? <MemoryTab record={current} refresh={refresh} /> : null}
      {tab === 'guidelines' ? <GuidelinesTab record={current} refresh={refresh} /> : null}
      {tab === 'search' ? <SearchTab record={current} /> : null}
      {tab === 'github' ? <GitHubTab record={current} /> : null}
    </div>
  )
}

function OverviewTab({ record }: { record: Orchestration }) {
  const results = record.results ?? []
  const passed = results.filter((x) => x.success).length
  const failed = results.filter((x) => x.success === false).length
  const total = record.plan?.subtasks?.length ?? results.length
  const usage = record.usage ?? {}
  const limits = record.limits ?? {}
  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_24rem]">
      <Panel>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label="Subtasks" value={total} />
          <Stat label="Passed" value={passed} tone="text-green-os" />
          <Stat label="Failed" value={failed} tone="text-red-os" />
          <Stat label="Agents" value={record.agents?.length ?? 0} tone="text-amber-os" />
        </div>
        <div className="mt-5 grid gap-3 rounded-os border border-line bg-void p-3 text-sm sm:grid-cols-2">
          <div><span className="text-soft">Budget</span><div className="break-words text-ink">{usage.budgetStatus || 'within_limits'}</div></div>
          <div><span className="text-soft">Duration</span><div className="break-words text-ink">{usage.duration || '-' } / {limits.maxDuration || '-'}</div></div>
          <div><span className="text-soft">Subtasks</span><div className="break-words text-ink">{usage.subtasksPlanned ?? total} / {limits.maxSubtasks || '-'}</div></div>
          <div><span className="text-soft">Repo Concurrency</span><div className="break-words text-ink">{limits.maxConcurrentRepoRuns || '-'}</div></div>
          <div><span className="text-soft">LLM Tokens</span><div className="break-words text-ink">{usage.llmTokensUsed ?? 0} / {usage.llmTokensBudget || limits.maxLlmTokens || '-'}</div></div>
          <div><span className="text-soft">GitHub Requests</span><div className="break-words text-ink">{usage.gitHubRequestsUsed ?? 0} / {usage.gitHubRequestsBudget || limits.maxGitHubRequests || '-'}</div></div>
          {usage.limitExceeded ? <div className="break-words text-red-os sm:col-span-2">{usage.limitExceeded}</div> : null}
        </div>
        <h2 className="mt-5 text-sm font-semibold text-ink">Summary</h2>
        {record.summary ? <pre className="mt-2 max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-os bg-void p-3 text-xs text-soft">{record.summary}</pre> : <p className="mt-2 text-sm text-soft">Pending.</p>}
      </Panel>
      <Panel>
        <h2 className="mb-3 text-sm font-semibold text-ink">Timeline</h2>
        <div className="grid gap-3">
          {(record.events ?? []).slice().reverse().map((e, idx) => (
            <div key={`${e.timestamp}-${idx}`} className="grid gap-1 rounded-os border border-line bg-void p-3 text-sm">
              <div className="flex flex-wrap gap-2"><Status value={e.type} /><span className="text-xs text-soft">{formatTime(e.timestamp)}</span></div>
              <p className="break-words text-soft">{e.message}</p>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  )
}

function Stat({ label, value, tone = 'text-cyan-os' }: { label: string; value: number; tone?: string }) {
  return <div className="rounded-os border border-line bg-void p-3 text-center"><div className={cx('text-2xl font-semibold', tone)}>{value}</div><div className="text-[11px] uppercase tracking-wide text-soft">{label}</div></div>
}

function RunsTab({ record, refresh }: { record: Orchestration; refresh: () => void }) {
  const [agent, setAgent] = useState(record.agents?.[0] ?? '')
  const [task, setTask] = useState('')
  const [description, setDescription] = useState('')
  const [preset, setPreset] = useState(record.llmPreset ?? '')
  const [status, setStatus] = useState('')
  const states = record.subtasks?.length ? record.subtasks : (record.plan?.subtasks ?? [])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setStatus('Starting...')
    try {
      await api('/api/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent, task, description, repo: record.repoPath || record.repo, llmPreset: preset }),
      })
      setStatus('Started.')
      refresh()
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error))
    }
  }

  return (
    <div className="grid gap-4">
      <Panel>
        <form className="grid gap-3" onSubmit={submit}>
          <div className="grid gap-3 sm:grid-cols-[12rem_1fr_auto]">
            <select className={inputClass} value={agent} onChange={(e) => setAgent(e.target.value)}>{(record.agents ?? []).map((a) => <option key={a} value={a}>{a}</option>)}</select>
            <input className={inputClass} required value={task} onChange={(e) => setTask(e.target.value)} placeholder="Task for selected agent" />
            <IconButton icon={<Play className="size-4" />}>Run</IconButton>
          </div>
          <div className="grid gap-3 sm:grid-cols-[12rem_1fr]">
            <input className={inputClass} value={preset} onChange={(e) => setPreset(e.target.value)} placeholder="LLM preset" />
            <input className={inputClass} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Description" />
          </div>
          {status ? <p className="text-sm text-soft">{status}</p> : null}
        </form>
      </Panel>
      <Panel className="p-0">
        <div className="divide-y divide-line">
          {states.length === 0 ? <div className="p-4 text-sm text-soft">No runs.</div> : null}
          {states.map((s) => {
            const result = s.result ?? (record.results ?? []).find((r) => r.subtask_id === s.id || r.subtaskId === s.id) ?? {}
            return (
              <div key={s.id} className="grid gap-2 p-4">
                <div className="flex flex-wrap items-center gap-2"><Status value={s.status ?? (result.success ? 'pass' : 'pending')} /><Tag>{s.agent_type ?? s.agentName ?? '-'}</Tag><span className="text-xs text-soft">{s.id}</span></div>
                <p className="break-words text-sm text-ink">{s.description}</p>
                {result.output || result.error || result.diff ? <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-os bg-void p-3 text-xs text-soft">{[result.error, result.output, result.diff].filter(Boolean).join('\n')}</pre> : null}
              </div>
            )
          })}
        </div>
      </Panel>
    </div>
  )
}

function EntryList({ entries, kind, actions }: { entries: Json[]; kind: string; actions?: (entry: Json) => ReactNode }) {
  if (!entries?.length) return <p className="text-sm text-soft">No {kind}.</p>
  return (
    <div className="divide-y divide-line">
      {entries.map((entry) => (
        <div key={entry.id ?? `${entry.title}-${entry.content}`} className="grid gap-2 py-3">
          <div className="flex flex-wrap gap-2"><Tag>{entry.type ?? kind}</Tag>{entry.status ? <Status value={entry.status} /> : null}{entry.required ? <Tag>required</Tag> : null}{entry.pinned ? <Tag>pinned</Tag> : null}{entry.runId ? <Tag>{entry.runId}</Tag> : null}</div>
          <div className="break-words text-sm text-ink">{entry.title ? <strong>{entry.title}: </strong> : null}{entry.content ?? entry.rule ?? ''}</div>
          {actions ? <div className="flex flex-wrap gap-2">{actions(entry)}</div> : null}
        </div>
      ))}
    </div>
  )
}

function MemoryTab({ record, refresh }: { record: Orchestration; refresh: () => void }) {
  const [all, setAll] = useState<Json[]>([])
  async function load() {
    const entries = await api<Json[]>(`/api/repository-memory?repo=${encodeURIComponent(record.repo || '.')}&baseBranch=${encodeURIComponent(record.baseBranch || 'main')}`)
    setAll(entries)
  }
  async function approve(id: string) { await api(`/api/repository-memory/${encodeURIComponent(id)}/approve`, { method: 'POST' }); refresh(); await load() }
  async function archive(entry: Json) { await api(`/api/repository-memory/${encodeURIComponent(entry.id)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...entry, status: 'archived' }) }); await load() }
  return (
    <Panel>
      <h2 className="text-sm font-semibold text-ink">Used</h2>
      <EntryList entries={record.memoryUsed ?? []} kind="memory" />
      <h2 className="mt-5 text-sm font-semibold text-ink">Proposed</h2>
      <EntryList entries={record.memoryProposals ?? []} kind="memory" actions={(m) => <>{m.status === 'pending' ? <IconButton icon={<Check className="size-4" />} onClick={() => approve(m.id)}>Approve</IconButton> : null}<IconButton tone="danger" icon={<Archive className="size-4" />} onClick={() => archive(m)}>Archive</IconButton></>} />
      <div className="mt-5"><IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={load}>Repository Memory</IconButton></div>
      <EntryList entries={all} kind="memory" actions={(m) => <IconButton tone="danger" icon={<Archive className="size-4" />} onClick={() => archive(m)}>Archive</IconButton>} />
    </Panel>
  )
}

function GuidelinesTab({ record }: { record: Orchestration; refresh: () => void }) {
  const [all, setAll] = useState<Json[]>([])
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [required, setRequired] = useState(false)
  async function load() {
    setAll(await api<Json[]>(`/api/repository-guidelines?repo=${encodeURIComponent(record.repo || '.')}&baseBranch=${encodeURIComponent(record.baseBranch || 'main')}`))
  }
  async function create(e: FormEvent) {
    e.preventDefault()
    await api('/api/repository-guidelines', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ repo: record.repo, baseBranch: record.baseBranch, title, content, required, type: 'general' }) })
    setTitle('')
    setContent('')
    setRequired(false)
    await load()
  }
  async function archive(id: string) { await api(`/api/repository-guidelines/${encodeURIComponent(id)}`, { method: 'DELETE' }); await load() }
  return (
    <Panel>
      <h2 className="text-sm font-semibold text-ink">Applied</h2>
      <EntryList entries={record.guidelinesUsed ?? []} kind="guideline" />
      <h2 className="mt-5 text-sm font-semibold text-ink">Required Misses</h2>
      <EntryList entries={record.missedRequiredGuidelines ?? []} kind="guideline" />
      <form className="mt-5 grid gap-3" onSubmit={create}>
        <div className="grid gap-3 sm:grid-cols-[1fr_10rem_auto]">
          <input className={inputClass} required value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Title" />
          <label className="flex items-center gap-2 text-sm text-ink"><input className="size-4 accent-cyan-os" type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} />Required</label>
          <IconButton icon={<Check className="size-4" />}>Create</IconButton>
        </div>
        <textarea className={textareaClass} required value={content} onChange={(e) => setContent(e.target.value)} placeholder="Guideline content" />
      </form>
      <div className="mt-5"><IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={load}>Repository Guidelines</IconButton></div>
      <EntryList entries={all} kind="guideline" actions={(g) => <IconButton tone="danger" icon={<Archive className="size-4" />} onClick={() => archive(g.id)}>Archive</IconButton>} />
    </Panel>
  )
}

function SearchTab({ record }: { record: Orchestration }) {
  const [query, setQuery] = useState('')
  const [source, setSource] = useState('')
  const [results, setResults] = useState<Json[]>([])
  async function search(e?: FormEvent) {
    e?.preventDefault()
    if (!query.trim()) return
    const params = new URLSearchParams({ q: query, repo: record.repo || '.', baseBranch: record.baseBranch || 'main' })
    if (source) params.set('source', source)
    setResults(await api<Json[]>(`/api/search?${params}`))
  }
  async function promote(item: Json, target: 'memory' | 'guideline') {
    if (target === 'memory') {
      await api('/api/repository-memory', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ repo: item.repo, baseBranch: item.branch, type: item.source || 'note', content: item.content || item.title, status: 'pending' }) })
    } else {
      await api('/api/repository-guidelines', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ repo: item.repo, baseBranch: item.branch, title: item.title || 'Search result guideline', type: item.source || 'general', content: item.content || item.title }) })
    }
  }
  async function stale(item: Json) {
    await api(`/api/repository-memory/${encodeURIComponent(item.id)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ content: item.content || '', type: item.metadata?.type || 'note', status: 'archived', pinned: false }) })
    await search()
  }
  return (
    <Panel>
      <form className="grid gap-3 sm:grid-cols-[1fr_12rem_auto]" onSubmit={search}>
        <input className={inputClass} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search repository context" />
        <select className={inputClass} value={source} onChange={(e) => setSource(e.target.value)}>
          <option value="">All Sources</option><option value="memory">Memory</option><option value="guideline">Guidelines</option><option value="run">Runs</option><option value="artifact">Artifacts</option><option value="github">GitHub</option><option value="kubernetes">Kubernetes</option><option value="code">Code/files</option>
        </select>
        <IconButton icon={<Search className="size-4" />}>Search</IconButton>
      </form>
      <div className="mt-4 divide-y divide-line">
        {results.map((item) => (
          <div key={item.id} className="grid gap-2 py-3">
            <div className="flex flex-wrap gap-2"><Tag>{item.source}</Tag><Tag>{item.repo}</Tag><Tag>{item.branch}</Tag>{item.runId ? <Tag>{item.runId}</Tag> : null}{item.score ? <Tag>score {Number(item.score).toFixed(1)}</Tag> : null}</div>
            <h3 className="break-words text-sm font-semibold text-ink">{item.title || item.id}</h3>
            <p className="break-words text-sm text-soft">{shortText(item.content, 800)}</p>
            <div className="flex flex-wrap gap-2"><IconButton tone="secondary" onClick={() => promote(item, 'memory')}>Promote Memory</IconButton><IconButton tone="secondary" onClick={() => promote(item, 'guideline')}>Promote Guideline</IconButton>{item.source === 'memory' ? <IconButton tone="danger" onClick={() => stale(item)}>Mark Stale</IconButton> : null}{item.url ? <a className="inline-flex min-h-9 items-center rounded-os border border-line bg-panel-2 px-3 text-sm text-ink" href={item.url} target="_blank" rel="noreferrer">Open</a> : null}</div>
          </div>
        ))}
      </div>
    </Panel>
  )
}

function GitHubTab({ record }: { record: Orchestration }) {
  const [repo, setRepo] = useState(repoForGitHub(record.repo))
  const [tab, setTab] = useState('issues')
  const [ref, setRef] = useState(record.baseBranch || 'main')
  const [items, setItems] = useState<Json[]>([])
  async function load() {
    const params = new URLSearchParams({ repo })
    if (tab === 'checks') params.set('ref', ref)
    const payload = await api<Json>(`/api/github/${tab}?${params}`)
    setItems(Array.isArray(payload) ? payload : payload.check_runs ?? payload.check_suites ?? payload.items ?? [])
  }
  return (
    <Panel>
      <div className="grid gap-3 sm:grid-cols-[1fr_12rem_10rem_auto]">
        <input className={inputClass} value={repo} onChange={(e) => setRepo(e.target.value)} placeholder="owner/repo" />
        <select className={inputClass} value={tab} onChange={(e) => setTab(e.target.value)}><option value="issues">Issues</option><option value="pulls">Pull Requests</option><option value="checks">CI Checks</option></select>
        <input className={inputClass} value={ref} onChange={(e) => setRef(e.target.value)} placeholder="ref" />
        <IconButton icon={<RefreshCw className="size-4" />} onClick={load}>Load</IconButton>
      </div>
      <div className="mt-4 divide-y divide-line">
        {items.map((item) => <div className="grid gap-1 py-3 text-sm" key={item.id ?? item.number ?? item.name}><div className="flex flex-wrap gap-2"><Status value={item.state ?? item.status ?? item.conclusion} />{item.number ? <Tag>#{item.number}</Tag> : null}</div><a className="break-words text-ink hover:text-cyan-os" href={item.html_url} target="_blank" rel="noreferrer">{item.title ?? item.name ?? item.head ?? 'GitHub item'}</a></div>)}
      </div>
    </Panel>
  )
}

function ApprovalActions({ id, refresh }: { id: string; refresh: () => void }) {
  async function submit(action: string) {
    await api(`/api/orchestrates/${encodeURIComponent(id)}/approval`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action }) })
    refresh()
  }
  return <><IconButton icon={<Check className="size-4" />} onClick={() => submit('approve')}>Approve</IconButton><IconButton tone="danger" icon={<CircleStop className="size-4" />} onClick={() => submit('reject')}>Reject</IconButton></>
}

function CancelButton({ id, refresh }: { id: string; refresh: () => void }) {
  async function cancel() {
    await api(`/api/orchestrates/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
    refresh()
  }
  return <IconButton tone="danger" icon={<CircleStop className="size-4" />} onClick={cancel}>Cancel</IconButton>
}

function AgentsPage({ agents, reload }: { agents: AgentInfo[]; reload: () => void }) {
  return (
    <Panel>
      <div className="mb-4 flex items-center justify-between gap-3"><h1 className="text-lg font-semibold text-ink">Agents</h1><IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={reload}>Refresh</IconButton></div>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {agents.map((agent) => <div key={agent.Name} className="grid gap-2 rounded-os border border-line bg-void p-4"><div className="flex items-center justify-between gap-2"><h2 className="break-words text-sm font-semibold text-ink">{agent.Name}</h2><Tag>v{agent.Version}</Tag></div><p className="break-words text-sm text-soft">{agent.Description}</p><div className="flex flex-wrap gap-2">{[...(agent.RequiredTools ?? []), ...(agent.Domains ?? [])].slice(0, 10).map((t) => <Tag key={t}>{t}</Tag>)}</div></div>)}
      </div>
    </Panel>
  )
}

function AuditPage({ audit, reload }: { audit: Json[]; reload: () => void }) {
  const thClass = 'px-3 py-2 font-semibold'
  const tdClass = 'px-3 py-3'

  return (
    <Panel>
      <div className="mb-4 flex items-center justify-between gap-3"><h1 className="text-lg font-semibold text-ink">Audit</h1><IconButton tone="secondary" icon={<RefreshCw className="size-4" />} onClick={reload}>Refresh</IconButton></div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[1040px] table-fixed text-left text-sm">
          <colgroup>
            <col className="w-[8.5rem]" />
            <col className="w-[9rem]" />
            <col className="w-[13rem]" />
            <col className="w-[7.5rem]" />
            <col className="w-[15rem]" />
            <col className="w-[9rem]" />
            <col />
          </colgroup>
          <thead className="text-xs text-soft"><tr><th className={thClass}>Time</th><th className={thClass}>Actor</th><th className={thClass}>Action</th><th className={thClass}>Outcome</th><th className={thClass}>Target</th><th className={thClass}>Run</th><th className={thClass}>Message</th></tr></thead>
          <tbody className="divide-y divide-line">
            {audit.map((e, idx) => <tr key={`${e.timestamp}-${idx}`} className="align-top"><td className={cx(tdClass, 'text-soft')}>{formatTime(e.timestamp)}</td><td className={cx(tdClass, 'break-all')}>{e.actor ?? '-'}</td><td className={cx(tdClass, 'break-all')}>{e.action ?? '-'}</td><td className={tdClass}><Status value={e.outcome} /></td><td className={cx(tdClass, 'break-words')}>{shortText(e.target ?? e.repo ?? '-', 64)}</td><td className={cx(tdClass, 'break-all')} title={String(e.runId ?? '-')}>{shortText(e.runId ?? '-', 22)}</td><td className={cx(tdClass, 'break-words')}>{shortText(e.message, 140)}</td></tr>)}
          </tbody>
        </table>
      </div>
    </Panel>
  )
}

export default App
