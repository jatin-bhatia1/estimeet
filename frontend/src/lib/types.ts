/** Shared types mirroring the Go API payloads. */

export type Mode = 'sync' | 'async'
export type TopicStatus = 'pending' | 'voting' | 'revealed' | 'estimated'

export interface RoomView {
  id: string
  code: string
  name: string
  mode: Mode
  deck: string[]
  currentTopicId: string | null
  autoReveal: boolean
  closed: boolean
  createdAt: string
  /** True when the room can connect to Jira at all. */
  jiraAvailable: boolean
  /** True when this server has an Atlassian OAuth app registered. */
  jiraOauthAvailable: boolean
  jiraConnected: boolean
  jiraAuthType?: 'oauth' | 'token'
  jiraAccountEmail?: string
  jiraSiteName?: string
  jiraSiteUrl?: string
}

export interface ParticipantView {
  id: string
  name: string
  /** Granted only to whoever created the session, and never transferred. */
  isHost: boolean
  isObserver: boolean
  online: boolean
  joinedAt: string
  votedTopics: number
}

export interface VoteView {
  participantId: string
  participantName: string
  value: string
}

export interface DistributionEntry {
  value: string
  count: number
}

export interface Stats {
  voteCount: number
  consensus: boolean
  min?: string
  max?: string
  average?: number
  median?: number
  suggested?: string
  spread: number
  distribution: DistributionEntry[]
}

export interface TopicView {
  id: string
  title: string
  description: string
  externalKey?: string
  externalUrl?: string
  position: number
  status: TopicStatus
  finalEstimate?: string
  createdAt: string
  revealedAt?: string
  revealed: boolean
  votedBy: string[]
  pendingVoters: string[]
  myVote: string | null
  votes: VoteView[]
  stats?: Stats
  isCurrent: boolean
  canVote: boolean
}

export interface BoardSummary {
  totalTopics: number
  estimatedTopics: number
  revealedTopics: number
  myRemaining: number
  totalPoints?: number
}

export interface RoomState {
  room: RoomView
  me: ParticipantView
  participants: ParticipantView[]
  topics: TopicView[]
  summary: BoardSummary
  serverTime: string
}

export interface SessionResponse {
  token: string
  roomCode: string
  participant: {
    id: string
    name: string
    isHost: boolean
    isObserver: boolean
  }
  state: RoomState
}

export interface RoomSummary {
  code: string
  name: string
  mode: Mode
  participants: number
  topics: number
  closed: boolean
}

export interface JiraProject {
  id: string
  key: string
  name: string
}

export interface JiraIssue {
  key: string
  summary: string
  description: string
  type: string
  status: string
  url: string
}

export interface ImportResult {
  imported: number
  skipped: string[]
}

/** The coffee card is stored as a plain word and rendered as an emoji. */
export const COFFEE_CARD = 'coffee'

export function cardLabel(value: string): string {
  return value === COFFEE_CARD ? '☕' : value
}
