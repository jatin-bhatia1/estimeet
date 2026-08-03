import type { RoomState } from './types'

export interface TopicDraft {
  title: string
  description: string
}

/** RoomActions is the set of mutations the boards can trigger. */
export interface RoomActions {
  vote(topicId: string, value: string): Promise<void>
  clearVote(topicId: string): Promise<void>
  reveal(topicId: string): Promise<void>
  reset(topicId: string): Promise<void>
  estimate(topicId: string, value: string): Promise<void>
  focusTopic(topicId: string): Promise<void>
  advance(direction: 'next' | 'prev'): Promise<void>
  addTopics(topics: TopicDraft[]): Promise<void>
  updateTopic(topicId: string, draft: TopicDraft): Promise<void>
  deleteTopic(topicId: string): Promise<void>
  kick(participantId: string): Promise<void>
  updateRoom(input: { name: string; autoReveal: boolean }): Promise<void>
  setRoster(input: { size: number; names: string[] }): Promise<void>
  applyState(state: RoomState): void
}
