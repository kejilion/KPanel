import { reactive } from 'vue'

// The same task PTY can be on screen twice, e.g. in the app list's job detail
// and in its script window. Only one view may drive the PTY size: views that
// fit different sizes would resize it back and forth and make a full-screen TUI
// redraw in a loop. The focused view claims the size; the others render at the
// size the owner last applied.

export interface TerminalSize {
  rows: number
  columns: number
}

interface SizeGroup {
  // Most recent claimant last, so a departing owner hands over to it.
  members: number[]
  owner: number
  size?: TerminalSize
}

export interface TerminalSizeMembership {
  isOwner(): boolean
  claim(): void
  publish(size: TerminalSize): void
  ownerSize(): TerminalSize | undefined
  leave(): void
}

const groups = reactive(new Map<string, SizeGroup>())
let nextMember = 0

export function joinTerminalSizeGroup(key: string): TerminalSizeMembership {
  const member = ++nextMember
  if (!groups.has(key)) groups.set(key, { members: [], owner: member })
  groups.get(key)!.members.push(member)
  let left = false
  return {
    isOwner: () => !left && groups.get(key)?.owner === member,
    claim() {
      const group = groups.get(key)
      if (left || !group) return
      group.members = [...group.members.filter((item) => item !== member), member]
      group.owner = member
    },
    publish(size) {
      const group = groups.get(key)
      if (!left && group?.owner === member) group.size = { ...size }
    },
    ownerSize: () => groups.get(key)?.size,
    leave() {
      const group = groups.get(key)
      if (left || !group) return
      left = true
      group.members = group.members.filter((item) => item !== member)
      if (!group.members.length) groups.delete(key)
      else if (group.owner === member) group.owner = group.members[group.members.length - 1]!
    },
  }
}
